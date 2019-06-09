// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
)

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_seedServiceFinalizer(t *testing.T) {
	esName := "foo"
	esNamespace := "bar"
	rc := &v1alpha1.RemoteCluster{
		Spec: v1alpha1.RemoteClusterSpec{
			Remote: v1alpha1.RemoteClusterRef{
				K8sLocalRef: commonv1alpha1.ObjectSelector{
					Name:      esName,
					Namespace: esNamespace,
				},
			},
		},
	}

	// remote cluster whose remote cluster has no namespace defined
	rcNoRemoteClusterNamespace := rc.DeepCopy()
	rcNoRemoteClusterNamespace.Namespace = esNamespace
	rcNoRemoteClusterNamespace.Spec.Remote.K8sLocalRef.Namespace = ""

	rcSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remoteClusterSeedServiceName(esName),
			Namespace: esNamespace,
		},
	}

	otherSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remoteClusterSeedServiceName(esName) + "-other",
			Namespace: esNamespace,
		},
	}

	type args struct {
		c             k8s.Client
		remoteCluster v1alpha1.RemoteCluster
	}
	tests := []struct {
		name string
		args args
		want func(t *testing.T, c k8s.Client, f finalizer.Finalizer)
	}{
		{
			name: "should delete the related remote cluster seed service",
			args: args{
				c:             k8s.WrapClient(fake.NewFakeClient(rc, rcSvc, otherSvc)),
				remoteCluster: *rc,
			},
			want: func(t *testing.T, c k8s.Client, f finalizer.Finalizer) {
				require.NoError(t, f.Execute())

				// the service should be deleted
				var svc v1.Service
				err := c.Get(k8s.ExtractNamespacedName(rcSvc), &svc)
				assert.Error(t, err)
				assert.True(t, errors.IsNotFound(err))

				// the other service should not be deleted
				err = c.Get(k8s.ExtractNamespacedName(otherSvc), &svc)
				assert.NoError(t, err)
			},
		},
		{
			name: "should default the namespace of the remote cluster to the namespace of the remote cluster resource",
			args: args{
				c:             k8s.WrapClient(fake.NewFakeClient(rc, rcSvc, otherSvc)),
				remoteCluster: *rc,
			},
			want: func(t *testing.T, c k8s.Client, f finalizer.Finalizer) {
				require.NoError(t, f.Execute())

				// the service should be deleted
				var svc v1.Service
				err := c.Get(k8s.ExtractNamespacedName(rcSvc), &svc)
				assert.Error(t, err)
				assert.True(t, errors.IsNotFound(err))

				// the other service should not be deleted
				err = c.Get(k8s.ExtractNamespacedName(otherSvc), &svc)
				assert.NoError(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := seedServiceFinalizer(tt.args.c, tt.args.remoteCluster)
			tt.want(t, tt.args.c, got)
		})
	}
}
