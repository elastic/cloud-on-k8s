/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package stack

import (
	"io/ioutil"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/client-go/kubernetes/scheme"
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

// StorageClassTemplate returns a provider specific storage class template
func StorageClassTemplate() (*v1.StorageClass, error) {
	var sc v1.StorageClass
	bytes, err := ioutil.ReadFile(params.StorageClassTemplate)
	if err != nil {
		return &sc, err
	}
	object, _, err := scheme.Codecs.UniversalDeserializer().Decode(bytes, nil, &sc)
	return object.(*v1.StorageClass), err
}
