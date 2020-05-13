// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package metricbeat

import (
	"crypto/sha256"
	"fmt"
	"hash"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonbeat "github.com/elastic/cloud-on-k8s/pkg/controller/common/beat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat/health"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	commonhash "github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

const (
	Type commonbeat.Type = "metricbeat"

	DockerSockVolumeName = "dockersock"
	DockerSockPath       = "/var/run/docker.sock"
	DockerSockMountPath  = "/var/run/docker.sock"

	ProcVolumeName = "proc"
	ProcPath       = "/proc"
	ProcMountPath  = "/hostfs/proc"

	CGroupVolumeName = "cgroup"
	CGroupPath       = "/sys/fs/cgroup"
	CGroupMountPath  = "/hostfs/sys/fs/cgroup"

	HostMetricbeatDataVolumeName   = "data"
	HostMetricbeatDataPathTemplate = "/var/lib/%s/%s/metricbeat-data"
	HostMetricbeatDataMountPath    = "/usr/share/metricbeat/data"

	ConfigVolumeName = "config"
	ConfigFileName   = "metricbeat.yml"
	ConfigMountPath  = "/etc/metricbeat.yml"

	CAVolumeName = "es-certs"
)

var (
	defaultResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("200Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("200Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
	}
)

type Driver struct {
	commonbeat.DriverParams
	commonbeat.Driver
}

func NewDriver(params commonbeat.DriverParams) commonbeat.Driver {
	return &Driver{DriverParams: params}
}

func (fd *Driver) Reconcile() commonbeat.DriverResults {
	results := commonbeat.NewDriverResults(fd.Context)

	if err := commonbeat.SetupAutodiscoverRBAC(fd.Context, fd.Logger, fd.Client, fd.Owner, fd.Labels); err != nil {
		results.WithError(err)
	}

	checksum := sha256.New224()
	err := commonbeat.ReconcileConfig(
		fd.DriverParams,
		"metricbeat.yml",
		defaultConfig,
		checksum)
	if err != nil {
		results.WithError(err)
		return results
	}

	if driverStatus, err := doReconcile(fd.DriverParams, checksum); err != nil {
		if apierrors.IsConflict(err) {
			fd.Logger.V(1).Info("Conflict while updating")
			results.WithResult(reconcile.Result{Requeue: true})
		}
		results.WithError(err)
	} else {
		results.Status = &driverStatus
	}

	return results
}

func doReconcile(dp commonbeat.DriverParams, checksum hash.Hash) (commonbeat.DriverStatus, error) {
	span, _ := apm.StartSpan(dp.Context, "reconcile_daemonSet", tracing.SpanTypeApp)
	defer span.End()

	podTemplate := dp.DaemonSet.PodTemplate

	builder := defaults.NewPodTemplateBuilder(podTemplate, string(Type)).
		WithTerminationGracePeriod(30).
		WithDockerImage(dp.Image, container.ImageRepository(container.MetricbeatImage, dp.Version)).
		WithEnv(corev1.EnvVar{
			Name: "NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			}}).
		WithResources(defaultResources).
		WithArgs("-e", "-c", ConfigMountPath, "-system.hostfs=/hostfs").
		WithHostNetwork().
		WithDNSPolicy(corev1.DNSClusterFirstWithHostNet).
		WithSecurityContext(corev1.SecurityContext{
			RunAsUser: pointer.Int64(0),
		}).
		WithAutomountServiceAccountToken().
		WithLabels(map[string]string{
			commonbeat.ConfigChecksumLabel: fmt.Sprintf("%x", checksum.Sum(nil)),
			commonbeat.VersionLabelName:    dp.Version})

	// If SA is already provided, assume that for this resource (despite operator configuration) the user took the
	// responsibility of configuring RBAC. Otherwise, use the default.
	if commonbeat.ShouldSetupAutodiscoverRBAC() && builder.PodTemplate.Spec.ServiceAccountName == "" {
		builder.WithServiceAccount(commonbeat.AutodiscoverServiceAccountName)
	}

	configVolume := volume.NewSecretVolume(
		dp.Namer.ConfigSecretName(string(Type), dp.Owner.GetName()),
		ConfigVolumeName,
		ConfigMountPath,
		ConfigFileName,
		0600)

	dockerSockVolume := volume.NewHostVolume(DockerSockVolumeName, DockerSockPath, DockerSockMountPath, false, corev1.HostPathUnset)
	procVolume := volume.NewReadOnlyHostVolume(ProcVolumeName, ProcPath, ProcMountPath)
	cgroupVolume := volume.NewReadOnlyHostVolume(CGroupVolumeName, CGroupPath, CGroupMountPath)

	kibanaCa := volume.NewSecretVolumeWithMountPath(
		"kibana-sample-kb-http-certs-public",
		"kibanaca",
		"/mnt/elastic-internal/kibana-certs")

	hostMetricbeatDataPath := fmt.Sprintf(HostMetricbeatDataPathTemplate, dp.Owner.GetNamespace(), dp.Owner.GetName())
	metricbeatDataVolume := volume.NewHostVolume(
		HostMetricbeatDataVolumeName,
		hostMetricbeatDataPath,
		HostMetricbeatDataMountPath,
		false,
		corev1.HostPathDirectoryOrCreate)

	volumes := []volume.VolumeLike{
		configVolume,
		dockerSockVolume,
		procVolume,
		cgroupVolume,
		metricbeatDataVolume,
		kibanaCa,
	}

	if dp.Associated.AssociationConf().IsConfigured() {
		volumes = append(volumes, volume.NewSelectiveSecretVolumeWithMountPath(
			dp.Associated.AssociationConf().CASecretName,
			CAVolumeName,
			commonbeat.CAMountPath,
			[]string{commonbeat.CAFileName}))
	}

	for _, v := range volumes {
		builder = builder.WithVolumes(v.Volume()).WithVolumeMounts(v.VolumeMount())
	}

	builder = builder.WithLabels(commonhash.SetTemplateHashLabel(dp.Labels, builder.PodTemplate))
	ds := daemonset.New(builder.PodTemplate, dp.Namer.Name(string(Type), dp.Owner.GetName()), dp.Owner, dp.Selectors)
	if err := controllerutil.SetControllerReference(dp.Owner, &ds, scheme.Scheme); err != nil {
		return commonbeat.DriverStatus{}, err
	}

	reconciled, err := daemonset.Reconcile(dp.Client, ds, dp.Owner)
	if err != nil {
		return commonbeat.DriverStatus{}, err
	}

	ready := reconciled.Status.NumberReady
	desired := reconciled.Status.DesiredNumberScheduled

	return commonbeat.DriverStatus{
		ExpectedNodes:  desired,
		AvailableNodes: ready,
		Health:         health.CalculateHealth(dp.Associated, ready, desired),
		Association:    dp.Associated.AssociationStatus(),
	}, nil
}
