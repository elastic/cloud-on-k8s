/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package stack

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateStorageClass(storageClass v1.StorageClass, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Should create a custom storage class",
		Test: func(t *testing.T) {
			// delete if exists
			_ = k.Client.Delete(&storageClass)
			// (re-) create
			require.NoError(t, k.Client.Create(&storageClass))
		},
	}
}

// DefaultStorageClass returns a provider specific storage class template
func DefaultStorageClass(k *helpers.K8sHelper) (*v1.StorageClass, error) {
	var scs v1.StorageClassList
	if err := k.Client.List(&client.ListOptions{}, &scs); err != nil {
		return nil, err
	}
	for _, sc := range scs.Items {
		if isDefaultAnnotation(sc.ObjectMeta) {
			return &sc, nil
		}

	}
	return nil, errors.New("no default storage class found")
}

const IsDefaultStorageClassAnnotation = "storageclass.kubernetes.io/is-default-class"
const BetaIsDefaultStorageClassAnnotation = "storageclass.beta.kubernetes.io/is-default-class"

// IsDefaultAnnotation returns a boolean if
// the annotation is set
func isDefaultAnnotation(obj metav1.ObjectMeta) bool {
	if obj.Annotations[IsDefaultStorageClassAnnotation] == "true" {
		return true
	}
	if obj.Annotations[BetaIsDefaultStorageClassAnnotation] == "true" {
		return true
	}

	return false
}
