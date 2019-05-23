// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func StartTrial(t *testing.T, c k8s.Client, namespace string) {
	l := v1alpha1.EnterpriseLicense{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trial-license",
			Namespace: namespace,
		},
		Spec: v1alpha1.EnterpriseLicenseSpec{
			Type: v1alpha1.LicenseTypeEnterpriseTrial,
		},
	}
	require.NoError(t, c.Create(&l))
	_, err := license.InitTrial(c, &l)
	require.NoError(t, err)
}
