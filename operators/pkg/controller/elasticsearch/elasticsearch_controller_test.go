package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/operators/pkg/about"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestMissingAnnotationOldVersion tests that we skip reconciling an object missing annotations that has already been reconciled by
// a previous operator version, and add an annotation indicating an old controller version
func TestMissingAnnotationOldVersion(t *testing.T) {
	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	pod := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "svc",
			Labels: map[string]string{
				label.ClusterNameLabelName: "es",
			},
		},
	}
	r := makeFakeReconciler(t, pod, es)
	compat, err := r.reconcileCompatibility(es)
	require.NoError(t, err)
	assert.False(t, compat)

	// check old version annotation was added
	require.NotNil(t, es.Annotations)
	assert.Equal(t, "0.8.0-UNKNOWN", es.Annotations[annotation.ControllerVersionAnnotation])
}

// TestMissingAnnotationNewObject tests that we add an annotation for new objects
func TestMissingAnnotationNewObject(t *testing.T) {
	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	// add existing pod that is not part of cluster to make sure we have label selectors correct
	// TODO this is currently broken due to an upstream bug in the fake client. when we upgrade controller runtime
	// to a version that contains this PR we can uncomment this and add the service to the client
	// https://github.com/kubernetes-sigs/controller-runtime/pull/311
	// svc := &corev1.Service{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Namespace: "ns",
	// 		Name:      "svc",
	// 		Labels: map[string]string{
	// 			label.ClusterNameLabelName: "literallyanything",
	// 		},
	// 	},
	// }
	// r := makeFakeReconciler(t, es, svc)
	r := makeFakeReconciler(t, es)
	compat, err := r.reconcileCompatibility(es)
	require.NoError(t, err)
	assert.True(t, compat)

	// check version annotation was added
	require.NotNil(t, es.Annotations)
	assert.Equal(t, r.OperatorInfo.BuildInfo.Version, es.Annotations[annotation.ControllerVersionAnnotation])
}

//
func TestSameAnnotation(t *testing.T) {
	r := makeFakeReconciler(t)
	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				// this needs to be the same as the reconciler
				annotation.ControllerVersionAnnotation: "0.9.0-SNAPSHOT",
			},
		},
	}
	compat, err := r.reconcileCompatibility(es)
	require.NoError(t, err)
	assert.True(t, compat)
}

func TestIncompatibleAnnotation(t *testing.T) {
	r := makeFakeReconciler(t)
	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				annotation.ControllerVersionAnnotation: "0.8.0-FOOBAR",
			},
		},
	}
	compat, err := r.reconcileCompatibility(es)
	require.NoError(t, err)
	assert.False(t, compat)
	// check we did not update the annotation
	assert.Equal(t, "0.8.0-FOOBAR", es.Annotations[annotation.ControllerVersionAnnotation])
}

func TestNewerAnnotation(t *testing.T) {
	r := makeFakeReconciler(t)
	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				annotation.ControllerVersionAnnotation: "2.0.0",
			},
		},
	}
	compat, err := r.reconcileCompatibility(es)
	assert.NoError(t, err)
	assert.True(t, compat)
}

// makeFakeReconciler generates a fake reconciler
func makeFakeReconciler(t *testing.T, objs ...runtime.Object) ReconcileElasticsearch {
	sc := scheme.Scheme
	err := v1alpha1.AddToScheme(sc)
	require.NoError(t, err)
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(scheme.Scheme, objs...))
	params := operator.Parameters{
		OperatorInfo: about.OperatorInfo{
			BuildInfo: about.BuildInfo{
				Version: "0.9.0-SNAPSHOT",
			},
		},
	}
	r := ReconcileElasticsearch{
		Client:     client,
		scheme:     sc,
		Parameters: params,
	}
	return r
}
