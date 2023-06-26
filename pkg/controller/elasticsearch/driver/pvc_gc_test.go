// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func buildSsetWithClaims(name string, replicas int32, claims ...string) appsv1.StatefulSet {
	s := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      name,
			Labels: map[string]string{
				label.ClusterNameLabelName: "es",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
	}
	for _, claim := range claims {
		s.Spec.VolumeClaimTemplates = append(s.Spec.VolumeClaimTemplates, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: claim,
			},
		})
	}
	return s
}

func buildPVC(name string, ownerRefs ...string) corev1.PersistentVolumeClaim {
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
		pvc.OwnerReferences = append(pvc.OwnerReferences, metav1.OwnerReference{Name: ref})
	}
	return pvc
}

func buildPVCPtr(name string, ownerRefs ...string) *corev1.PersistentVolumeClaim {
	pvc := buildPVC(name, ownerRefs...)
	return &pvc
}

func Test_pvcsToRemove(t *testing.T) {
	type args struct {
		pvcs                 []corev1.PersistentVolumeClaim
		actualStatefulSets   sset.StatefulSetList
		expectedStatefulSets sset.StatefulSetList
	}
	tests := []struct {
		name string
		args args
		want []corev1.PersistentVolumeClaim
	}{
		{
			name: "no pvc in the cache: nothing to remove",
			args: args{
				pvcs:                 nil,
				actualStatefulSets:   sset.StatefulSetList{buildSsetWithClaims("sset1", 3, "claim1")},
				expectedStatefulSets: sset.StatefulSetList{buildSsetWithClaims("sset1", 4, "claim1", "claim2")},
			},
			want: nil,
		},
		{
			name: "expected pvcs are there: nothing to remove",
			args: args{
				pvcs:                 []corev1.PersistentVolumeClaim{buildPVC("claim1-sset1-0"), buildPVC("claim1-sset1-1")},
				actualStatefulSets:   sset.StatefulSetList{buildSsetWithClaims("sset1", 2, "claim1")},
				expectedStatefulSets: sset.StatefulSetList{buildSsetWithClaims("sset1", 2, "claim1")},
			},
			want: nil,
		},
		{
			name: "don't remove PVCs of expected pods that may be created concurrently, or existing pods that are not deleted yet",
			args: args{
				pvcs:                 []corev1.PersistentVolumeClaim{buildPVC("claim1-sset1-0"), buildPVC("claim1-sset1-1"), buildPVC("claim1-sset2-0")},
				actualStatefulSets:   sset.StatefulSetList{buildSsetWithClaims("sset1", 2, "claim1")},
				expectedStatefulSets: sset.StatefulSetList{buildSsetWithClaims("sset2", 2, "claim1")},
			},
			want: nil,
		},
		{
			name: "remove PVCs that don't match actual nor expected ssets",
			args: args{
				pvcs:                 []corev1.PersistentVolumeClaim{buildPVC("claim1-sset1-0"), buildPVC("claim1-sset3-0"), buildPVC("claim1-sset3-1")},
				actualStatefulSets:   sset.StatefulSetList{buildSsetWithClaims("sset1", 2, "claim1")},
				expectedStatefulSets: sset.StatefulSetList{buildSsetWithClaims("sset2", 2, "claim1")},
			},
			want: []corev1.PersistentVolumeClaim{buildPVC("claim1-sset3-0"), buildPVC("claim1-sset3-1")},
		},
		{
			name: "remove PVCs corresponding to claims that don't exist anymore in sset specs",
			args: args{
				pvcs:                 []corev1.PersistentVolumeClaim{buildPVC("oldclaim-sset1-0"), buildPVC("newclaim-sset1-0")},
				actualStatefulSets:   sset.StatefulSetList{buildSsetWithClaims("sset1", 1, "newclaim")},
				expectedStatefulSets: sset.StatefulSetList{buildSsetWithClaims("sset2", 1, "newclaim")},
			},
			want: []corev1.PersistentVolumeClaim{buildPVC("oldclaim-sset1-0")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pvcsToRemove(tt.args.pvcs, tt.args.actualStatefulSets, tt.args.expectedStatefulSets); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("pvcsToRemove() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGarbageCollectPVCs(t *testing.T) {
	existingPVCS := []client.Object{
		buildPVCPtr("claim1-sset1-0"),   // should not be removed
		buildPVCPtr("claim1-oldsset-0"), // should be removed
	}
	type args struct {
		k8sClient            k8s.Client
		es                   esv1.Elasticsearch
		actualStatefulSets   sset.StatefulSetList
		expectedStatefulSets sset.StatefulSetList
	}
	tests := []struct {
		name     string
		args     args
		wantErr  bool
		wantPVCs int
	}{
		{
			name: "Remove on default scale down policy",
			args: args{
				k8sClient:            k8s.NewFakeClient(existingPVCS...),
				es:                   esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}},
				actualStatefulSets:   sset.StatefulSetList{buildSsetWithClaims("sset1", 1, "claim1")},
				expectedStatefulSets: sset.StatefulSetList{buildSsetWithClaims("sset2", 1, "claim1")},
			},
			wantErr:  false,
			wantPVCs: 1,
		},
		{
			name: "Remove on any other explicitly set policy",
			args: args{
				k8sClient: k8s.NewFakeClient(existingPVCS...),
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
					Spec: esv1.ElasticsearchSpec{
						VolumeClaimDeletePolicy: esv1.DeleteOnScaledownOnlyPolicy,
					}},
				actualStatefulSets:   sset.StatefulSetList{buildSsetWithClaims("sset1", 1, "claim1")},
				expectedStatefulSets: sset.StatefulSetList{buildSsetWithClaims("sset2", 1, "claim1")},
			},
			wantErr:  false,
			wantPVCs: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := GarbageCollectPVCs(context.Background(), tt.args.k8sClient, tt.args.es, tt.args.actualStatefulSets, tt.args.expectedStatefulSets); (err != nil) != tt.wantErr {
				t.Errorf("GarbageCollectPVCs() error = %v, wantErr %v", err, tt.wantErr)
			}
			var retrievedPVCs corev1.PersistentVolumeClaimList
			require.NoError(t, tt.args.k8sClient.List(context.Background(), &retrievedPVCs))
			require.Equal(t, tt.wantPVCs, len(retrievedPVCs.Items))
		})
	}
}
