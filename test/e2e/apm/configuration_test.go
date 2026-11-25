// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build apm || e2e

package apm

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"

	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	apmcontroller "github.com/elastic/cloud-on-k8s/v3/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

const (
	APMKeystoreBin    = "/usr/share/apm-server/apm-server"
	APMKeystoreOption = "keystore"
)

var APMKeystoreCmd = []string{APMKeystoreBin, APMKeystoreOption}

const (
	// secureSettingKey is the keystore entry referenced in the APM Server configuration
	secureSettingKey = "E2E_RESPONSE_HEADER"
	// secureSettingResponseHeader is the HTTP response header APM Server sets to the value of secureSettingKey
	secureSettingResponseHeader = "X-E2E-Secure-Setting"
)

func TestUpdateConfiguration(t *testing.T) {
	if test.Ctx().ContainerSuffix == container.WolfiSuffix &&
		!apmcontroller.SupportsKeystoreAddFromDir(version.MustParse(test.Ctx().ElasticStackVersion)) {
		// the APM Server Wolfi image has no shell to run the keystore script, which is only needed by versions
		// that do not support populating the keystore with `keystore add --from-dir`
		t.SkipNow()
	}
	// user-provided secure settings secret
	secureSettingsSecretName := "secure-settings-secret"
	secureSettingValue := rand.String(16)
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			secureSettingKey: []byte(secureSettingValue),
		},
	}

	name := "test-apm-configuration"
	namespace := test.Ctx().ManagedNamespace(0)
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)
	apmBuilder := apmserver.NewBuilder(name).
		WithNamespace(namespace).
		WithElasticsearchRef(esBuilder.Ref()).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithRestrictedSecurityContext().
		WithoutIntegrationCheck()

	apmName := apmBuilder.ApmServer.Name
	apmNamespacedName := types.NamespacedName{
		Name:      apmName,
		Namespace: namespace,
	}
	apmPodListOpts := test.ApmServerPodListOptions(namespace, apmName)

	initStepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Create secure settings secret",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdateSecrets(secureSettings)
				}),
			},
			// Keystore should be empty
			test.CheckKeystoreEntries(k, APMKeystoreCmd, nil, apmPodListOpts...),
		}
	}

	var previousPodUID *types.UID
	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			// the response header is not configured by default
			checkResponseHeader(t, k, apmNamespacedName, ""),
			test.Step{
				Name: "Add a Keystore to the APM server",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(apmPodListOpts...)
					if err != nil {
						return err
					}
					if len(pods) != 1 {
						return fmt.Errorf("1 APM pod expected, got %d", len(pods))
					}
					previousPodUID = &pods[0].UID

					var apm apmv1.ApmServer
					if err := k.Client.Get(context.Background(), apmNamespacedName, &apm); err != nil {
						return err
					}
					apm.Spec.SecureSettings = []commonv1.SecretSource{
						{SecretName: secureSettingsSecretName},
					}
					return k.Client.Update(context.Background(), &apm)
				}),
			},
			test.Step{
				Name: "APM Pod should be recreated",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(apmPodListOpts...)
					if err != nil {
						return err
					}
					if len(pods) != 1 {
						return fmt.Errorf("1 APM pod expected, got %d", len(pods))
					}
					if pods[0].UID == *previousPodUID {
						return fmt.Errorf("APM pod is still the same, uid: %s", pods[0].UID)
					}
					return nil
				}),
			},

			test.CheckKeystoreEntries(k, APMKeystoreCmd, []string{secureSettingKey}, apmPodListOpts...),

			test.Step{
				Name: "Customize configuration of the APM server",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(apmPodListOpts...)
					if err != nil {
						return err
					}
					if len(pods) != 1 {
						return fmt.Errorf("expected 1 APM Pod, got %d", len(pods))
					}
					previousPodUID = &pods[0].UID

					var apm apmv1.ApmServer
					if err := k.Client.Get(t.Context(), apmNamespacedName, &apm); err != nil {
						return err
					}
					if apm.Spec.Config == nil {
						apm.Spec.Config = &commonv1.Config{Data: map[string]any{}}
					}
					// resolved by APM Server from the keystore, it fails to start if the keystore entry is missing
					apm.Spec.Config.Data["apm-server.response_headers"] = map[string]any{
						secureSettingResponseHeader: []any{fmt.Sprintf("${%s}", secureSettingKey)},
					}
					return k.Client.Update(t.Context(), &apm)
				}),
			},
			test.Step{
				Name: "APM Pod should be recreated",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(apmPodListOpts...)
					if err != nil {
						return err
					}
					if len(pods) != 1 {
						return fmt.Errorf("1 APM pod expected, got %d", len(pods))
					}
					if pods[0].UID == *previousPodUID {
						return fmt.Errorf("APM pod is still the same, uid: %s", pods[0].UID)
					}
					return nil
				}),
			},

			// the response header is set to the secure setting value
			checkResponseHeader(t, k, apmNamespacedName, secureSettingValue),

			// cleanup extra resources
			test.Step{
				Name: "Delete secure settings secret",
				Test: test.Eventually(func() error {
					err := k.Client.Delete(context.Background(), &secureSettings)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					return nil
				}),
			},
		}
	}

	test.Sequence(initStepsFn, stepsFn, esBuilder, apmBuilder).RunSequential(t)
}

// checkResponseHeader checks that the APM Server HTTP responses carry the secure setting response header with the
// expected value, an empty value meaning the header is not set.
func checkResponseHeader(t *testing.T, k *test.K8sClient, apmNamespacedName types.NamespacedName, expected string) test.Step {
	return test.Step{
		Name: fmt.Sprintf("APM Server responses should carry the %s header with value %q", secureSettingResponseHeader, expected),
		Test: test.Eventually(func() error {
			var apm apmv1.ApmServer
			if err := k.Client.Get(t.Context(), apmNamespacedName, &apm); err != nil {
				return err
			}
			apmClient, err := apmserver.NewApmServerClient(apm, k)
			if err != nil {
				return err
			}
			headers, err := apmClient.ResponseHeaders(t.Context())
			if err != nil {
				return err
			}
			if got := headers.Get(secureSettingResponseHeader); got != expected {
				return fmt.Errorf("expected header %s to be %q but got %q", secureSettingResponseHeader, expected, got)
			}
			return nil
		}),
	}
}
