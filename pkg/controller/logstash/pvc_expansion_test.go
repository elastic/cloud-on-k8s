// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	controllerscheme "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	sampleStorageClass = storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name: "sample-sc"}}

	sampleClaim = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-claim"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: pointer.String(sampleStorageClass.Name),
			Resources: corev1.ResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}
	sampleClaim2 = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-claim-2"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: pointer.String(sampleStorageClass.Name),
			Resources: corev1.ResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}

	sampleSset = appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"}}
)

func withVolumeExpansion(sc storagev1.StorageClass) *storagev1.StorageClass {
	sc.AllowVolumeExpansion = pointer.Bool(true)
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
	es := logstashv1alpha1.Logstash{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ls"}}
	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             pointer.Int32(3),
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
	pvcPtrs := func(pvcs []corev1.PersistentVolumeClaim) []client.Object {
		var ptrs []client.Object
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
		runtimeObjs  []client.Object
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
			k8sClient := k8s.NewFakeClient(append(tt.runtimeObjs, &es)...)
			recreate, err := handleVolumeExpansion(context.Background(), k8sClient, es, tt.args.expectedSset, tt.args.actualSset, tt.args.validateStorageClass)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleVolumeExpansion() error = %v, wantErr %v", err, tt.wantErr)
			}
			require.Equal(t, tt.wantRecreate, recreate)

			// all expected PVCs should exist in the apiserver
			var pvcs corev1.PersistentVolumeClaimList
			err = k8sClient.List(context.Background(), &pvcs)
			require.NoError(t, err)
			require.Len(t, pvcs.Items, len(tt.expectedPVCs))
			for i := range tt.expectedPVCs {
				comparison.RequireEqual(t, &tt.expectedPVCs[i], &pvcs.Items[i])
			}

			// Elasticsearch should be annotated with the sset to recreate
			var retrievedES logstashv1alpha1.Logstash
			err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&es), &retrievedES)
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
						"logstash.k8s.elastic.co/recreate-" + tt.args.actualSset.Name: *wantUpdatedSset},
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
		name string
		args args
		want bool
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsRecreate(tt.args.expectedSset, tt.args.actualSset)
			if got != tt.want {
				t.Errorf("needsRecreate() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_recreateStatefulSets(t *testing.T) {
	controllerscheme.SetupScheme()
	ls := func() *logstashv1alpha1.Logstash {
		return &logstashv1alpha1.Logstash{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ls", UID: "ls-uid"}, TypeMeta: metav1.TypeMeta{Kind: logstashv1alpha1.Kind}}
	}
	withAnnotation := func(ls *logstashv1alpha1.Logstash, key, value string) *logstashv1alpha1.Logstash {
		if ls.Annotations == nil {
			ls.Annotations = map[string]string{}
		}
		ls.Annotations[key] = value
		return ls
	}

	sset1 := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1", UID: "sset1-uid"}}
	sset1Bytes, _ := json.Marshal(sset1)
	sset1JSON := string(sset1Bytes)
	sset1DifferentUID := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1", UID: "sset1-differentuid"}}
	pod1 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1-0", Labels: map[string]string{
		labels.StatefulSetNameLabelName: sset1.Name,
	}}}
	pod1WithOwnerRef := pod1.DeepCopy()
	require.NoError(t, controllerutil.SetOwnerReference(ls(), pod1WithOwnerRef, scheme.Scheme))

	sset2 := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset2", UID: "sset2-uid"}}
	sset2Bytes, _ := json.Marshal(sset2)
	sset2JSON := string(sset2Bytes)

	type args struct {
		runtimeObjs []client.Object
		ls          logstashv1alpha1.Logstash
	}
	tests := []struct {
		name string
		args
		wantLS          logstashv1alpha1.Logstash
		wantSsets       []appsv1.StatefulSet
		wantPods        []corev1.Pod
		wantRecreations int
	}{
		{
			name: "no annotation: nothing to do",
			args: args{
				runtimeObjs: []client.Object{sset1, pod1},
				ls:          *ls(),
			},
			wantLS:          *ls(),
			wantPods:        []corev1.Pod{*pod1},
			wantRecreations: 0,
		},
		{
			name: "StatefulSet to delete",
			args: args{
				runtimeObjs: []client.Object{sset1, pod1}, // sset exists with the same UID
				ls:          *withAnnotation(ls(), "logstash.k8s.elastic.co/recreate-sset1", sset1JSON),
			},
			wantLS:          *withAnnotation(ls(), "logstash.k8s.elastic.co/recreate-sset1", sset1JSON),
			wantSsets:       nil,                             // deleted
			wantPods:        []corev1.Pod{*pod1WithOwnerRef}, // owner ref set to the ES resource
			wantRecreations: 1,
		},
		{
			name: "StatefulSet to create",
			args: args{
				runtimeObjs: []client.Object{pod1}, // sset doesn't exist
				ls:          *withAnnotation(ls(), "logstash.k8s.elastic.co/recreate-sset1", sset1JSON),
			},
			wantLS: *withAnnotation(ls(), "logstash.k8s.elastic.co/recreate-sset1", sset1JSON),
			// created, no UUID due to how the fake client creates objects
			wantSsets:       []appsv1.StatefulSet{{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1"}}},
			wantPods:        []corev1.Pod{*pod1}, // unmodified
			wantRecreations: 1,
		},
		{
			name: "StatefulSet already recreated: remove the annotation",
			args: args{
				runtimeObjs: []client.Object{sset1DifferentUID, pod1WithOwnerRef}, // sset recreated
				ls:          *withAnnotation(ls(), "logstash.k8s.elastic.co/recreate-sset1", sset1JSON),
			},
			wantLS:          *ls(),                                    // annotation removed
			wantSsets:       []appsv1.StatefulSet{*sset1DifferentUID}, // same
			wantPods:        []corev1.Pod{*pod1},                      // ownerRef removed
			wantRecreations: 0,
		},
		{
			name: "multiple statefulsets to handle",
			args: args{
				runtimeObjs: []client.Object{sset1, sset2, pod1},
				ls: *withAnnotation(withAnnotation(ls(),
					"logstash.k8s.elastic.co/recreate-sset1", sset1JSON),
					"logstash.k8s.elastic.co/recreate-sset2", sset2JSON),
			},
			wantLS: *withAnnotation(withAnnotation(ls(),
				"logstash.k8s.elastic.co/recreate-sset1", sset1JSON),
				"logstash.k8s.elastic.co/recreate-sset2", sset2JSON),
			wantSsets:       nil,
			wantPods:        []corev1.Pod{*pod1WithOwnerRef}, // ownerRef removed
			wantRecreations: 2,
		},
		{
			name: "additional annotations are ignored",
			args: args{
				runtimeObjs: []client.Object{sset1DifferentUID, pod1}, // sset recreated
				ls: *withAnnotation(withAnnotation(ls(),
					"logstash.k8s.elastic.co/recreate-sset1", sset1JSON),
					"another-annotation-key", sset2JSON),
			},
			// sset annotation removed, other annotation preserved
			wantLS:          *withAnnotation(ls(), "another-annotation-key", sset2JSON),
			wantSsets:       nil,
			wantPods:        []corev1.Pod{*pod1},
			wantRecreations: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(append(tt.args.runtimeObjs, &tt.args.ls)...)
			got, err := recreateStatefulSets(context.Background(), k8sClient, tt.args.ls)
			require.NoError(t, err)
			require.Equal(t, tt.wantRecreations, got)

			var retrievedLS logstashv1alpha1.Logstash
			err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&tt.args.ls), &retrievedLS)
			require.NoError(t, err)
			comparison.RequireEqual(t, &tt.wantLS, &retrievedLS)

			var retrievedSsets appsv1.StatefulSetList
			err = k8sClient.List(context.Background(), &retrievedSsets)
			require.NoError(t, err)
			for i := range tt.wantSsets {
				comparison.RequireEqual(t, &tt.wantSsets[i], &retrievedSsets.Items[i])
			}

			var retrievedPods corev1.PodList
			err = k8sClient.List(context.Background(), &retrievedPods)
			require.NoError(t, err)
			for i := range tt.wantPods {
				comparison.RequireEqual(t, &tt.wantPods[i], &retrievedPods.Items[i])
			}
		})
	}
}

