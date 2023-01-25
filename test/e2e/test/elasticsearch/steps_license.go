// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/stringsutil"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
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
	//nolint:thelper
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
	l, err := ltctx.esClient.GetLicense(context.Background())
	if err != nil {
		return err
	}
	expectedStrings := make([]string, len(expectedTypes))
	for i, et := range expectedTypes {
		expectedStrings[i] = string(et)
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
	//nolint:thelper
	return test.Step{
		Name: "Creating an Enterprise license secret",
		Test: func(t *testing.T) {
			test.CreateEnterpriseLicenseSecret(t, ltctx.k, secretName, licenseBytes)
		},
	}
}

func (ltctx *LicenseTestContext) CreateTrialExtension(secretName string, privateKey *rsa.PrivateKey, esLicenseType client.ElasticsearchLicenseType) test.Step {
	//nolint:thelper
	return test.Step{
		Name: "Creating a trial extension secret",
		Test: func(t *testing.T) {
			signer := license.NewSigner(privateKey)
			clusterLicense, err := GenerateTestLicense(signer, esLicenseType)
			require.NoError(t, err)
			trialExtension := license.EnterpriseLicense{
				License: license.LicenseSpec{
					// reuse ES license values where possible for simplicity
					UID:                clusterLicense.UID,
					Type:               license.LicenseTypeEnterpriseTrial,
					IssueDateInMillis:  clusterLicense.IssueDateInMillis,
					ExpiryDateInMillis: clusterLicense.ExpiryDateInMillis,
					MaxResourceUnits:   100,
					IssuedTo:           clusterLicense.IssuedTo,
					Issuer:             clusterLicense.Issuer,
					StartDateInMillis:  clusterLicense.StartDateInMillis,
					ClusterLicenses: []license.ElasticsearchLicense{
						{
							License: clusterLicense,
						},
					},
				},
			}
			signature, err := signer.Sign(trialExtension)
			require.NoError(t, err)
			trialExtension.License.Signature = string(signature)
			trialExtensionBytes, err := json.Marshal(trialExtension)
			require.NoError(t, err)
			ltctx.CreateEnterpriseLicenseSecret(secretName, trialExtensionBytes).Test(t)
		},
	}
}

func (ltctx *LicenseTestContext) CreateEnterpriseTrialLicenseSecret(secretName string) test.Step {
	return test.Step{
		Name: "Creating enterprise trial license secret",
		Test: test.Eventually(func() error {
			sec := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: test.Ctx().ManagedNamespace(0),
					Name:      secretName,
					Labels: map[string]string{
						license.LicenseLabelType: string(license.LicenseTypeEnterpriseTrial),
						commonv1.TypeLabelName:   license.Type,
					},
					Annotations: map[string]string{
						license.EULAAnnotation: license.EULAAcceptedValue,
					},
				},
			}
			return ltctx.k.CreateOrUpdate(&sec)
		}),
	}
}

func (ltctx *LicenseTestContext) checkEnterpriseTrialLicenseValidation(secretName string, valid bool) test.Step {
	return test.Step{
		Name: "Check enterprise trial license is annotated as invalid",
		Test: test.Eventually(func() error {
			var licenseSecret corev1.Secret
			err := ltctx.k.Client.Get(context.Background(), types.NamespacedName{
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
		Test: test.Eventually(func() error {
			// Delete operator license secret
			sec := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: test.Ctx().ManagedNamespace(0),
					Name:      licenseSecretName,
				},
			}
			err := ltctx.k.Client.Delete(context.Background(), &sec)
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}),
	}
}

func (ltctx *LicenseTestContext) DeleteAllEnterpriseLicenseSecrets() test.Step {
	//nolint:thelper
	return test.Step{
		Name: "Removing all Enterprise license secrets",
		Test: func(t *testing.T) {
			test.DeleteAllEnterpriseLicenseSecrets(t, ltctx.k)
		},
	}
}
