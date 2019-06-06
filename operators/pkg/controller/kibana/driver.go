// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"crypto/sha256"
	"fmt"
	"path"

	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	kbname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/version/version6"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/version/version7"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

type driver struct {
	client             k8s.Client
	scheme             *runtime.Scheme
	newPodTemplateSpec func(kb kbtype.Kibana) corev1.PodTemplateSpec
	dynamicWatches     watches.DynamicWatches
}

func secretWatchKey(kibana kbtype.Kibana) string {
	return fmt.Sprintf("%s-%s-es-auth-secret", kibana.Namespace, kibana.Name)
}

func secretWatchFinalizer(kibana kbtype.Kibana, watches watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "es-auth-secret.finalizers.kibana.k8s.elastic.co",
		Execute: func() error {
			watches.Secrets.RemoveHandlerForKey(secretWatchKey(kibana))
			return nil
		},
	}
}

func (d *driver) deploymentParams(kb *kbtype.Kibana) (*DeploymentParams, error) {
	kibanaPodSpec := d.newPodTemplateSpec(*kb)

	// build a checksum of the configuration, which we can use to cause the Deployment to roll the Kibana
	// instances in the deployment when the ca file contents or credentials change. this is done because Kibana does not support
	// updating the CA file contents or credentials without restarting the process.
	configChecksum := sha256.New224()
	// we need to deref the secret here (if any) to include it in the checksum otherwise Kibana will not be rolled on contents changes
	if kb.Spec.Elasticsearch.Auth.SecretKeyRef != nil {
		ref := kb.Spec.Elasticsearch.Auth.SecretKeyRef
		esAuthSecret := types.NamespacedName{Name: ref.Name, Namespace: kb.Namespace}
		if err := d.dynamicWatches.Secrets.AddHandler(watches.NamedWatch{
			Name:    secretWatchKey(*kb),
			Watched: esAuthSecret,
			Watcher: k8s.ExtractNamespacedName(kb),
		}); err != nil {
			return nil, err
		}
		sec := corev1.Secret{}
		if err := d.client.Get(esAuthSecret, &sec); err != nil {
			return nil, err
		}
		configChecksum.Write(sec.Data[ref.Key])
	} else {
		d.dynamicWatches.Secrets.RemoveHandlerForKey(secretWatchKey(*kb))
	}

	if kb.Spec.Elasticsearch.CaCertSecret != "" {
		// TODO: use kibanaCa to generate cert for deployment

		// TODO: this is a little ugly as it reaches into the ES controller bits
		esCertsVolume := volume.NewSecretVolumeWithMountPath(
			kb.Spec.Elasticsearch.CaCertSecret,
			"elasticsearch-certs",
			"/usr/share/kibana/config/elasticsearch-certs",
		)

		var esPublicCASecret corev1.Secret
		key := types.NamespacedName{Namespace: kb.Namespace, Name: kb.Spec.Elasticsearch.CaCertSecret}
		// watch for changes in the CA secret
		if err := d.dynamicWatches.Secrets.AddHandler(watches.NamedWatch{
			Name:    secretWatchKey(*kb),
			Watched: key,
			Watcher: k8s.ExtractNamespacedName(kb),
		}); err != nil {
			return nil, err
		}

		if err := d.client.Get(key, &esPublicCASecret); err != nil {
			return nil, err
		}
		if capem, ok := esPublicCASecret.Data[certificates.CAFileName]; ok {
			configChecksum.Write(capem)
		}

		kibanaPodSpec.Spec.Volumes = append(kibanaPodSpec.Spec.Volumes, esCertsVolume.Volume())

		for i, container := range kibanaPodSpec.Spec.InitContainers {
			kibanaPodSpec.Spec.InitContainers[i].VolumeMounts = append(container.VolumeMounts, esCertsVolume.VolumeMount())
		}

		kibanaContainer := pod.GetKibanaContainer(kibanaPodSpec.Spec)
		kibanaContainer.VolumeMounts = append(kibanaContainer.VolumeMounts, esCertsVolume.VolumeMount())
		kibanaContainer.Env = append(
			kibanaContainer.Env,
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
	// we add the checksum to a label for the deployment and its pods (the important bit is that the pod template
	// changes, which will trigger a rolling update)
	kibanaPodSpec.Labels[configChecksumLabel] = fmt.Sprintf("%x", configChecksum.Sum(nil))

	deploymentLabels := label.NewLabels(kb.Name)

	return &DeploymentParams{
		Name:            kbname.KBNamer.Suffix(kb.Name),
		Namespace:       kb.Namespace,
		Replicas:        kb.Spec.NodeCount,
		Selector:        deploymentLabels,
		Labels:          deploymentLabels,
		PodTemplateSpec: kibanaPodSpec,
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
		switch {
		case version.Minor >= 6:
			// 6.6 docker container already defaults to v7 settings
			d.newPodTemplateSpec = version7.NewPodTemplateSpec
		default:
			d.newPodTemplateSpec = version6.NewPodTemplateSpec
		}
	case 7:
		d.newPodTemplateSpec = version7.NewPodTemplateSpec
	default:
		return nil, fmt.Errorf("unsupported version: %s", version)
	}
	return &d, nil

}
