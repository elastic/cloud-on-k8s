// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	commonesclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	commonlabels "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

type fakeEsClient struct {
	esclient.Client

	fileSettings esclient.FileSettings
	err          error
}

var fakeClientProvider = func(fileSettings esclient.FileSettings, err error) commonesclient.Provider {
	return func(ctx context.Context, c k8s.Client, dialer net.Dialer, es esv1.Elasticsearch) (esclient.Client, error) {
		fakeEsClient := fakeEsClient{
			fileSettings: fileSettings,
			err:          err,
		}
		return fakeEsClient, nil
	}
}

func (c fakeEsClient) GetClusterState(_ context.Context) (esclient.ClusterState, error) {
	if c.err != nil {
		return esclient.ClusterState{}, c.err
	}
	clusterState := esclient.ClusterState{}
	clusterState.Metadata.ReservedState.FileSettings = c.fileSettings
	return clusterState, nil
}

func (r ReconcileStackConfigPolicy) getSettings(t *testing.T, nsn types.NamespacedName) filesettings.Settings {
	t.Helper()
	var secret corev1.Secret
	err := r.Client.Get(context.Background(), nsn, &secret)
	assert.NoError(t, err)
	var settings filesettings.Settings
	err = json.Unmarshal(secret.Data[filesettings.SettingsSecretKey], &settings)
	assert.NoError(t, err)
	return settings
}

func (r ReconcileStackConfigPolicy) getPolicy(t *testing.T, nsn types.NamespacedName) policyv1alpha1.StackConfigPolicy {
	t.Helper()
	var policy policyv1alpha1.StackConfigPolicy
	err := r.Client.Get(context.Background(), nsn, &policy)
	assert.NoError(t, err)
	return policy
}

func fetchEvents(recorder *record.FakeRecorder) []string {
	close(recorder.Events)
	events := make([]string, 0)
	for event := range recorder.Events {
		events = append(events, event)
	}
	return events
}

func getEsPod(namespace string, annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-es-default-0",
			Namespace: namespace,
			Labels: map[string]string{
				label.ClusterNameLabelName: "test-es",
			},
			Annotations: annotations,
		},
	}
}

