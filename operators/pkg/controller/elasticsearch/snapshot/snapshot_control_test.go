// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package snapshot

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	esClient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	validSnapshotCredentials = `{
                      "type": "service_account",
                      "project_id": "your-project-id",
                      "private_key_id": "...",
                      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
                      "client_email": "service-account-for-your-repository@your-project-id.iam.gserviceaccount.com",
                      "client_id": "...",
                      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
                      "token_uri": "https://accounts.google.com/o/oauth2/token",
                      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
                      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/your-bucket@your-project-id.iam.gserviceaccount.com"
                    }`
)

func registerScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := v1alpha1.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}
	return sc
}

func TestReconcileStack_ReconcileSnapshotterCronJob(t *testing.T) {
	testName := types.NamespacedName{Namespace: "test-namespace", Name: "test-es-name"}
	cronName := types.NamespacedName{Namespace: testName.Namespace, Name: CronJobName(testName)}
	esSample := v1alpha1.ElasticsearchCluster{
		ObjectMeta: k8s.ToObjectMeta(testName),
	}
	type args struct {
		es             v1alpha1.ElasticsearchCluster
		user           esClient.User
		initialObjects []runtime.Object
	}

	tests := []struct {
		name            string
		args            args
		wantErr         bool
		clientAssertion func(c k8s.Client)
	}{
		{
			name:    "no snapshot config no creation",
			args:    args{esSample, esClient.User{}, []runtime.Object{}},
			wantErr: false,
			clientAssertion: func(c k8s.Client) {
				assert.True(t, errors.IsNotFound(c.Get(cronName, &batchv1beta1.CronJob{})))

			},
		},
		{
			name: "no snapshot config but cronjob exists delete job",
			args: args{
				esSample,
				esClient.User{},
				[]runtime.Object{&batchv1beta1.CronJob{ObjectMeta: k8s.ToObjectMeta(cronName)}},
			},
			wantErr: false,
			clientAssertion: func(c k8s.Client) {
				assert.True(t, errors.IsNotFound(c.Get(cronName, &batchv1beta1.CronJob{})))
			},
		},
		{
			name: "snapshot config exists create job",
			args: args{
				v1alpha1.ElasticsearchCluster{
					ObjectMeta: k8s.ToObjectMeta(testName),
					Spec: v1alpha1.ElasticsearchSpec{
						SnapshotRepository: &v1alpha1.SnapshotRepository{
							Type: v1alpha1.SnapshotRepositoryTypeGCS,
						},
					},
				},
				esClient.User{},
				[]runtime.Object{},
			},
			wantErr: false,
			clientAssertion: func(c k8s.Client) {
				assert.NoError(t, c.Get(cronName, &batchv1beta1.CronJob{}))
			},
		},
	}

	sc := registerScheme(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := k8s.WrapClient(fake.NewFakeClient(tt.args.initialObjects...))
			if err := ReconcileSnapshotterCronJob(fakeClient, sc, tt.args.es, tt.args.user, "operator-image"); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileElasticsearch.ReconcileSnapshotterCronJob() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.clientAssertion(fakeClient)
		})
	}
}

func TestReconcileElasticsearch_ReconcileSnapshotCredentials(t *testing.T) {
	owner := v1alpha1.ElasticsearchCluster{ObjectMeta: metav1.ObjectMeta{
		Name:      "my-cluster",
		Namespace: "baz",
	}}

	type args struct {
		repoConfig     *v1alpha1.SnapshotRepository
		initialObjects []runtime.Object
	}
	tests := []struct {
		name    string
		args    args
		want    corev1.Secret
		wantErr bool
	}{
		{
			name: "no config does not blow up",
			args: args{repoConfig: nil},
			want: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      keystore.ManagedSecretName(owner.Name),
					Namespace: "baz",
				},
				Data: map[string][]byte{},
			},
			wantErr: false,
		},
		{
			name:    "invalid credentials leads to error",
			args:    args{repoConfig: &v1alpha1.SnapshotRepository{}},
			wantErr: true,
		},
		{
			name: "valid config succeeds",
			args: args{
				repoConfig: &v1alpha1.SnapshotRepository{
					Type: v1alpha1.SnapshotRepositoryTypeGCS,
					Settings: v1alpha1.SnapshotRepositorySettings{
						BucketName: "foo",
						Credentials: corev1.SecretReference{
							Name:      "bar",
							Namespace: "baz",
						},
					},
				},
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: "baz",
						},
						Data: map[string][]byte{
							"foo.json": []byte(validSnapshotCredentials),
						},
					},
					&owner,
				},
			},
			want: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      keystore.ManagedSecretName(owner.Name),
					Namespace: "baz",
				},
				Data: map[string][]byte{
					"gcs.client.elastic-internal.credentials_file": []byte(validSnapshotCredentials),
				},
			},
			wantErr: false,
		},
	}

	sc := registerScheme(t)
	watched := watches.NewDynamicWatches()
	assert.NoError(t, watched.InjectScheme(sc))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileSnapshotCredentials(
				k8s.WrapClient(fake.NewFakeClientWithScheme(sc, tt.args.initialObjects...)),
				sc, owner, tt.args.repoConfig, watched,
			)

			if err != nil {
				if !tt.wantErr {
					t.Errorf("ReconcileElasticsearch.ReconcileSnapshotCredentials() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			assert.NoError(t, controllerutil.SetControllerReference(&owner, &tt.want, sc)) // to facilitate comparison
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileElasticsearch.ReconcileSnapshotCredentials() = %v, want %v", got, tt.want)
			}
		})
	}
}
