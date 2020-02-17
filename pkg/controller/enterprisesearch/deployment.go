package enterprisesearch

import (
	"context"

	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	entsname "github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

func (r *ReconcileEnterpriseSearch) reconcileDeployment(
	ctx context.Context,
	state State,
	ents entsv1beta1.EnterpriseSearch,
) (State, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	// TODO: config
	//reconciledConfigSecret, err := config.Reconcile(r.Client, r.scheme, as)
	//if err != nil {
	//	return state, err
	//}

	params, err := r.deploymentParams(ents)
	if err != nil {
		return state, err
	}

	deploy := deployment.New(params)
	result, err := deployment.Reconcile(r.K8sClient(), r.Scheme(), deploy, &ents)
	if err != nil {
		return state, err
	}
	state.UpdateEnterpriseSearchState(result)
	return state, nil
}


func (r *ReconcileEnterpriseSearch) deploymentParams(ents entsv1beta1.EnterpriseSearch) (deployment.Params, error) {
	podSpec := newPodSpec(ents)
	podLabels := NewLabels(ents.Name)

	//// Build a checksum of the configuration, add it to the pod labels so a change triggers a rolling update
	//configChecksum := sha256.New224()
	//_, _ = configChecksum.Write(params.ConfigSecret.Data[config.ApmCfgSecretKey])
	//if params.keystoreResources != nil {
	//	_, _ = configChecksum.Write([]byte(params.keystoreResources.Version))
	//}

	//if ents.AssociationConf().CAIsConfigured() {
	//	esCASecretName := ents.AssociationConf().GetCASecretName()
	//	// TODO: this is a little ugly as it reaches into the ES controller bits
	//	esCAVolume := volume.NewSecretVolumeWithMountPath(
	//		esCASecretName,
	//		"elasticsearch-certs",
	//		filepath.Join(ApmBaseDir, config.CertificatesDir),
	//	)
	//
	//	// build a checksum of the cert file used by ES, which we can use to cause the Deployment to roll the Apm Server
	//	// instances in the deployment when the ca file contents change. this is done because Apm Server do not support
	//	// updating the CA file contents without restarting the process.
	//	certsChecksum := ""
	//	var esPublicCASecret corev1.Secret
	//	key := types.NamespacedName{Namespace: as.Namespace, Name: esCASecretName}
	//	if err := r.Get(key, &esPublicCASecret); err != nil {
	//		return deployment.Params{}, err
	//	}
	//	if certPem, ok := esPublicCASecret.Data[certificates.CertFileName]; ok {
	//		certsChecksum = fmt.Sprintf("%x", sha256.Sum224(certPem))
	//	}
	//	// we add the checksum to a label for the deployment and its pods (the important bit is that the pod template
	//	// changes, which will trigger a rolling update)
	//	podLabels[esCAChecksumLabelName] = certsChecksum
	//
	//	podSpec.Spec.Volumes = append(podSpec.Spec.Volumes, esCAVolume.Volume())
	//
	//	for i := range podSpec.Spec.InitContainers {
	//		podSpec.Spec.InitContainers[i].VolumeMounts = append(podSpec.Spec.InitContainers[i].VolumeMounts, esCAVolume.VolumeMount())
	//	}
	//
	//	for i := range podSpec.Spec.Containers {
	//		podSpec.Spec.Containers[i].VolumeMounts = append(podSpec.Spec.Containers[i].VolumeMounts, esCAVolume.VolumeMount())
	//	}
	//}
	//
	//if as.Spec.HTTP.TLS.Enabled() {
	//	// fetch the secret to calculate the checksum
	//	var httpCerts corev1.Secret
	//	err := r.Get(types.NamespacedName{
	//		Namespace: as.Namespace,
	//		Name:      certificates.HTTPCertsInternalSecretName(apmname.APMNamer, as.Name),
	//	}, &httpCerts)
	//	if err != nil {
	//		return deployment.Params{}, err
	//	}
	//	if httpCert, ok := httpCerts.Data[certificates.CertFileName]; ok {
	//		_, _ = configChecksum.Write(httpCert)
	//	}
	//	httpCertsVolume := http.HTTPCertSecretVolume(apmname.APMNamer, as.Name)
	//	podSpec.Spec.Volumes = append(podSpec.Spec.Volumes, httpCertsVolume.Volume())
	//	apmServerContainer := pod.ContainerByName(podSpec.Spec, apmv1.ApmServerContainerName)
	//	apmServerContainer.VolumeMounts = append(apmServerContainer.VolumeMounts, httpCertsVolume.VolumeMount())
	//}
	//
	//podLabels[configChecksumLabelName] = fmt.Sprintf("%x", configChecksum.Sum(nil))
	//// TODO: also need to hash secret token?

	podSpec.Labels = maps.MergePreservingExistingKeys(podSpec.Labels, podLabels)

	return deployment.Params{
		Name:            entsname.Deployment(ents.Name),
		Namespace:       ents.Namespace,
		Replicas:        ents.Spec.Count,
		Selector:        NewLabels(ents.Name),
		Labels:          NewLabels(ents.Name),
		PodTemplateSpec: podSpec,
		Strategy:        appsv1.RollingUpdateDeploymentStrategyType,
	}, nil
}

