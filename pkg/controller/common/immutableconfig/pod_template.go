// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package immutableconfig

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodTemplateExtractor extracts pod templates from resources that own pods.
// Implement this interface to support custom resource types for GC protection.
// The extractor encapsulates both the resource type and the label selector used
// to find resources whose pod templates should be protected from garbage collection.
type PodTemplateExtractor interface {
	// ListPodTemplates returns pod templates from resources in the given namespace.
	// The returned templates are used to determine which immutable resources are still
	// in use and should be protected from garbage collection.
	ListPodTemplates(ctx context.Context, c client.Client, namespace string) ([]corev1.PodTemplateSpec, error)
}

// ReplicaSetExtractor extracts pod templates from ReplicaSets matching the given labels.
// Use this for Deployments, which create ReplicaSets to manage pods.
type ReplicaSetExtractor struct {
	labels client.MatchingLabels
}

// NewReplicaSetExtractor creates a ReplicaSetExtractor that finds ReplicaSets matching the given labels.
// The labels parameter should not be empty; passing nil or empty labels matches all ReplicaSets
// in the namespace, which may lead to overly broad GC protection decisions.
func NewReplicaSetExtractor(labels client.MatchingLabels) ReplicaSetExtractor {
	return ReplicaSetExtractor{labels: labels}
}

// ListPodTemplates lists ReplicaSets matching the configured labels and returns their pod templates.
func (e ReplicaSetExtractor) ListPodTemplates(ctx context.Context, c client.Client, namespace string) ([]corev1.PodTemplateSpec, error) {
	var rsList appsv1.ReplicaSetList
	if err := c.List(ctx, &rsList, client.InNamespace(namespace), e.labels); err != nil {
		return nil, err
	}
	templates := make([]corev1.PodTemplateSpec, len(rsList.Items))
	for i := range rsList.Items {
		templates[i] = rsList.Items[i].Spec.Template
	}
	return templates, nil
}
