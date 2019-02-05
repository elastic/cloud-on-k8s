package watches

import (
	"testing"

	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/label"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const testHandlerKey = "pod-expectations"

var testCluster = types.NamespacedName{
	Namespace: "namespace",
	Name:      "cluster",
}

func TestExpectationsWatch_Key(t *testing.T) {
	w := NewExpectationsWatch(testHandlerKey, nil, label.ClusterFromResourceLabels)
	require.Equal(t, testHandlerKey, w.Key())
}

func createPodMetaObject(t *testing.T, name string) metav1.Object {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testCluster.Namespace,
			Labels: map[string]string{
				label.ClusterNameLabelName: testCluster.Name,
			},
		},
	}
	asMetaObj, err := meta.Accessor(pod1)
	require.NoError(t, err)
	return asMetaObj
}

func TestExpectationsWatch_Create(t *testing.T) {
	expectations := reconciler.NewExpectations()
	w := NewExpectationsWatch(testHandlerKey, expectations, label.ClusterFromResourceLabels)

	tests := []struct {
		name              string
		events            func()
		expectedFulfilled bool
	}{
		{
			name:              "initially fulfilled",
			events:            func() {},
			expectedFulfilled: true,
		},
		{
			name: "expect 2 creations",
			events: func() {
				expectations.ExpectCreation(testCluster)
				expectations.ExpectCreation(testCluster)
			},
			expectedFulfilled: false,
		},
		{
			name: "observe 1 creation",
			events: func() {
				w.Create(event.CreateEvent{
					Meta: createPodMetaObject(t, "pod1"),
				}, nil)
			},
			expectedFulfilled: false,
		},
		{
			name: "observe the 2nd creation",
			events: func() {
				w.Create(event.CreateEvent{
					Meta: createPodMetaObject(t, "pod2"),
				}, nil)
			},
			expectedFulfilled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.events()
			require.Equal(t, tt.expectedFulfilled, expectations.Fulfilled(testCluster))
		})
	}
}

func TestExpectationsWatch_Delete(t *testing.T) {
	expectations := reconciler.NewExpectations()
	w := NewExpectationsWatch(testHandlerKey, expectations, label.ClusterFromResourceLabels)

	tests := []struct {
		name              string
		events            func()
		expectedFulfilled bool
	}{
		{
			name:              "initially fulfilled",
			events:            func() {},
			expectedFulfilled: true,
		},
		{
			name: "expect 2 deletions",
			events: func() {
				expectations.ExpectDeletion(testCluster)
				expectations.ExpectDeletion(testCluster)
			},
			expectedFulfilled: false,
		},
		{
			name: "observe 1 deletion",
			events: func() {
				w.Delete(event.DeleteEvent{
					Meta: createPodMetaObject(t, "pod1"),
				}, nil)
			},
			expectedFulfilled: false,
		},
		{
			name: "observe the 2nd deletions",
			events: func() {
				w.Delete(event.DeleteEvent{
					Meta: createPodMetaObject(t, "pod2"),
				}, nil)
			},
			expectedFulfilled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.events()
			require.Equal(t, tt.expectedFulfilled, expectations.Fulfilled(testCluster))
		})
	}
}
