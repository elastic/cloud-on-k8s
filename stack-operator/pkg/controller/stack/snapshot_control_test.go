package stack

import (
	"context"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch/snapshots"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	esClient "github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch/client"
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
	testName := types.NamespacedName{Namespace: "test-namespace", Name: "test-stack-name"}
	cronName := types.NamespacedName{Namespace: testName.Namespace, Name: snapshots.CronJobName(testName)}
	stackSample := deploymentsv1alpha1.Stack{
		ObjectMeta: asObjectMeta(testName),
	}
	type args struct {
		stack deploymentsv1alpha1.Stack
		user  esClient.User
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
			args:    args{stackSample, esClient.User{}, []runtime.Object{}},
			wantErr: false,
			clientAssertion: func(c client.Client) {
				cron := &batchv1beta1.CronJob{}
				err := c.Get(context.TODO(), cronName, cron)
				assert.True(t, errors.IsNotFound(err))

			},
		},
		{
			name: "no snapshot config but cronjob exists delete job",
			args: args{
				stackSample,
				esClient.User{},
				[]runtime.Object{&batchv1beta1.CronJob{ObjectMeta:asObjectMeta(cronName)}},
			},
			wantErr: false,
			clientAssertion: func(c client.Client) {
				cron := &batchv1beta1.CronJob{}
				err := c.Get(context.TODO(), cronName, cron)
				assert.True(t, errors.IsNotFound(err))
			},
		},
		{
			name: "snapshot config exists create job",
			args: args{
				deploymentsv1alpha1.Stack{
					ObjectMeta: asObjectMeta(testName),
					Spec: deploymentsv1alpha1.StackSpec{
						Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
							SnapshotRepository: deploymentsv1alpha1.SnapshotRepository{
								Type: deploymentsv1alpha1.SnapshotRepositoryTypeGCS,
							},
						},
					},
				},
				esClient.User{},
				[]runtime.Object{},
			},
			wantErr: false,
			clientAssertion: func(c client.Client) {
				cron := &batchv1beta1.CronJob{}
				assert.NoError(t, c.Get(context.TODO(), cronName, cron))
			},
		},
	}

	scheme, err := deploymentsv1alpha1.SchemeBuilder.Build()
	if err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ReconcileStack{
				Client: fake.NewFakeClient(tt.args.initialObjects...),
				scheme: scheme,
			}
			if err := r.ReconcileSnapshotterCronJob(tt.args.stack, tt.args.user); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileStack.ReconcileSnapshotterCronJob() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.clientAssertion(r)
		})
	}
}
