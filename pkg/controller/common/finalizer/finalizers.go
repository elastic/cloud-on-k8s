// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package finalizer

import (
	"context"
	"regexp"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var finalizersRegExp = regexp.MustCompile(`^finalizer\.(.*)\.k8s.elastic.co\/(.*)$`)

// RemoveAll removes all existing Elastic Finalizers on an Object
func RemoveAll(c k8s.Client, obj client.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	if len(accessor.GetFinalizers()) == 0 {
		return nil
	}
	filterFinalizers := filterFinalizers(accessor.GetFinalizers())
	accessor.SetFinalizers(filterFinalizers)
	return c.Update(context.Background(), obj)
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
