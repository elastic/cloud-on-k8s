// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_listAffectedLicenses(t *testing.T) {
	trueVal := true

	type args struct {
		initialObjects []client.Object
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
				initialObjects: []client.Object{
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo-cluster",
							Namespace: "default",
							SelfLink:  "/apis/elasticsearch.k8s.elastic.co/",
						},
					},
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar-cluster",
							Namespace: "default",
							SelfLink:  "/apis/elasticsearch.k8s.elastic.co/",
						},
					},
				},
			},
			want: []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "bar-cluster",
					},
				},
				{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "foo-cluster",
					},
				},
			},
			wantErr: false,
		},
		{
			name:          "list error",
			args:          args{},
			injectedError: errors.New("listing failed"),
			wantErr:       trueVal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.args.initialObjects...)
			if tt.injectedError != nil {
				client = k8s.NewFailingClient(tt.injectedError)
			}

			got, err := reconcileRequestsForAllClusters(client, logr.Discard())
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileRequestsForAllClusters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reconcileRequestsForAllClusters() = %v, want %v", got, tt.want)
			}
		})
	}
}
