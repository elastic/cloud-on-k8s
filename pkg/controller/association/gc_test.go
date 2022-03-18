// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	ApmAssociationLabelName         = "apmassociation.k8s.elastic.co/name"
	ApmAssociationLabelNamespace    = "apmassociation.k8s.elastic.co/namespace"
	KibanaAssociationLabelName      = "kibanaassociation.k8s.elastic.co/name"
	KibanaAssociationLabelNamespace = "kibanaassociation.k8s.elastic.co/namespace"
)

func newUserSecret(
	namespace, name,
	associationNamespaceLabel, associationNameLabel,
	associationNamespaceValue, associationNameValue string,
) runtime.Object {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				associationNameLabel:      associationNameValue,
				associationNamespaceLabel: associationNamespaceValue,
				common.TypeLabelName:      esuser.AssociatedUserType,
			},
		},
	}
}

func newServiceAccountSecret(
	namespace, name,
	associationNamespaceLabel, associationNameLabel,
	associationNamespaceValue, associationNameValue string,
) runtime.Object {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				associationNameLabel:      associationNameValue,
				associationNamespaceLabel: associationNamespaceValue,
				common.TypeLabelName:      esuser.ServiceAccountTokenType,
			},
		},
	}
}

func TestUsersGarbageCollector_GC(t *testing.T) {
	client := k8s.NewFakeClient(
		// Create 5 secrets, 3 actually used and 2 orphaned
		newUserSecret("es", "ns1-kb-orphaned-xxxx-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns1", "orphaned-kibana"),
		newUserSecret("es", "ns1-kb-kibana1-w2fz-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns1", "kibana1"),
		newUserSecret("es", "ns1-kb-kibana2-fy8i-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns2", "kibana2"),
		newUserSecret("es", "ns1-kb-orphaned-xxxx-apm-user", ApmAssociationLabelNamespace, ApmAssociationLabelName, "ns1", "orphaned-apm"),
		newUserSecret("es", "ns1-kb-apm1-yrfa-apm-user", ApmAssociationLabelNamespace, ApmAssociationLabelName, "ns1", "apm1"),
		&kbv1.Kibana{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kibana1",
				Namespace: "ns1",
			},
		},
		&kbv1.Kibana{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kibana2",
				Namespace: "ns2",
			},
		},
		&apmv1.ApmServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "apm1",
				Namespace: "ns1",
			},
		},
	)

	ugc := &UsersGarbageCollector{
		client:            client,
		managedNamespaces: []string{AllNamespaces},
	}

	// register some resources
	ugc.For(&apmv1.ApmServerList{}, ApmAssociationLabelNamespace, ApmAssociationLabelName)
	ugc.For(&kbv1.KibanaList{}, KibanaAssociationLabelNamespace, KibanaAssociationLabelName)

	err := ugc.DoGarbageCollection()
	if err != nil {
		t.Errorf("UsersGarbageCollector.DoGarbageCollection() error = %v", err)
		return
	}

	// kibana1, kibana2 and apm1 user Secret must still be present
	s := &corev1.Secret{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-kibana1-w2fz-kibana-user",
	}, s)
	assert.NoError(t, err)
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-kibana2-fy8i-kibana-user",
	}, s)
	assert.NoError(t, err)
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-apm1-yrfa-apm-user",
	}, s)
	assert.NoError(t, err)

	// Orphaned secret must have been deleted
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-orphaned-xxxx-kibana-user",
	}, s)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))

	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-orphaned-xxxx-apm-user",
	}, s)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestServiceAccountsGarbageCollector_GC(t *testing.T) {
	client := k8s.NewFakeClient(
		// Create 5 secrets, 3 actually used and 2 orphaned
		newServiceAccountSecret("es", "ns1-kb-orphaned-xxxx-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns1", "orphaned-kibana"),
		newServiceAccountSecret("es", "ns1-kb-kibana1-w2fz-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns1", "kibana1"),
		newServiceAccountSecret("es", "ns1-kb-kibana2-fy8i-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns2", "kibana2"),
		newUserSecret("es", "ns1-kb-orphaned-xxxx-apm-user", ApmAssociationLabelNamespace, ApmAssociationLabelName, "ns1", "orphaned-apm"),
		newUserSecret("es", "ns1-kb-apm1-yrfa-apm-user", ApmAssociationLabelNamespace, ApmAssociationLabelName, "ns1", "apm1"),
		&kbv1.Kibana{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kibana1",
				Namespace: "ns1",
			},
		},
		&kbv1.Kibana{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kibana2",
				Namespace: "ns2",
			},
		},
		&apmv1.ApmServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "apm1",
				Namespace: "ns1",
			},
		},
	)

	ugc := &UsersGarbageCollector{
		client:            client,
		managedNamespaces: []string{AllNamespaces},
	}

	// register some resources
	ugc.For(&apmv1.ApmServerList{}, ApmAssociationLabelNamespace, ApmAssociationLabelName)
	ugc.For(&kbv1.KibanaList{}, KibanaAssociationLabelNamespace, KibanaAssociationLabelName)

	err := ugc.DoGarbageCollection()
	if err != nil {
		t.Errorf("UsersGarbageCollector.DoGarbageCollection() error = %v", err)
		return
	}

	// kibana1, kibana2 and apm1 user Secret must still be present
	s := &corev1.Secret{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-kibana1-w2fz-kibana-user",
	}, s)
	assert.NoError(t, err)
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-kibana2-fy8i-kibana-user",
	}, s)
	assert.NoError(t, err)
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-apm1-yrfa-apm-user",
	}, s)
	assert.NoError(t, err)

	// Orphaned secret must have been deleted
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-orphaned-xxxx-kibana-user",
	}, s)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))

	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-orphaned-xxxx-apm-user",
	}, s)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}
