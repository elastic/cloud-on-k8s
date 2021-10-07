// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// UpdateControllerVersion tests that updates annotation if there is an older version
func TestAnnotationUpdated(t *testing.T) {
	kibana := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "kibana",
			Annotations: map[string]string{
				ControllerVersionAnnotation: "1.6.0",
			},
		},
	}
	obj := kibana.DeepCopy()
	client := k8s.NewFakeClient(obj)
	err := UpdateControllerVersion(context.Background(), client, obj, "1.7.0")
	require.NoError(t, err)
	require.Equal(t, obj.GetAnnotations()[ControllerVersionAnnotation], "1.7.0")
}

// UpdateControllerVersion tests that creates an annotation even if there are no current annotations
func TestAnnotationCreated(t *testing.T) {
	kibana := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "kibana",
		},
	}

	obj := kibana.DeepCopy()
	client := k8s.NewFakeClient(obj)
	err := UpdateControllerVersion(context.Background(), client, obj, "1.7.0")
	require.NoError(t, err)
	actualKibana := &kbv1.Kibana{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}, actualKibana)
	require.NoError(t, err)
	require.NotNil(t, actualKibana.GetAnnotations())
	assert.Equal(t, actualKibana.GetAnnotations()[ControllerVersionAnnotation], "1.7.0")
}

// TestMissingAnnotationOldVersion tests that if an object is missing annotations that has already been reconciled by
// a previous operator version, we add an annotation indicating an old controller version
func TestMissingAnnotationOldVersion(t *testing.T) {
	trueVar := true
	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			UID:       "bc99a102-1385-4c53-be43-23d53e46bb5d",
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "svc",
			Labels: map[string]string{
				"elasticsearch.k8s.elastic.co/cluster-name": "es",
				"common.k8s.elastic.co/type":                "elasticsearch",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "elasticsearch.k8s.elastic.co/v1alpha1",
					Kind:               "Elasticsearch",
					Name:               "es",
					UID:                "bc99a102-1385-4c53-be43-23d53e46bb5d",
					Controller:         &trueVar,
					BlockOwnerDeletion: &trueVar,
				},
			},
		},
	}
	client := k8s.NewFakeClient(es, svc)
	selector := getElasticsearchSelector(es)
	err := ReconcileCompatibility(context.Background(), client, es, selector, MinCompatibleControllerVersion)
	require.NoError(t, err)

	// check old version annotation was added
	require.NotNil(t, es.Annotations)
	assert.Equal(t, UnknownControllerVersion, es.Annotations[ControllerVersionAnnotation])
}

// TestMissingAnnotationNewObject tests that we add an annotation for new objects
func TestMissingAnnotationNewObject(t *testing.T) {
	es := &esv1.Elasticsearch{
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

	client := k8s.NewFakeClient(es, svc)
	selector := getElasticsearchSelector(es)
	err := ReconcileCompatibility(context.Background(), client, es, selector, MinCompatibleControllerVersion)
	require.NoError(t, err)

	// check version annotation was added
	require.NotNil(t, es.Annotations)
	assert.Equal(t, MinCompatibleControllerVersion, es.Annotations[ControllerVersionAnnotation])
}

func TestSameAnnotation(t *testing.T) {
	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				ControllerVersionAnnotation: MinCompatibleControllerVersion,
			},
		},
	}
	client := k8s.NewFakeClient(es)
	selector := getElasticsearchSelector(es)
	err := ReconcileCompatibility(context.Background(), client, es, selector, MinCompatibleControllerVersion)
	require.NoError(t, err)
	assert.Equal(t, MinCompatibleControllerVersion, es.Annotations[ControllerVersionAnnotation])
}

func TestIncompatibleAnnotation(t *testing.T) {
	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				ControllerVersionAnnotation: "0.8.0-FOOBAR",
			},
		},
	}
	client := k8s.NewFakeClient(es)
	selector := getElasticsearchSelector(es)
	err := ReconcileCompatibility(context.Background(), client, es, selector, MinCompatibleControllerVersion)
	require.NoError(t, err)
	// check we did not update the annotation
	assert.Equal(t, "0.8.0-FOOBAR", es.Annotations[ControllerVersionAnnotation])
}

func TestNewerAnnotation(t *testing.T) {
	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				ControllerVersionAnnotation: "2.0.0",
			},
		},
	}
	client := k8s.NewFakeClient(es)
	selector := getElasticsearchSelector(es)
	err := ReconcileCompatibility(context.Background(), client, es, selector, MinCompatibleControllerVersion)
	assert.NoError(t, err)
}

// TestInvalidAnnotation tests that an invalid version cannot be used for the annotation
func TestInvalidAnnotation(t *testing.T) {
	kibana := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "kibana",
		},
	}

	obj := kibana.DeepCopy()
	client := k8s.NewFakeClient(obj)
	err := UpdateControllerVersion(context.Background(), client, obj, "errorverison")
	require.Error(t, err)
	err = UpdateControllerVersion(context.Background(), client, obj, "1.7.0")
	require.NoError(t, err)
}

func getElasticsearchSelector(es *esv1.Elasticsearch) map[string]string {
	return map[string]string{label.ClusterNameLabelName: es.Name}
}
