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
)

func StartTrial(t *testing.T, c k8s.Client, namespace string) {
	l := license.SourceEnterpriseLicense{
		Data: license.SourceLicenseData{
			Type: string(v1alpha1.LicenseTypeEnterpriseTrial),
		},
	}
	_, err := license.InitTrial(c, namespace, &l)
	require.NoError(t, err)
}
