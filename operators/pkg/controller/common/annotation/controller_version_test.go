// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"testing"

	kibanav1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	assoctype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/associations/v1alpha1"
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
	actualKibana := &kibanav1alpha1.Kibana{}
	err = client.Get(types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}, actualKibana)
	require.NoError(t, err)
	require.NotNil(t, actualKibana.GetAnnotations())
	assert.Equal(t, actualKibana.GetAnnotations()[ControllerVersionAnnotation], "newversion")
}

// setupScheme creates a scheme to use for our fake clients so they know about our custom resources
// TODO move this into one of the upper level common packages and make public, refactor out this code that's in a lot of our tests
func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := assoctype.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add Association types")
	}
	if err := apmtype.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add APM types")
	}
	if err := estype.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add ES types")
	}
	if err := kibanav1alpha1.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add Kibana types")
	}
	return sc
}
