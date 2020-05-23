// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"path"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat/health"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	commonhash "github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	CAVolumeName = "es-certs"
	CAMountPath  = "/mnt/elastic-internal/es-certs/"
	CAFileName   = "ca.crt"

	ConfigVolumeName   = "config"
	ConfigMountDirPath = "/etc"

	// ConfigChecksumLabel is a label used to store beats config checksum.
	ConfigChecksumLabel = "beat.k8s.elastic.co/config-checksum"

	// VersionLabelName is a label used to track the version of a Beat Pod.
	VersionLabelName = "beat.k8s.elastic.co/version"
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

func Reconcile(fd DriverParams, defaultConfig *settings.CanonicalConfig, defaultImage container.Image, f func(builder *defaults.PodTemplateBuilder)) DriverResults {
	results := NewDriverResults(fd.Context)

	if err := SetupAutodiscoverRBAC(fd.Context, fd.Logger, fd.Client, fd.Owner, fd.Labels); err != nil {
		results.WithError(err)
	}

	checksum := sha256.New224()
	err := reconcileConfig(
		fd,
		defaultConfig,
		checksum)
	if err != nil {
		results.WithError(err)
		return results
	}

	if driverStatus, err := reconcilePodVehicle(fd, defaultImage, f, checksum); err != nil {
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

// SetOutput will set output section in Beat config according to association configuration.
func setOutput(cfg *settings.CanonicalConfig, client k8s.Client, associated commonv1.Associated) error {
	if associated.AssociationConf().IsConfigured() {
		username, password, err := association.ElasticsearchAuthSettings(client, associated)
		if err != nil {
			return err
		}

		return cfg.MergeWith(settings.MustCanonicalConfig(
			map[string]interface{}{
				"output.elasticsearch": map[string]interface{}{
					"hosts":                       []string{associated.AssociationConf().GetURL()},
					"username":                    username,
					"password":                    password,
					"ssl.certificate_authorities": path.Join(CAMountPath, CAFileName),
				},
			}))
	}

	return nil
}

func build(
	client k8s.Client,
	associated commonv1.Associated,
	defaultConfig *settings.CanonicalConfig,
	userConfig *commonv1.Config) ([]byte, error) {
	cfg := settings.NewCanonicalConfig()

	if defaultConfig == nil && userConfig == nil {
		return nil, fmt.Errorf("both default and user configs are nil")
	}

	if err := setOutput(cfg, client, associated); err != nil {
		return nil, err
	}

	// use only the default config or only the provided config - no overriding, no merging
	if userConfig == nil {
		if err := cfg.MergeWith(defaultConfig); err != nil {
			return nil, err
		}
	} else {
		userCfg, err := settings.NewCanonicalConfigFrom(userConfig.Data)
		if err != nil {
			return nil, err
		}

		if err = cfg.MergeWith(userCfg); err != nil {
			return nil, err
		}
	}

	return cfg.Render()
}

func reconcileConfig(
	params DriverParams,
	defaultConfig *settings.CanonicalConfig,
	checksum hash.Hash) error {

	cfgBytes, err := build(params.Client, params.Associated, defaultConfig, params.Config)
	if err != nil {
		return err
	}

	// create resource
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Owner.GetNamespace(),
			Name:      params.Namer.ConfigSecretName(params.Type, params.Owner.GetName()),
			Labels:    common.AddCredentialsLabel(params.Labels),
		},
		Data: map[string][]byte{
			configFileName(params.Type): cfgBytes,
		},
	}

	// reconcile
	if _, err = reconciler.ReconcileSecret(params.Client, expected, params.Owner); err != nil {
		return err
	}

	// we need to deref the secret here (if any) to include it in the checksum otherwise Beat will not be rolled on contents changes
	assocConf := params.Associated.AssociationConf()
	if assocConf.AuthIsConfigured() {
		esAuthKey := types.NamespacedName{Name: assocConf.GetAuthSecretName(), Namespace: params.Owner.GetNamespace()}
		esAuthSecret := corev1.Secret{}
		if err := params.Client.Get(esAuthKey, &esAuthSecret); err != nil {
			return err
		}
		_, _ = checksum.Write(esAuthSecret.Data[assocConf.GetAuthSecretKey()])
	}

	_, _ = checksum.Write(cfgBytes)

	return nil
}

func configFileName(typ string) string {
	return fmt.Sprintf("%s.yml", typ)
}

func ConfigMountPath(typ string) string {
	return path.Join(ConfigMountDirPath, configFileName(typ))
}

