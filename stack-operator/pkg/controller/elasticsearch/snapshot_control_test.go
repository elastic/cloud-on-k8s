package elasticsearch

import (
	"context"
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshots"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	esClient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func asObjectMeta(n types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      n.Name,
		Namespace: n.Namespace,
	}
}

func TestReconcileStack_ReconcileSnapshotterCronJob(t *testing.T) {
	testName := types.NamespacedName{Namespace: "test-namespace", Name: "test-es-name"}
	cronName := types.NamespacedName{Namespace: testName.Namespace, Name: snapshots.CronJobName(testName)}
	esSample := v1alpha1.Elasticsearch{
		ObjectMeta: asObjectMeta(testName),
	}
	type args struct {
		es             v1alpha1.Elasticsearch
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
				[]runtime.Object{&batchv1beta1.CronJob{ObjectMeta: asObjectMeta(cronName)}},
			},
			wantErr: false,
			clientAssertion: func(c client.Client) {
				assert.True(t, errors.IsNotFound(c.Get(context.TODO(), cronName, &batchv1beta1.CronJob{})))
			},
		},
		{
			name: "snapshot config exists create job",
			args: args{
				v1alpha1.Elasticsearch{
					ObjectMeta: asObjectMeta(testName),
					Spec: v1alpha1.ElasticsearchSpec{
						SnapshotRepository: v1alpha1.SnapshotRepository{
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

	scheme, err := v1alpha1.SchemeBuilder.Build()
	if err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ReconcileElasticsearch{
				Client: fake.NewFakeClient(tt.args.initialObjects...),
				scheme: scheme,
			}
			if err := r.ReconcileSnapshotterCronJob(tt.args.es, tt.args.user); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileElasticsearch.ReconcileSnapshotterCronJob() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.clientAssertion(r)
		})
	}
}
