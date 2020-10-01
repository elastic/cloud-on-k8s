// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	defaultStorageClass = storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "default-sc",
			Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}}
	defaultBetaStorageClass = storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name:        "default-beta-sc",
		Annotations: map[string]string{"storageclass.beta.kubernetes.io/is-default-class": "true"}}}

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
		expectedSset         appsv1.StatefulSet
		actualSset           appsv1.StatefulSet
		validateStorageClass bool
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
				expectedSset:         sset,
				actualSset:           sset,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("1Gi", "1Gi", "1Gi"),
			wantRecreate: false,
		},
		{
			name: "all pvcs should be resized",
			args: args{
				expectedSset:         resizedSset,
				actualSset:           sset,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),
			wantRecreate: true,
		},
		{
			name: "2 pvcs left to resize",
			args: args{
				expectedSset:         resizedSset,
				actualSset:           sset,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("3Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),
			wantRecreate: true,
		},
		{
			name: "one pvc is missing: resize what's there, don't error out",
			args: args{
				expectedSset:         resizedSset,
				actualSset:           sset,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("3Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi"),
			wantRecreate: true,
		},
		{
			name: "storage decrease is not supported: error out",
			args: args{
				expectedSset:         sset,        // 1Gi
				actualSset:           resizedSset, // 3Gi
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("3Gi", "3Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi"),
			wantErr:      true,
		},
		{
			name: "volume expansion not supported: error out",
			args: args{
				expectedSset:         resizedSset,
				actualSset:           sset,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), &sampleStorageClass), // no expansion
			expectedPVCs: pvcsWithSize("1Gi", "1Gi", "1Gi"),                                       // not resized
			wantRecreate: false,
			wantErr:      true,
		},
		{
			name: "volume expansion not supported but no storage class validation: attempt to resize",
			args: args{
				expectedSset:         resizedSset,
				actualSset:           sset,
				validateStorageClass: false,
			},
			runtimeObjs:  append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), &sampleStorageClass), // no expansion
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),                                       // still resized
			wantRecreate: true,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrappedFakeClient(append(tt.runtimeObjs, &es)...)
			recreate, err := handleVolumeExpansion(k8sClient, es, tt.args.expectedSset, tt.args.actualSset, tt.args.validateStorageClass)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleVolumeExpansion() error = %v, wantErr %v", err, tt.wantErr)
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
		k8sClient            k8s.Client
		expectedSset         appsv1.StatefulSet
		actualSset           appsv1.StatefulSet
		validateStorageClass bool
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
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				expectedSset:         withClaims(sampleSset, sampleClaim, withStorageReq(sampleClaim2, "3Gi")),
				actualSset:           withClaims(sampleSset, sampleClaim, sampleClaim2),
				validateStorageClass: true,
			},
			want: true,
		},
		{
			name: "volume expansion not supported: error out",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(&sampleStorageClass),
				expectedSset:         withClaims(sampleSset, sampleClaim, withStorageReq(sampleClaim2, "3Gi")),
				actualSset:           withClaims(sampleSset, sampleClaim, sampleClaim2),
				validateStorageClass: true,
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "volume expansion not supported but no storage class validation: recreate",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(&sampleStorageClass),
				expectedSset:         withClaims(sampleSset, sampleClaim, withStorageReq(sampleClaim2, "3Gi")),
				actualSset:           withClaims(sampleSset, sampleClaim, sampleClaim2),
				validateStorageClass: false,
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "no claim in the StatefulSet",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				expectedSset:         sampleSset,
				actualSset:           sampleSset,
				validateStorageClass: true,
			},
			want: false,
		},
		{
			name: "no change in the claim",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				expectedSset:         withClaims(sampleSset, sampleClaim),
				actualSset:           withClaims(sampleSset, sampleClaim),
				validateStorageClass: true,
			},
			want: false,
		},
		{
			name: "requested storage decrease: error out",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				expectedSset:         withClaims(sampleSset, sampleClaim, withStorageReq(sampleClaim2, "0.5Gi")),
				actualSset:           withClaims(sampleSset, sampleClaim, sampleClaim2),
				validateStorageClass: true,
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := needsRecreate(tt.args.k8sClient, tt.args.expectedSset, tt.args.actualSset, tt.args.validateStorageClass)
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
	controllerscheme.SetupScheme()
	es := func() *esv1.Elasticsearch {
		return &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es", UID: "es-uid"}, TypeMeta: metav1.TypeMeta{Kind: "Elasticsearch"}}
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
	sset1JSON := string(sset1Bytes)
	sset1DifferentUID := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1", UID: "sset1-differentuid"}}
	pod1 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1-0", Labels: map[string]string{
		label.StatefulSetNameLabelName: sset1.Name,
	}}}
	pod1WithOwnerRef := pod1.DeepCopy()
	require.NoError(t, controllerutil.SetOwnerReference(es(), pod1WithOwnerRef, scheme.Scheme))

	sset2 := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset2", UID: "sset2-uid"}}
	sset2Bytes, _ := json.Marshal(sset2)
	sset2JSON := string(sset2Bytes)

	type args struct {
		runtimeObjs []runtime.Object
		es          esv1.Elasticsearch
	}
	tests := []struct {
		name string
		args
		wantES          esv1.Elasticsearch
		wantSsets       []appsv1.StatefulSet
		wantPods        []corev1.Pod
		wantRecreations int
	}{
		{
			name: "no annotation: nothing to do",
			args: args{
				runtimeObjs: []runtime.Object{sset1, pod1},
				es:          *es(),
			},
			wantES:          *es(),
			wantPods:        []corev1.Pod{*pod1},
			wantRecreations: 0,
		},
		{
			name: "StatefulSet to delete",
			args: args{
				runtimeObjs: []runtime.Object{sset1, pod1}, // sset exists with the same UID
				es:          *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			},
			wantES:          *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			wantSsets:       nil,                             // deleted
			wantPods:        []corev1.Pod{*pod1WithOwnerRef}, // owner ref set to the ES resource
			wantRecreations: 1,
		},
		{
			name: "StatefulSet to create",
			args: args{
				runtimeObjs: []runtime.Object{pod1}, // sset doesn't exist
				es:          *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			},
			wantES: *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			// created, no UUID due to how the fake client creates objects
			wantSsets:       []appsv1.StatefulSet{{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1"}}},
			wantPods:        []corev1.Pod{*pod1}, // unmodified
			wantRecreations: 1,
		},
		{
			name: "StatefulSet already recreated: remove the annotation",
			args: args{
				runtimeObjs: []runtime.Object{sset1DifferentUID, pod1WithOwnerRef}, // sset recreated
				es:          *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			},
			wantES:          *es(),                                    // annotation removed
			wantSsets:       []appsv1.StatefulSet{*sset1DifferentUID}, // same
			wantPods:        []corev1.Pod{*pod1},                      // ownerRef removed
			wantRecreations: 0,
		},
		{
			name: "multiple statefulsets to handle",
			args: args{
				runtimeObjs: []runtime.Object{sset1, sset2, pod1},
				es: *withAnnotation(withAnnotation(es(),
					"elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
					"elasticsearch.k8s.elastic.co/recreate-sset2", sset2JSON),
			},
			wantES: *withAnnotation(withAnnotation(es(),
				"elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
				"elasticsearch.k8s.elastic.co/recreate-sset2", sset2JSON),
			wantSsets:       nil,
			wantPods:        []corev1.Pod{*pod1WithOwnerRef}, // ownerRef removed
			wantRecreations: 2,
		},
		{
			name: "additional annotations are ignored",
			args: args{
				runtimeObjs: []runtime.Object{sset1DifferentUID, pod1}, // sset recreated
				es: *withAnnotation(withAnnotation(es(),
					"elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
					"another-annotation-key", sset2JSON),
			},
			// sset annotation removed, other annotation preserved
			wantES:          *withAnnotation(es(), "another-annotation-key", sset2JSON),
			wantSsets:       nil,
			wantPods:        []corev1.Pod{*pod1},
			wantRecreations: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrappedFakeClient(append(tt.args.runtimeObjs, &tt.args.es)...)
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

			var retrievedPods corev1.PodList
			err = k8sClient.List(&retrievedPods)
			require.NoError(t, err)
			for i := range tt.wantPods {
				comparison.RequireEqual(t, &tt.wantPods[i], &retrievedPods.Items[i])
			}
		})
	}
}

var (
	sampleEs = esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es", UID: "es-uid"}, TypeMeta: metav1.TypeMeta{Kind: "Elasticsearch"}}
	sset1    = appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1", UID: "sset1-uid"}}
	pod1     = corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1-0", Labels: map[string]string{
		label.StatefulSetNameLabelName: sset1.Name,
	}}}
	pod2 = corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1-1", Labels: map[string]string{
		label.StatefulSetNameLabelName: sset1.Name,
	}}}
	pod1WithOwnerRef = *pod1.DeepCopy()
	pod2WithOwnerRef = *pod2.DeepCopy()
)

