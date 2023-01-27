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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	commonesclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/filesettings"
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

func (c fakeEsClient) GetClusterState(ctx context.Context) (esclient.ClusterState, error) {
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
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{
					"indices.recovery.max_bytes_per_sec": "42mb",
				}},
			},
		},
	}
	esFixture := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns",
		Name:      "test-es",
		Labels:    map[string]string{"label": "test"},
	},
		Spec: esv1.ElasticsearchSpec{Version: "8.6.1"},
	}
	secretFixture := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "test-es-es-file-settings",
			Labels: map[string]string{
				"common.k8s.elastic.co/type":                "elasticsearch",
				"elasticsearch.k8s.elastic.co/cluster-name": "test-es",
				"eck.k8s.elastic.co/owner-kind":             "StackConfigPolicy",
				"eck.k8s.elastic.co/owner-namespace":        "ns",
				"eck.k8s.elastic.co/owner-name":             "test-policy",
			},
		},
		Data: map[string][]byte{"settings.json": []byte(`{"metadata":{"version":"42","compatibility":"8.4.0"},"state":{"cluster_settings":{"indices.recovery.max_bytes_per_sec":"42mb"},"snapshot_repositories":{},"slm":{},"role_mappings":{},"autoscaling":{},"ilm":{},"ingest_pipelines":{},"index_templates":{"component_templates":{},"composable_index_templates":{}}}}`)},
	}
	secretHash, err := getSettingsHash(secretFixture)
	assert.NoError(t, err)
	secretFixture.Annotations = map[string]string{"policy.k8s.elastic.co/settings-hash": secretHash}

	conflictingSecretFixture := secretFixture.DeepCopy()
	conflictingSecretFixture.Labels["eck.k8s.elastic.co/owner-name"] = "another-policy"

	orphanSecretFixture := secretFixture.DeepCopy()
	orphanSecretFixture.Name = "another-es-es-file-settings"
	orphanSecretFixture.Labels["elasticsearch.k8s.elastic.co/cluster-name"] = "another-es"

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
				client: k8s.NewFakeClient(&esFixture, &secretFixture),
			},
			pre: func(r ReconcileStackConfigPolicy) {
				// after the reconciliation, settings are empty
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
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture, orphanSecretFixture, orphanEsFixture),
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
				client:         k8s.NewFakeClient(&policyFixture, &secretFixture, &oldVersionEsFixture),
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
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture),
				licenseChecker:   &license.MockLicenseChecker{EnterpriseEnabled: true},
				esClientProvider: fakeClientProvider(esclient.FileSettings{}, errors.New("elasticsearch client failed")),
			},
			post: func(r ReconcileStackConfigPolicy, recorder record.FakeRecorder) {
				policy := r.getPolicy(t, k8s.ExtractNamespacedName(&policyFixture))
				assert.Equal(t, 1, policy.Status.Resources)
				assert.Equal(t, 0, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.UnknownPhase, policy.Status.ResourcesStatuses["ns/test-es"].Phase)
				assert.Equal(t, policyv1alpha1.ReadyPhase, policy.Status.Phase)
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Settings secret must be updated to reflect the policy settings",
			args: args{
				client:           k8s.NewFakeClient(updatedPolicyFixture, &esFixture, &secretFixture),
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
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture),
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
				assert.Equal(t, "invalid cluster settings", policy.Status.ResourcesStatuses["ns/test-es"].Error.Message)
			},
			wantErr:          false,
			wantRequeue:      true,
			wantRequeueAfter: true,
		},
		{
			name: "Current settings version is different from the expected one",
			args: args{
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture),
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
				client:           k8s.NewFakeClient(&policyFixture, &esFixture, &secretFixture),
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
				assert.Equal(t, 1, policy.Status.Resources)
				assert.Equal(t, 1, policy.Status.Ready)
				assert.Equal(t, policyv1alpha1.ReadyPhase, policy.Status.Phase)
			},
			wantErr:          false,
			wantRequeue:      false,
			wantRequeueAfter: false,
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
