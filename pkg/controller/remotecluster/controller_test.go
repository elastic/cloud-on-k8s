// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remotecluster

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/remotecluster/keystore"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

func TestRemoteCluster_Reconcile(t *testing.T) {
	type fields struct {
		clusters       []client.Object
		accessReviewer rbac.AccessReviewer
		licenseChecker license.Checker
	}
	type args struct {
		request reconcile.Request
	}
	type wantEsAPICalls struct {
		getCrossClusterAPIKeys           []string
		invalidateCrossClusterAPIKey     []string
		crossClusterAPIKeyCreateRequests []esclient.CrossClusterAPIKeyCreateRequest
		updateCrossClusterAPIKey         map[string]esclient.CrossClusterAPIKeyUpdateRequest
	}
	tests := []struct {
		name   string
		fields fields
		args   args

		expectedCASecrets []*corev1.Secret
		unexpectedSecrets []types.NamespacedName
		want              reconcile.Result
		wantErr           bool

		// API keys fake client and expected resources.
		fakeESClient            *fakeESClient
		wantEsAPICalls          wantEsAPICalls
		expectedKeystoreSecrets []*corev1.Secret
	}{
		{
			name: "Simple remote cluster ns1/es1 -> ns2/es2, ns1",
			fields: fields{
				clusters: slices.Concat(
					newClusterBuilder("ns1", "es1", "7.0.0").withRemoteCluster("ns2", "es2").build(),
					newClusterBuilder("ns2", "es2", "7.0.0").build(),
					[]client.Object{
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns2", "es2"),
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedCASecrets: []*corev1.Secret{
				remoteCa("ns1", "es1", "ns2", "es2"),
				remoteCa("ns2", "es2", "ns1", "es1"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "[No API Keys] Bi-directional remote cluster ns1/es1 <-> ns2/es2",
			fields: fields{
				clusters: slices.Concat(
					newClusterBuilder("ns1", "es1", "7.0.0").withRemoteCluster("ns2", "es2").build(),
					newClusterBuilder("ns2", "es2", "7.0.0").withRemoteCluster("ns1", "es1").build(),
					[]client.Object{
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns2", "es2"),
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedCASecrets: []*corev1.Secret{
				remoteCa("ns1", "es1", "ns2", "es2"),
				remoteCa("ns2", "es2", "ns1", "es1"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			// The test below simulates the following situation:
			// * ns1/es1 is the cluster reconciled.
			// * ns1/es2 is the client cluster.
			name: "With API Keys, simple topology",
			fields: fields{
				clusters: slices.Concat(
					// Clusters
					newClusterBuilder("ns1", "es1", "8.15.0").build(),
					newClusterBuilder("ns1", "es2", "8.15.0").
						// ns1/es2 -> ns1/es1
						withAPIKey("ns1", "es1", &esv1.RemoteClusterAPIKey{}).
						build(),
					[]client.Object{
						// Certificates
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns1", "es2"),
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					// reconciled cluster is es1
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedCASecrets: []*corev1.Secret{
				remoteCa("ns1", "es1", "ns1", "es2"),
				remoteCa("ns1", "es2", "ns1", "es1"),
			},
			wantEsAPICalls: wantEsAPICalls{
				getCrossClusterAPIKeys: []string{"eck-*"},
				crossClusterAPIKeyCreateRequests: []esclient.CrossClusterAPIKeyCreateRequest{
					{
						// ns1/es1 is expected to create an API key for ns1/es2
						Name: "eck-ns1-es2-generated-alias-from-ns1-es2-to-ns1-es1-with-api-key",
						CrossClusterAPIKeyUpdateRequest: esclient.CrossClusterAPIKeyUpdateRequest{
							Metadata: map[string]any{
								"elasticsearch.k8s.elastic.co/config-hash": "1384987056",
								"elasticsearch.k8s.elastic.co/managed-by":  "eck",
								"elasticsearch.k8s.elastic.co/name":        "es2",
								"elasticsearch.k8s.elastic.co/namespace":   "ns1",
								"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
							},
						},
					},
				},
			},
			expectedKeystoreSecrets: []*corev1.Secret{
				{
					// Keystore for es2 must be updated.
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"generated-alias-from-ns1-es2-to-ns1-es1-with-api-key":{"namespace":"ns1","name":"es1","id":"generated-id-from-fake-es-client-eck-ns1-es2-generated-alias-from-ns1-es2-to-ns1-es1-with-api-key"}}`,
						},
						Labels: map[string]string{
							"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
							"eck.k8s.elastic.co/credentials":            "true",
							"elasticsearch.k8s.elastic.co/cluster-name": "es2",
						},
						Namespace: "ns1",
						Name:      "es2-es-remote-api-keys",
					},
					Data: map[string][]byte{
						"cluster.remote.generated-alias-from-ns1-es2-to-ns1-es1-with-api-key.credentials": []byte("generated-encoded-key-from-fake-es-client-for-eck-ns1-es2-generated-alias-from-ns1-es2-to-ns1-es1-with-api-key"),
					},
				},
			},
		},
		{
			// The test below simulates the following situation:
			// * ns1/es1 is the cluster reconciled.
			// * There are 3 remote cluster accessing ns1/es1:
			//   * ns2/es2: new cluster, API key must be created
			//   * ns3/es3: new cluster, API key must be created
			//   * ns4/es4: existing remote cluster: one key must be updated, the other one must be deleted.
			//   * ns/es5: this cluster no long exists, key must be deleted.
			name: "With API Keys, complex topology",
			fields: fields{
				clusters: slices.Concat(
					// Clusters
					newClusterBuilder("ns1", "es1", "8.15.0").
						// es1 -> es2
						withAPIKey("ns2", "es2", &esv1.RemoteClusterAPIKey{}).
						// es1 -> es3
						withAPIKey("ns3", "es3", &esv1.RemoteClusterAPIKey{}).
						build(),
					newClusterBuilder("ns2", "es2", "8.15.0").
						// es2 -> es1
						withAPIKey("ns1", "es1", &esv1.RemoteClusterAPIKey{}).
						build(),
					newClusterBuilder("ns3", "es3", "8.15.0").
						// es3 -> es1
						withAPIKey("ns1", "es1", &esv1.RemoteClusterAPIKey{}).
						build(),
					newClusterBuilder("ns4", "es4", "8.15.0").
						// es4-> es1
						withAPIKey("ns1", "es1", &esv1.RemoteClusterAPIKey{}).
						build(),
					[]client.Object{
						// Certificates
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns2", "es2"),
						fakePublicCa("ns3", "es3"),
						fakePublicCa("ns4", "es4"),
						// Assume that ns2/es2 has already a key in its keystore, the goal is to test that the existing keys are not altered.
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"existing-api-key-to-esx":{"namespace":"foo","name":"bar","id":"apikey-to-esx"}}`,
								},
								Labels: map[string]string{
									"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
									"eck.k8s.elastic.co/credentials":            "true",
									"elasticsearch.k8s.elastic.co/cluster-name": "es2",
								},
								Namespace: "ns2",
								Name:      "es2-es-remote-api-keys",
								UID:       uuid.NewUUID(),
							},
							Data: map[string][]byte{
								"cluster.remote.existing-api-key-to-esx.credentials": []byte("encoded-key-for-existing-api-key"),
							},
						},
						// Keystore for ns4/es4, key already exists, this Secret is not expected to be modified.
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"generated-alias-from-ns4-es4-to-ns1-es1-with-api-key":{"namespace":"ns1","name":"es1","id":"generated-id-from-fake-es-client-eck-ns4-es4-generated-alias-from-ns4-es4-to-ns1-es1-with-api-key"}}`,
								},
								Labels: map[string]string{
									"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
									"eck.k8s.elastic.co/credentials":            "true",
									"elasticsearch.k8s.elastic.co/cluster-name": "es4",
								},
								Namespace: "ns4",
								Name:      "es4-es-remote-api-keys",
								UID:       uuid.NewUUID(),
							},
							Data: map[string][]byte{
								"cluster.remote.generated-alias-from-ns4-es4-to-ns1-es1-with-api-key.credentials": []byte("encoded-key-for-existing-api-key"),
							},
						},
						// Assume that ns1/es1 keystore already exists, unexpected keys should be removed.
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"generated-alias-from-ns1-es1-to-ns2-es2-with-api-key":{"namespace":"ns2","name":"es2","id":"apikey-to-es2"},"generated-alias-from-ns1-es1-to-ns3-es3-with-api-key":{"namespace":"ns3","name":"es3","id":"apikey-to-es3"},"api-key-to-non-existent-alias":{"namespace":"nsx","name":"esx","id":"apikey-to-esx"}}`,
								},
								Labels: map[string]string{
									"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
									"eck.k8s.elastic.co/credentials":            "true",
									"elasticsearch.k8s.elastic.co/cluster-name": "es1",
								},
								Namespace: "ns1",
								Name:      "es1-es-remote-api-keys",
								UID:       uuid.NewUUID(),
							},
							Data: map[string][]byte{
								"cluster.remote.generated-alias-from-ns1-es1-to-ns2-es2-with-api-key.credentials": []byte("encoded-key-for-existing-api-key"),
								"cluster.remote.generated-alias-from-ns1-es1-to-ns3-es3-with-api-key.credentials": []byte("encoded-key-for-existing-api-key"),
								// The key below should be removed
								"cluster.remote.api-key-to-non-existent-alias.credentials": []byte("encoded-key-for-api-for-non-existent-cluster"),
							},
						},
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					// reconciled cluster is es1
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedCASecrets: []*corev1.Secret{
				remoteCa("ns1", "es1", "ns2", "es2"),
				remoteCa("ns2", "es2", "ns1", "es1"),
			},
			want: reconcile.Result{},
			fakeESClient: &fakeESClient{
				existingCrossClusterAPIKeys: esclient.CrossClusterAPIKeyList{
					// API key for es4 already exists but with a wrong hash we should expect an update.
					APIKeys: []esclient.CrossClusterAPIKey{
						{
							ID:   "generated-id-from-fake-es-client-eck-ns4-es4-generated-alias-from-ns4-es4-to-ns1-es1-with-api-key",
							Name: "eck-ns4-es4-generated-alias-from-ns4-es4-to-ns1-es1-with-api-key",
							Metadata: map[string]any{
								"elasticsearch.k8s.elastic.co/config-hash": "unexpected-hash",
								"elasticsearch.k8s.elastic.co/managed-by":  "eck",
								"elasticsearch.k8s.elastic.co/name":        "es4",
								"elasticsearch.k8s.elastic.co/namespace":   "ns4",
								"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
							},
						},
						// The key below belongs to a cluster which no longer exists, it must be invalidated.
						{
							ID:   "apikey-from-es5-to-es1",
							Name: "eck-ns5-es5-generated-ns1-es1-0-with-api-key",
							Metadata: map[string]any{
								"elasticsearch.k8s.elastic.co/config-hash": "1384987056",
								"elasticsearch.k8s.elastic.co/managed-by":  "eck",
								"elasticsearch.k8s.elastic.co/name":        "es5",
								"elasticsearch.k8s.elastic.co/namespace":   "ns5",
								"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
							},
						},
						// The key below belongs to the existing cluster es4 which is no longer referencing es1 using that alias, it must be invalidated.
						{
							ID:   "apikey-from-es4-to-es1-old-alias",
							Name: "eck-ns4-es4-to-ns1-es1-0-old-alias",
							Metadata: map[string]any{
								"elasticsearch.k8s.elastic.co/config-hash": "unexpected-hash",
								"elasticsearch.k8s.elastic.co/managed-by":  "eck",
								"elasticsearch.k8s.elastic.co/name":        "es4",
								"elasticsearch.k8s.elastic.co/namespace":   "ns4",
								"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
							},
						},
					},
				},
			},
			wantEsAPICalls: wantEsAPICalls{
				getCrossClusterAPIKeys:       []string{"eck-*"},
				invalidateCrossClusterAPIKey: []string{"eck-ns4-es4-to-ns1-es1-0-old-alias", "eck-ns5-es5-generated-ns1-es1-0-with-api-key"},
				updateCrossClusterAPIKey: map[string]esclient.CrossClusterAPIKeyUpdateRequest{
					"generated-id-from-fake-es-client-eck-ns4-es4-generated-alias-from-ns4-es4-to-ns1-es1-with-api-key": {
						RemoteClusterAPIKey: esv1.RemoteClusterAPIKey{},
						Metadata: map[string]any{
							"elasticsearch.k8s.elastic.co/config-hash": "1384987056",
							"elasticsearch.k8s.elastic.co/managed-by":  "eck",
							"elasticsearch.k8s.elastic.co/name":        "es4",
							"elasticsearch.k8s.elastic.co/namespace":   "ns4",
							"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
						},
					},
				},
				// We expect 2 keys to be created for ns1/es1
				crossClusterAPIKeyCreateRequests: []esclient.CrossClusterAPIKeyCreateRequest{
					{
						Name: "eck-ns2-es2-generated-alias-from-ns2-es2-to-ns1-es1-with-api-key",
						CrossClusterAPIKeyUpdateRequest: esclient.CrossClusterAPIKeyUpdateRequest{
							Metadata: map[string]any{
								"elasticsearch.k8s.elastic.co/config-hash": "1384987056",
								"elasticsearch.k8s.elastic.co/managed-by":  "eck",
								"elasticsearch.k8s.elastic.co/name":        "es2",
								"elasticsearch.k8s.elastic.co/namespace":   "ns2",
								"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
							},
						},
					},
					{
						Name: "eck-ns3-es3-generated-alias-from-ns3-es3-to-ns1-es1-with-api-key",
						CrossClusterAPIKeyUpdateRequest: esclient.CrossClusterAPIKeyUpdateRequest{
							Metadata: map[string]any{
								"elasticsearch.k8s.elastic.co/config-hash": "1384987056",
								"elasticsearch.k8s.elastic.co/managed-by":  "eck",
								"elasticsearch.k8s.elastic.co/name":        "es3",
								"elasticsearch.k8s.elastic.co/namespace":   "ns3",
								"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
							},
						},
					},
				},
			},
			expectedKeystoreSecrets: []*corev1.Secret{
				// Unexpected keys in es1 keystore must be removed.
				{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"generated-alias-from-ns1-es1-to-ns2-es2-with-api-key":{"namespace":"ns2","name":"es2","id":"apikey-to-es2"},"generated-alias-from-ns1-es1-to-ns3-es3-with-api-key":{"namespace":"ns3","name":"es3","id":"apikey-to-es3"}}`,
						},
						Labels: map[string]string{
							"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
							"eck.k8s.elastic.co/credentials":            "true",
							"elasticsearch.k8s.elastic.co/cluster-name": "es1",
						},
						Namespace: "ns1",
						Name:      "es1-es-remote-api-keys",
					},
					Data: map[string][]byte{
						"cluster.remote.generated-alias-from-ns1-es1-to-ns2-es2-with-api-key.credentials": []byte("encoded-key-for-existing-api-key"),
						"cluster.remote.generated-alias-from-ns1-es1-to-ns3-es3-with-api-key.credentials": []byte("encoded-key-for-existing-api-key"),
					},
				},
				{
					// Keystore for es2 must be updated.
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"existing-api-key-to-esx":{"namespace":"foo","name":"bar","id":"apikey-to-esx"},"generated-alias-from-ns2-es2-to-ns1-es1-with-api-key":{"namespace":"ns1","name":"es1","id":"generated-id-from-fake-es-client-eck-ns2-es2-generated-alias-from-ns2-es2-to-ns1-es1-with-api-key"}}`,
						},
						Labels: map[string]string{
							"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
							"eck.k8s.elastic.co/credentials":            "true",
							"elasticsearch.k8s.elastic.co/cluster-name": "es2",
						},
						Namespace: "ns2",
						Name:      "es2-es-remote-api-keys",
					},
					Data: map[string][]byte{
						"cluster.remote.existing-api-key-to-esx.credentials":                              []byte("encoded-key-for-existing-api-key"),
						"cluster.remote.generated-alias-from-ns2-es2-to-ns1-es1-with-api-key.credentials": []byte("generated-encoded-key-from-fake-es-client-for-eck-ns2-es2-generated-alias-from-ns2-es2-to-ns1-es1-with-api-key"),
					},
				},
				{
					// Keystore for es3 must be created.
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"generated-alias-from-ns3-es3-to-ns1-es1-with-api-key":{"namespace":"ns1","name":"es1","id":"generated-id-from-fake-es-client-eck-ns3-es3-generated-alias-from-ns3-es3-to-ns1-es1-with-api-key"}}`,
						},
						Labels: map[string]string{
							"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
							"eck.k8s.elastic.co/credentials":            "true",
							"elasticsearch.k8s.elastic.co/cluster-name": "es3",
						},
						Namespace: "ns3",
						Name:      "es3-es-remote-api-keys",
					},
					Data: map[string][]byte{
						"cluster.remote.generated-alias-from-ns3-es3-to-ns1-es1-with-api-key.credentials": []byte("generated-encoded-key-from-fake-es-client-for-eck-ns3-es3-generated-alias-from-ns3-es3-to-ns1-es1-with-api-key"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"generated-alias-from-ns4-es4-to-ns1-es1-with-api-key":{"namespace":"ns1","name":"es1","id":"generated-id-from-fake-es-client-eck-ns4-es4-generated-alias-from-ns4-es4-to-ns1-es1-with-api-key"}}`,
						},
						Labels: map[string]string{
							"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
							"eck.k8s.elastic.co/credentials":            "true",
							"elasticsearch.k8s.elastic.co/cluster-name": "es4",
						},
						Namespace: "ns4",
						Name:      "es4-es-remote-api-keys",
					},
					Data: map[string][]byte{
						"cluster.remote.generated-alias-from-ns4-es4-to-ns1-es1-with-api-key.credentials": []byte("encoded-key-for-existing-api-key"),
					},
				},
			},
		},
		{
			// Same test as above but the associations are no longer permitted.
			// * ns1/es1 is the cluster reconciled.
			// * There are 3 remote cluster accessing ns1/es1:
			//   * ns2/es2: new cluster, API key must not be created
			//   * ns3/es3: new cluster, API key must not be created
			//   * ns4/es4: existing remote cluster: 2 keys must be deleted.
			//   * ns/es5: this cluster no long exists, key must be deleted.
			name: "With API Keys: associations are no longer permitted",
			fields: fields{
				clusters: slices.Concat(
					// Clusters
					newClusterBuilder("ns1", "es1", "8.15.0").
						// es1 -> es2
						withAPIKey("ns2", "es2", &esv1.RemoteClusterAPIKey{}).
						// es1 -> es3
						withAPIKey("ns3", "es3", &esv1.RemoteClusterAPIKey{}).
						build(),
					newClusterBuilder("ns2", "es2", "8.15.0").
						// es2 -> es1
						withAPIKey("ns1", "es1", &esv1.RemoteClusterAPIKey{}).
						build(),
					newClusterBuilder("ns3", "es3", "8.15.0").
						// es3 -> es1
						withAPIKey("ns1", "es1", &esv1.RemoteClusterAPIKey{}).
						build(),
					newClusterBuilder("ns4", "es4", "8.15.0").
						// es4-> es1
						withAPIKey("ns1", "es1", &esv1.RemoteClusterAPIKey{}).
						build(),
					[]client.Object{
						// Certificates
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns2", "es2"),
						fakePublicCa("ns3", "es3"),
						fakePublicCa("ns4", "es4"),
						// Assume that ns2/es2 has already a key in its keystore, the goal is to test that the existing keys are not altered.
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"existing-api-key-to-esx":{"namespace":"foo","name":"bar","id":"apikey-to-esx"}}`,
								},
								Labels: map[string]string{
									"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
									"eck.k8s.elastic.co/credentials":            "true",
									"elasticsearch.k8s.elastic.co/cluster-name": "es2",
								},
								Namespace: "ns2",
								Name:      "es2-es-remote-api-keys",
								UID:       uuid.NewUUID(),
							},
							Data: map[string][]byte{
								"cluster.remote.existing-api-key-to-esx.credentials": []byte("encoded-key-for-existing-api-key"),
							},
						},
						// Keystore for ns4/es4, key already exists, this Secret must be deleted since association is no longer permitted.
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"generated-ns1-es1-0-with-api-key":{"namespace":"ns1","name":"es1","id":"apikey-from-es4-to-es1"}}`,
								},
								Labels: map[string]string{
									"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
									"eck.k8s.elastic.co/credentials":            "true",
									"elasticsearch.k8s.elastic.co/cluster-name": "es4",
								},
								Namespace: "ns4",
								Name:      "es4-es-remote-api-keys",
								UID:       uuid.NewUUID(),
							},
							Data: map[string][]byte{
								"cluster.remote.generated-ns1-es1-0-with-api-key.credentials": []byte("encoded-key-for-existing-api-key"),
							},
						},
						// Assume that ns1/es1 keystore already exists, unexpected keys should be removed.
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"generated-alias-from-ns1-es1-to-ns2-es2-with-api-key":{"namespace":"ns2","name":"es2","id":"apikey-to-es2"},"generated-alias-from-ns1-es1-to-ns3-es3-with-api-key":{"namespace":"ns3","name":"es3","id":"apikey-to-es3"},"api-key-to-non-existent-alias":{"namespace":"nsx","name":"esx","id":"apikey-to-esx"}}`,
								},
								Labels: map[string]string{
									"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
									"eck.k8s.elastic.co/credentials":            "true",
									"elasticsearch.k8s.elastic.co/cluster-name": "es1",
								},
								Namespace: "ns1",
								Name:      "es1-es-remote-api-keys",
								UID:       uuid.NewUUID(),
							},
							Data: map[string][]byte{
								"cluster.remote.generated-alias-from-ns1-es1-to-ns2-es2-with-api-key.credentials": []byte("encoded-key-for-existing-api-key"),
								"cluster.remote.generated-alias-from-ns1-es1-to-ns3-es3-with-api-key.credentials": []byte("encoded-key-for-existing-api-key"),
								// The key below should be removed
								"cluster.remote.api-key-to-non-existent-alias.credentials": []byte("encoded-key-for-api-for-non-existent-cluster"),
							},
						},
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: false},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					// reconciled cluster is es1
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			fakeESClient: &fakeESClient{
				existingCrossClusterAPIKeys: esclient.CrossClusterAPIKeyList{
					// API key for es4 already exists but with a wrong hash we should expect an update.
					APIKeys: []esclient.CrossClusterAPIKey{
						{
							ID:   "apikey-from-es4-to-es1",
							Name: "eck-ns4-es4-generated-ns1-es1-0-with-api-key",
							Metadata: map[string]any{
								"elasticsearch.k8s.elastic.co/config-hash": "unexpected-hash",
								"elasticsearch.k8s.elastic.co/managed-by":  "eck",
								"elasticsearch.k8s.elastic.co/name":        "es4",
								"elasticsearch.k8s.elastic.co/namespace":   "ns4",
								"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
							},
						},
						// The key below belongs to a cluster which no longer exists, it must be invalidated.
						{
							ID:   "apikey-from-es5-to-es1",
							Name: "eck-ns5-es5-generated-ns1-es1-0-with-api-key",
							Metadata: map[string]any{
								"elasticsearch.k8s.elastic.co/config-hash": "1384987056",
								"elasticsearch.k8s.elastic.co/managed-by":  "eck",
								"elasticsearch.k8s.elastic.co/name":        "es5",
								"elasticsearch.k8s.elastic.co/namespace":   "ns5",
								"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
							},
						},
						// The key below belongs to the existing cluster es4 which is no longer referencing es1 using that alias, it must be invalidated.
						{
							ID:   "apikey-from-es4-to-es1-old-alias",
							Name: "eck-ns4-es4-to-ns1-es1-0-old-alias",
							Metadata: map[string]any{
								"elasticsearch.k8s.elastic.co/config-hash": "unexpected-hash",
								"elasticsearch.k8s.elastic.co/managed-by":  "eck",
								"elasticsearch.k8s.elastic.co/name":        "es4",
								"elasticsearch.k8s.elastic.co/namespace":   "ns4",
								"elasticsearch.k8s.elastic.co/uid":         types.UID(""),
							},
						},
					},
				},
			},
			wantEsAPICalls: wantEsAPICalls{
				getCrossClusterAPIKeys:       []string{"eck-*"},
				invalidateCrossClusterAPIKey: []string{"eck-ns4-es4-generated-ns1-es1-0-with-api-key", "eck-ns4-es4-to-ns1-es1-0-old-alias", "eck-ns5-es5-generated-ns1-es1-0-with-api-key"},
				// No update allowed.
				updateCrossClusterAPIKey: nil,
				// No creation allowed.
				crossClusterAPIKeyCreateRequests: nil,
			},
			unexpectedSecrets: []types.NamespacedName{
				{Namespace: "ns1", Name: "es1-es-remote-api-keys"},
				{Namespace: "ns3", Name: "es3-es-remote-api-keys"},
				{Namespace: "ns4", Name: "es4-es-remote-api-keys"},
			},
			expectedKeystoreSecrets: []*corev1.Secret{
				{
					// Keystore for es2 must not be updated with es1 key.
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"elasticsearch.k8s.elastic.co/remote-cluster-api-keys": `{"existing-api-key-to-esx":{"namespace":"foo","name":"bar","id":"apikey-to-esx"}}`,
						},
						Labels: map[string]string{
							"common.k8s.elastic.co/type":                "remote-cluster-api-keys",
							"eck.k8s.elastic.co/credentials":            "true",
							"elasticsearch.k8s.elastic.co/cluster-name": "es2",
						},
						Namespace: "ns2",
						Name:      "es2-es-remote-api-keys",
					},
					Data: map[string][]byte{
						// This entry is going to be removed once ns2/es2 is reconciled.
						"cluster.remote.existing-api-key-to-esx.credentials": []byte("encoded-key-for-existing-api-key"),
					},
				},
			},
		},
		{
			name: "Deleted remote cluster",
			fields: fields{
				clusters: slices.Concat(
					newClusterBuilder("ns1", "es1", "7.0.0").build(),
					newClusterBuilder("ns2", "es2", "7.0.0").build(),
					[]client.Object{
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns2", "es2"),
						remoteCa("ns1", "es1", "ns2", "es2"),
						remoteCa("ns2", "es2", "ns1", "es1"),
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			unexpectedSecrets: []types.NamespacedName{
				{
					Namespace: "ns1",
					Name: remoteCASecretName("es1", types.NamespacedName{
						Namespace: "ns2",
						Name:      "es2",
					}),
				},
				{
					Namespace: "ns2",
					Name: remoteCASecretName("es2", types.NamespacedName{
						Namespace: "ns1",
						Name:      "es1",
					}),
				},
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "CA content has been updated, remote ca must be reconciled",
			fields: fields{
				clusters: slices.Concat(
					newClusterBuilder("ns1", "es1", "7.0.0").withRemoteCluster("ns2", "es2").build(),
					newClusterBuilder("ns2", "es2", "7.0.0").build(),
					[]client.Object{
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns2", "es2"),
						withDataCert(remoteCa("ns1", "es1", "ns2", "es2"), []byte("foo")),
						withDataCert(remoteCa("ns2", "es2", "ns1", "es1"), []byte("bar")),
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedCASecrets: []*corev1.Secret{
				remoteCa("ns1", "es1", "ns2", "es2"),
				remoteCa("ns2", "es2", "ns1", "es1"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			// ns1/es1 has been deleted - all related secrets in other namespaces must be deleted
			name: "Deleted cluster",
			fields: fields{
				clusters: slices.Concat(
					// ns2/es2
					newClusterBuilder("ns2", "es2", "7.0.0").withRemoteCluster("ns1", "es1").build(),
					// ns3/es3
					newClusterBuilder("ns3", "es3", "7.0.0").withRemoteCluster("ns1", "es1").build(),
					[]client.Object{
						fakePublicCa("ns2", "es2"),
						remoteCa("ns2", "es2", "ns1", "es1"),
						fakePublicCa("ns3", "es3"),
						remoteCa("ns3", "es3", "ns1", "es1"),
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			unexpectedSecrets: []types.NamespacedName{
				{
					Namespace: "ns3",
					Name: remoteCASecretName("es3", types.NamespacedName{
						Namespace: "ns1",
						Name:      "es1",
					}),
				},
				{
					Namespace: "ns2",
					Name: remoteCASecretName("es2", types.NamespacedName{
						Namespace: "ns1",
						Name:      "es1",
					}),
				},
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "No enterprise license, remote ca are not created",
			fields: fields{
				clusters: slices.Concat(
					newClusterBuilder("ns1", "es1", "7.0.0").withRemoteCluster("ns2", "es2").build(),
					newClusterBuilder("ns2", "es2", "7.0.0").build(),
					[]client.Object{
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns2", "es2"),
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: false},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			unexpectedSecrets: []types.NamespacedName{
				{
					Namespace: "ns1",
					Name: remoteCASecretName("es1", types.NamespacedName{
						Namespace: "ns2",
						Name:      "es2",
					}),
				},
				{
					Namespace: "ns2",
					Name: remoteCASecretName("es2", types.NamespacedName{
						Namespace: "ns1",
						Name:      "es1",
					}),
				},
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "No enterprise license, existing remote ca are left untouched",
			fields: fields{
				clusters: slices.Concat(
					newClusterBuilder("ns1", "es1", "7.0.0").withRemoteCluster("ns2", "es2").build(),
					newClusterBuilder("ns2", "es2", "7.0.0").build(),
					[]client.Object{
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns2", "es2"),
						remoteCa("ns1", "es1", "ns2", "es2"),
						remoteCa("ns2", "es2", "ns1", "es1"),
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: false},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedCASecrets: []*corev1.Secret{
				remoteCa("ns1", "es1", "ns2", "es2"),
				remoteCa("ns2", "es2", "ns1", "es1"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "Association is not allowed, existing remote ca are removed",
			fields: fields{
				clusters: slices.Concat(
					newClusterBuilder("ns1", "es1", "7.0.0").withRemoteCluster("ns2", "es2").build(),
					newClusterBuilder("ns2", "es2", "7.0.0").build(),
					[]client.Object{
						fakePublicCa("ns1", "es1"),
						fakePublicCa("ns2", "es2"),
						remoteCa("ns1", "es1", "ns2", "es2"),
						remoteCa("ns2", "es2", "ns1", "es1"),
					},
				),
				accessReviewer: &fakeAccessReviewer{allowed: false},
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			unexpectedSecrets: []types.NamespacedName{
				{
					Namespace: "ns1",
					Name:      remoteCASecretName("es1", types.NamespacedName{Namespace: "ns2", Name: "es2"}),
				},
				{
					Namespace: "ns2",
					Name:      remoteCASecretName("es2", types.NamespacedName{Namespace: "ns1", Name: "es1"}),
				},
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := watches.NewDynamicWatches()
			fakeESClient := &fakeESClient{}
			if tt.fakeESClient != nil {
				fakeESClient = tt.fakeESClient
			}
			k8sClient := k8s.NewFakeClient(tt.fields.clusters...)
			r := &ReconcileRemoteClusters{
				Client:         k8sClient,
				accessReviewer: tt.fields.accessReviewer,
				watches:        w,
				licenseChecker: tt.fields.licenseChecker,
				recorder:       record.NewFakeRecorder(10),
				esClientProvider: func(_ context.Context, _ k8s.Client, _ net.Dialer, _ esv1.Elasticsearch) (esclient.Client, error) {
					return fakeESClient, nil
				},
				keystoreProvider: keystore.NewProvider(k8sClient),
			}
			fakeCtx := context.Background()
			got, err := r.Reconcile(fakeCtx, tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileRemoteCa.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileRemoteCa.Reconcile() = %v, want %v", got, tt.want)
			}
			// Check that expected secrets are here
			for _, expectedSecret := range tt.expectedCASecrets {
				var actualSecret corev1.Secret
				assert.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Namespace: expectedSecret.Namespace, Name: expectedSecret.Name}, &actualSecret))
				// Compare content
				actualCa, ok := actualSecret.Data[certificates.CAFileName]
				assert.True(t, ok)
				assert.Equal(t, expectedSecret.Data[certificates.CAFileName], actualCa)
				// Compare labels
				assert.NotNil(t, actualSecret.Labels)
				assert.Equal(t, expectedSecret.Labels, actualSecret.Labels)
			}
			// Check that unexpected secrets does not exist
			for _, unexpectedSecret := range tt.unexpectedSecrets {
				var actualSecret corev1.Secret
				err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: unexpectedSecret.Namespace, Name: unexpectedSecret.Name}, &actualSecret)
				assert.True(t, apierrors.IsNotFound(err), "unexpected Secret %s/%s", unexpectedSecret.Namespace, unexpectedSecret.Name)
			}
			// Fake ES client assertions
			assert.ElementsMatch(t, tt.wantEsAPICalls.getCrossClusterAPIKeys, fakeESClient.getCrossClusterAPIKeys, "unexpected calls to GetCrossClusterAPIKeys")
			assert.ElementsMatch(t, tt.wantEsAPICalls.invalidateCrossClusterAPIKey, fakeESClient.invalidateCrossClusterAPIKey, "unexpected calls to InvalidateCrossClusterAPIKey")
			assert.ElementsMatch(
				t,
				tt.wantEsAPICalls.crossClusterAPIKeyCreateRequests,
				fakeESClient.crossClusterAPIKeyCreateRequests,
				"unexpected calls to CreateCrossClusterAPIKey\n%s\n", cmp.Diff(tt.wantEsAPICalls.crossClusterAPIKeyCreateRequests, fakeESClient.crossClusterAPIKeyCreateRequests),
			)
			assert.Equal(
				t,
				tt.wantEsAPICalls.updateCrossClusterAPIKey,
				fakeESClient.updateCrossClusterAPIKey,
				"unexpected calls to UpdateCrossClusterAPIKey\n%s\n", cmp.Diff(tt.wantEsAPICalls.updateCrossClusterAPIKey, fakeESClient.updateCrossClusterAPIKey),
			)

			// Keystore assertions
			for _, expectedSecret := range tt.expectedKeystoreSecrets {
				actualSecret := &corev1.Secret{}
				if err := k8sClient.Get(fakeCtx, types.NamespacedName{
					Namespace: expectedSecret.Namespace,
					Name:      expectedSecret.Name,
				}, actualSecret); err != nil {
					t.Fatalf("error while retrieving keystore %s/%s: %v", expectedSecret.Namespace, expectedSecret.Name, err)
				}
				if diff := cmp.Diff(expectedSecret.Labels, actualSecret.Labels); len(diff) > 0 {
					t.Errorf("unexpected labels on Secret %s/%s:\n%s\n", expectedSecret.Namespace, expectedSecret.Name, diff)
				}
				if diff := cmp.Diff(expectedSecret.Annotations, actualSecret.Annotations); len(diff) > 0 {
					t.Errorf("unexpected annotations on Secret %s/%s:\n%s\n", expectedSecret.Namespace, expectedSecret.Name, diff)
				}
				if diff := cmp.Diff(expectedSecret.Data, actualSecret.Data); len(diff) > 0 {
					t.Errorf("unexpected data in Secret %s/%s:\n%s\n", expectedSecret.Namespace, expectedSecret.Name, diff)
				}
			}
		})
	}
}
