/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package stack

import (
	"context"
	"fmt"
	"testing"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	licenseSecretName = "e2e-enterprise-license"
)

type licenseTestContext struct {
	esClient client.Client
	k        *helpers.K8sHelper
	es       estype.Elasticsearch
}

func NewLicenseTestContext(k *helpers.K8sHelper, es estype.Elasticsearch) licenseTestContext {
	return licenseTestContext{
		k:  k,
		es: es,
	}
}

func (ltctx *licenseTestContext) WrapSteps(steps ...helpers.TestStep) helpers.TestStepList {
	testSteps := helpers.TestStepList{
		ltctx.createESClient(),
	}
	testSteps = append(testSteps, steps...)
	return testSteps
}

func (ltctx *licenseTestContext) createESClient() helpers.TestStep {
	return helpers.TestStep{
		Name: "Creating Elasticsearch client",
		Test: func(t *testing.T) {
			esclient, err := helpers.NewElasticsearchClient(ltctx.es, ltctx.k)
			require.NoError(t, err)
			ltctx.esClient = esclient
		},
	}
}

func (ltctx *licenseTestContext) CheckElasticsearchLicense(expectedTypes ...license.ElasticsearchLicenseType) helpers.TestStep {
	return helpers.TestStep{
		Name: fmt.Sprintf("Elasticsearch license should be %v", expectedTypes),
		Test: helpers.Eventually(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()

			l, err := ltctx.esClient.GetLicense(ctx)
			if err != nil {
				return err
			}
			var expectedStrings []string
			for i := range expectedTypes {
				expectedStrings = append(expectedStrings, string(expectedTypes[i]))
			}
			if !stringsutil.StringInSlice(l.Type, expectedStrings) {
				return fmt.Errorf("expectedTypes license type %v got %s", expectedStrings, l.Type)
			}
			return nil
		}),
	}
}

func (ltctx *licenseTestContext) CreateEnterpriseLicenseSecret(licenseBytes []byte) helpers.TestStep {
	return helpers.TestStep{
		Name: "Creating enterprise license secret",
		Test: func(t *testing.T) {
			sec := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: params.Namespace,
					Name:      licenseSecretName,
					Labels: map[string]string{
						license.LicenseLabelType: string(license.LicenseTypeEnterprise),
					},
				},
				Data: map[string][]byte{
					license.FileName: licenseBytes,
				},
			}
			// delete if left over from previous run
			_ = ltctx.k.Client.Delete(&sec)
			require.NoError(t, ltctx.k.Client.Create(&sec))
		},
	}
}
