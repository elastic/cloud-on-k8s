// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package otherbeat

import (
	"context"
	"crypto/sha256"
	"fmt"
	"hash"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	commonbeat "github.com/elastic/cloud-on-k8s/pkg/controller/common/beat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat/health"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	commonhash "github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

const (
	Type commonbeat.Type = "otherbeat"
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

var ConfigMountPath = "/etc/otherbeat.yml"

type Driver struct {
	commonbeat.DriverParams
	commonbeat.Driver
}

func NewDriver(params commonbeat.DriverParams) commonbeat.Driver {
	return &Driver{DriverParams: params}
}

func (fd *Driver) Reconcile() commonbeat.DriverResults {
	// this is a poc to show how it could look like, todo should be refactored
	results := commonbeat.NewDriverResults(fd.Context)

	cfg := settings.NewCanonicalConfig()

	if err := commonbeat.SetOutput(cfg, fd.Client, fd.Associated); err != nil {
		results.WithError(err)
	}

	userCfg, err := settings.NewCanonicalConfigFrom(fd.Config.Data)
	if err != nil {
		results.WithError(err)
	}

	if err = cfg.MergeWith(userCfg); err != nil {
		results.WithError(err)
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		results.WithError(err)
	}

	// create resource
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: fd.Owner.GetNamespace(),
			Name:      fd.Namer.ConfigSecretName(string(Type), fd.Owner.GetName()),
			Labels:    common.AddCredentialsLabel(fd.Labels),
		},
		Data: map[string][]byte{
			"otherbeat.yml": cfgBytes,
		},
	}

	// reconcile
	if _, err = reconciler.ReconcileSecret(fd.Client, expected, fd.Owner); err != nil {
		results.WithError(err)
	}

	checksum := sha256.New224()
	_, _ = checksum.Write(cfgBytes)

	// reconcile pod vehicle
	if driverStatus, err := doReconcile(
		fd.Context,
		fd.Client,
		fd.Associated,
		fd.Owner,
		fd.Labels,
		fd.Namer,
		checksum,
		fd.Image,
		fd.Version,
		fd.Selectors,
		fd.Deployment); err != nil {
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

func doReconcile(ctx context.Context,
	client k8s.Client,
	associated v1.Associated,
	owner metav1.Object,
	labels map[string]string,
	namer commonbeat.Namer,
	checksum hash.Hash,
	image, version string,
	selectors map[string]string,
	deploymentSpec commonbeat.DeploymentSpec,
) (commonbeat.DriverStatus, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	podTemplate := deploymentSpec.PodTemplate

	//image has to be there

	builder := defaults.NewPodTemplateBuilder(podTemplate, string(Type)).
		WithTerminationGracePeriod(30).
		WithDockerImage(image, "").
		WithEnv(corev1.EnvVar{
			Name: "NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			}}).
		WithResources(defaultResources).
		WithArgs("-e", "-c", ConfigMountPath).
		WithHostNetwork().
		WithDNSPolicy(corev1.DNSClusterFirstWithHostNet).
		WithSecurityContext(corev1.SecurityContext{
			RunAsUser: pointer.Int64(0),
		}).
		WithAutomountServiceAccountToken().
		WithLabels(map[string]string{
			commonbeat.ConfigChecksumLabel: fmt.Sprintf("%x", checksum.Sum(nil)),
			commonbeat.VersionLabelName:    version})

	// If SA is already provided, assume that for this resource (despite operator configuration) the user took the
	// responsibility of configuring RBAC. Otherwise, use the default.
	if commonbeat.ShouldSetupAutodiscoveryRBAC() && builder.PodTemplate.Spec.ServiceAccountName == "" {
		builder.WithServiceAccount(commonbeat.AutodiscoveryServiceAccountName)
	}

	configVolume := volume.NewSecretVolume(
		namer.ConfigSecretName(string(Type), owner.GetName()),
		"config", ConfigMountPath, "otherbeat.yml", 0600)
	esCaVolume := volume.NewSelectiveSecretVolumeWithMountPath(associated.AssociationConf().CASecretName, "es-certs", commonbeat.CAMountPath, []string{commonbeat.CAFileName})

	for _, v := range []volume.VolumeLike{
		configVolume,
		esCaVolume,
	} {
		builder = builder.WithVolumes(v.Volume()).WithVolumeMounts(v.VolumeMount())
	}

	builder = builder.WithLabels(commonhash.SetTemplateHashLabel(labels, builder.PodTemplate))

	d := deployment.New(deployment.Params{
		Name:            namer.Name(string(Type), owner.GetName()),
		Namespace:       owner.GetNamespace(),
		Selector:        selectors,
		Labels:          labels,
		PodTemplateSpec: builder.PodTemplate,
		Replicas:        1,
	})

	if err := controllerutil.SetControllerReference(owner, &d, scheme.Scheme); err != nil {
		return commonbeat.DriverStatus{}, err
	}

	// sync
	reconciled, err := deployment.Reconcile(client, d, owner)
	if err != nil {
		return commonbeat.DriverStatus{}, err
	}

	ready := reconciled.Status.ReadyReplicas
	desired := reconciled.Status.Replicas

	return commonbeat.DriverStatus{
		ExpectedNodes:  desired,
		AvailableNodes: ready,
		Health:         health.CalculateHealth(associated, ready, desired),
		Association:    associated.AssociationStatus(),
	}, nil
}
