// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestReconcileTransportCertificatesSecrets(t *testing.T) {
	type args struct {
		ca             *certificates.CA
		es             *esv1.Elasticsearch
		rotationParams certificates.RotationParams
		initialObjects []runtime.Object
	}
	tests := []struct {
		name          string
		args          args
		want          *reconciler.Results
		assertSecrets func(t *testing.T, secrets corev1.SecretList)
	}{
		{
			name: "Initial state, 3 nodeSets cluster, transport certs Secrets don't exists yet",
			args: args{
				ca: testRSACA,
				es: newEsBuilder().addNodeSet("sset1", 2).addNodeSet("sset2", 3).addNodeSet("sset3", 4).build(),
				initialObjects: []runtime.Object{
					// 2 Pods in sset1
					newPodBuilder().forEs(testEsName).inNodeSet("sset1").withIndex(0).withIP("1.1.1.2").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset1").withIndex(1).withIP("1.1.1.3").build(),
					// 3 Pods in sset2
					newPodBuilder().forEs(testEsName).inNodeSet("sset2").withIndex(0).withIP("1.1.2.2").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset2").withIndex(1).withIP("1.1.2.3").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset2").withIndex(2).withIP("1.1.2.4").build(),
					// 4 Pods in sset3
					newPodBuilder().forEs(testEsName).inNodeSet("sset3").withIndex(0).withIP("1.1.3.2").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset3").withIndex(1).withIP("1.1.3.3").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset3").withIndex(2).withIP("1.1.3.4").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset3").withIndex(3).withIP("1.1.3.5").build(),
				},
			},
			want: &reconciler.Results{},
			assertSecrets: func(t *testing.T, secrets corev1.SecretList) {
				t.Helper()
				// Check that there is 1 Secret per StatefulSet
				assert.Equal(t, 3, len(secrets.Items))

				transportCerts1 := getSecret(secrets, "test-es-name-es-sset1-es-transport-certs")
				assert.NotNil(t, transportCerts1)
				// 5 items are expected in the Secret: the CA + 2 * (crt and private keys)
				assert.Equal(t, 5, len(transportCerts1.Data))
				// Check that ca.crt exists
				assert.Equal(t, testRSACABytes, transportCerts1.Data["ca.crt"])
				// Check the labels elasticsearch.k8s.elastic.co/cluster-name and elasticsearch.k8s.elastic.co/statefulset-name
				assert.Equal(t, testEsName, transportCerts1.Labels["elasticsearch.k8s.elastic.co/cluster-name"])
				assert.Equal(t, "test-es-name-es-sset1", transportCerts1.Labels["elasticsearch.k8s.elastic.co/statefulset-name"])

				transportCerts2 := getSecret(secrets, "test-es-name-es-sset2-es-transport-certs")
				assert.NotNil(t, transportCerts2)
				// 7 items are expected in the Secret: the CA + 3 * (crt and private keys)
				assert.Equal(t, 7, len(transportCerts2.Data))
				// Check that ca.crt exists
				assert.Equal(t, testRSACABytes, transportCerts2.Data["ca.crt"])
				// Check the labels elasticsearch.k8s.elastic.co/cluster-name and elasticsearch.k8s.elastic.co/statefulset-name
				assert.Equal(t, testEsName, transportCerts2.Labels["elasticsearch.k8s.elastic.co/cluster-name"])
				assert.Equal(t, "test-es-name-es-sset2", transportCerts2.Labels["elasticsearch.k8s.elastic.co/statefulset-name"])

				transportCerts3 := getSecret(secrets, "test-es-name-es-sset3-es-transport-certs")
				assert.NotNil(t, transportCerts3)
				// 9 items are expected in the Secret: the CA + 4 * (crt and private keys)
				assert.Equal(t, 9, len(transportCerts3.Data))
				// Check that ca.crt exists
				assert.Equal(t, testRSACABytes, transportCerts3.Data["ca.crt"])
				// Check the labels elasticsearch.k8s.elastic.co/cluster-name and elasticsearch.k8s.elastic.co/statefulset-name
				assert.Equal(t, testEsName, transportCerts3.Labels["elasticsearch.k8s.elastic.co/cluster-name"])
				assert.Equal(t, "test-es-name-es-sset3", transportCerts3.Labels["elasticsearch.k8s.elastic.co/statefulset-name"])
			},
		},
		{
			name: "Should reuse and update existing transport certs Secrets",
			args: args{
				ca: testRSACA,
				es: newEsBuilder().addNodeSet("sset1", 2).addNodeSet("sset2", 2).build(),
				initialObjects: []runtime.Object{
					newPodBuilder().forEs(testEsName).inNodeSet("sset1").withIndex(0).withIP("1.1.1.2").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset1").withIndex(1).withIP("1.1.1.3").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset2").withIndex(0).withIP("1.1.2.2").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset2").withIndex(1).withIP("1.1.2.3").build(),
					// Create 3 Secrets but create transport certs only for the first Pods in each StatefulSet
					newtransportCertsSecretBuilder(testEsName, "sset1").forPodIndices(0).build(),
					newtransportCertsSecretBuilder(testEsName, "sset2").forPodIndices(0).build(),
					// Add an existing statefulSet which is not part of the Spec
					newStatefulSet(testEsName, "sset3"),
					newPodBuilder().forEs(testEsName).inNodeSet("sset3").withIndex(0).withIP("1.1.3.2").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset3").withIndex(1).withIP("1.1.3.3").build(),
				},
			},
			want: &reconciler.Results{},
			assertSecrets: func(t *testing.T, secrets corev1.SecretList) {
				t.Helper()
				// Check that there is 1 Secret per StatefulSet
				assert.Equal(t, 3, len(secrets.Items))

				transportCerts1 := getSecret(secrets, "test-es-name-es-sset1-es-transport-certs")
				assert.NotNil(t, transportCerts1)
				// 5 items are expected in the Secret: the CA + 2 * (crt and private keys)
				assert.Equal(t, 5, len(transportCerts1.Data))
				// Check that ca.crt exists
				assert.Equal(t, testRSACABytes, transportCerts1.Data["ca.crt"])
				// Check the labels elasticsearch.k8s.elastic.co/cluster-name and elasticsearch.k8s.elastic.co/statefulset-name
				assert.Equal(t, testEsName, transportCerts1.Labels["elasticsearch.k8s.elastic.co/cluster-name"])
				assert.Equal(t, "test-es-name-es-sset1", transportCerts1.Labels["elasticsearch.k8s.elastic.co/statefulset-name"])

				transportCerts2 := getSecret(secrets, "test-es-name-es-sset2-es-transport-certs")
				assert.NotNil(t, transportCerts2)
				// 5 items are expected in the Secret: the CA + 2 * (crt and private keys)
				assert.Equal(t, 5, len(transportCerts2.Data))
				// Check that ca.crt exists
				assert.Equal(t, testRSACABytes, transportCerts2.Data["ca.crt"])
				// Check the labels elasticsearch.k8s.elastic.co/cluster-name and elasticsearch.k8s.elastic.co/statefulset-name
				assert.Equal(t, testEsName, transportCerts2.Labels["elasticsearch.k8s.elastic.co/cluster-name"])
				assert.Equal(t, "test-es-name-es-sset2", transportCerts2.Labels["elasticsearch.k8s.elastic.co/statefulset-name"])
			},
		},
		{
			name: "Should remove any non used transport certs",
			args: args{
				ca: testRSACA,
				es: newEsBuilder().addNodeSet("sset1", 2).addNodeSet("sset2", 2).build(),
				initialObjects: []runtime.Object{
					newPodBuilder().forEs(testEsName).inNodeSet("sset1").withIndex(0).withIP("1.1.1.2").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset2").withIndex(0).withIP("1.1.2.2").build(),
					newPodBuilder().forEs(testEsName).inNodeSet("sset2").withIndex(1).withIP("1.1.2.3").build(),
					// Create the 2 Secrets and create transport certs for Pods which does not exist anymore
					newtransportCertsSecretBuilder(testEsName, "sset1").forPodIndices(0, 1).build(),    // Pod 1 does not exist
					newtransportCertsSecretBuilder(testEsName, "sset2").forPodIndices(0, 1, 2).build(), // Pod 2 does not exist
				},
			},
			want: &reconciler.Results{},
			assertSecrets: func(t *testing.T, secrets corev1.SecretList) {
				t.Helper()
				// Check that there is 1 Secret per StatefulSet
				assert.Equal(t, 2, len(secrets.Items))

				transportCerts1 := getSecret(secrets, "test-es-name-es-sset1-es-transport-certs")
				assert.NotNil(t, transportCerts1)
				// 5 items are expected in the Secret: the CA + 1 existing Pods * (crt and private keys)
				assert.Equal(t, 3, len(transportCerts1.Data))
				// Transport certs for Pod 1 is still there
				assert.Contains(t, transportCerts1.Data, "test-es-name-es-sset1-0.tls.crt")
				assert.Contains(t, transportCerts1.Data, "test-es-name-es-sset1-0.tls.key")
				// Transport certs for Pod 2 should have been removed
				assert.NotContains(t, transportCerts1.Data, "test-es-name-es-sset1-1.tls.crt")
				assert.NotContains(t, transportCerts1.Data, "test-es-name-es-sset1-1.tls.key")

				transportCerts2 := getSecret(secrets, "test-es-name-es-sset2-es-transport-certs")
				assert.NotNil(t, transportCerts2)
				// 5 items are expected in the Secret: the CA + 2 existing Pods * (crt and private keys)
				assert.Equal(t, 5, len(transportCerts2.Data))
				// Transport certs for Pod 1 and 2 are still there
				assert.Contains(t, transportCerts2.Data, "test-es-name-es-sset2-0.tls.crt")
				assert.Contains(t, transportCerts2.Data, "test-es-name-es-sset2-0.tls.key")
				assert.Contains(t, transportCerts2.Data, "test-es-name-es-sset2-1.tls.crt")
				assert.Contains(t, transportCerts2.Data, "test-es-name-es-sset2-1.tls.key")
				// Transport certs for Pod 3 should have been removed
				assert.NotContains(t, transportCerts2.Data, "test-es-name-es-sset2-2.tls.crt")
				assert.NotContains(t, transportCerts2.Data, "test-es-name-es-sset2-2.tls.key")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(tt.args.initialObjects...)
			if got := ReconcileTransportCertificatesSecrets(k8sClient, tt.args.ca, *tt.args.es, tt.args.rotationParams); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileTransportCertificatesSecrets() = %v, want %v", got, tt.want)
			}
			// Check Secrets
			var secrets corev1.SecretList
			matchLabels := label.NewLabelSelectorForElasticsearch(*tt.args.es)
			ns := client.InNamespace(tt.args.es.Namespace)
			assert.NoError(t, k8sClient.List(context.Background(), &secrets, matchLabels, ns))
			tt.assertSecrets(t, secrets)
		})
	}
}

