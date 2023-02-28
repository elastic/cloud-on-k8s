// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"hash"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
)

func reconcilePipeline(params Params, configHash hash.Hash) error {
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

	if _, err = reconciler.ReconcileSecret(params.Context, params.Client, expected, &params.Logstash); err != nil {
		return err
	}

	_, _ = configHash.Write(cfgBytes)

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

	cfg := defaultPipeline()
	return cfg.Render()
}

// getUserPipeline extracts the pipeline either from the spec `pipeline` field or from the Secret referenced by spec
// `pipelineRef` field.
func getUserPipeline(params Params) (*PipelinesConfig, error) {
	if params.Logstash.Spec.Pipelines != nil {
		return NewPipelinesConfigFrom(params.Logstash.Spec.Pipelines)
	}
	return ParseConfigRef(params, &params.Logstash, params.Logstash.Spec.PipelinesRef, PipelineFileName)
}

func defaultPipeline() *PipelinesConfig {
	pipelines := []map[string]string{
		{
			"pipeline.id":   "demo",
			"config.string": "input { exec { command => \"uptime\" interval => 5 } } output { stdout{} }",
		},
	}

	return MustPipelinesConfig(pipelines)
}