func init() {
	controllerscheme.SetupScheme()
	if err := controllerutil.SetOwnerReference(&sampleEs, &pod1WithOwnerRef, scheme.Scheme); err != nil {
		panic(err)
	}
	if err := controllerutil.SetOwnerReference(&sampleEs, &pod2WithOwnerRef, scheme.Scheme); err != nil {
		panic(err)
	}
}

func Test_updatePodOwners(t *testing.T) {
	type args struct {
		k8sClient   k8s.Client
		es          esv1.Elasticsearch
		statefulSet appsv1.StatefulSet
	}
	tests := []struct {
		name     string
		args     args
		wantPods []corev1.Pod
	}{
		{
			name: "happy path: set an owner ref to the ES resource on all Pods for that StatefulSet",
			args: args{
				k8sClient:   k8s.WrappedFakeClient(&pod1, &pod2),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1WithOwnerRef, pod2WithOwnerRef},
		},
		{
			name: "owner ref already set: the function is idempotent",
			args: args{
				k8sClient:   k8s.WrappedFakeClient(&pod1WithOwnerRef, &pod2WithOwnerRef),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1WithOwnerRef, pod2WithOwnerRef},
		},
		{
			name: "one owner ref already set, one missing",
			args: args{
				k8sClient:   k8s.WrappedFakeClient(&pod1WithOwnerRef, &pod2),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1WithOwnerRef, pod2WithOwnerRef},
		},
		{
			name: "no Pods: nothing to do",
			args: args{
				k8sClient:   k8s.WrappedFakeClient(),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := updatePodOwners(tt.args.k8sClient, tt.args.es, tt.args.statefulSet)
			require.NoError(t, err)

			var retrievedPods corev1.PodList
			err = tt.args.k8sClient.List(&retrievedPods)
			require.NoError(t, err)
			for i := range tt.wantPods {
				comparison.RequireEqual(t, &tt.wantPods[i], &retrievedPods.Items[i])
			}
		})
	}
}

