// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package helper

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// SetNamespaceLabel sets the given label on the namespace identified by name.
func SetNamespaceLabel(ctx context.Context, k k8s.Client, name, key, value string) error {
	var ns corev1.Namespace
	if err := k.Get(context.Background(), types.NamespacedName{Name: name}, &ns); err != nil {
		return err
	}
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	ns.Labels[key] = value
	return k.Update(ctx, &ns)
}

func DeleteNamespaceLabel(ctx context.Context, k k8s.Client, labelName string, namespacesNames ...string) error {
	errs := make([]error, 0, len(namespacesNames))
	for _, nsName := range namespacesNames {
		var ns corev1.Namespace
		if err := k.Get(context.Background(), types.NamespacedName{Name: nsName}, &ns); err == nil {
			delete(ns.Labels, labelName)
			errs = append(errs, k.Update(context.Background(), &ns))
		}
	}

	return errors.Join(errs...)
}