func reconcilePodVehicle(dp DriverParams, defaultImage container.Image, f func(builder *defaults.PodTemplateBuilder), checksum hash.Hash) (DriverStatus, error) {
	var podTemplate corev1.PodTemplateSpec
	if dp.DaemonSet != nil {
		podTemplate = dp.DaemonSet.PodTemplate
	} else if dp.Deployment != nil {
		podTemplate = dp.Deployment.PodTemplate
	}

	// Token mounting gets defaulted to false, which prevents from detecting whether user set it.
	// Instead, checking that here, before the default is applied.
	if podTemplate.Spec.AutomountServiceAccountToken == nil {
		t := true
		podTemplate.Spec.AutomountServiceAccountToken = &t
	}

	builder := defaults.NewPodTemplateBuilder(podTemplate, dp.Type).
		WithTerminationGracePeriod(30).
		WithEnv(corev1.EnvVar{
			Name: "NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			}}).
		WithResources(defaultResources).
		WithHostNetwork().
		WithLabels(map[string]string{
			ConfigChecksumLabel: fmt.Sprintf("%x", checksum.Sum(nil)),
			VersionLabelName:    dp.Version}).
		WithDockerImage(dp.Image, container.ImageRepository(defaultImage, dp.Version)).
		WithArgs("-e", "-c", ConfigMountPath(dp.Type)).
		WithDNSPolicy(corev1.DNSClusterFirstWithHostNet).
		WithSecurityContext(corev1.SecurityContext{
			RunAsUser: pointer.Int64(0),
		})

	if ShouldSetupAutodiscoverRBAC() {
		autodiscoverServiceAccountName := ServiceAccountName(dp.Owner.GetName())
		// If SA is already provided, the call will be no-op. This is fine as we then assume
		// that for this resource (despite operator configuration) the user took the responsibility
		// of configuring RBAC.
		builder.WithServiceAccount(autodiscoverServiceAccountName)
	}

	volumes := []volume.VolumeLike{
		volume.NewSecretVolume(
			dp.Namer.ConfigSecretName(dp.Type, dp.Owner.GetName()),
			ConfigVolumeName,
			ConfigMountPath(dp.Type),
			configFileName(dp.Type),
			0600),
	}

	if dp.Associated.AssociationConf().IsConfigured() {
		volumes = append(volumes, volume.NewSelectiveSecretVolumeWithMountPath(
			dp.Associated.AssociationConf().CASecretName,
			CAVolumeName,
			CAMountPath,
			[]string{CAFileName}))
	}

	for _, v := range volumes {
		builder = builder.WithVolumes(v.Volume()).WithVolumeMounts(v.VolumeMount())
	}

	if f != nil {
		f(builder)
	}

	builder = builder.WithLabels(commonhash.SetTemplateHashLabel(dp.Labels, builder.PodTemplate))

	name := dp.Namer.Name(dp.Type, dp.Owner.GetName())
	ds := daemonset.New(builder.PodTemplate, name, dp.Owner, dp.Selectors)

	replicas := int32(1)
	if dp.Deployment != nil && dp.Deployment.Replicas != nil {
		replicas = *dp.Deployment.Replicas
	}
	d := deployment.New(deployment.Params{
		Name:            name,
		Namespace:       dp.Owner.GetNamespace(),
		Selector:        dp.Selectors,
		Labels:          dp.Labels,
		PodTemplateSpec: builder.PodTemplate,
		Replicas:        replicas,
	})

	var ready, desired int32
	var toDelete runtime.Object
	switch {
	case dp.DaemonSet != nil:
		{
			if err := controllerutil.SetControllerReference(dp.Owner, &ds, scheme.Scheme); err != nil {
				return DriverStatus{}, err
			}
			reconciled, err := daemonset.Reconcile(dp.Client, ds, dp.Owner)
			if err != nil {
				return DriverStatus{}, err
			}
			ready = reconciled.Status.NumberReady
			desired = reconciled.Status.DesiredNumberScheduled
			toDelete = &d
		}
	case dp.Deployment != nil:
		{
			if err := controllerutil.SetControllerReference(dp.Owner, &d, scheme.Scheme); err != nil {
				return DriverStatus{}, err
			}
			// sync
			reconciled, err := deployment.Reconcile(dp.Client, d, dp.Owner)
			if err != nil {
				return DriverStatus{}, err
			}
			ready = reconciled.Status.ReadyReplicas
			desired = reconciled.Status.Replicas
			toDelete = &ds
		}
	}

	if err := dp.Client.Delete(toDelete); err != nil && !apierrors.IsNotFound(err) {
		return DriverStatus{}, err
	}

	return DriverStatus{
		ExpectedNodes:  desired,
		AvailableNodes: ready,
		Health:         health.CalculateHealth(dp.Associated, ready, desired),
		Association:    dp.Associated.AssociationStatus(),
	}, nil
}
