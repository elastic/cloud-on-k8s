package kibana

import (
	"crypto/sha256"
	"fmt"
	"path"

	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/kibana/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/kibana/version/version6"
	"github.com/elastic/k8s-operators/operators/pkg/controller/kibana/version/version7"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type driver struct {
	client     k8s.Client
	scheme     *runtime.Scheme
	newPodSpec func(params pod.SpecParams) corev1.PodSpec
}

func (d *driver) Reconcile(
	state *State,
	kb *kbtype.Kibana,
) *reconciler.Results {
	results := reconciler.Results{}
	if !kb.Spec.Elasticsearch.IsConfigured() {
		log.Info("Aborting Kibana deployment reconciliation as no Elasticsearch backend is configured")
		return &results
	}

	kibanaPodSpecParams := pod.SpecParams{
		Version:          kb.Spec.Version,
		CustomImageName:  kb.Spec.Image,
		ElasticsearchUrl: kb.Spec.Elasticsearch.URL,
		User:             kb.Spec.Elasticsearch.Auth,
	}

	kibanaPodSpec := d.newPodSpec(kibanaPodSpecParams)
	labels := NewLabels(kb.Name)
	podLabels := NewLabels(kb.Name)

	if kb.Spec.Elasticsearch.CaCertSecret != nil {
		// TODO: use kibanaCa to generate cert for deployment
		// to do that, EnsureNodeCertificateSecretExists needs a deployment variant.

		// TODO: this is a little ugly as it reaches into the ES controller bits
		esCertsVolume := volume.NewSecretVolumeWithMountPath(
			*kb.Spec.Elasticsearch.CaCertSecret,
			"elasticsearch-certs",
			"/usr/share/kibana/config/elasticsearch-certs",
		)

		// build a checksum of the ca file used by ES, which we can use to cause the Deployment to roll the Kibana
		// instances in the deployment when the ca file contents change. this is done because Kibana do not support
		// updating the CA file contents without restarting the process.
		caChecksum := ""
		var esPublicCASecret corev1.Secret
		key := types.NamespacedName{Namespace: kb.Namespace, Name: *kb.Spec.Elasticsearch.CaCertSecret}
		if err := d.client.Get(key, &esPublicCASecret); err != nil {
			return results.WithError(err)
		}
		if capem, ok := esPublicCASecret.Data[certificates.CAFileName]; ok {
			caChecksum = fmt.Sprintf("%x", sha256.Sum224(capem))
		}
		// we add the checksum to a label for the deployment and its pods (the important bit is that the pod template
		// changes, which will trigger a rolling update)
		podLabels[caChecksumLabelName] = caChecksum

		kibanaPodSpec.Volumes = append(kibanaPodSpec.Volumes, esCertsVolume.Volume())

		for i, container := range kibanaPodSpec.InitContainers {
			kibanaPodSpec.InitContainers[i].VolumeMounts = append(container.VolumeMounts, esCertsVolume.VolumeMount())
		}

		for i, container := range kibanaPodSpec.Containers {
			kibanaPodSpec.Containers[i].VolumeMounts = append(container.VolumeMounts, esCertsVolume.VolumeMount())

			kibanaPodSpec.Containers[i].Env = append(
				kibanaPodSpec.Containers[i].Env,
				corev1.EnvVar{
					Name:  "ELASTICSEARCH_SSL_CERTIFICATEAUTHORITIES",
					Value: path.Join(esCertsVolume.VolumeMount().MountPath, certificates.CAFileName),
				},
				corev1.EnvVar{
					Name:  "ELASTICSEARCH_SSL_VERIFICATIONMODE",
					Value: "certificate",
				},
			)
		}
	}

	deploy := NewDeployment(DeploymentParams{
		// TODO: revisit naming?
		Name:      PseudoNamespacedResourceName(*kb),
		Namespace: kb.Namespace,
		Replicas:  kb.Spec.NodeCount,
		Selector:  labels,
		Labels:    labels,
		PodLabels: podLabels,
		PodSpec:   kibanaPodSpec,
	})
	result, err := ReconcileDeployment(d.client, d.scheme, deploy, kb)
	if err != nil {
		return results.WithError(err)
	}
	state.UpdateKibanaState(result)
	res, err := common.ReconcileService(d.client, d.scheme, NewService(*kb), kb)
	if err != nil {
		// TODO: consider updating some status here?
		return results.WithError(err)
	}
	return results.WithResult(res)
}

func newDriver(client k8s.Client, scheme *runtime.Scheme, version version.Version) *driver {
	d := driver{
		client: client,
		scheme: scheme,
	}
	switch version.Major {
	case 6:
		d.newPodSpec = version6.NewPodSpec
	case 7:
		d.newPodSpec = version7.NewPodSpec
	}
	return &d

}
