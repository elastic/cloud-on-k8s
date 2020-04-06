// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

type clusterBuilder struct {
	name, namespace string
	remoteClusters  []commonv1.ObjectSelector
}

func newClusteBuilder(namespace, name string) *clusterBuilder {
	return &clusterBuilder{
		name:      name,
		namespace: namespace,
	}
}

func (cb *clusterBuilder) withRemoteCluster(namespace, name string) *clusterBuilder {
	cb.remoteClusters = append(cb.remoteClusters, commonv1.ObjectSelector{
		Name:      name,
		Namespace: namespace,
	})
	return cb
}

func (cb *clusterBuilder) build() *esv1.Elasticsearch {
	remoteClusters := make([]esv1.RemoteCluster, len(cb.remoteClusters))
	i := 0
	for _, remoteCluster := range cb.remoteClusters {
		remoteClusters[i] = esv1.RemoteCluster{
			ElasticsearchRef: commonv1.ObjectSelector{
				Name:      remoteCluster.Name,
				Namespace: remoteCluster.Namespace,
			}}
		i++
	}

	return &esv1.Elasticsearch{
		ObjectMeta: v1.ObjectMeta{
			Namespace: cb.namespace,
			Name:      cb.name,
		},
		Spec: esv1.ElasticsearchSpec{
			RemoteClusters: remoteClusters,
		},
	}
}

type fakeAccessReviewer struct {
	allowed bool
	err     error
}

func (f *fakeAccessReviewer) AccessAllowed(_ string, _ string, _ runtime.Object) (bool, error) {
	return f.allowed, f.err
}

type fakeLicenseChecker struct {
	enterpriseFeaturesEnabled bool
}

func (f fakeLicenseChecker) CurrentEnterpriseLicense() (*license.EnterpriseLicense, error) {
	return nil, nil
}
func (f fakeLicenseChecker) EnterpriseFeaturesEnabled() (bool, error) {
	return f.enterpriseFeaturesEnabled, nil
}
func (f fakeLicenseChecker) Valid(l license.EnterpriseLicense) (bool, error) {
	return f.enterpriseFeaturesEnabled, nil
}

func fakePublicCa(namespace, name string) *corev1.Secret {
	namespacedName := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	transportPublicCertKey := transport.PublicCertsSecretRef(namespacedName)
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: transportPublicCertKey.Namespace,
			Name:      transportPublicCertKey.Name,
		},
		Data: map[string][]byte{
			certificates.CAFileName: []byte(namespacedName.String()),
		},
	}
}

// remoteCa builds an expected remote Ca
func remoteCa(localNamespace, localName, remoteNamespace, remoteName string) *corev1.Secret {
	remoteNamespacedName := types.NamespacedName{
		Name:      remoteName,
		Namespace: remoteNamespace,
	}
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: localNamespace,
			Name:      remoteCASecretName(localName, remoteNamespacedName),
			Labels: map[string]string{
				"common.k8s.elastic.co/type":                            "remote-ca",
				"elasticsearch.k8s.elastic.co/cluster-name":             localName,
				"elasticsearch.k8s.elastic.co/remote-cluster-name":      remoteName,
				"elasticsearch.k8s.elastic.co/remote-cluster-namespace": remoteNamespace,
			},
		},
		Data: map[string][]byte{
			certificates.CAFileName: []byte(remoteNamespacedName.String()),
		},
	}
}

func withDataCert(caSecret *corev1.Secret, newCa []byte) *corev1.Secret {
	caSecret.Data[certificates.CAFileName] = newCa
	return caSecret
}