func TestReconcileStackConfigPolicy_Reconcile(t *testing.T) {
	nsnFixture := types.NamespacedName{
		Namespace: "ns",
		Name:      "test-policy",
	}
	policyFixture := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "test-policy",
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{MatchLabels: map[string]string{"label": "test"}},
			SecureSettings: []commonv1.SecretSource{
				{
					SecretName: "shared-secret1",
				},
			},
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{
					"indices.recovery.max_bytes_per_sec": "42mb",
				}},
				SecretMounts: []policyv1alpha1.SecretMount{
					{
						SecretName: "test-secret-mount",
						MountPath:  "/usr/test",
					},
				},
				SecureSettings: []commonv1.SecretSource{
					{
						SecretName: "shared-secret",
					},
				},
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"logger.org.elasticsearch.discovery": "DEBUG",
					},
				},
			},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"xpack.canvas.enabled": true,
					},
				},
				SecureSettings: []commonv1.SecretSource{
					{
						SecretName: "shared-secret",
					},
				},
			},
		},
	}
	elasticsearchConfigAndMountsHash := getElasticsearchConfigAndMountsHash(policyFixture.Spec.Elasticsearch.Config, policyFixture.Spec.Elasticsearch.SecretMounts)
	esPodFixture := getEsPod("ns", map[string]string{
		commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation: elasticsearchConfigAndMountsHash,
	})
	esFixture := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns",
		Name:      "test-es",
		Labels:    map[string]string{"label": "test"},
	},
		Spec: esv1.ElasticsearchSpec{Version: "8.6.1"},
	}
	secretMountsSecretFixture := getSecretMountSecret(t, "test-secret-mount", "ns", "test-policy", "ns", "delete")
	secretFixture := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "test-es-es-file-settings",
			Labels: map[string]string{
				"common.k8s.elastic.co/type":                    "elasticsearch",
				"elasticsearch.k8s.elastic.co/cluster-name":     "test-es",
				"eck.k8s.elastic.co/owner-kind":                 "StackConfigPolicy",
				"eck.k8s.elastic.co/owner-namespace":            "ns",
				"eck.k8s.elastic.co/owner-name":                 "test-policy",
				commonlabels.StackConfigPolicyOnDeleteLabelName: commonlabels.OrphanSecretResetOnPolicyDelete,
			},
		},
		Data: map[string][]byte{"settings.json": []byte(`{"metadata":{"version":"42","compatibility":"8.4.0"},"state":{"cluster_settings":{"indices.recovery.max_bytes_per_sec":"42mb"},"snapshot_repositories":{},"slm":{},"role_mappings":{},"autoscaling":{},"ilm":{},"ingest_pipelines":{},"index_templates":{"component_templates":{},"composable_index_templates":{}}}}`)},
	}
	secretHash, err := getSettingsHash(secretFixture)
	assert.NoError(t, err)
	secretFixture.Annotations = map[string]string{"policy.k8s.elastic.co/settings-hash": secretHash,
		"policy.k8s.elastic.co/secure-settings-secrets": `[{"namespace":"ns","secretName":"shared-secret"},{"namespace":"ns","secretName":"shared-secret1"}]`}

	conflictingSecretFixture := secretFixture.DeepCopy()
	conflictingSecretFixture.Labels["eck.k8s.elastic.co/owner-name"] = "another-policy"

	orphanSecretFixture := secretFixture.DeepCopy()
	orphanSecretFixture.Name = "another-es-es-file-settings"
	orphanSecretFixture.Labels["elasticsearch.k8s.elastic.co/cluster-name"] = "another-es"

	orphanElasticsearchConfigSecretFixture := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.StackConfigElasticsearchConfigSecretName("another-es"),
			Namespace: "ns",
			Labels: map[string]string{
				"elasticsearch.k8s.elastic.co/cluster-name":     "another-es",
				"common.k8s.elastic.co/type":                    "elasticsearch",
				commonlabels.StackConfigPolicyOnDeleteLabelName: commonlabels.OrphanSecretDeleteOnPolicyDelete,
				reconciler.SoftOwnerNamespaceLabel:              policyFixture.Namespace,
				reconciler.SoftOwnerNameLabel:                   policyFixture.Name,
				reconciler.SoftOwnerKindLabel:                   policyv1alpha1.Kind,
			},
		},
	}

	orphanSecretMountsSecretFixture := getSecretMountSecret(t, esv1.ESNamer.Suffix("another-es", "test-secret-mount"), "ns", "test-policy", "ns", "delete")

	updatedPolicyFixture := policyFixture.DeepCopy()
	updatedPolicyFixture.Spec.Elasticsearch.ClusterSettings = &commonv1.Config{Data: map[string]interface{}{
		"indices.recovery.max_bytes_per_sec": "43mb",
	}}

	orphanEsFixture := esFixture.DeepCopy()
	orphanEsFixture.Name = "another-es"
	orphanEsFixture.Labels["label"] = "another"

	oldVersionEsFixture := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns",
		Name:      "test-es",
		Labels:    map[string]string{"label": "test"},
	},
		Spec: esv1.ElasticsearchSpec{Version: "8.0.0"},
	}

	clusterStateFileSettingsFixture := func(v int64, err error) esclient.FileSettings {
		if err != nil {
			return esclient.FileSettings{
				Version: v,
				Errors: &esclient.FileSettingsErrors{
					Version: -1,
					Errors:  []string{err.Error()},
				},
			}
		}
		return esclient.FileSettings{
			Version: v,
		}
	}

	kibanaFixture := kibanav1.Kibana{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns",
		Name:      "test-kb",
		Labels:    map[string]string{"label": "test"},
	}}

	kibanaConfigSecretFixture := MkKibanaConfigSecret("ns", policyFixture.Name, policyFixture.Namespace, "3077592849")
	addSecureSettingsAnnotationToSecret(kibanaConfigSecretFixture, "ns")

	type args struct {
		client           k8s.Client
		esClientProvider commonesclient.Provider
		licenseChecker   license.Checker
	}

	tests := []struct {
		name             string
		args             args
		pre              func(r ReconcileStackConfigPolicy)
		post             func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder)
		wantRequeue      bool
		wantRequeueAfter bool
		wantErr          bool
	}{
		{
			name: "Resource StackConfigPolicy not found: nothing to do",
			args: args{
				client: k8s.NewFakeClient(),
			},
			wantErr:     false,
			wantRequeue: false,
		},
		{
			name: "Settings secret doesn't exist yet: requeue",
			args: args{
				client:         k8s.NewFakeClient(&policyFixture, &esFixture),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Reset settings secret on StackConfigPolicy deletion",
			args: args{
				client: k8s.NewFakeClient(&esFixture, &secretFixture, secretMountsSecretFixture),
			},
			pre: func(r ReconcileStackConfigPolicy) {
				// before the reconciliation, settings are not empty
				settings := r.getSettings(t, k8s.ExtractNamespacedName(&secretFixture))
				assert.NotEmpty(t, settings.State.ClusterSettings.Data)
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				// after the reconciliation, settings are empty
				settings := r.getSettings(t, k8s.ExtractNamespacedName(&secretFixture))
				assert.Empty(t, settings.State.ClusterSettings.Data)
			},
			wantErr: false,
		},
		{
			name: "Reset orphan soft owned secrets when an Elasticsearch is no more configured by a StackConfigPolicy",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture, orphanSecretFixture, orphanEsFixture, secretMountsSecretFixture, esPodFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(42, nil), nil),
			},
			pre: func(r ReconcileStackConfigPolicy) {
				// before the reconciliation, settings are not empty
				settings := r.getSettings(t, k8s.ExtractNamespacedName(orphanSecretFixture))
				assert.NotEmpty(t, settings.State.ClusterSettings)
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				// after the reconciliation, settings are empty
				settings := r.getSettings(t, k8s.ExtractNamespacedName(orphanSecretFixture))
				assert.Empty(t, settings.State.ClusterSettings.Data)
			},
			wantErr: false,
		},
		{
			name: "Reset orphan soft owned secrets when the stackconfigpolicy no longer exists",
			args: args{
				client:           k8s.NewFakeClient(&esFixture, &secretFixture, orphanSecretFixture, orphanEsFixture, secretMountsSecretFixture, esPodFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(42, nil), nil),
			},
			pre: func(r ReconcileStackConfigPolicy) {
				// before the reconciliation, settings are not empty
				settings := r.getSettings(t, k8s.ExtractNamespacedName(&secretFixture))
				assert.NotEmpty(t, settings.State.ClusterSettings.Data)
				settings = r.getSettings(t, k8s.ExtractNamespacedName(orphanSecretFixture))
				assert.NotEmpty(t, settings.State.ClusterSettings)
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				// after the reconciliation, settings are empty
				settings := r.getSettings(t, k8s.ExtractNamespacedName(&secretFixture))
				assert.Empty(t, settings.State.ClusterSettings.Data)
				settings = r.getSettings(t, k8s.ExtractNamespacedName(orphanSecretFixture))
				assert.Empty(t, settings.State.ClusterSettings.Data)
			},
			wantErr: false,
		},
		{
			name: "Reconcile policy without Enterprise license",
			args: args{
				client:         k8s.NewFakeClient(&policyFixture),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: false},
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				events := fetchEvents(&recorder)

				assert.ElementsMatch(t, []string{"Warning ReconciliationError StackConfigPolicy is an enterprise feature. Enterprise features are disabled"}, events)

				policy := r.getPolicy(t, k8s.ExtractNamespacedName(&policyFixture))
				assert.Equal(t, 0, policy.Status.Resources)
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Reconcile Kibana already owned by another policy",
			args: args{
				client:         k8s.NewFakeClient(&policyFixture, &kibanaFixture, MkKibanaConfigSecret("ns", "another-policy", "ns", "testvalue")),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				events := fetchEvents(&recorder)
				assert.ElementsMatch(t, []string{"Warning Unexpected conflict: resource Kibana ns/test-kb already configured by StackConfigpolicy ns/another-policy"}, events)

				policy := r.getPolicy(t, k8s.ExtractNamespacedName(&policyFixture))
				assert.Equal(t, 1, policy.Status.Resources)
				assert.Equal(t, 0, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.ConflictPhase, policy.Status.Phase)
			},
			wantErr:          true,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Reconcile Elasticsearch already owned by another policy",
			args: args{
				client:         k8s.NewFakeClient(&policyFixture, &esFixture, conflictingSecretFixture),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				events := fetchEvents(&recorder)
				assert.ElementsMatch(t, []string{"Warning Unexpected conflict: resource Elasticsearch ns/test-es already configured by StackConfigpolicy ns/another-policy"}, events)

				policy := r.getPolicy(t, k8s.ExtractNamespacedName(&policyFixture))
				assert.Equal(t, 1, policy.Status.Resources)
				assert.Equal(t, 0, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.ConflictPhase, policy.Status.Phase)
			},
			wantErr:          true,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Elasticsearch cluster in old version without support for file based settings",
			args: args{
				client:         k8s.NewFakeClient(&policyFixture, &secretFixture, &oldVersionEsFixture, esPodFixture),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				events := fetchEvents(&recorder)
				assert.ElementsMatch(t, []string{"Warning Unexpected invalid version to configure resource Elasticsearch ns/test-es: actual 8.0.0, expected >= 8.6.1"}, events)

				policy := r.getPolicy(t, k8s.ExtractNamespacedName(&policyFixture))
				assert.Equal(t, 1, policy.Status.Resources)
				assert.Equal(t, 0, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.ErrorPhase, policy.Status.Phase)
			},
			wantErr:          true,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Elasticsearch cluster is unreachable",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture, secretMountsSecretFixture, esPodFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(esclient.FileSettings{}, errors.New("elasticsearch client failed")),
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				policy := r.getPolicy(t, k8s.ExtractNamespacedName(&policyFixture))
				assert.Equal(t, 1, policy.Status.Resources)
				assert.Equal(t, 0, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.UnknownPhase, policy.Status.Details["elasticsearch"]["ns/test-es"].Phase)
				assert.Equal(t, policyv1alpha1.ReadyPhase, policy.Status.Phase)
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Settings secret must be updated to reflect the policy settings",
			args: args{
				client:           k8s.NewFakeClient(updatedPolicyFixture, &esFixture, &secretFixture, secretMountsSecretFixture, esPodFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(43, nil), nil),
			},
			pre: func(r ReconcileStackConfigPolicy) {
				settings := r.getSettings(t, k8s.ExtractNamespacedName(&secretFixture))
				assert.Equal(t, "42mb", settings.State.ClusterSettings.Data["indices.recovery.max_bytes_per_sec"])
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				settings := r.getSettings(t, k8s.ExtractNamespacedName(&secretFixture))
				assert.Equal(t, "43mb", settings.State.ClusterSettings.Data["indices.recovery.max_bytes_per_sec"])

				var policy policyv1alpha1.StackConfigPolicy
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Namespace: "ns",
					Name:      "test-policy",
				}, &policy)
				assert.NoError(t, err)
				assert.Equal(t, 1, policy.Status.Resources)
				assert.Equal(t, 0, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.ApplyingChangesPhase, policy.Status.Phase)
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Current settings are wrong",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture, secretMountsSecretFixture, esPodFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(42, errors.New("invalid cluster settings")), nil),
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				var policy policyv1alpha1.StackConfigPolicy
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Namespace: "ns",
					Name:      "test-policy",
				}, &policy)
				assert.NoError(t, err)
				assert.Equal(t, 1, policy.Status.Resources)
				assert.Equal(t, 0, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.ApplyingChangesPhase, policy.Status.Phase)
				assert.Equal(t, "invalid cluster settings", policy.Status.Details["elasticsearch"]["ns/test-es"].Error.Message)
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Current settings version is different from the expected one",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture, secretMountsSecretFixture, esPodFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(40, nil), nil),
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				var policy policyv1alpha1.StackConfigPolicy
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Namespace: "ns",
					Name:      "test-policy",
				}, &policy)
				assert.NoError(t, err)
				assert.Equal(t, 1, policy.Status.Resources)
				assert.Equal(t, 0, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.ApplyingChangesPhase, policy.Status.Phase)
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Happy path",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture, secretMountsSecretFixture, esPodFixture, &kibanaFixture, mkKibanaPod("ns", true, "3077592849")),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(42, nil), nil),
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				var policy policyv1alpha1.StackConfigPolicy
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Namespace: "ns",
					Name:      "test-policy",
				}, &policy)
				assert.NoError(t, err)
				assert.Equal(t, 2, policy.Status.Resources)
				assert.Equal(t, 2, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.ReadyPhase, policy.Status.Phase)
				var esSecret corev1.Secret
				// Verify the config secret created by the stack config policy controller
				err = r.Client.Get(context.Background(), types.NamespacedName{
					Namespace: "ns",
					Name:      esv1.StackConfigElasticsearchConfigSecretName(esFixture.Name),
				}, &esSecret)
				assert.NoError(t, err)
				elasticsearchConfigJSONData, err := json.Marshal(policy.Spec.Elasticsearch.Config)
				assert.NoError(t, err)
				secretMountsJSONData, err := json.Marshal(policy.Spec.Elasticsearch.SecretMounts)
				assert.NoError(t, err)
				assert.Equal(t, esSecret.Data[ElasticSearchConfigKey], elasticsearchConfigJSONData)
				assert.Equal(t, esSecret.Data[SecretsMountKey], secretMountsJSONData)

				// Verify the secret mounts secret
				assertExpectedESSecretContent(t, r.Client, esFixture.Name, *secretMountsSecretFixture, policy.Spec.Elasticsearch.SecretMounts)
				assertKibanaConfigSecret(t, r.Client, kibanaFixture.Name, *kibanaConfigSecretFixture)

				// Verify dynamic watches are added
				assert.NotEmpty(t, r.dynamicWatches.Secrets.Registrations())
			},
			wantErr:          false,
			wantRequeue:      false,
			wantRequeueAfter: false,
		},
		{
			name: "Delete orphan soft owned elasticsearch config and secret mounts secrets when an Elasticsearch is no more configured by a StackConfigPolicy",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture, orphanSecretFixture, orphanEsFixture, secretMountsSecretFixture, esPodFixture, &orphanElasticsearchConfigSecretFixture, orphanSecretMountsSecretFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(42, nil), nil),
			},
			pre: func(r ReconcileStackConfigPolicy) {
				// before the reconciliation, settings are not empty
				settings := r.getSettings(t, k8s.ExtractNamespacedName(orphanSecretFixture))
				assert.NotEmpty(t, settings.State.ClusterSettings)

				// before the reconciliation, settings exist
				var configSecret, secretMountsSecret corev1.Secret
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Name:      esv1.StackConfigElasticsearchConfigSecretName("another-es"),
					Namespace: "ns",
				}, &configSecret)
				assert.NoError(t, err)
				err = r.Client.Get(context.Background(), types.NamespacedName{
					Name:      esv1.ESNamer.Suffix("another-es", "test-secret-mount"),
					Namespace: "ns",
				}, &secretMountsSecret)
				assert.NoError(t, err)
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				// after the reconciliation, settings are empty
				settings := r.getSettings(t, k8s.ExtractNamespacedName(orphanSecretFixture))
				assert.Empty(t, settings.State.ClusterSettings.Data)

				var esConfigSecret, secretMountsSecretInEsNamespace corev1.Secret
				// after the reconciliation, the config and secret mount secrets do not exist
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Name:      esv1.StackConfigElasticsearchConfigSecretName("another-es"),
					Namespace: "ns",
				}, &esConfigSecret)
				assert.True(t, apierrors.IsNotFound(err))

				err = r.Client.Get(context.Background(), types.NamespacedName{
					Name:      esv1.ESNamer.Suffix("another-es", "test-secret-mount"),
					Namespace: "ns",
				}, &secretMountsSecretInEsNamespace)
				assert.True(t, apierrors.IsNotFound(err))
			},
			wantErr: false,
		},
		{
			name: "Delete orphan soft owned kibana config secrets when StackConfigPolicy does not exist",
			args: args{
				client:           k8s.NewFakeClient(&esFixture, &kibanaFixture, kibanaConfigSecretFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(42, nil), nil),
			},
			pre: func(r ReconcileStackConfigPolicy) {
				// before the reconciliation, config exist
				var configSecret corev1.Secret
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Name:      "test-kb-kb-policy-config",
					Namespace: "ns",
				}, &configSecret)
				assert.NoError(t, err)
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				// after the reconciliation, the config secrets do not exist
				var kibanaConfigSecret corev1.Secret
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Name:      "test-kb-kb-policy-config",
					Namespace: "ns",
				}, &kibanaConfigSecret)
				assert.True(t, apierrors.IsNotFound(err))
			},
			wantErr: false,
		},
		{
			name: "Delete orphan soft owned kibana config secrets when Kibana does not exist",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, kibanaConfigSecretFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(42, nil), nil),
			},
			pre: func(r ReconcileStackConfigPolicy) {
				// before the reconciliation, config exist
				var configSecret corev1.Secret
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Name:      "test-kb-kb-policy-config",
					Namespace: "ns",
				}, &configSecret)
				assert.NoError(t, err)
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				// after the reconciliation, the config secrets do not exist
				var kibanaConfigSecret corev1.Secret
				err := r.Client.Get(context.Background(), types.NamespacedName{
					Name:      "test-kb-kb-policy-config",
					Namespace: "ns",
				}, &kibanaConfigSecret)
				assert.True(t, apierrors.IsNotFound(err))
			},
			wantErr: false,
		},
		{
			name: "Elasticsearch cluster is unreachable and Kibana has reconciled successfully",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture, secretMountsSecretFixture, esPodFixture, &kibanaFixture, mkKibanaPod("ns", true, "3077592849")),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(esclient.FileSettings{}, errors.New("elasticsearch client failed")),
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				policy := r.getPolicy(t, k8s.ExtractNamespacedName(&policyFixture))
				assert.Equal(t, 2, policy.Status.Resources)
				assert.Equal(t, 1, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.UnknownPhase, policy.Status.Details["elasticsearch"]["ns/test-es"].Phase)
				assert.Equal(t, policyv1alpha1.ReadyPhase, policy.Status.Details["kibana"]["ns/test-kb"].Phase)
				assert.Equal(t, policyv1alpha1.ReadyPhase, policy.Status.Phase)
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Elasticsearch reconciled successfully and Kibana config not yet applied",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture, secretMountsSecretFixture, esPodFixture, &kibanaFixture, mkKibanaPod("ns", true, "testhash")),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(clusterStateFileSettingsFixture(42, nil), nil),
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				policy := r.getPolicy(t, k8s.ExtractNamespacedName(&policyFixture))
				assert.Equal(t, 2, policy.Status.Resources)
				assert.Equal(t, 1, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.ReadyPhase, policy.Status.Details["elasticsearch"]["ns/test-es"].Phase)
				assert.Equal(t, policyv1alpha1.ApplyingChangesPhase, policy.Status.Details["kibana"]["ns/test-kb"].Phase)
				assert.Equal(t, policyv1alpha1.ApplyingChangesPhase, policy.Status.Phase)
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRecorder := record.NewFakeRecorder(100)
			reconciler := ReconcileStackConfigPolicy{
				Client:           tt.args.client,
				esClientProvider: tt.args.esClientProvider,
				recorder:         fakeRecorder,
				licenseChecker:   tt.args.licenseChecker,
				dynamicWatches:   watches.NewDynamicWatches(),
			}
			if tt.pre != nil {
				tt.pre(reconciler)
			}
			got, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: nsnFixture})
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Requeue != tt.wantRequeue {
				t.Errorf("Reconcile() got = %v, wantRequeue %v", got, tt.wantRequeue)
			}
			if got.RequeueAfter > 0 != tt.wantRequeueAfter {
				t.Errorf("Reconcile() got = %v, wantRequeueAfter %v", got, tt.wantRequeueAfter)
			}
			if tt.post != nil {
				tt.post(reconciler, *fakeRecorder)
			}
		})
	}
}

