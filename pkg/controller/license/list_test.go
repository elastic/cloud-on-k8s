// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type failingClient struct {
	k8s.Client
	Error error
}

func (f *failingClient) List(list runtime.Object, opts ...client.ListOption) error {
	return f.Error
}

var _ k8s.Client = &failingClient{}

func Test_listAffectedLicenses(t *testing.T) {
	s := scheme.Scheme
	if err := v1beta1.SchemeBuilder.AddToScheme(s); err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}

	true := true

	type args struct {
		license        string
		initialObjects []runtime.Object
	}
	tests := []struct {
		name          string
		args          args
		injectedError error
		want          []reconcile.Request
		wantErr       bool
	}{
		{
			name: "happy path",
			args: args{
				license: "enterprise-license",
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo-cluster",
							Namespace: "default",
							SelfLink:  "/apis/elasticsearch.k8s.elastic.co/",
							Labels: map[string]string{
								license.LicenseLabelName: "enterprise-license",
							},
						},
					},
				},
			},
			want: []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "foo-cluster",
				},
			}},
			wantErr: false,
		},
		{
			name: "list error",
			args: args{
				license: "bar",
			},
			injectedError: errors.New("listing failed"),
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(tt.args.initialObjects...))
			if tt.injectedError != nil {
				client = &failingClient{Client: client, Error: tt.injectedError}
			}

			got, err := listAffectedLicenses(client, tt.args.license)
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
