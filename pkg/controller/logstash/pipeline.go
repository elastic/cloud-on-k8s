// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/pipelines"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

func reconcilePipeline(params Params) error {
	defer tracing.Span(&params.Context)()

	cfgBytes, err := buildPipeline(params)
	if err != nil {
		return err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Logstash.Namespace,
			Name:      logstashv1alpha1.PipelineSecretName(params.Logstash.Name),
			Labels:    labels.AddCredentialsLabel(NewLabels(params.Logstash)),
		},
		Data: map[string][]byte{
			PipelineFileName: cfgBytes,
		},
	}

	if err := reconcileSecretWithFastUpdate(params, expected); err != nil {
		return err
	}
	return nil
}

// This function reconciles the secret, but then adds a postUpdate step to mark the pods as updated
// to trigger a quicker reload of the updated secret than waiting for the kubelet sync interval to kick in
func reconcileSecretWithFastUpdate(params Params, expected corev1.Secret) error {
	var reconciled corev1.Secret

	return reconciler.ReconcileResource(reconciler.Params{
		Context:    params.Context,
		Client:     params.Client,
		Owner:      &params.Logstash,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			// update if expected labels and annotations are not there
			return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
				!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
				// or if secret data is not strictly equal
				!reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			// set expected annotations and labels, but don't remove existing ones
			// that may have been defaulted or set by the user on the existing resource
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			reconciled.Data = expected.Data
		},
		PostUpdate: func() {
			annotation.MarkPodsAsUpdated(params.Context, params.Client,
				client.InNamespace(params.Logstash.Namespace),
				NewLabelSelectorForLogstash(params.Logstash),
			)
		},
	})
}

func buildPipeline(params Params) ([]byte, error) {
	userProvidedCfg, err := getUserPipeline(params)
	if err != nil {
		return nil, err
	}

	if userProvidedCfg != nil {
		return userProvidedCfg.Render()
	}

	cfg := defaultPipeline
	return cfg.Render()
}

// getUserPipeline extracts the pipeline either from the spec `pipeline` field or from the Secret referenced by spec
// `pipelineRef` field.
func getUserPipeline(params Params) (*pipelines.Config, error) {
	if params.Logstash.Spec.Pipelines != nil {
		pipes := make([]map[string]interface{}, 0, len(params.Logstash.Spec.Pipelines))
		for _, p := range params.Logstash.Spec.Pipelines {
			pipes = append(pipes, p.Data)
		}

		return pipelines.FromSpec(pipes)
	}
	return pipelines.ParsePipelinesRef(params, &params.Logstash, params.Logstash.Spec.PipelinesRef, PipelineFileName)
}

var (
	defaultPipeline = pipelines.MustFromSpec([]map[string]string{
		{
			"pipeline.id": "main",
			"path.config": "/usr/share/logstash/pipeline",
		},
	})
)
