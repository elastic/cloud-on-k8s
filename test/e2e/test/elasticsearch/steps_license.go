// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"fmt"
	"testing"

	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	licenseSecretName = "e2e-enterprise-license"
)

type LicenseTestContext struct {
	esClient client.Client
	k        *test.K8sClient
	es       estype.Elasticsearch
}

func NewLicenseTestContext(k *test.K8sClient, es estype.Elasticsearch) LicenseTestContext {
	return LicenseTestContext{
		k:  k,
		es: es,
	}
}

func (ltctx *LicenseTestContext) Init() test.Step {
	return test.Step{
		Name: "Creating Elasticsearch client",
		Test: func(t *testing.T) {
			esClient, err := NewElasticsearchClient(ltctx.es, ltctx.k)
			require.NoError(t, err)
			ltctx.esClient = esClient
		},
	}
}

func (ltctx *LicenseTestContext) CheckElasticsearchLicense(expectedTypes ...license.ElasticsearchLicenseType) test.Step {
	return test.Step{
		Name: fmt.Sprintf("Elasticsearch license should be %v", expectedTypes),
		Test: test.Eventually(func() error {
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

func (ltctx *LicenseTestContext) CreateEnterpriseLicenseSecret(licenseBytes []byte) test.Step {
	return test.Step{
		Name: "Creating enterprise license secret",
		Test: func(t *testing.T) {
			sec := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: test.Ctx().ManagedNamespace(0),
					Name:      licenseSecretName,
					Labels: map[string]string{
						license.LicenseLabelType: string(license.LicenseTypeEnterprise),
					},
				},
				Data: map[string][]byte{
					license.FileName: licenseBytes,
				},
			}
			require.NoError(t, ltctx.k.Client.Create(&sec))
		},
	}
}

func (ltctx *LicenseTestContext) DeleteEnterpriseLicenseSecret() test.Step {
	return test.Step{
		Name: "Removing any test enterprise licenses",
		Test: func(t *testing.T) {
			sec := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: test.Ctx().ManagedNamespace(0),
					Name:      licenseSecretName,
				},
			}
			_ = ltctx.k.Client.Delete(&sec)
		},
	}
}
