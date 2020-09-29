// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"encoding/json"
	"fmt"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
)

var (
	sampleStorageClass = storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name: "sample-sc"}}

	sampleClaim = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-claim"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr(sampleStorageClass.Name),
			Resources: corev1.ResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}
	sampleClaim2 = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-claim-2"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr(sampleStorageClass.Name),
			Resources: corev1.ResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}

	sampleSset = appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"}}
)

func withVolumeExpansion(sc storagev1.StorageClass) *storagev1.StorageClass {
	sc.AllowVolumeExpansion = pointer.BoolPtr(true)
	return &sc
}

func withClaims(sset appsv1.StatefulSet, claims ...corev1.PersistentVolumeClaim) appsv1.StatefulSet {
	s := sset.DeepCopy()
	s.Spec.VolumeClaimTemplates = append(s.Spec.VolumeClaimTemplates, claims...)
	return *s
}

func withStorageReq(claim corev1.PersistentVolumeClaim, size string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(size)
	return *c
}

func Test_handleVolumeExpansion(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             pointer.Int32Ptr(3),
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim},
		},
	}
	resizedSset := *sset.DeepCopy()
	resizedSset.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("3Gi")
	pvcsWithSize := func(size ...string) []corev1.PersistentVolumeClaim {
		var pvcs []corev1.PersistentVolumeClaim
		for i, s := range size {
			pvcs = append(pvcs, withStorageReq(corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: fmt.Sprintf("sample-claim-sample-sset-%d", i)},
				Spec:       sampleClaim.Spec,
			}, s))
		}
		return pvcs
	}
	pvcPtrs := func(pvcs []corev1.PersistentVolumeClaim) []runtime.Object {
		var ptrs []runtime.Object
		for i := range pvcs {
			ptrs = append(ptrs, &pvcs[i])
		}
		return ptrs
	}

	type args struct {
		expectedSset appsv1.StatefulSet
		actualSset   appsv1.StatefulSet
	}
	tests := []struct {
		name         string
		args         args
		runtimeObjs  []runtime.Object
		expectedPVCs []corev1.PersistentVolumeClaim
		wantErr      bool
		wantRecreate bool
	}{
		{
			name: "no pvc to resize",
			args: args{
				expectedSset: sset,
				actualSset:   sset,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("1Gi", "1Gi", "1Gi"),
			wantRecreate: false,
		},
		{
			name: "all pvcs should be resized",
			args: args{
				expectedSset: resizedSset,
				actualSset:   sset,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),
			wantRecreate: true,
		},
		{
			name: "2 pvcs left to resize",
			args: args{
				expectedSset: resizedSset,
				actualSset:   sset,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("3Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),
			wantRecreate: true,
		},
		{
			name: "one pvc is missing: resize what's there, don't error out",
			args: args{
				expectedSset: resizedSset,
				actualSset:   sset,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("3Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi"),
			wantRecreate: true,
		},
		{
			name: "storage decrease is not supported: error out",
			args: args{
				expectedSset: sset,        // 1Gi
				actualSset:   resizedSset, // 3Gi
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("3Gi", "3Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi"),
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrappedFakeClient(append(tt.runtimeObjs, &es)...)
			recreate, err := handleVolumeExpansion(k8sClient, es, tt.args.expectedSset, tt.args.actualSset)
			if (err != nil) != tt.wantErr {
				t.Errorf("resizePVCs() error = %v, wantErr %v", err, tt.wantErr)
			}
			require.Equal(t, tt.wantRecreate, recreate)

			// all expected PVCs should exist in the apiserver
			var pvcs corev1.PersistentVolumeClaimList
			err = k8sClient.List(&pvcs)
			require.NoError(t, err)
			require.Len(t, pvcs.Items, len(tt.expectedPVCs))
			for i, expectedPVC := range tt.expectedPVCs {
				comparison.RequireEqual(t, &expectedPVC, &pvcs.Items[i])
			}

			// Elasticsearch should be annotated with the sset to recreate
			var retrievedES esv1.Elasticsearch
			err = k8sClient.Get(k8s.ExtractNamespacedName(&es), &retrievedES)
			require.NoError(t, err)
			if tt.wantRecreate {
				require.Len(t, retrievedES.Annotations, 1)
				wantUpdatedSset := tt.args.actualSset.DeepCopy()
				// should have the expected claims
				wantUpdatedSset.Spec.VolumeClaimTemplates = tt.args.expectedSset.Spec.VolumeClaimTemplates

				// test ssetsToRecreate along the way
				toRecreate, err := ssetsToRecreate(retrievedES)
				require.NoError(t, err)
				require.Equal(t,
					map[string]appsv1.StatefulSet{
						"elasticsearch.k8s.elastic.co/recreate-" + tt.args.actualSset.Name: *wantUpdatedSset},
					toRecreate)
			} else {
				require.Empty(t, retrievedES.Annotations)
			}
		})
	}
}

func Test_needsRecreate(t *testing.T) {
	type args struct {
		expectedSset appsv1.StatefulSet
		actualSset   appsv1.StatefulSet
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "requested storage increase in the 2nd claim: recreate",
			args: args{
				expectedSset: withClaims(sampleSset, sampleClaim, withStorageReq(sampleClaim2, "3Gi")),
				actualSset:   withClaims(sampleSset, sampleClaim, sampleClaim2),
			},
			want: true,
		},
		{
			name: "no claim in the StatefulSet",
			args: args{
				expectedSset: sampleSset,
				actualSset:   sampleSset,
			},
			want: false,
		},
		{
			name: "no change in the claim",
			args: args{
				expectedSset: withClaims(sampleSset, sampleClaim),
				actualSset:   withClaims(sampleSset, sampleClaim),
			},
			want: false,
		},
		{
			name: "requested storage decrease: error out",
			args: args{
				expectedSset: withClaims(sampleSset, sampleClaim, withStorageReq(sampleClaim2, "0.5Gi")),
				actualSset:   withClaims(sampleSset, sampleClaim, sampleClaim2),
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := needsRecreate(tt.args.expectedSset, tt.args.actualSset)
			if (err != nil) != tt.wantErr {
				t.Errorf("needsRecreate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("needsRecreate() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isStorageExpansion(t *testing.T) {
	type args struct {
		expectedSize *resource.Quantity
		actualSize   *resource.Quantity
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "expected == actual: false",
			args: args{
				expectedSize: resource.NewQuantity(1, resource.DecimalSI),
				actualSize:   resource.NewQuantity(1, resource.DecimalSI),
			},
			want: false,
		},
		{
			name: "expected > actual: true",
			args: args{
				expectedSize: resource.NewQuantity(2, resource.DecimalSI),
				actualSize:   resource.NewQuantity(1, resource.DecimalSI),
			},
			want: true,
		},
		{
			name: "expected < actual: error out",
			args: args{
				expectedSize: resource.NewQuantity(1, resource.DecimalSI),
				actualSize:   resource.NewQuantity(2, resource.DecimalSI),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "expected is nil",
			args: args{
				expectedSize: nil,
				actualSize:   resource.NewQuantity(1, resource.DecimalSI),
			},
			want: false,
		},
		{
			name: "actual is nil",
			args: args{
				expectedSize: resource.NewQuantity(1, resource.DecimalSI),
				actualSize:   nil,
			},
			want: false,
		},
		{
			name: "expected and actual are nil",
			args: args{
				expectedSize: nil,
				actualSize:   nil,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isStorageExpansion(tt.args.expectedSize, tt.args.actualSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("isStorageExpansion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isStorageExpansion() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_recreateStatefulSets(t *testing.T) {
	es := func() *esv1.Elasticsearch {
		return &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	}
	withAnnotation := func(es *esv1.Elasticsearch, key, value string) *esv1.Elasticsearch {
		if es.Annotations == nil {
			es.Annotations = map[string]string{}
		}
		es.Annotations[key] = value
		return es
	}

	sset1 := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1", UID: "sset1-uid"}}
	sset1Bytes, _ := json.Marshal(sset1)
	sset1Json := string(sset1Bytes)
	sset1DifferentUID := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1", UID: "sset1-differentuid"}}

	sset2 := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset2", UID: "sset2-uid"}}
	sset2Bytes, _ := json.Marshal(sset2)
	sset2Json := string(sset2Bytes)

	type args struct {
		existingSsets []runtime.Object
		es            esv1.Elasticsearch
	}
	tests := []struct {
		name string
		args
		wantES          esv1.Elasticsearch
		wantSsets       []appsv1.StatefulSet
		wantRecreations int
	}{
		{
			name: "no annotation: nothing to do",
			args: args{
				existingSsets: []runtime.Object{sset1},
				es:            *es(),
			},
			wantES:          *es(),
			wantRecreations: 0,
		},
		{
			name: "StatefulSet to delete",
			args: args{
				existingSsets: []runtime.Object{sset1}, // exists with the same UID
				es:            *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1Json),
			},
			wantES:          *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1Json),
			wantSsets:       nil, // deleted
			wantRecreations: 1,
		},
		{
			name: "StatefulSet to create",
			args: args{
				existingSsets: []runtime.Object{}, // doesn't exist
				es:            *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1Json),
			},
			wantES: *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1Json),
			// created, no UUID due to how the fake client creates objects
			wantSsets:       []appsv1.StatefulSet{{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1"}}},
			wantRecreations: 1,
		},
		{
			name: "StatefulSet already recreated: remove the annotation",
			args: args{
				existingSsets: []runtime.Object{sset1DifferentUID}, // recreated
				es:            *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1Json),
			},
			wantES:          *es(),                                    // annotation removed
			wantSsets:       []appsv1.StatefulSet{*sset1DifferentUID}, // same
			wantRecreations: 1,                                        // not considered done yet
		},
		{
			name: "multiple statefulsets to handle",
			args: args{
				existingSsets: []runtime.Object{sset1, sset2},
				es: *withAnnotation(withAnnotation(es(),
					"elasticsearch.k8s.elastic.co/recreate-sset1", sset1Json),
					"elasticsearch.k8s.elastic.co/recreate-sset2", sset2Json),
			},
			wantES: *withAnnotation(withAnnotation(es(),
				"elasticsearch.k8s.elastic.co/recreate-sset1", sset1Json),
				"elasticsearch.k8s.elastic.co/recreate-sset2", sset2Json),
			wantSsets:       nil,
			wantRecreations: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrappedFakeClient(append(tt.args.existingSsets, &tt.args.es)...)
			got, err := recreateStatefulSets(k8sClient, tt.args.es)
			require.NoError(t, err)
			require.Equal(t, tt.wantRecreations, got)

			var retrievedES esv1.Elasticsearch
			err = k8sClient.Get(k8s.ExtractNamespacedName(&tt.args.es), &retrievedES)
			require.NoError(t, err)
			comparison.RequireEqual(t, &tt.wantES, &retrievedES)

			var retrievedSsets appsv1.StatefulSetList
			err = k8sClient.List(&retrievedSsets)
			require.NoError(t, err)
			for i := range tt.wantSsets {
				comparison.RequireEqual(t, &tt.wantSsets[i], &retrievedSsets.Items[i])
			}
		})
	}
}
