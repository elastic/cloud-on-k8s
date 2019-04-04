package annotation

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMarkPodAsUpdated(t *testing.T) {
	key := types.NamespacedName{
		Namespace: "ns1",
		Name:      "foo",
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "foo",
		},
	}
	type args struct {
		c   k8s.Client
		pod corev1.Pod
	}
	tests := []struct {
		name string
		args args
	}{
		{
			args: args{
				c:   k8s.WrapClient(fake.NewFakeClient(pod.DeepCopy())),
				pod: pod,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			MarkPodAsUpdated(tt.args.c, tt.args.pod)
			// Ensure the label is present
			actualPod := &corev1.Pod{}
			assert.NoError(t, tt.args.c.Get(key, actualPod))
			assert.NotNil(t, actualPod.Annotations)
			previousValue, ok := actualPod.Annotations[UpdateAnnotation]
			assert.True(t, ok)
			// Trigger a new update
			MarkPodAsUpdated(tt.args.c, tt.args.pod)
			// Ensure the label is updated
			actualPod = &corev1.Pod{}
			assert.NoError(t, tt.args.c.Get(key, actualPod))
			assert.NotNil(t, actualPod.Annotations)
			newValue, ok := actualPod.Annotations[UpdateAnnotation]
			assert.True(t, newValue > previousValue)
		})
	}
}