func withOwnerRef(pod corev1.Pod, ownerRef metav1.OwnerReference) *corev1.Pod {
	pod = *pod.DeepCopy()
	pod.OwnerReferences = append(pod.OwnerReferences, ownerRef)
	return &pod
}

func Test_removeESPodOwner(t *testing.T) {
	type args struct {
		k8sClient   k8s.Client
		es          esv1.Elasticsearch
		statefulSet appsv1.StatefulSet
	}
	tests := []struct {
		name     string
		args     args
		wantPods []corev1.Pod
	}{
		{
			name: "happy path: remove the owner ref from all Pods",
			args: args{
				k8sClient:   k8s.WrappedFakeClient(&pod1WithOwnerRef, &pod2WithOwnerRef),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, pod2},
		},
		{
			name: "owner refs already removed: function is idempotent",
			args: args{
				k8sClient:   k8s.WrappedFakeClient(&pod1, &pod2),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, pod2},
		},
		{
			name: "one owner ref already removed, the other not yet removed",
			args: args{
				k8sClient:   k8s.WrappedFakeClient(&pod1WithOwnerRef, &pod2),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, pod2},
		},
		{
			name: "no Pods: nothing to do",
			args: args{
				k8sClient:   k8s.WrappedFakeClient(),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{},
		},
		{
			name: "preserve existing unrelated owner refs",
			args: args{
				k8sClient: k8s.WrappedFakeClient(&pod1WithOwnerRef, withOwnerRef(pod2WithOwnerRef, metav1.OwnerReference{
					Kind: "kind",
					Name: "name",
					UID:  "uid",
				})),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, *withOwnerRef(pod2, metav1.OwnerReference{
				Kind: "kind",
				Name: "name",
				UID:  "uid",
			})},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := removeESPodOwner(tt.args.k8sClient, tt.args.es, tt.args.statefulSet)
			require.NoError(t, err)

			var retrievedPods corev1.PodList
			err = tt.args.k8sClient.List(&retrievedPods)
			require.NoError(t, err)
			for i := range tt.wantPods {
				comparison.RequireEqual(t, &tt.wantPods[i], &retrievedPods.Items[i])
			}
		})
	}
}

func Test_ensureClaimSupportsExpansion(t *testing.T) {
	tests := []struct {
		name                string
		k8sClient           k8s.Client
		claim               corev1.PersistentVolumeClaim
		validateStoragClass bool
		wantErr             bool
	}{
		{
			name:                "specified storage class supports volume expansion",
			k8sClient:           k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
			claim:               sampleClaim,
			validateStoragClass: true,
			wantErr:             false,
		},
		{
			name:                "specified storage class does not support volume expansion",
			k8sClient:           k8s.WrappedFakeClient(&sampleStorageClass),
			claim:               sampleClaim,
			validateStoragClass: true,
			wantErr:             true,
		},
		{
			name:                "default storage class supports volume expansion",
			k8sClient:           k8s.WrappedFakeClient(withVolumeExpansion(defaultStorageClass)),
			claim:               corev1.PersistentVolumeClaim{},
			validateStoragClass: true,
			wantErr:             false,
		},
		{
			name:                "default storage class does not support volume expansion",
			k8sClient:           k8s.WrappedFakeClient(&defaultStorageClass),
			claim:               corev1.PersistentVolumeClaim{},
			validateStoragClass: true,
			wantErr:             true,
		},
		{
			name:                "storage class vlaidation disabled: no-op",
			k8sClient:           k8s.WrappedFakeClient(&sampleStorageClass), // would otherwise be refused: no expansion
			claim:               sampleClaim,
			validateStoragClass: false,
			wantErr:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ensureClaimSupportsExpansion(tt.k8sClient, tt.claim, tt.validateStoragClass); (err != nil) != tt.wantErr {
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
