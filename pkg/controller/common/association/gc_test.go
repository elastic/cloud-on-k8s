// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	kibanatype "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				associationNameLabel:      associationNameValue,
				associationNamespaceLabel: associationNamespaceValue,
				common.TypeLabelName:      user.UserType,
			},
		},
	}
}

func TestUsersGarbageCollector_GC(t *testing.T) {

	client := k8s.WrappedFakeClient(
		// Create 5 secrets, 3 actually used and 2 orphaned
		newUserSecret("es", "ns1-kb-orphaned-xxxx-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns1", "orphaned-kibana"),
		newUserSecret("es", "ns1-kb-kibana1-w2fz-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns1", "kibana1"),
		newUserSecret("es", "ns1-kb-kibana2-fy8i-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns2", "kibana2"),
		newUserSecret("es", "ns1-kb-orphaned-xxxx-apm-user", ApmAssociationLabelNamespace, ApmAssociationLabelName, "ns1", "orphaned-apm"),
		newUserSecret("es", "ns1-kb-apm1-yrfa-apm-user", ApmAssociationLabelNamespace, ApmAssociationLabelName, "ns1", "apm1"),
		&kibanatype.Kibana{
			ObjectMeta: v1.ObjectMeta{
				Name:      "kibana1",
				Namespace: "ns1",
			},
		},
		&kibanatype.Kibana{
			ObjectMeta: v1.ObjectMeta{
				Name:      "kibana2",
				Namespace: "ns2",
			},
		},
		&apmtype.ApmServer{
			ObjectMeta: v1.ObjectMeta{
				Name:      "apm1",
				Namespace: "ns1",
			},
		},
	)

	ugc := &UsersGarbageCollector{
		client: client,
		scheme: k8s.Scheme(),
	}

	// register some resources
	ugc.For(&apmtype.ApmServerList{}, ApmAssociationLabelNamespace, ApmAssociationLabelName)
	ugc.For(&kibanatype.KibanaList{}, KibanaAssociationLabelNamespace, KibanaAssociationLabelName)

	err := ugc.DoGarbageCollection()
	if err != nil {
		t.Errorf("UsersGarbageCollector.DoGarbageCollection() error = %v", err)
		return
	}

	// kibana1, kibana2 and apm1 user Secret must still be present
	s := &corev1.Secret{}
	err = client.Get(types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-kibana1-w2fz-kibana-user",
	}, s)
	assert.NoError(t, err)
	err = client.Get(types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-kibana2-fy8i-kibana-user",
	}, s)
	assert.NoError(t, err)
	err = client.Get(types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-apm1-yrfa-apm-user",
	}, s)
	assert.NoError(t, err)

	// Orphaned secret must have been deleted
	err = client.Get(types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-orphaned-xxxx-kibana-user",
	}, s)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))

	err = client.Get(types.NamespacedName{
		Namespace: "es",
		Name:      "ns1-kb-orphaned-xxxx-apm-user",
	}, s)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}
