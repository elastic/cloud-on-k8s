// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build logstash || e2e

package logstash

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	logstashcontroller "github.com/elastic/cloud-on-k8s/v3/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	clientauth "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/client-auth"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/logstash"
)

// TestClientAuthRequiredTransition tests that when Elasticsearch transitions from client authentication
// required to disabled, Logstash remains healthy and its client certificate secrets are cleaned up.
func TestClientAuthRequiredTransition(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-ls-mtls-trans"
	namespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	lsBuilder := logstash.NewBuilder(name).
		WithNodeCount(1).
		WithPipelines([]commonv1.Config{
			{
				Data: map[string]interface{}{
					"pipeline.id": "main",
					"config.string": `input { exec { command => 'uptime' interval => 10 } }
output {
  elasticsearch {
    hosts => [ "${PRODUCTION_ES_HOSTS}" ]
    ssl_enabled => true
    ssl_certificate_authorities => "${PRODUCTION_ES_SSL_CERTIFICATE_AUTHORITY}"
    user => "${PRODUCTION_ES_USER}"
    password => "${PRODUCTION_ES_PASSWORD}"
    ssl_key => "${PRODUCTION_ES_SSL_KEY}"
    ssl_certificate => "${PRODUCTION_ES_SSL_CERTIFICATE}"
  }
}`,
				},
			},
		}).
		WithElasticsearchRefs(
			logstashv1alpha1.ElasticsearchCluster{
				ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esBuilder.Ref()},
				ClusterName:           "production",
			},
		)

	// Wrap the ES builder with license setup and PostCheckSteps to verify client cert secret exists.
	esWithLicense := test.LicenseTestBuilder(esBuilder)
	esWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		return test.StepList{
			clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 1),
			{
				// Delete the Logstash client cert secret if its PKCS#8 key's last DER byte falls in
				// the ASCII whitespace range. Logstash crashes on startup with InvalidKeySpecException
				// because manticore calls String#strip on the raw binary DER payload, truncating the
				// key by one byte. Fixed in manticore v0.9.2 (cheald/manticore#113, #119); Logstash
				// 8.18.x bundles v0.9.1. Deleting forces the operator to regenerate with a safe key.
				Name: "Delete Logstash client certificate secret if PKCS#8 key last DER byte is ASCII whitespace",
				Test: test.Eventually(func() error {
					var secretList corev1.SecretList
					if err := k.Client.List(t.Context(), &secretList,
						k8sclient.InNamespace(namespace),
						k8sclient.MatchingLabels{
							labels.ClientCertificateLabelName:               "true",
							logstashcontroller.LogstashAssociationLabelName: lsBuilder.Logstash.Name,
						},
					); err != nil {
						return err
					}
					if len(secretList.Items) != 1 {
						return fmt.Errorf("expected 1 Logstash client cert secret, got %d", len(secretList.Items))
					}
					secret := secretList.Items[0]
					whitespaceByteAtEnd, err := helper.PKCS8KeyEndsWithWhitespaceByte(secret.Data[certificates.KeyFileName])
					if err != nil {
						return err
					}
					if whitespaceByteAtEnd {
						_ = k.Client.Delete(t.Context(), &secret)
						return fmt.Errorf("client cert secret %s has trailing whitespace byte; deleted to force regeneration", secret.Name)
					}
					return nil
				}),
			},
			lsBuilder.CheckMetricsRequest(k,
				logstash.Request{Name: "stats events", Path: "/_node/stats/events"},
				logstash.Want{
					MatchFunc: map[string]func(string) bool{
						"events.out": func(cntStr string) bool {
							cnt, err := strconv.Atoi(cntStr)
							if err != nil {
								return false
							}
							return cnt > 0
						},
					},
				},
			),
		}
	}

	// Transition ES to client auth disabled — also update the Logstash pipeline to remove
	// ssl_key/ssl_certificate since those env vars are no longer injected without client auth.
	esMutated := esBuilder.DeepCopy().WithMutatedFrom(&esBuilder)
	esMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false

	lsMutated := lsBuilder.DeepCopy().WithMutatedFrom(&lsBuilder).
		WithPipelines([]commonv1.Config{
			{
				Data: map[string]interface{}{
					"pipeline.id": "main",
					"config.string": `input { exec { command => 'uptime' interval => 10 } }
output {
  elasticsearch {
    hosts => [ "${PRODUCTION_ES_HOSTS}" ]
    ssl_enabled => true
    ssl_certificate_authorities => "${PRODUCTION_ES_SSL_CERTIFICATE_AUTHORITY}"
    user => "${PRODUCTION_ES_USER}"
    password => "${PRODUCTION_ES_PASSWORD}"
  }
}`,
				},
			},
		})

	esMutatedWrapped := test.WrappedBuilder{
		BuildingThis: esMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			// First wait for all Logstash pods to be ready after ES transition before checking cleanup.
			return test.CheckTestSteps(lsMutated, k).
				WithSteps(test.StepList{
					clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 0),
					{
						Name: "Verify Logstash has no client cert in association conf",
						Test: test.Eventually(func() error {
							var ls logstashv1alpha1.Logstash
							if err := k.Client.Get(context.Background(), types.NamespacedName{
								Namespace: namespace,
								Name:      lsBuilder.Logstash.Name,
							}, &ls); err != nil {
								return err
							}
							for _, assoc := range ls.GetAssociations() {
								assocConf, err := assoc.AssociationConf()
								if err != nil {
									return err
								}
								if assocConf.ClientCertIsConfigured() {
									return fmt.Errorf("Logstash association conf should not have a client cert secret after ES transition, got %s", assocConf.GetClientCertSecretName())
								}
							}
							return nil
						}),
					},
				})
		},
	}

	test.RunMutations(t, []test.Builder{esWithLicense, lsBuilder}, []test.Builder{lsMutated, esMutatedWrapped})
}

