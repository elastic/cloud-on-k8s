// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package finalizer

import (
	"context"
	"errors"
	"regexp"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var finalizersRegExp = regexp.MustCompile(`^finalizer\.(.*)\.k8s.elastic.co\/(.*)$`)

// RemoveAll removes all existing Elastic Finalizers on an Object
func RemoveAll(ctx context.Context, c k8s.Client, obj client.Object) error {
	if len(obj.GetFinalizers()) == 0 {
		return nil
	}
	filtered := filterFinalizers(obj.GetFinalizers())
	if len(filtered) == len(obj.GetFinalizers()) {
		return nil
	}
	base, ok := obj.DeepCopyObject().(client.Object)
	if !ok {
		return errors.New("failed to convert deep copy to client.Object")
	}
	obj.SetFinalizers(filtered)
	return c.Patch(ctx, obj, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
}

// filterFinalizers removes Elastic finalizers
func filterFinalizers(finalizers []string) []string {
	filteredFinalizers := make([]string, 0)
	for _, finalizer := range finalizers {
		if !finalizersRegExp.MatchString(finalizer) {
			filteredFinalizers = append(filteredFinalizers, finalizer)
		}
	}
	return filteredFinalizers
}