func TestDeleteStatefulSetTransportCertificate(t *testing.T) {
	type args struct {
		client   k8s.Client
		es       esv1.Elasticsearch
		ssetName string
	}
	tests := []struct {
		name      string
		args      args
		assertErr func(*testing.T, error)
	}{
		{
			name: "StatefulSet transport Secret exists",
			args: args{
				client: k8s.NewFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-name-es-sset1-es-transport-certs",
						Namespace: testNamespace,
					},
				}),
				es:       testES,
				ssetName: esv1.StatefulSet(testEsName, "sset1"),
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				assert.Nil(t, err)
			},
		},
		{
			name: "StatefulSet transport Secret does not exist",
			args: args{
				client:   k8s.NewFakeClient(),
				es:       testES,
				ssetName: esv1.StatefulSet(testEsName, "sset1"),
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				assert.NotNil(t, err)
				assert.True(t, errors.IsNotFound(err))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DeleteStatefulSetTransportCertificate(tt.args.client, tt.args.es.Namespace, tt.args.ssetName)
			tt.assertErr(t, err)
		})
	}
}

func TestDeleteLegacyTransportCertificate(t *testing.T) {
	type args struct {
		client k8s.Client
		es     esv1.Elasticsearch
	}
	tests := []struct {
		name       string
		args       args
		wantDelete bool
		wantErr    bool
	}{
		{
			name: "Former cluster transport Secret exists",
			args: args{
				client: k8s.NewFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-name-es-transport-certificates", // Create a Secret with the former name
						Namespace: testNamespace,
					},
				}),
				es: testES,
			},
			wantDelete: true,
			wantErr:    false,
		},
		{
			name: "Former cluster transport Secret does not exist",
			args: args{
				client: k8s.NewFakeClient(),
				es:     testES,
			},
			wantDelete: false,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trackedClient := trackingK8sClient{
				Client: tt.args.client,
			}
			err := DeleteLegacyTransportCertificate(&trackedClient, tt.args.es)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteLegacyTransportCertificate wantErr %v, got %v", tt.wantErr, err)
			}
			if tt.wantDelete != trackedClient.deleteCalled {
				t.Errorf("DeleteLegacyTransportCertificate wantDelete %v, deleteCalled %v", tt.wantDelete, trackedClient.deleteCalled)
			}
		})
	}
}