// TestClientAuthRequiredCustomCertificate tests that Logstash works with a user-provided client certificate
// when Elasticsearch requires client authentication.
func TestClientAuthRequiredCustomCertificate(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-ls-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

	var certPEM, keyPEM []byte
	test.Eventually(func() error {
		certPEM, keyPEM = helper.GenerateSelfSignedClientCertPKCS8(t, name)
		whitespaceByteAtEnd, err := helper.PKCS8KeyEndsWithWhitespaceByte(keyPEM)
		if err != nil {
			return err
		}
		if whitespaceByteAtEnd {
			// Manticore v0.9.1 calls String#strip on the raw binary DER payload, truncating the
			// key if its last byte is ASCII whitespace and causing InvalidKeySpecException.
			// Fixed in manticore v0.9.2 (cheald/manticore#113, #119). Regenerate until safe.
			return fmt.Errorf("regenerating client cert: PKCS#8 key ends with ASCII whitespace byte")
		}
		return nil
	})(t)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	lsBuilder := logstash.NewBuilder(name).
		WithNodeCount(1).
		WithPipelines([]commonv1.Config{
			{
				Data: map[string]interface{}{
					"pipeline.id": "main",
					"config.string": `input { exec { command => 'uptime' interval => 10 } }
output {
  elasticsearch {
	hosts => [ "${PRODUCTION_ES_HOSTS}" ]
	ssl_enabled => true
	ssl_certificate_authorities => "${PRODUCTION_ES_SSL_CERTIFICATE_AUTHORITY}"
	user => "${PRODUCTION_ES_USER}"
	password => "${PRODUCTION_ES_PASSWORD}"
	ssl_key => "${PRODUCTION_ES_SSL_KEY}"
	ssl_certificate => "${PRODUCTION_ES_SSL_CERTIFICATE}"
  }
}`,
				},
			},
		}).
		WithElasticsearchRefs(
			logstashv1alpha1.ElasticsearchCluster{
				ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{
					Name:      esBuilder.Elasticsearch.Name,
					Namespace: esBuilder.Elasticsearch.Namespace,
				}},
				ClusterName: "production",
			},
		).
		WithClientCertificateSecret(0, userCertSecretName)

	// Wrap the Logstash builder to add post-check verification steps.
	lsWrapped := test.WrappedBuilder{
		BuildingThis: lsBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Verify Logstash association conf has client cert configured",
					Test: test.Eventually(func() error {
						var ls logstashv1alpha1.Logstash
						if err := k.Client.Get(context.Background(), types.NamespacedName{
							Namespace: namespace,
							Name:      lsBuilder.Logstash.Name,
						}, &ls); err != nil {
							return err
						}
						for _, assoc := range ls.GetAssociations() {
							assocConf, err := assoc.AssociationConf()
							if err != nil {
								return err
							}
							if !assocConf.ClientCertIsConfigured() {
								return fmt.Errorf("Logstash association conf should have a client cert secret configured")
							}
						}
						return nil
					}),
				},
				clientauth.CheckClientCertificateDataStep(k, namespace, esBuilder.Elasticsearch.Name, "", "", certPEM, keyPEM),
			}
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), lsWrapped).RunSequential(t)
}
