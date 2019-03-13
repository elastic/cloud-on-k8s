// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"crypto/sha256"
	"fmt"
	"path"

	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/finalizer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
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
	client         k8s.Client
	scheme         *runtime.Scheme
	newPodSpec     func(params pod.SpecParams) corev1.PodSpec
	dynamicWatches watches.DynamicWatches
}

func secretWatchKey(kibana kbtype.Kibana) string {
	return fmt.Sprintf("%s-%s-es-auth-secret", kibana.Namespace, kibana.Name)
}

func secretWatchFinalizer(kibana kbtype.Kibana, watches watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "es-auth-secret.kibana.k8s.elastic.co",
		Execute: func() error {
			watches.Secrets.RemoveHandlerForKey(secretWatchKey(kibana))
			return nil
		},
	}
}

func (d *driver) deploymentParams(kb *kbtype.Kibana) (*DeploymentParams, error) {
	kibanaPodSpecParams := pod.SpecParams{
		Version:          kb.Spec.Version,
		CustomImageName:  kb.Spec.Image,
		ElasticsearchUrl: kb.Spec.Elasticsearch.URL,
		User:             kb.Spec.Elasticsearch.Auth,
	}

	kibanaPodSpec := d.newPodSpec(kibanaPodSpecParams)
	labels := NewLabels(kb.Name)
	podLabels := NewLabels(kb.Name)

	// build a checksum of the configuration, which we can use to cause the Deployment to roll the Kibana
	// instances in the deployment when the ca file contents or credentials change. this is done because Kibana does not support
	// updating the CA file contents or credentials without restarting the process.
	configChecksum := sha256.New224()
	// we need to deref the secret here (if any) to include it in the checksum otherwise Kibana will not be rolled on contents changes
	if kb.Spec.Elasticsearch.Auth.SecretKeyRef != nil {
		ref := kb.Spec.Elasticsearch.Auth.SecretKeyRef
		esAuthSecret := types.NamespacedName{Name: ref.Name, Namespace: kb.Namespace}
		d.dynamicWatches.Secrets.AddHandler(watches.NamedWatch{
			Name:    secretWatchKey(*kb),
			Watched: esAuthSecret,
			Watcher: k8s.ExtractNamespacedName(kb),
		})
		sec := corev1.Secret{}
		if err := d.client.Get(esAuthSecret, &sec); err != nil {
			return nil, err
		}
		configChecksum.Write(sec.Data[ref.Key])
	} else {
		d.dynamicWatches.Secrets.RemoveHandlerForKey(secretWatchKey(*kb))
	}

	if kb.Spec.Elasticsearch.CaCertSecret != nil {
		// TODO: use kibanaCa to generate cert for deployment
		// to do that, EnsureNodeCertificateSecretExists needs a deployment variant.

		// TODO: this is a little ugly as it reaches into the ES controller bits
		esCertsVolume := volume.NewSecretVolumeWithMountPath(
			*kb.Spec.Elasticsearch.CaCertSecret,
			"elasticsearch-certs",
			"/usr/share/kibana/config/elasticsearch-certs",
		)

		var esPublicCASecret corev1.Secret
		key := types.NamespacedName{Namespace: kb.Namespace, Name: *kb.Spec.Elasticsearch.CaCertSecret}
		if err := d.client.Get(key, &esPublicCASecret); err != nil {
			return nil, err
		}
		if capem, ok := esPublicCASecret.Data[certificates.CAFileName]; ok {
			configChecksum.Write(capem)
		}

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
	// we add the checksum to a label for the deployment and its pods (the important bit is that the pod template
	// changes, which will trigger a rolling update)
	podLabels[configChecksumLabel] = fmt.Sprintf("%x", configChecksum.Sum(nil))

	return &DeploymentParams{
		// TODO: revisit naming?
		Name:      PseudoNamespacedResourceName(*kb),
		Namespace: kb.Namespace,
		Replicas:  kb.Spec.NodeCount,
		Selector:  labels,
		Labels:    labels,
		PodLabels: podLabels,
		PodSpec:   kibanaPodSpec,
	}, nil
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

	params, err := d.deploymentParams(kb)
	if err != nil {
		return results.WithError(err)
	}
	expectedDp := NewDeployment(*params)
	reconciledDp, err := ReconcileDeployment(d.client, d.scheme, expectedDp, kb)
	if err != nil {
		return results.WithError(err)
	}
	state.UpdateKibanaState(reconciledDp)
	res, err := common.ReconcileService(d.client, d.scheme, NewService(*kb), kb)
	if err != nil {
		// TODO: consider updating some status here?
		return results.WithError(err)
	}
	return results.WithResult(res)
}

func newDriver(
	client k8s.Client,
	scheme *runtime.Scheme,
	version version.Version,
	watches watches.DynamicWatches,
) (*driver, error) {
	d := driver{
		client:         client,
		scheme:         scheme,
		dynamicWatches: watches,
	}
	switch version.Major {
	case 6:
		d.newPodSpec = version6.NewPodSpec
	case 7:
		d.newPodSpec = version7.NewPodSpec
	default:
		return nil, fmt.Errorf("unsupported version: %s", version)
	}
	return &d, nil

}
