// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operator

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// ShouldManageNamespace determines if the operator should manage resources in the given namespace
// based on the configured namespace label selector.
func (p Parameters) ShouldManageNamespace(ctx context.Context, c client.Client, namespace string) (bool, error) {
	// If no namespace label selector is configured, manage all namespaces (backwards compatibility)
	if p.NamespaceLabelSelector == nil {
		return true, nil
	}

	log := ulog.FromContext(ctx)

	// Get the namespace object
	var ns corev1.Namespace
	if err := c.Get(ctx, client.ObjectKey{Name: namespace}, &ns); err != nil {
		log.Error(err, "Failed to get namespace", "namespace", namespace)
		return false, err
	}

	// Convert LabelSelector to labels.Selector
	selector, err := metav1.LabelSelectorAsSelector(p.NamespaceLabelSelector)
	if err != nil {
		log.Error(err, "Failed to convert namespace label selector", "selector", p.NamespaceLabelSelector)
		return false, err
	}

	// Check if namespace labels match the selector
	matches := selector.Matches(labels.Set(ns.Labels))

	log.V(1).Info("Namespace filtering check",
		"namespace", namespace,
		"labels", ns.Labels,
		"selector", p.NamespaceLabelSelector,
		"matches", matches)

	return matches, nil
}
