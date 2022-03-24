// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_reconcilePVCOwnerRefs(t *testing.T) {
	type args struct {
		c  k8s.Client
		es esv1.Elasticsearch
	}

	esFixture := func(policy esv1.VolumeClaimDeletePolicy) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: "ns"},
			Spec:       esv1.ElasticsearchSpec{VolumeClaimDeletePolicy: policy},
		}
	}

	pvcFixture := func(name string, ownerRefs ...string) corev1.PersistentVolumeClaim {
		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      name,
				Labels: map[string]string{
					label.ClusterNameLabelName: "es",
				},
			},
		}
		for _, ref := range ownerRefs {
			pvc.OwnerReferences = append(pvc.OwnerReferences, metav1.OwnerReference{
				Name:       ref,
				Kind:       "Elasticsearch",
				APIVersion: "elasticsearch.k8s.elastic.co/v1",
			})
		}
		return pvc
	}

	pvcFixturePtr := func(name string, ownerRefs ...string) *corev1.PersistentVolumeClaim {
		pvc := pvcFixture(name, ownerRefs...)
		return &pvc
	}

	tests := []struct {
		name       string
		args       args
		want       []corev1.PersistentVolumeClaim
		wantUpdate bool
		wantErr    bool
	}{
		{
			name: "remove references on DeleteOnScaledownOnlyPolicy",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0", "es")),
				es: esFixture(esv1.DeleteOnScaledownOnlyPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "avoid unnecessary updates when reference already removed",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0")),
				es: esFixture(esv1.DeleteOnScaledownOnlyPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0")},
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name: "add references for DeleteOnScaledownAndClusterDeletionPolicy",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0")),
				es: esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0", "es")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "avoid unnecessary updates when reference already added",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0", "es")),
				es: esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0", "es")},
			wantErr:    false,
			wantUpdate: false,
		},
		{
			name: "keep references set by other controllers when removing owner ref",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0", "es", "some-other-ref")),
				es: esFixture(esv1.DeleteOnScaledownOnlyPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0", "some-other-ref")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "keep references set by other controllers when setting owner ref",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0", "some-other-ref")),
				es: esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0", "some-other-ref", "es")},
			wantErr:    false,
			wantUpdate: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trackedClient := trackingK8sClient{Client: tt.args.c}
			if err := reconcilePVCOwnerRefs(&trackedClient, tt.args.es); (err != nil) != tt.wantErr {
				t.Errorf("reconcilePVCOwnerRefs() error = %v, wantErr %v", err, tt.wantErr)
			}
			var pvcs corev1.PersistentVolumeClaimList
			if err := tt.args.c.List(context.Background(), &pvcs); err != nil {
				t.Errorf("reconcilePVCOwnerRefs(), failed to list pvcs: %v", err)
			}
			require.Equal(t, len(tt.want), len(pvcs.Items), "unexpected number of pvcs")
			for i := 0; i < len(tt.want); i++ {
				comparison.AssertEqual(t, &pvcs.Items[i], &tt.want[i])
			}
			require.Equal(t, tt.wantUpdate, trackedClient.updateCalled, "unexpected client interaction: update called")
		})
	}
}

type trackingK8sClient struct {
	k8s.Client
	updateCalled bool
}

func (t *trackingK8sClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	t.updateCalled = true
	return t.Client.Update(ctx, obj, opts...)
}
