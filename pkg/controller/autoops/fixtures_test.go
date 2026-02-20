// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

func newAutoOpsAgentPolicy(modifiers ...func(*autoopsv1alpha1.AutoOpsAgentPolicy)) autoopsv1alpha1.AutoOpsAgentPolicy {
	a := autoopsv1alpha1.AutoOpsAgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "ns-1",
		},
		Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
			Version: "9.2.1",
			AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
				SecretName: "config-secret",
			},
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "elasticsearch"},
			},
		},
	}

	for _, m := range modifiers {
		m(&a)
	}

	return a
}

func newSecret(modifiers ...func(*corev1.Secret)) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-secret",
			Namespace: "ns-1",
		},
		Data: map[string][]byte{
			"cloud-connected-mode-api-key": []byte("test-key"),
			"autoops-otel-url":             []byte("https://test-url"),
			"autoops-token":                []byte("test-token"),
		},
	}

	for _, m := range modifiers {
		m(s)
	}

	return s
}

func newElasticsearch(modifiers ...func(*esv1.Elasticsearch)) *esv1.Elasticsearch {
	s := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-1",
			Namespace: "ns-1",
			Labels:    map[string]string{"app": "elasticsearch"},
		},
		Status: esv1.ElasticsearchStatus{
			Phase: esv1.ElasticsearchReadyPhase,
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "9.2.1",
		},
	}

	for _, m := range modifiers {
		m(s)
	}

	return s
}
