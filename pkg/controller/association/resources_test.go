// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	esuser "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

func Test_deleteOrphanedResources(t *testing.T) {
	userInEsNamespace := "default-kibana-foo-kibana-user" // in the es namespace
	userInKibanaNamespace := "kibana-foo-kibana-es-user"

	kibanaESAssociationName := "kibana-es"
	esFixture := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-foo",
			Namespace: "default",
		},
	}
	kibanaFixtureObjectMeta := metav1.ObjectMeta{
		Name:      "kibana-foo",
		Namespace: "default",
	}
	kibanaFixture := kbv1.Kibana{
		ObjectMeta: kibanaFixtureObjectMeta,
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ObjectSelector{
				Name:      esFixture.Name,
				Namespace: esFixture.Namespace,
			},
		},
	}
	associationLabels := map[string]string{
		"kibanaassociation.k8s.elastic.co/name":      kibanaFixtureObjectMeta.Name,
		"kibanaassociation.k8s.elastic.co/namespace": kibanaFixtureObjectMeta.Namespace,
		"kibana.k8s.elastic.co/name":                 esFixture.Name,
		"kibana.k8s.elastic.co/namespace":            esFixture.Namespace,
	}

	userSecretLabels := maps.Merge(map[string]string{commonv1.TypeLabelName: esuser.AssociatedUserType}, associationLabels)

	assertExpectObjectsExist := func(t *testing.T, c k8s.Client) {
		t.Helper()
		// user secret should be in ES namespace
		assert.NoError(t, c.Get(context.Background(), types.NamespacedName{
			Namespace: esFixture.Namespace,
			Name:      userInEsNamespace,
		}, &corev1.Secret{}))
		// user secret should be in Kibana namespace
		assert.NoError(t, c.Get(context.Background(), types.NamespacedName{
			Namespace: kibanaFixture.Namespace,
			Name:      userInKibanaNamespace,
		}, &corev1.Secret{}))
		// ca secret should be in Kibana namespace
		assert.NoError(t, c.Get(context.Background(), types.NamespacedName{
			Namespace: kibanaFixture.Namespace,
			Name:      CACertSecretName(kibanaFixture.EsAssociation(), kibanaESAssociationName),
		}, &corev1.Secret{}))
	}

	info := AssociationInfo{
		Labels:                                func(associated types.NamespacedName) map[string]string { return associationLabels },
		AssociationResourceNameLabelName:      "kibana.k8s.elastic.co/name",
		AssociationResourceNamespaceLabelName: "kibana.k8s.elastic.co/namespace",
	}

	tests := []struct {
		name           string
		kibana         kbv1.Kibana
		es             esv1.Elasticsearch
		initialObjects []client.Object
		postCondition  func(c k8s.Client)
		wantErr        bool
	}{
		{
			name: "Do not delete if there's no namespace in the ref",
			kibana: kbv1.Kibana{
				ObjectMeta: kibanaFixtureObjectMeta,
				Spec: kbv1.KibanaSpec{
					ElasticsearchRef: commonv1.ObjectSelector{ // ElasticsearchRef without a namespace
						Name: esFixture.Name,
						// Namespace: esFixture.Namespace, No namespace on purpose
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userInKibanaNamespace,
						Namespace: kibanaFixture.Namespace,
						Labels:    associationLabels,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      CACertSecretName(kibanaFixture.EsAssociation(), kibanaESAssociationName),
						Namespace: kibanaFixture.Namespace,
						Labels:    associationLabels,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userInEsNamespace,
						Namespace: "default",
						Labels:    userSecretLabels,
					},
				},
			},
			postCondition: func(c k8s.Client) {
				assertExpectObjectsExist(t, c) // all objects must be exist
			},
			wantErr: false,
		},
		{
			name: "ES namespace has changed ",
			kibana: kbv1.Kibana{
				ObjectMeta: kibanaFixtureObjectMeta,
				Spec: kbv1.KibanaSpec{
					ElasticsearchRef: commonv1.ObjectSelector{
						Name:      esFixture.Name,
						Namespace: "ns2", // Kibana does not reference the default namespace anymore
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userInKibanaNamespace,
						Namespace: kibanaFixture.Namespace,
						Labels:    associationLabels,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      CACertSecretName(kibanaFixture.EsAssociation(), kibanaESAssociationName),
						Namespace: kibanaFixture.Namespace,
						Labels:    associationLabels,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userInEsNamespace,
						Namespace: "default", // but we still have a user secret in default
						Labels:    userSecretLabels,
					},
				},
			},
			postCondition: func(c k8s.Client) {
				// user CR should be in ES namespace
				assert.Error(t, c.Get(context.Background(), types.NamespacedName{
					Namespace: esFixture.Namespace,
					Name:      userInEsNamespace,
				}, &corev1.Secret{}),
					"Previous user secret should have been removed")
			},
			wantErr: false,
		},
		{
			name:    "nothing to delete",
			kibana:  kbv1.Kibana{},
			wantErr: false,
		},
		{
			name:   "only valid objects",
			kibana: kibanaFixture,
			es:     esFixture,
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userInKibanaNamespace,
						Namespace: kibanaFixture.Namespace,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      CACertSecretName(kibanaFixture.EsAssociation(), kibanaESAssociationName),
						Namespace: kibanaFixture.Namespace,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userInEsNamespace,
						Namespace: kibanaFixture.Namespace,
					},
				},
			},
			postCondition: func(c k8s.Client) {
				assertExpectObjectsExist(t, c)
			},
			wantErr: false,
		},
		{
			name: "No more es ref in Kibana, orphan user & CA for previous es ref exist",
			kibana: kbv1.Kibana{
				ObjectMeta: kibanaFixtureObjectMeta,
			},
			es: esFixture,
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userInKibanaNamespace,
						Namespace: kibanaFixture.Namespace,
						Labels:    associationLabels,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userInEsNamespace,
						Namespace: kibanaFixture.Namespace,
						Labels:    userSecretLabels,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      CACertSecretName(kibanaFixture.EsAssociation(), kibanaESAssociationName),
						Namespace: kibanaFixture.Namespace,
						Labels:    associationLabels,
					},
				},
			},
			postCondition: func(c k8s.Client) {
				// This works even without labels because mock client currently ignores labels
				assert.Error(t, c.Get(context.Background(), types.NamespacedName{
					Namespace: kibanaFixture.Namespace,
					Name:      userInEsNamespace,
				}, &corev1.Secret{}))
				assert.Error(t, c.Get(context.Background(), types.NamespacedName{
					Namespace: kibanaFixture.Spec.ElasticsearchRef.Namespace,
					Name:      userInKibanaNamespace,
				}, &corev1.Secret{}))
				assert.Error(t, c.Get(context.Background(), types.NamespacedName{
					Namespace: kibanaFixture.Spec.ElasticsearchRef.Namespace,
					Name:      CACertSecretName(kibanaFixture.EsAssociation(), kibanaESAssociationName),
				}, &corev1.Secret{}))
			},
			wantErr: false,
		},
		{
			name: "No more es ref in Kibana, orphan user for previous es ref in a different namespace still exists",
			kibana: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kibana-foo",
					Namespace: "ns2",
				},
			},
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-foo",
					Namespace: "ns1",
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kibana-foo-kibana-user",
						Namespace: "ns2",
						Labels:    associationLabels,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ns2-kibana-foo-kibana-user",
						Namespace: "ns1",
						Labels:    userSecretLabels,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kibana-foo-kb-es-ca",
						Namespace: "ns2",
						Labels:    associationLabels,
					},
				},
			},
			postCondition: func(c k8s.Client) {
				assert.Error(t, c.Get(context.Background(), types.NamespacedName{
					Namespace: "ns2",
					Name:      "kibana-foo-kibana-user",
				}, &corev1.Secret{}))
				assert.Error(t, c.Get(context.Background(), types.NamespacedName{
					Namespace: "ns1",
					Name:      "ns2-kibana-foo-kibana-user",
				}, &corev1.Secret{}))
				assert.Error(t, c.Get(context.Background(), types.NamespacedName{
					Namespace: "ns2",
					Name:      "kibana-foo-kb-es-ca",
				}, &corev1.Secret{}))
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.initialObjects...)
			if err := deleteOrphanedResources(context.Background(), c, info, tt.kibana.EsAssociation().AssociationRef().WithDefaultNamespace(tt.kibana.Namespace).NamespacedName(), tt.kibana.GetAssociations()); (err != nil) != tt.wantErr {
				t.Errorf("deleteOrphanedResources() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.postCondition != nil {
				tt.postCondition(c)
			}
		})
	}
}