type trackingK8sClient struct {
	k8s.Client
	deleteCalled bool
}

func (t *trackingK8sClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	t.deleteCalled = true
	return t.Client.Delete(ctx, obj, opts...)
}

func Test_ensureTransportCertificateSecretExists(t *testing.T) {
	defaultSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.StatefulSetTransportCertificatesSecret(esv1.StatefulSet(testES.Name, "sset1")),
			Namespace: testES.Namespace,
			Labels: map[string]string{
				label.ClusterNameLabelName:     testES.Name,
				label.StatefulSetNameLabelName: esv1.StatefulSet(testES.Name, "sset1"),
			},
		},
		Data: make(map[string][]byte),
	}

	defaultSecretWith := func(setter func(secret *corev1.Secret)) *corev1.Secret {
		secret := defaultSecret.DeepCopy()
		setter(secret)
		return secret
	}

	type args struct {
		c     k8s.Client
		owner esv1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    func(*testing.T, *corev1.Secret)
		wantErr bool
	}{
		{
			name: "should create a secret if it does not already exist",
			args: args{
				c:     k8s.NewFakeClient(),
				owner: testES,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				t.Helper()
				// owner references are set upon creation, so ignore for comparison
				expected := defaultSecretWith(func(s *corev1.Secret) {
					s.OwnerReferences = secret.OwnerReferences
				})
				comparison.AssertEqual(t, expected, secret)
			},
		},
		{
			name: "should update an existing secret",
			args: args{
				c: k8s.NewFakeClient(defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
				})),
				owner: testES,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				t.Helper()
				// UID should be kept the same
				comparison.AssertEqual(t, defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
				}), secret)
			},
		},
		{
			name: "should not modify the secret data if already exists",
			args: args{
				c: k8s.NewFakeClient(defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
					secret.Data = map[string][]byte{
						"existing": []byte("data"),
					}
				})),
				owner: testES,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				t.Helper()
				// UID and data should be kept
				comparison.AssertEqual(t, defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
					secret.Data = map[string][]byte{
						"existing": []byte("data"),
					}
				}), secret)
			},
		},
		{
			name: "should allow additional labels in the secret",
			args: args{
				c: k8s.NewFakeClient(defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.Labels["foo"] = "bar"
				})),
				owner: testES,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				t.Helper()
				comparison.AssertEqual(t, defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.Labels["foo"] = "bar"
				}), secret)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ensureTransportCertificatesSecretExists(tt.args.c, tt.args.owner, esv1.StatefulSet(testES.Name, "sset1"))
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureTransportCertificateSecretExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.want(t, got)
		})
	}
}
