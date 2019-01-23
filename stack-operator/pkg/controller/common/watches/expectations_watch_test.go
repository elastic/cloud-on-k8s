package watches

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"

	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/label"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sctl "k8s.io/kubernetes/pkg/controller"
)

const (
	testNamespace   = "test-namespace"
	testClusterName = "cluster-name"
	testHandlerKey  = "pod-expectations"
)

func TestExpectationsWatch_Key(t *testing.T) {
	w := NewExpectationsWatch(testHandlerKey, nil)
	require.Equal(t, testHandlerKey, w.Key())
}
func TestExpectationsWatch_Create(t *testing.T) {
	expectations := k8sctl.NewUIDTrackingControllerExpectations(k8sctl.NewControllerExpectations())
	w := NewExpectationsWatch(testHandlerKey, expectations)

	// simulate we expect 2 creations
	err := expectations.ExpectCreations(testNamespace+"/"+testClusterName, 2)
	require.NoError(t, err)
	checkExpectations(t, expectations, 2, 0)

	// simulate creation event for pod1
	w.Create(event.CreateEvent{
		Meta: createPodMetaObject(t, "pod1"),
	}, nil)
	// we should have lowered our expectations
	checkExpectations(t, expectations, 1, 0)

	// simulate creation event for pod2
	w.Create(event.CreateEvent{
		Meta: createPodMetaObject(t, "pod2"),
	}, nil)
	// we should have lowered our expectations
	checkExpectations(t, expectations, 0, 0)

	// simulate creation event for unexpected pod3
	w.Create(event.CreateEvent{
		Meta: createPodMetaObject(t, "pod3"),
	}, nil)
	// we should keep a positive expectation value (should normally not happen)
	checkExpectations(t, expectations, 0, 0)
	// TODO: doesn't work, which is not great. Will fix this when moving on to our own implementation.
}
func TestExpectationsWatch_Delete(t *testing.T) {
	expectations := k8sctl.NewUIDTrackingControllerExpectations(k8sctl.NewControllerExpectations())
	w := NewExpectationsWatch(testHandlerKey, expectations)

	// simulate we expect 2 deletions
	pod1 := createPodMetaObject(t, "pod1")
	pod2 := createPodMetaObject(t, "pod2")
	err := expectations.ExpectDeletions(testNamespace+"/"+testClusterName, []string{"pod1", "pod2"})
	require.NoError(t, err)
	checkExpectations(t, expectations, 0, 2)

	// simulate deletion event for pod1
	w.Delete(event.DeleteEvent{
		Meta: pod1,
	}, nil)
	// we should have lowered our expectations
	checkExpectations(t, expectations, 0, 1)

	// simulate deletion event for pod2
	w.Delete(event.DeleteEvent{
		Meta: pod2,
	}, nil)
	// we should have lowered our expectations
	checkExpectations(t, expectations, 0, 0)

	// simulate deletion event for unexpected pod3
	w.Delete(event.DeleteEvent{
		Meta: createPodMetaObject(t, "pod3"),
	}, nil)
	// we should keep a positive expectation value (should normally not happen)
	checkExpectations(t, expectations, 0, 0)
}

func checkExpectations(t *testing.T, expectations *k8sctl.UIDTrackingControllerExpectations, expectedCreations int64, expectedDeletions int64) {
	exp, exists, err := expectations.GetExpectations(testNamespace + "/" + testClusterName)
	require.NoError(t, err)
	require.True(t, exists)
	creations, deletions := exp.GetExpectations()
	require.Equal(t, expectedCreations, creations)
	require.Equal(t, expectedDeletions, deletions)
}

func createPodMetaObject(t *testing.T, name string) metav1.Object {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				label.ClusterNameLabelName: testClusterName,
			},
		},
	}
	asMetaObj, err := meta.Accessor(pod1)
	require.NoError(t, err)
	return asMetaObj
}
