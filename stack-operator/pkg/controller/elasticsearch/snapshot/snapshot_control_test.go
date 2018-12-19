package snapshot

import (
	"context"
	"reflect"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	esClient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	scheme, err := v1alpha1.SchemeBuilder.Build()
	if err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}
	return scheme
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
		clientAssertion func(c client.Client)
	}{
		{
			name:    "no snapshot config no creation",
			args:    args{esSample, esClient.User{}, []runtime.Object{}},
			wantErr: false,
			clientAssertion: func(c client.Client) {
				assert.True(t, errors.IsNotFound(c.Get(context.TODO(), cronName, &batchv1beta1.CronJob{})))

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
			clientAssertion: func(c client.Client) {
				assert.True(t, errors.IsNotFound(c.Get(context.TODO(), cronName, &batchv1beta1.CronJob{})))
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
			clientAssertion: func(c client.Client) {
				assert.NoError(t, c.Get(context.TODO(), cronName, &batchv1beta1.CronJob{}))
			},
		},
	}

	scheme := registerScheme(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewFakeClient(tt.args.initialObjects...)
			if err := ReconcileSnapshotterCronJob(client, scheme, tt.args.es, tt.args.user); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileElasticsearch.ReconcileSnapshotterCronJob() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.clientAssertion(client)
		})
	}
}

func TestReconcileElasticsearch_ReconcileSnapshotCredentials(t *testing.T) {
	type args struct {
		repoConfig     *v1alpha1.SnapshotRepository
		initialObjects []runtime.Object
	}
	tests := []struct {
		name    string
		args    args
		want    keystore.Config
		wantErr bool
	}{
		{
			name:    "no config does not blow up",
			args:    args{repoConfig: nil},
			want:    keystore.Config{},
			wantErr: false,
		},
		{
			name:    "invalid credentials leads to error",
			args:    args{repoConfig: &v1alpha1.SnapshotRepository{}},
			want:    keystore.Config{},
			wantErr: true,
		},
		{
			name: "valid config succeeds",
			args: args{
				repoConfig: &v1alpha1.SnapshotRepository{
					Type: v1alpha1.SnapshotRepositoryTypeGCS,
					Settings: v1alpha1.SnapshotRepositorySettings{
						BucketName: "foo",
						Credentials: v1.SecretReference{
							Name:      "bar",
							Namespace: "baz",
						},
					},
				},
				initialObjects: []runtime.Object{&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bar",
						Namespace: "baz",
					},
					Data: map[string][]byte{
						"foo.json": []byte(validSnapshotCredentials),
					},
				}},
			},
			want: keystore.Config{
				KeystoreSecretRef: v1.SecretReference{
					Name:      "bar",
					Namespace: "baz",
				},
				KeystoreSettings: []keystore.Setting{
					keystore.Setting{
						Key:           "gcs.client.elastic-internal.credentials_file",
						ValueFilePath: "/keystore-secrets/foo.json",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileSnapshotCredentials(fake.NewFakeClient(tt.args.initialObjects...), tt.args.repoConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileElasticsearch.ReconcileSnapshotCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileElasticsearch.ReconcileSnapshotCredentials() = %v, want %v", got, tt.want)
			}
		})
	}
}