var (
	sampleLs = logstashv1alpha1.Logstash{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ls", UID: "ls-uid"}, TypeMeta: metav1.TypeMeta{Kind: logstashv1alpha1.Kind}}
	sset1    = appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1", UID: "sset1-uid"}}
	pod1     = corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1-0", Labels: map[string]string{
		labels.StatefulSetNameLabelName: sset1.Name,
	}}}
	pod2 = corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1-1", Labels: map[string]string{
		labels.StatefulSetNameLabelName: sset1.Name,
	}}}
	pod1WithOwnerRef = *pod1.DeepCopy()
	pod2WithOwnerRef = *pod2.DeepCopy()
)

func init() {
	controllerscheme.SetupScheme()
	if err := controllerutil.SetOwnerReference(&sampleLs, &pod1WithOwnerRef, scheme.Scheme); err != nil {
		panic(err)
	}
	if err := controllerutil.SetOwnerReference(&sampleLs, &pod2WithOwnerRef, scheme.Scheme); err != nil {
		panic(err)
	}
}

func Test_updatePodOwners(t *testing.T) {
	type args struct {
		k8sClient   k8s.Client
		ls          logstashv1alpha1.Logstash
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
				k8sClient:   k8s.NewFakeClient(&pod1, &pod2),
				ls:          sampleLs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1WithOwnerRef, pod2WithOwnerRef},
		},
		{
			name: "owner ref already set: the function is idempotent",
			args: args{
				k8sClient:   k8s.NewFakeClient(&pod1WithOwnerRef, &pod2WithOwnerRef),
				ls:          sampleLs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1WithOwnerRef, pod2WithOwnerRef},
		},
		{
			name: "one owner ref already set, one missing",
			args: args{
				k8sClient:   k8s.NewFakeClient(&pod1WithOwnerRef, &pod2),
				ls:          sampleLs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1WithOwnerRef, pod2WithOwnerRef},
		},
		{
			name: "no Pods: nothing to do",
			args: args{
				k8sClient:   k8s.NewFakeClient(),
				ls:          sampleLs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := updatePodOwners(context.Background(), tt.args.k8sClient, tt.args.ls, tt.args.statefulSet)
			require.NoError(t, err)

			var retrievedPods corev1.PodList
			err = tt.args.k8sClient.List(context.Background(), &retrievedPods)
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
		ls          logstashv1alpha1.Logstash
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
				k8sClient:   k8s.NewFakeClient(&pod1WithOwnerRef, &pod2WithOwnerRef),
				ls:          sampleLs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, pod2},
		},
		{
			name: "owner refs already removed: function is idempotent",
			args: args{
				k8sClient:   k8s.NewFakeClient(&pod1, &pod2),
				ls:          sampleLs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, pod2},
		},
		{
			name: "one owner ref already removed, the other not yet removed",
			args: args{
				k8sClient:   k8s.NewFakeClient(&pod1WithOwnerRef, &pod2),
				ls:          sampleLs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, pod2},
		},
		{
			name: "no Pods: nothing to do",
			args: args{
				k8sClient:   k8s.NewFakeClient(),
				ls:          sampleLs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{},
		},
		{
			name: "preserve existing unrelated owner refs",
			args: args{
				k8sClient: k8s.NewFakeClient(&pod1WithOwnerRef, withOwnerRef(pod2WithOwnerRef, metav1.OwnerReference{
					Kind: "kind",
					Name: "name",
					UID:  "uid",
				})),
				ls:          sampleLs,
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
			err := removeLSPodOwner(context.Background(), tt.args.k8sClient, tt.args.ls, tt.args.statefulSet)
			require.NoError(t, err)

			var retrievedPods corev1.PodList
			err = tt.args.k8sClient.List(context.Background(), &retrievedPods)
			require.NoError(t, err)
			for i := range tt.wantPods {
				comparison.RequireEqual(t, &tt.wantPods[i], &retrievedPods.Items[i])
			}
		})
	}
}
