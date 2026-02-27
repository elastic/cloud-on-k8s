// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// ManagedAnnotation annotation
	LegacyPauseAnnoation = "common.k8s.elastic.co/pause"
	ManagedAnnotation    = "eck.k8s.elastic.co/managed"
)

// IsUnmanaged checks if a given resource is currently unmanaged.
func IsUnmanaged(ctx context.Context, object metav1.Object) bool {
	managed, exists := object.GetAnnotations()[ManagedAnnotation]
	if exists && managed == "false" {
		return true
	}

	paused, exists := object.GetAnnotations()[LegacyPauseAnnoation]
	if exists {
		ulog.FromContext(ctx).Info(fmt.Sprintf("%s is deprecated, please use %s", LegacyPauseAnnoation, ManagedAnnotation), "namespace", object.GetNamespace(), "name", object.GetName())
	}
	return exists && paused == "true"
}

// IsUnmanagedOrFiltered checks if a given resource is currently unmanaged or if its namespace
// should not be managed based on the operator's namespace label selector configuration.
func IsUnmanagedOrFiltered(ctx context.Context, c client.Client, object metav1.Object, params operator.Parameters) (bool, error) {
	log := ulog.FromContext(ctx)

	// First check if the resource is explicitly unmanaged
	if IsUnmanaged(ctx, object) {
		return true, nil
	}

	// Then check namespace filtering
	shouldManage, err := params.ShouldManageNamespace(ctx, c, object.GetNamespace())
	if err != nil {
		return false, err
	}

	if !shouldManage {
		log.V(1).Info("Namespace is excluded by namespace label selector", "namespace", object.GetNamespace(), "name", object.GetName())
		return true, nil
	}

	return false, nil
}
