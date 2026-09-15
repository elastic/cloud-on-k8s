// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func EnsureNamespace(c k8s.Client, ns string) error {
	expected := corev1.Namespace{
		Name: ns,
	}
	existing := corev1.Namespace{}
	err := c.Get(context.Background(), types.NamespacedName{Name: ns}, &existing)
	if errors.IsNotFound(err) {
		return c.Create(context.Background(), &expected)
	}
	return err
}
