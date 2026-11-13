// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esav1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoscaling/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
)

type webhookInformerRecordingCache struct {
	cache.Cache
	objects map[reflect.Type]int
	err     error
}

func (c *webhookInformerRecordingCache) GetInformer(_ context.Context, obj client.Object, _ ...cache.InformerGetOption) (cache.Informer, error) {
	c.objects[reflect.TypeOf(obj)]++
	return nil, c.err
}

func TestRegisterWebhookInformers(t *testing.T) {
	informerErr := errors.New("informer error")
	tests := []struct {
		name                 string
		validateStorageClass bool
		namespaceSelector    labels.Selector
		informerErr          error
		wantObjects          map[reflect.Type]int
		wantErrContains      string
	}{
		{
			name: "base webhook dependencies",
			wantObjects: map[reflect.Type]int{
				reflect.TypeFor[*corev1.Secret]():                       1,
				reflect.TypeFor[*corev1.ConfigMap]():                    1,
				reflect.TypeFor[*corev1.Pod]():                          1,
				reflect.TypeFor[*appsv1.StatefulSet]():                  1,
				reflect.TypeFor[*esv1.Elasticsearch]():                  1,
				reflect.TypeFor[*esav1alpha1.ElasticsearchAutoscaler](): 1,
			},
		},
		{
			name:                 "storage class validation",
			validateStorageClass: true,
			wantObjects: map[reflect.Type]int{
				reflect.TypeFor[*corev1.Secret]():                       1,
				reflect.TypeFor[*corev1.ConfigMap]():                    1,
				reflect.TypeFor[*corev1.Pod]():                          1,
				reflect.TypeFor[*appsv1.StatefulSet]():                  1,
				reflect.TypeFor[*esv1.Elasticsearch]():                  1,
				reflect.TypeFor[*esav1alpha1.ElasticsearchAutoscaler](): 1,
				reflect.TypeFor[*storagev1.StorageClass]():              1,
			},
		},
		{
			name:              "dynamic namespace selection",
			namespaceSelector: labels.Everything(),
			wantObjects: map[reflect.Type]int{
				reflect.TypeFor[*corev1.Secret]():                       1,
				reflect.TypeFor[*corev1.ConfigMap]():                    1,
				reflect.TypeFor[*corev1.Pod]():                          1,
				reflect.TypeFor[*appsv1.StatefulSet]():                  1,
				reflect.TypeFor[*esv1.Elasticsearch]():                  1,
				reflect.TypeFor[*esav1alpha1.ElasticsearchAutoscaler](): 1,
				reflect.TypeFor[*corev1.Namespace]():                    1,
			},
		},
		{
			name:                 "all webhook dependencies",
			validateStorageClass: true,
			namespaceSelector:    labels.Everything(),
			wantObjects: map[reflect.Type]int{
				reflect.TypeFor[*corev1.Secret]():                       1,
				reflect.TypeFor[*corev1.ConfigMap]():                    1,
				reflect.TypeFor[*corev1.Pod]():                          1,
				reflect.TypeFor[*appsv1.StatefulSet]():                  1,
				reflect.TypeFor[*esv1.Elasticsearch]():                  1,
				reflect.TypeFor[*esav1alpha1.ElasticsearchAutoscaler](): 1,
				reflect.TypeFor[*storagev1.StorageClass]():              1,
				reflect.TypeFor[*corev1.Namespace]():                    1,
			},
		},
		{
			name:            "propagates informer error",
			informerErr:     informerErr,
			wantObjects:     map[reflect.Type]int{reflect.TypeFor[*corev1.Secret](): 1},
			wantErrContains: "pre-register webhook informer for *v1.Secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordingCache := &webhookInformerRecordingCache{
				objects: make(map[reflect.Type]int),
				err:     tt.informerErr,
			}

			err := registerWebhookInformers(
				t.Context(),
				recordingCache,
				tt.validateStorageClass,
				nsmatch.NewNamespaceMatcher(tt.namespaceSelector, "elastic-system"),
			)
			if tt.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, informerErr)
				require.ErrorContains(t, err, tt.wantErrContains)
			}
			require.Equal(t, tt.wantObjects, recordingCache.objects)
		})
	}
}
