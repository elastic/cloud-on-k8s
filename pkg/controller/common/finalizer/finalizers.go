// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package finalizer

import (
	"regexp"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

const finalizersRegExp = `^finalizer\.(.*)\.k8s.elastic.co\/(.*)$`

// RemoveAll removes all existing Finalizers on an Object
func RemoveAll(c k8s.Client, obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	if len(accessor.GetFinalizers()) == 0 {
		return nil
	}
	filterFinalizers, err := filterFinalizers(accessor.GetFinalizers())
	if err != nil {
		return err
	}
	accessor.SetFinalizers(filterFinalizers)
	return c.Update(obj)
}

// filterFinalizers removes Elastic finalizers
func filterFinalizers(finalizers []string) ([]string, error) {
	filteredFinalizers := make([]string, 0)
	r, err := regexp.Compile(finalizersRegExp)
	if err != nil {
		return filteredFinalizers, err
	}
	for _, finalizer := range finalizers {
		if !r.MatchString(finalizer) {
			filteredFinalizers = append(filteredFinalizers, finalizer)
		}
	}
	return filteredFinalizers, nil
}
