// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
)

var (
	defaultStorageClass = storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "default-sc",
			Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}}
	defaultBetaStorageClass = storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name:        "default-beta-sc",
		Annotations: map[string]string{"storageclass.beta.kubernetes.io/is-default-class": "true"}}}
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

func Test_resizePVCs(t *testing.T) {
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
	}{
		{
			name: "no pvc to resize",
			args: args{
				expectedSset: sset,
				actualSset:   sset,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("1Gi", "1Gi", "1Gi"),
		},
		{
			name: "all pvcs should be resized",
			args: args{
				expectedSset: resizedSset,
				actualSset:   sset,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),
		},
		{
			name: "2 pvcs left to resize",
			args: args{
				expectedSset: resizedSset,
				actualSset:   sset,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("3Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),
		},
		{
			name: "one pvc is missing: resize what's there, don't error out",
			args: args{
				expectedSset: resizedSset,
				actualSset:   sset,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("3Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi"),
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
		{
			name: "storage class does not support volume expansion: error out",
			args: args{
				expectedSset: resizedSset,
				actualSset:   sset,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), &sampleStorageClass),
			expectedPVCs: pvcsWithSize("1Gi", "1Gi", "1Gi"),
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrappedFakeClient(tt.runtimeObjs...)
			if err := resizePVCs(k8sClient, tt.args.expectedSset, tt.args.actualSset); (err != nil) != tt.wantErr {
				t.Errorf("resizePVCs() error = %v, wantErr %v", err, tt.wantErr)
			}

			// all expected PVCs should exist in the apiserver
			var pvcs corev1.PersistentVolumeClaimList
			err := k8sClient.List(&pvcs)
			require.NoError(t, err)
			require.Len(t, pvcs.Items, len(tt.expectedPVCs))
			for i, expectedPVC := range tt.expectedPVCs {
				comparison.RequireEqual(t, &expectedPVC, &pvcs.Items[i])
			}
		})
	}
}

func Test_deleteSsetForClaimResize(t *testing.T) {
	type args struct {
		k8sClient    k8s.Client
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
				k8sClient:    k8s.WrappedFakeClient(&sampleSset, withVolumeExpansion(sampleStorageClass)),
				expectedSset: withClaims(sampleSset, sampleClaim, withStorageReq(sampleClaim2, "3Gi")),
				actualSset:   withClaims(sampleSset, sampleClaim, sampleClaim2),
			},
			want: true,
		},
		{
			name: "requested storage increase in the 2nd claim, but storage class does not allow it: error out",
			args: args{
				k8sClient:    k8s.WrappedFakeClient(&sampleSset, &sampleStorageClass),
				expectedSset: withClaims(sampleSset, sampleClaim, withStorageReq(sampleClaim2, "3Gi")),
				actualSset:   withClaims(sampleSset, sampleClaim, sampleClaim2),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "no claim in the StatefulSet",
			args: args{
				k8sClient:    k8s.WrappedFakeClient(&sampleSset),
				expectedSset: sampleSset,
				actualSset:   sampleSset,
			},
			want: false,
		},
		{
			name: "no change in the claim",
			args: args{
				k8sClient:    k8s.WrappedFakeClient(&sampleSset),
				expectedSset: withClaims(sampleSset, sampleClaim),
				actualSset:   withClaims(sampleSset, sampleClaim),
			},
			want: false,
		},
		{
			name: "requested storage decrease: error out",
			args: args{
				k8sClient:    k8s.WrappedFakeClient(&sampleSset, withVolumeExpansion(sampleStorageClass)),
				expectedSset: withClaims(sampleSset, sampleClaim, withStorageReq(sampleClaim2, "0.5Gi")),
				actualSset:   withClaims(sampleSset, sampleClaim, sampleClaim2),
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleted, err := deleteSsetForClaimResize(tt.args.k8sClient, tt.args.expectedSset, tt.args.actualSset)
			if (err != nil) != tt.wantErr {
				t.Errorf("deleteSsetForClaimResize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if deleted != tt.want {
				t.Errorf("deleteSsetForClaimResize() got = %v, want %v", deleted, tt.want)
			}

			// double-check if the sset is indeed deleted
			var retrieved appsv1.StatefulSet
			err = tt.args.k8sClient.Get(k8s.ExtractNamespacedName(&tt.args.actualSset), &retrieved)
			if deleted {
				require.True(t, apierrors.IsNotFound(err))
			} else {
				require.NoError(t, err)
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

func Test_ensureClaimSupportsExpansion(t *testing.T) {
	tests := []struct {
		name      string
		k8sClient k8s.Client
		claim     corev1.PersistentVolumeClaim
		wantErr   bool
	}{
		{
			name:      "specified storage class supports volume expansion",
			k8sClient: k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
			claim:     sampleClaim,
			wantErr:   false,
		},
		{
			name:      "specified storage class does not support volume expansion",
			k8sClient: k8s.WrappedFakeClient(&sampleStorageClass),
			claim:     sampleClaim,
			wantErr:   true,
		},
		{
			name:      "default storage class supports volume expansion",
			k8sClient: k8s.WrappedFakeClient(withVolumeExpansion(defaultStorageClass)),
			claim:     corev1.PersistentVolumeClaim{},
			wantErr:   false,
		},
		{
			name:      "default storage class does not support volume expansion",
			k8sClient: k8s.WrappedFakeClient(&defaultStorageClass),
			claim:     corev1.PersistentVolumeClaim{},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ensureClaimSupportsExpansion(tt.k8sClient, tt.claim); (err != nil) != tt.wantErr {
				t.Errorf("ensureClaimSupportsExpansion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_allowsVolumeExpansion(t *testing.T) {
	tests := []struct {
		name string
		sc   storagev1.StorageClass
		want bool
	}{
		{
			name: "allow volume expansion: true",
			sc:   storagev1.StorageClass{AllowVolumeExpansion: pointer.BoolPtr(true)},
			want: true,
		},
		{
			name: "allow volume expansion: false",
			sc:   storagev1.StorageClass{AllowVolumeExpansion: pointer.BoolPtr(false)},
			want: false,
		},
		{
			name: "allow volume expansion: nil",
			sc:   storagev1.StorageClass{AllowVolumeExpansion: nil},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowsVolumeExpansion(tt.sc); got != tt.want {
				t.Errorf("allowsVolumeExpansion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isDefaultStorageClass(t *testing.T) {
	tests := []struct {
		name string
		sc   storagev1.StorageClass
		want bool
	}{
		{
			name: "annotated as default",
			sc:   defaultStorageClass,
			want: true,
		},
		{
			name: "annotated as default (beta)",
			sc:   defaultBetaStorageClass,
			want: true,
		},
		{
			name: "annotated as default (+ beta)",
			sc: storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				"storageclass.kubernetes.io/is-default-class":      "true",
				"storageclass.beta.kubernetes.io/is-default-class": "true",
			}}},
			want: true,
		},
		{
			name: "no annotations",
			sc:   storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Annotations: nil}},
			want: false,
		},
		{
			name: "not annotated as default",
			sc:   sampleStorageClass,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDefaultStorageClass(tt.sc); got != tt.want {
				t.Errorf("isDefaultStorageClass() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDefaultStorageClass(t *testing.T) {
	tests := []struct {
		name      string
		k8sClient k8s.Client
		want      storagev1.StorageClass
		wantErr   bool
	}{
		{
			name:      "return the default storage class",
			k8sClient: k8s.WrappedFakeClient(&sampleStorageClass, &defaultStorageClass),
			want:      defaultStorageClass,
		},
		{
			name:      "default storage class not found",
			k8sClient: k8s.WrappedFakeClient(&sampleStorageClass),
			want:      storagev1.StorageClass{},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getDefaultStorageClass(tt.k8sClient)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDefaultStorageClass() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getDefaultStorageClass() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getStorageClass(t *testing.T) {
	tests := []struct {
		name      string
		k8sClient k8s.Client
		claim     corev1.PersistentVolumeClaim
		want      storagev1.StorageClass
		wantErr   bool
	}{
		{
			name:      "return the specified storage class",
			k8sClient: k8s.WrappedFakeClient(&sampleStorageClass, &defaultStorageClass),
			claim:     corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: pointer.StringPtr(sampleStorageClass.Name)}},
			want:      sampleStorageClass,
			wantErr:   false,
		},
		{
			name:      "error out if not found",
			k8sClient: k8s.WrappedFakeClient(&defaultStorageClass),
			claim:     corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: pointer.StringPtr(sampleStorageClass.Name)}},
			want:      storagev1.StorageClass{},
			wantErr:   true,
		},
		{
			name:      "fallback to the default storage class if unspecified",
			k8sClient: k8s.WrappedFakeClient(&sampleStorageClass, &defaultStorageClass),
			claim:     corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{}},
			want:      defaultStorageClass,
			wantErr:   false,
		},
		{
			name:      "error out if unspecified and default storage class not found",
			k8sClient: k8s.WrappedFakeClient(&sampleStorageClass),
			claim:     corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{}},
			want:      storagev1.StorageClass{},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getStorageClass(tt.k8sClient, tt.claim)
			if (err != nil) != tt.wantErr {
				t.Errorf("getStorageClass() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !comparison.Equal(&got, &tt.want) {
				t.Errorf("getStorageClass() got = %v, want %v", got, tt.want)
			}
		})
	}
}
