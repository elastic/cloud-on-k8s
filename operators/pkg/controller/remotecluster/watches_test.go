// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"testing"

	v1alpha12 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_allRemoteClustersWithMatchingSeedServiceMapper(t *testing.T) {
	esName := "foo"
	esNamespace := "bar"

	rc := &v1alpha1.RemoteCluster{
		ObjectMeta: v12.ObjectMeta{
			Namespace: esNamespace,
			Name:      "my-rc",
		},
		Spec: v1alpha1.RemoteClusterSpec{
			Remote: v1alpha1.RemoteClusterRef{
				K8sLocalRef: v1alpha12.ObjectSelector{
					Name:      esName,
					Namespace: esNamespace,
				},
			},
		},
	}

	// another RC that uses the same remote as rc
	rc2 := rc.DeepCopy()
	rc2.ObjectMeta.Name = "my-rc-2"

	otherRc := &v1alpha1.RemoteCluster{
		ObjectMeta: v12.ObjectMeta{
			Namespace: esNamespace,
			Name:      "my-rc-3",
		},
		Spec: v1alpha1.RemoteClusterSpec{
			Remote: v1alpha1.RemoteClusterRef{
				K8sLocalRef: v1alpha12.ObjectSelector{
					Name:      "not-" + esName,
					Namespace: esNamespace,
				},
			},
		},
	}

	svc := v1.Service{
		ObjectMeta: v12.ObjectMeta{
			Namespace: esNamespace,
			Name:      remoteClusterSeedServiceName(esName),
			Labels: map[string]string{
				RemoteClusterSeedServiceForLabelName: esName,
			},
		},
	}

	type args struct {
		c k8s.Client
	}
	tests := []struct {
		name string
		args args
		want func(t *testing.T, c k8s.Client, mapper handler.Mapper)
	}{
		{
			name: "should return requests for only relevant remote clusters",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(rc, rc2, otherRc)),
			},
			want: func(t *testing.T, c k8s.Client, mapper handler.Mapper) {
				requests := mapper.Map(handler.MapObject{Meta: &svc, Object: &svc})

				assert.Len(t, requests, 2)
				assert.Contains(t, requests, reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(rc)})
				assert.Contains(t, requests, reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(rc2)})

				// otherRc should not show up in the reconciled requests.
				assert.NotContains(t, requests, reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(otherRc)})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allRemoteClustersWithMatchingSeedServiceMapper(tt.args.c)
			tt.want(t, tt.args.c, got)
		})
	}
}