func Test_cleanStackTrace(t *testing.T) {
	stacktrace := "Error processing slm state change: java.lang.IllegalArgumentException: Error on validating SLM requests\n\tat org.elasticsearch.ilm@8.6.1/org.elasticsearch.xpack.slm.action.ReservedSnapshotAction.prepare(ReservedSnapshotAction.java:66)\n\tat org.elasticsearch.ilm@8.6.1/org.elasticsearch.xpack.slm.action.ReservedSnapshotAction.transform(ReservedSnapshotAction.java:77)\n\tat org.elasticsearch.server@8.6.1/org.elasticsearch.reservedstate.service.ReservedClusterStateService.trialRun(ReservedClusterStateService.java:328)\n\tat org.elasticsearch.server@8.6.1/org.elasticsearch.reservedstate.service.ReservedClusterStateService.process(ReservedClusterStateService.java:169)\n\tat org.elasticsearch.server@8.6.1/org.elasticsearch.reservedstate.service.ReservedClusterStateService.process(ReservedClusterStateService.java:122)\n\tat org.elasticsearch.server@8.6.1/org.elasticsearch.reservedstate.service.FileSettingsService.processFileSettings(FileSettingsService.java:389)\n\tat org.elasticsearch.server@8.6.1/org.elasticsearch.reservedstate.service.FileSettingsService.lambda$startWatcher$3(FileSettingsService.java:312)\n\tat java.base/java.lang.Thread.run(Thread.java:833)\n\tSuppressed: java.lang.IllegalArgumentException: no such repository [badrepo]\n\t\tat org.elasticsearch.ilm@8.6.1/org.elasticsearch.xpack.slm.SnapshotLifecycleService.validateRepositoryExists(SnapshotLifecycleService.java:244)\n\t\tat org.elasticsearch.ilm@8.6.1/org.elasticsearch.xpack.slm.action.ReservedSnapshotAction.prepare(ReservedSnapshotAction.java:57)\n\t\t... 7 more\n"
	err := cleanStackTrace([]string{stacktrace})
	expected := "Error processing slm state change: java.lang.IllegalArgumentException: Error on validating SLM requests\n\tSuppressed: java.lang.IllegalArgumentException: no such repository [badrepo]"
	assert.Equal(t, expected, err)
}

