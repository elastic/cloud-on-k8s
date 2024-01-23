// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	lslabels "github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/pipelines"
)

const (
	PipelineFileName = "pipelines.yml"
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
			Labels:    labels.AddCredentialsLabel(lslabels.NewLabels(params.Logstash)),
		},
		Data: map[string][]byte{
			PipelineFileName: cfgBytes,
		},
	}

	if _, err := reconciler.ReconcileSecret(params.Context, params.Client, expected, &params.Logstash,
		reconciler.WithPostUpdate(func() {
			annotation.MarkPodsAsUpdated(params.Context, params.Client,
				client.InNamespace(params.Logstash.Namespace),
				lslabels.NewLabelSelectorForLogstash(params.Logstash),
			)
		}),
	); err != nil {
		return err
	}
	return nil
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
