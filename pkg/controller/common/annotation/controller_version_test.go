// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"testing"

	kibanav1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apmtype "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
)

// Test UpdateControllerVersion updates annotation if there is an older version
func TestAnnotationUpdated(t *testing.T) {
	kibana := kibanav1beta1.Kibana{
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
	kibana := kibanav1beta1.Kibana{
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
	actualKibana := &kibanav1beta1.Kibana{}
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

	es := &v1beta1.Elasticsearch{
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
	selector := getElasticsearchSelector(es)
	compat, err := ReconcileCompatibility(client, es, selector, MinCompatibleControllerVersion)
	require.NoError(t, err)
	assert.False(t, compat)

	// check old version annotation was added
	require.NotNil(t, es.Annotations)
	assert.Equal(t, UnknownControllerVersion, es.Annotations[ControllerVersionAnnotation])
}

// TestMissingAnnotationNewObject tests that we add an annotation for new objects
func TestMissingAnnotationNewObject(t *testing.T) {
	es := &v1beta1.Elasticsearch{
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
				label.ClusterNameLabelName: "literallyanything",
			},
		},
	}

	sc := setupScheme(t)
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, es, svc))
	selector := getElasticsearchSelector(es)
	compat, err := ReconcileCompatibility(client, es, selector, MinCompatibleControllerVersion)
	require.NoError(t, err)
	assert.True(t, compat)

	// check version annotation was added
	require.NotNil(t, es.Annotations)
	assert.Equal(t, MinCompatibleControllerVersion, es.Annotations[ControllerVersionAnnotation])
}

//
func TestSameAnnotation(t *testing.T) {
	es := &v1beta1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				ControllerVersionAnnotation: MinCompatibleControllerVersion,
			},
		},
	}
	sc := setupScheme(t)
	client := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, es))
	selector := getElasticsearchSelector(es)
	compat, err := ReconcileCompatibility(client, es, selector, MinCompatibleControllerVersion)
	require.NoError(t, err)
	assert.True(t, compat)
	assert.Equal(t, MinCompatibleControllerVersion, es.Annotations[ControllerVersionAnnotation])
}

func TestIncompatibleAnnotation(t *testing.T) {
	es := &v1beta1.Elasticsearch{
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
	selector := getElasticsearchSelector(es)
	compat, err := ReconcileCompatibility(client, es, selector, MinCompatibleControllerVersion)
	require.NoError(t, err)
	assert.False(t, compat)
	// check we did not update the annotation
	assert.Equal(t, "0.8.0-FOOBAR", es.Annotations[ControllerVersionAnnotation])
}

func TestNewerAnnotation(t *testing.T) {
	es := &v1beta1.Elasticsearch{
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
	selector := getElasticsearchSelector(es)
	compat, err := ReconcileCompatibility(client, es, selector, MinCompatibleControllerVersion)
	assert.NoError(t, err)
	assert.True(t, compat)
}

// setupScheme creates a scheme to use for our fake clients so they know about our custom resources
func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	err := apmtype.AddToScheme(sc)
	require.NoError(t, err)
	err = estype.SchemeBuilder.AddToScheme(sc)
	require.NoError(t, err)
	err = kibanav1beta1.SchemeBuilder.AddToScheme(sc)
	require.NoError(t, err)
	return sc
}

func getElasticsearchSelector(es *v1beta1.Elasticsearch) map[string]string {
	return map[string]string{label.ClusterNameLabelName: es.Name}
}
