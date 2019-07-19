// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"testing"

	kibanav1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	assoctype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/associations/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
)

// Test UpdateControllerVersion updates annotation if there is an older version
func TestAnnotationUpdated(t *testing.T) {
	kibana := kibanav1alpha1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "kibana",
			Annotations: map[string]string{
				ControllerVersionAnnotation: "oldversion",
			},
		},
	}
	obj := kibana.DeepCopy()
	sc := setupScheme(t)
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, obj))
	err := UpdateControllerVersion(client, obj, "newversion")
	require.NoError(t, err)
	require.Equal(t, obj.GetAnnotations()[ControllerVersionAnnotation], "newversion")
}

// Test UpdateControllerVersion creates an annotation even if there are no current annotations
func TestAnnotationCreated(t *testing.T) {
	kibana := kibanav1alpha1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "kibana",
		},
	}

	obj := kibana.DeepCopy()
	sc := setupScheme(t)
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, obj))
	err := UpdateControllerVersion(client, obj, "newversion")
	require.NoError(t, err)
	actualKibana := &kibanav1alpha1.Kibana{}
	err = client.Get(types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}, actualKibana)
	require.NoError(t, err)
	require.NotNil(t, actualKibana.GetAnnotations())
	assert.Equal(t, actualKibana.GetAnnotations()[ControllerVersionAnnotation], "newversion")
}

// TestMissingAnnotationOldVersion tests that we skip reconciling an object missing annotations that has already been reconciled by
// a previous operator version, and add an annotation indicating an old controller version
func TestMissingAnnotationOldVersion(t *testing.T) {

	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "svc",
			Labels: map[string]string{
				label.ClusterNameLabelName: "es",
			},
		},
	}
	sc := setupScheme(t)
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, es, svc))

	compat, err := ReconcileCompatibility(client, es, "0.9.0-SNAPSHOT")
	require.NoError(t, err)
	assert.False(t, compat)

	// check old version annotation was added
	require.NotNil(t, es.Annotations)
	assert.Equal(t, "0.8.0-UNKNOWN", es.Annotations[ControllerVersionAnnotation])
}

// TestMissingAnnotationNewObject tests that we add an annotation for new objects
func TestMissingAnnotationNewObject(t *testing.T) {
	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	// TODO this is currently broken due to an upstream bug in the fake client. when we upgrade controller runtime
	// to a version that contains this PR we can uncomment this and add the service to the client

	// add existing svc that is not part of cluster to make sure we have label selectors correct
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

	sc := setupScheme(t)
	// client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, es, svc))
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, es))
	compat, err := ReconcileCompatibility(client, es, "0.9.0-SNAPSHOT")
	require.NoError(t, err)
	assert.True(t, compat)

	// check version annotation was added
	require.NotNil(t, es.Annotations)
	assert.Equal(t, "0.9.0-SNAPSHOT", es.Annotations[ControllerVersionAnnotation])
}

//
func TestSameAnnotation(t *testing.T) {
	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				ControllerVersionAnnotation: "0.9.0-SNAPSHOT",
			},
		},
	}
	sc := setupScheme(t)
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, es))
	compat, err := ReconcileCompatibility(client, es, "0.9.0-SNAPSHOT")
	require.NoError(t, err)
	assert.True(t, compat)
	assert.Equal(t, "0.9.0-SNAPSHOT", es.Annotations[ControllerVersionAnnotation])
}

func TestIncompatibleAnnotation(t *testing.T) {
	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				ControllerVersionAnnotation: "0.8.0-FOOBAR",
			},
		},
	}
	sc := setupScheme(t)
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, es))
	compat, err := ReconcileCompatibility(client, es, "0.9.0-SNAPSHOT")
	require.NoError(t, err)
	assert.False(t, compat)
	// check we did not update the annotation
	assert.Equal(t, "0.8.0-FOOBAR", es.Annotations[ControllerVersionAnnotation])
}

func TestNewerAnnotation(t *testing.T) {
	es := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				ControllerVersionAnnotation: "2.0.0",
			},
		},
	}
	sc := setupScheme(t)
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, es))
	compat, err := ReconcileCompatibility(client, es, "0.9.0-SNAPSHOT")
	assert.NoError(t, err)
	assert.True(t, compat)
}

// setupScheme creates a scheme to use for our fake clients so they know about our custom resources
func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	err := assoctype.SchemeBuilder.AddToScheme(sc)
	require.NoError(t, err)
	err = apmtype.SchemeBuilder.AddToScheme(sc)
	require.NoError(t, err)
	err = estype.SchemeBuilder.AddToScheme(sc)
	require.NoError(t, err)
	err = kibanav1alpha1.SchemeBuilder.AddToScheme(sc)
	require.NoError(t, err)
	return sc
}