func getSettingsHash(secret corev1.Secret) (string, error) {
	var settings filesettings.Settings
	err := json.Unmarshal(secret.Data["settings.json"], &settings)
	if err != nil {
		return "", err
	}
	return hash.HashObject(settings.State), nil
}

func assertExpectedESSecretContent(t *testing.T, c client.Client, esName string, expectedSecret corev1.Secret, actualSecretMounts []policyv1alpha1.SecretMount) {
	t.Helper()
	for _, secretMount := range actualSecretMounts {
		var secretMountsSecret corev1.Secret
		err := c.Get(context.Background(), types.NamespacedName{
			Namespace: "ns",
			Name:      esv1.StackConfigAdditionalSecretName(esName, secretMount.SecretName),
		}, &secretMountsSecret)
		assert.NoError(t, err)
		assert.Equal(t, expectedSecret.Data, secretMountsSecret.Data)
	}
}

func assertKibanaConfigSecret(t *testing.T, c client.Client, kibanaName string, expectedKibanaSecret corev1.Secret) {
	t.Helper()
	var kibanaSecret corev1.Secret
	err := c.Get(context.Background(), types.NamespacedName{
		Namespace: "ns",
		Name:      kibanaName + "-kb-policy-config",
	}, &kibanaSecret)
	require.NoError(t, err)
	require.Equal(t, expectedKibanaSecret.Labels, kibanaSecret.Labels)
	require.Equal(t, expectedKibanaSecret.Annotations, kibanaSecret.Annotations)
	require.Equal(t, expectedKibanaSecret.Data, kibanaSecret.Data)
}
