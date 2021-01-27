// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package finalizer

import (
	"context"
	"testing"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRemoveAll(t *testing.T) {
	sampleObject := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
			Finalizers: []string{
				"finalizer.elasticsearch.k8s.elastic.co/secure-settings-secret",
				"finalizer.foo.bar.com/secure-settings-secret",
				"finalizer.elasticsearch.k8s.elastic.co/http-certificates-secret",
				"finalizer.elasticsearch.k8s.elastic.co/observer",
				"finalizer.association.apmserver.k8s.elastic.co/external-user",
				"finalizer.apmserver.k8s.elastic.co/secure-settings-secret",
				"finalizer.kibana.k8s.elastic.co/secure-settings-secret",
				"finalizer.association.kibana.k8s.elastic.co/elasticsearch",
				"finalizer.foo.bar.co/elasticsearch",
			},
		},
	}
	type args struct {
		c   k8s.Client
		obj client.Object
	}
	tests := []struct {
		name           string
		args           args
		wantFinalizers []string
		wantErr        bool
	}{
		{
			name: "No Finalizers",
			args: args{
				c: k8s.NewFakeClient(&kbv1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
					},
				}),
				obj: &kbv1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
					},
				},
			},
			wantFinalizers: []string{},
		},
		{
			name: "Only remove Elastic Finalizers",
			args: args{
				c:   k8s.NewFakeClient(sampleObject),
				obj: sampleObject,
			},
			wantFinalizers: []string{
				"finalizer.foo.bar.com/secure-settings-secret",
				"finalizer.foo.bar.co/elasticsearch",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := RemoveAll(tt.args.c, tt.args.obj); (err != nil) != tt.wantErr {
				t.Errorf("RemoveAll() error = %v, wantErr %v", err, tt.wantErr)
			}
			savedObject := &kbv1.Kibana{}
			err := tt.args.c.Get(context.Background(), types.NamespacedName{
				Namespace: "bar",
				Name:      "foo",
			}, savedObject)
			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.wantFinalizers, savedObject.Finalizers)
		})
	}
}
