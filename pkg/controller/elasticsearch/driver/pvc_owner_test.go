// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/go-test/deep"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	updated := func(pvc corev1.PersistentVolumeClaim) corev1.PersistentVolumeClaim {
		pvc.ResourceVersion = "1000" // fake client starts at 999
		return pvc
	}

	tests := []struct {
		name    string
		args    args
		want    []corev1.PersistentVolumeClaim
		wantErr bool
	}{
		{
			name: "remove references on retain",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0", "es")),
				es: esFixture(esv1.RetainPolicy),
			},
			want:    []corev1.PersistentVolumeClaim{updated(pvcFixture("es-data-0"))},
			wantErr: false,
		},
		{
			name: "add references for remove policies",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0")),
				es: esFixture(esv1.RemoveOnClusterDeletionPolicy),
			},
			want:    []corev1.PersistentVolumeClaim{updated(pvcFixture("es-data-0", "es"))},
			wantErr: false,
		},
		{
			name: "keep references set by other controllers when retaining",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0", "es", "some-other-ref")),
				es: esFixture(esv1.RetainPolicy),
			},
			want:    []corev1.PersistentVolumeClaim{updated(pvcFixture("es-data-0", "some-other-ref"))},
			wantErr: false,
		},
		{
			name: "keep references set by other controllers when removing",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0", "some-other-ref")),
				es: esFixture(esv1.RemoveOnScaleDownPolicy),
			},
			want:    []corev1.PersistentVolumeClaim{updated(pvcFixture("es-data-0", "some-other-ref", "es"))},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := reconcilePVCOwnerRefs(tt.args.c, tt.args.es); (err != nil) != tt.wantErr {
				t.Errorf("reconcilePVCOwnerRefs() error = %v, wantErr %v", err, tt.wantErr)
			}
			var pvcs corev1.PersistentVolumeClaimList
			if err := tt.args.c.List(context.Background(), &pvcs); err != nil {
				t.Errorf("reconcilePVCOwnerRefs(), failed to list pvcs: %v", err)
			}
			if diff := deep.Equal(pvcs.Items, tt.want); diff != nil {
				t.Errorf("reconcilePVCOwnerRefs(), diff %v", diff)
			}
		})
	}
}
