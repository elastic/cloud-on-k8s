// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

type LicenseTestContext struct {
	esClient client.Client
	k        *test.K8sClient
	es       esv1.Elasticsearch
}

func NewLicenseTestContext(k *test.K8sClient, es esv1.Elasticsearch) LicenseTestContext {
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

func (ltctx *LicenseTestContext) CheckElasticsearchLicenseFn(expectedTypes ...client.ElasticsearchLicenseType) error {
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()

	l, err := ltctx.esClient.GetLicense(ctx)
	if err != nil {
		return err
	}
	expectedStrings := make([]string, len(expectedTypes))
	for i := range expectedTypes {
		expectedStrings = append(expectedStrings, string(expectedTypes[i]))
	}
	if !stringsutil.StringInSlice(l.Type, expectedStrings) {
		return fmt.Errorf("expectedTypes license type %v got %s", expectedStrings, l.Type)
	}
	return nil
}

func (ltctx *LicenseTestContext) CheckElasticsearchLicense(expectedTypes ...client.ElasticsearchLicenseType) test.Step {
	return test.Step{
		Name: fmt.Sprintf("Elasticsearch license should be %v", expectedTypes),
		Test: test.Eventually(func() error {
			return ltctx.CheckElasticsearchLicenseFn(expectedTypes...)
		}),
	}
}

func (ltctx *LicenseTestContext) CreateEnterpriseLicenseSecret(secretName string, licenseBytes []byte) test.Step {
	return test.Step{
		Name: "Creating enterprise license secret",
		Test: func(t *testing.T) {
			sec := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: test.Ctx().ManagedNamespace(0),
					Name:      secretName,
					Labels: map[string]string{
						common.TypeLabelName:      license.Type,
						license.LicenseLabelScope: string(license.LicenseScopeOperator),
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

func (ltctx *LicenseTestContext) CreateEnterpriseTrialLicenseSecret(secretName string) test.Step {
	return test.Step{
		Name: "Creating enterprise trial license secret",
		Test: func(t *testing.T) {
			sec := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: test.Ctx().ManagedNamespace(0),
					Name:      secretName,
					Labels: map[string]string{
						license.LicenseLabelType: string(license.LicenseTypeEnterpriseTrial),
						common.TypeLabelName:     license.Type,
					},
					Annotations: map[string]string{
						license.EULAAnnotation: license.EULAAcceptedValue,
					},
				},
			}
			require.NoError(t, ltctx.k.Client.Create(&sec))
		},
	}
}

func (ltctx *LicenseTestContext) checkEnterpriseTrialLicenseValidation(secretName string, valid bool) test.Step {
	return test.Step{
		Name: "Check enterprise trial license is annotated as invalid",
		Test: test.Eventually(func() error {
			var licenseSecret corev1.Secret
			err := ltctx.k.Client.Get(types.NamespacedName{
				Namespace: test.Ctx().ManagedNamespace(0),
				Name:      secretName,
			}, &licenseSecret)
			if err != nil {
				return err
			}
			_, exists := licenseSecret.Annotations[license.LicenseInvalidAnnotation]
			if exists == valid {
				return fmt.Errorf("trial license should have validation annotation [%v] and annotation was present [%v]", !valid, exists)
			}
			return nil
		}),
	}
}

func (ltctx *LicenseTestContext) CheckEnterpriseTrialLicenseValid(secretName string) test.Step {
	return ltctx.checkEnterpriseTrialLicenseValidation(secretName, true)
}

func (ltctx *LicenseTestContext) CheckEnterpriseTrialLicenseInvalid(secretName string) test.Step {
	return ltctx.checkEnterpriseTrialLicenseValidation(secretName, false)
}

func (ltctx *LicenseTestContext) DeleteEnterpriseLicenseSecret(licenseSecretName string) test.Step {
	return test.Step{
		Name: "Removing any test enterprise license secrets",
		Test: func(t *testing.T) {
			// Delete operator license secret
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

func (ltctx *LicenseTestContext) DeleteAllEnterpriseLicenseSecrets() test.Step {
	return test.Step{
		Name: "Removing any test enterprise license secrets",
		Test: func(t *testing.T) {
			// Delete operator license secret
			var licenseSecrets corev1.SecretList
			err := ltctx.k.Client.List(&licenseSecrets, k8sclient.MatchingLabels(map[string]string{common.TypeLabelName: license.Type}))
			if err != nil {
				t.Log(err)
			}
			for _, s := range licenseSecrets.Items {
				_ = ltctx.k.Client.Delete(&s)
			}
		},
	}
}
