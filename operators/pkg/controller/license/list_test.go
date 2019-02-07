// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.
package license

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_listAffectedLicenses(t *testing.T) {
	s := scheme.Scheme
	if err := v1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}

	clusterRef, err := reference.GetReference(s, &v1alpha1.ElasticsearchCluster{
		ObjectMeta: v1.ObjectMeta{
			Name:         "foo",
			GenerateName: "",
			Namespace:    "default",
			SelfLink:     "/apis/elasticsearch.k8s.elastic.co/",
			UID:          "not-a-real-uid",
		},
	})
	assert.NoError(t, err)
	true := true

	type args struct {
		license        types.NamespacedName
		initialObjects []runtime.Object
	}
	tests := []struct {
		name    string
		args    args
		want    []reconcile.Request
		wantErr bool
	}{
		{
			name: "happy path",
			args: args{
				license: types.NamespacedName{
					Namespace: "default",
					Name:      "enterprise-license",
				},
				initialObjects: []runtime.Object{
					&v1alpha1.ClusterLicense{
						ObjectMeta: v1.ObjectMeta{
							Name:      "foo-license",
							Namespace: "default",
							SelfLink:  "/apis/elasticsearch.k8s.elastic.co/",
							OwnerReferences: []v1.OwnerReference{
								{
									APIVersion:         clusterRef.APIVersion,
									Kind:               clusterRef.Kind,
									Name:               clusterRef.Name,
									UID:                clusterRef.UID,
									Controller:         &true,
									BlockOwnerDeletion: &true,
								},
							},
						},
					},
				},
			},
			want: []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "foo",
				},
			}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewFakeClient(tt.args.initialObjects...)

			got, err := listAffectedLicenses(client, s, tt.args.license)
			if (err != nil) != tt.wantErr {
				t.Errorf("listAffectedLicenses() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("listAffectedLicenses() = %v, want %v", got, tt.want)
			}
		})
	}
}