func TestReconcileRemoteCa_Reconcile(t *testing.T) {
	type fields struct {
		clusters       []runtime.Object
		accessReviewer rbac.AccessReviewer
		licenseChecker license.Checker
	}
	type args struct {
		request reconcile.Request
	}
	tests := []struct {
		name   string
		fields fields
		args   args

		expectedSecrets   []*corev1.Secret
		unexpectedSecrets []types.NamespacedName
		want              reconcile.Result
		wantErr           bool
	}{
		{
			name: "Simple remote cluster ns1/es1 -> ns2/es2",
			fields: fields{
				clusters: []runtime.Object{
					newClusteBuilder("ns1", "es1").withRemoteCluster("ns2", "es2").build(),
					fakePublicCa("ns1", "es1"),
					newClusteBuilder("ns2", "es2").build(),
					fakePublicCa("ns2", "es2"),
				},
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: &fakeLicenseChecker{enterpriseFeaturesEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedSecrets: []*corev1.Secret{
				remoteCa("ns1", "es1", "ns2", "es2"),
				remoteCa("ns2", "es2", "ns1", "es1"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "Bi-directional remote cluster ns1/es1 <-> ns2/es2",
			fields: fields{
				clusters: []runtime.Object{
					newClusteBuilder("ns1", "es1").withRemoteCluster("ns2", "es2").build(),
					fakePublicCa("ns1", "es1"),
					newClusteBuilder("ns2", "es2").withRemoteCluster("ns1", "es1").build(),
					fakePublicCa("ns2", "es2"),
				},
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: &fakeLicenseChecker{enterpriseFeaturesEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedSecrets: []*corev1.Secret{
				remoteCa("ns1", "es1", "ns2", "es2"),
				remoteCa("ns2", "es2", "ns1", "es1"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "Deleted remote cluster",
			fields: fields{
				clusters: []runtime.Object{
					newClusteBuilder("ns1", "es1").build(),
					fakePublicCa("ns1", "es1"),
					newClusteBuilder("ns2", "es2").build(),
					fakePublicCa("ns2", "es2"),
					remoteCa("ns1", "es1", "ns2", "es2"),
					remoteCa("ns2", "es2", "ns1", "es1"),
				},
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: &fakeLicenseChecker{enterpriseFeaturesEnabled: true},
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
				clusters: []runtime.Object{
					newClusteBuilder("ns1", "es1").withRemoteCluster("ns2", "es2").build(),
					fakePublicCa("ns1", "es1"),
					newClusteBuilder("ns2", "es2").build(),
					fakePublicCa("ns2", "es2"),
					withDataCert(remoteCa("ns1", "es1", "ns2", "es2"), []byte("foo")),
					withDataCert(remoteCa("ns2", "es2", "ns1", "es1"), []byte("bar")),
				},
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: &fakeLicenseChecker{enterpriseFeaturesEnabled: true},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedSecrets: []*corev1.Secret{
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
				clusters: []runtime.Object{
					// ns2/es2
					newClusteBuilder("ns2", "es2").withRemoteCluster("ns1", "es1").build(),
					fakePublicCa("ns2", "es2"),
					remoteCa("ns2", "es2", "ns1", "es1"),
					// ns3/es3
					newClusteBuilder("ns3", "es3").withRemoteCluster("ns1", "es1").build(),
					fakePublicCa("ns3", "es3"),
					remoteCa("ns3", "es3", "ns1", "es1"),
				},
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: &fakeLicenseChecker{enterpriseFeaturesEnabled: true},
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
				clusters: []runtime.Object{
					newClusteBuilder("ns1", "es1").withRemoteCluster("ns2", "es2").build(),
					fakePublicCa("ns1", "es1"),
					newClusteBuilder("ns2", "es2").build(),
					fakePublicCa("ns2", "es2"),
				},
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: &fakeLicenseChecker{enterpriseFeaturesEnabled: false},
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
				clusters: []runtime.Object{
					newClusteBuilder("ns1", "es1").withRemoteCluster("ns2", "es2").build(),
					fakePublicCa("ns1", "es1"),
					newClusteBuilder("ns2", "es2").build(),
					fakePublicCa("ns2", "es2"),
					remoteCa("ns1", "es1", "ns2", "es2"),
					remoteCa("ns2", "es2", "ns1", "es1"),
				},
				accessReviewer: &fakeAccessReviewer{allowed: true},
				licenseChecker: &fakeLicenseChecker{enterpriseFeaturesEnabled: false},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "es1",
						Namespace: "ns1",
					},
				},
			},
			expectedSecrets: []*corev1.Secret{
				remoteCa("ns1", "es1", "ns2", "es2"),
				remoteCa("ns2", "es2", "ns1", "es1"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "Association is not allowed, existing remote ca are removed",
			fields: fields{
				clusters: []runtime.Object{
					newClusteBuilder("ns1", "es1").withRemoteCluster("ns2", "es2").build(),
					fakePublicCa("ns1", "es1"),
					newClusteBuilder("ns2", "es2").build(),
					fakePublicCa("ns2", "es2"),
					remoteCa("ns1", "es1", "ns2", "es2"),
					remoteCa("ns2", "es2", "ns1", "es1"),
				},
				accessReviewer: &fakeAccessReviewer{allowed: false},
				licenseChecker: &fakeLicenseChecker{enterpriseFeaturesEnabled: true},
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
			r := &ReconcileRemoteCa{
				Client:         k8s.WrappedFakeClient(tt.fields.clusters...),
				accessReviewer: tt.fields.accessReviewer,
				watches:        w,
				licenseChecker: tt.fields.licenseChecker,
				recorder:       record.NewFakeRecorder(10),
			}
			got, err := r.Reconcile(tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileRemoteCa.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileRemoteCa.Reconcile() = %v, want %v", got, tt.want)
			}
			// Check that expected secrets are here
			for _, expectedSecret := range tt.expectedSecrets {
				var actualSecret corev1.Secret
				assert.NoError(t, r.Client.Get(types.NamespacedName{Namespace: expectedSecret.Namespace, Name: expectedSecret.Name}, &actualSecret))
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
				err := r.Client.Get(types.NamespacedName{Namespace: unexpectedSecret.Namespace, Name: unexpectedSecret.Name}, &actualSecret)
				assert.True(t, apierrors.IsNotFound(err))
			}
		})
	}
}
