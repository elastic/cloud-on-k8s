// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package clientauth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

// SkipIfPKINotSupported skips the test when no enterprise test license is configured or when the stack version
// is too old for the PKI authentication mode.
func SkipIfPKINotSupported(t *testing.T) {
	t.Helper()
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping PKI authentication test: no enterprise test license configured")
	}
	if version.MustParse(test.Ctx().ElasticStackVersion).LT(commonv1.PKIAuthMinVersion) {
		t.Skipf("Skipping PKI authentication test: requires stack version >= %s, got %s",
			version.WithoutPre(commonv1.PKIAuthMinVersion), test.Ctx().ElasticStackVersion)
	}
}

// NewPKIElasticsearchBuilder returns an Elasticsearch builder with client authentication required and the
// ECK-managed PKI realm enabled.
func NewPKIElasticsearchBuilder(name string) elasticsearch.Builder {
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()
	esBuilder.Elasticsearch.Spec.Auth.PKI.Enabled = true
	return esBuilder
}

// RunPKIAuthenticationModeTransition runs the PKI authentication mode lifecycle of a component:
//  1. Elasticsearch with mTLS and the PKI realm, plus the component with authenticationMode: PKI. The component
//     builder's own checks must pass, which proves it can talk to Elasticsearch with its client certificate only,
//     and the file-settings Secret must contain eck_managed_pki_ role mapping entries.
//  2. The component is mutated to authenticationMode: Credentials. The mutated builder's own checks must pass, and
//     the eck_managed_pki_ entries must be cleaned up from the file-settings Secret.
//
// pkiComponent and credentialsComponent are the same component in both modes; credentialsComponent must be derived
// from pkiComponent with DeepCopy().WithMutatedFrom. Any additional builders are created together with the others.
func RunPKIAuthenticationModeTransition(
	t *testing.T,
	esBuilder elasticsearch.Builder,
	pkiComponent, credentialsComponent test.Builder,
	additional ...test.Builder,
) {
	t.Helper()
	namespace := esBuilder.Elasticsearch.Namespace
	esName := esBuilder.Elasticsearch.Name

	pkiWrapped := test.WrappedBuilder{
		BuildingThis: pkiComponent,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				CheckFileSettingsPKIEntriesStep(t.Context(), k, namespace, esName, true),
			}
		},
	}
	credentialsWrapped := test.WrappedBuilder{
		BuildingThis: credentialsComponent,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				CheckFileSettingsPKIEntriesStep(t.Context(), k, namespace, esName, false),
			}
		},
	}

	initial := append([]test.Builder{test.LicenseTestBuilder(esBuilder), pkiWrapped}, additional...)
	test.RunMutations(t, initial, []test.Builder{credentialsWrapped})
}

// CheckFileSettingsPKIEntriesStep returns a Step that verifies the presence (wantPresent=true) or absence
// (wantPresent=false) of eck_managed_pki_ prefixed role mapping entries in the file-settings Secret of the given
// Elasticsearch cluster.
func CheckFileSettingsPKIEntriesStep(ctx context.Context, k *test.K8sClient, namespace, esName string, wantPresent bool) test.Step {
	name := "File settings Secret must contain eck_managed_pki_ role mapping entries while authenticationMode is PKI"
	if !wantPresent {
		name = "File settings Secret must not contain eck_managed_pki_ role mappings after switching to Credentials"
	}
	return test.Step{
		Name: name,
		Test: test.Eventually(func() error {
			return checkFileSettingsPKIEntries(ctx, k, namespace, esName, wantPresent)
		}),
	}
}

func checkFileSettingsPKIEntries(ctx context.Context, k *test.K8sClient, namespace, esName string, wantPresent bool) error {
	var secret corev1.Secret
	if err := k.Client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      esv1.FileSettingsSecretName(esName),
	}, &secret); err != nil {
		return err
	}
	raw, ok := secret.Data[filesettings.SettingsSecretKey]
	if !ok {
		return fmt.Errorf("file settings Secret %s/%s missing key %q", namespace, esName, filesettings.SettingsSecretKey)
	}
	var settings filesettings.Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return fmt.Errorf("failed to unmarshal file settings: %w", err)
	}

	var foundKeys []string
	if settings.State.RoleMappings != nil {
		for key := range settings.State.RoleMappings.Data {
			if strings.HasPrefix(key, commonv1.ECKPKIRoleMappingPrefix) {
				foundKeys = append(foundKeys, key)
			}
		}
	}

	if wantPresent && len(foundKeys) == 0 {
		return fmt.Errorf("expected at least one %s* role mapping in file settings, found none", commonv1.ECKPKIRoleMappingPrefix)
	}
	if !wantPresent && len(foundKeys) > 0 {
		return fmt.Errorf("expected no %s* role mappings in file settings, found: %v", commonv1.ECKPKIRoleMappingPrefix, foundKeys)
	}
	return nil
}
