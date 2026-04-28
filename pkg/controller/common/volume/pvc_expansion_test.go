// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/comparison"
	controllerscheme "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var (
	sampleStorageClass = storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name: "sample-sc"}}

	sampleClaim = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-claim"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To[string](sampleStorageClass.Name),
			Resources: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}
	sampleClaim2 = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-claim-2"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To[string](sampleStorageClass.Name),
			Resources: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}

	sampleSset = appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"}}
)

func withVolumeExpansion(sc storagev1.StorageClass) *storagev1.StorageClass {
	sc.AllowVolumeExpansion = ptr.To[bool](true)
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

func withLabels(claim corev1.PersistentVolumeClaim, labels map[string]string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	if c.Labels == nil {
		c.Labels = map[string]string{}
	}
	maps.Copy(c.Labels, labels)
	return *c
}

func Test_syncPVCLabels(t *testing.T) {
	tests := []struct {
		name     string
		pvc      corev1.PersistentVolumeClaim
		expected map[string]string
		wantPVC  corev1.PersistentVolumeClaim
		wantDiff bool
	}{
		{
			name:     "nil expected: no change",
			pvc:      corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}}},
			expected: nil,
			wantPVC:  corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}}},
			wantDiff: false,
		},
		{
			name:     "empty expected: no change",
			pvc:      corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}}},
			expected: map[string]string{},
			wantPVC:  corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}}},
			wantDiff: false,
		},
		{
			name:     "add new label",
			pvc:      corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}}},
			expected: map[string]string{"b": "2"},
			wantPVC:  corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1", "b": "2"}}},
			wantDiff: true,
		},
		{
			name:     "add label to nil labels map",
			pvc:      corev1.PersistentVolumeClaim{},
			expected: map[string]string{"b": "2"},
			wantPVC:  corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"b": "2"}}},
			wantDiff: true,
		},
		{
			name:     "update existing label value",
			pvc:      corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}}},
			expected: map[string]string{"a": "updated"},
			wantPVC:  corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "updated"}}},
			wantDiff: true,
		},
		{
			name:     "existing labels not in expected are preserved",
			pvc:      corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"preserved": "yes", "a": "old"}}},
			expected: map[string]string{"a": "new"},
			wantPVC:  corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"preserved": "yes", "a": "new"}}},
			wantDiff: true,
		},
		{
			name:     "label already matches expected: no diff",
			pvc:      corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}}},
			expected: map[string]string{"a": "1"},
			wantPVC:  corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}}},
			wantDiff: false,
		},
		{
			// additive-only semantics: removing a key from the VCT does not propagate to the PVC.
			name:     "key removed from expected: PVC retains the old label, no diff",
			pvc:      corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1", "team": "search"}}},
			expected: map[string]string{"a": "1"},
			wantPVC:  corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1", "team": "search"}}},
			wantDiff: false,
		},
		{
			// defensive guard: reserved keys never make it from the VCT onto the PVC.
			name:     "reserved key in expected is skipped",
			pvc:      corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1"}}},
			expected: map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "evil", "team": "search"},
			wantPVC:  corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "1", "team": "search"}}},
			wantDiff: true,
		},
		{
			// reserved keys already on the PVC (e.g. operator-managed) are not touched
			// even when other expected (non-reserved) keys are being applied.
			name: "reserved key already on PVC is not modified",
			pvc: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"elasticsearch.k8s.elastic.co/cluster-name": "es",
			}}},
			expected: map[string]string{
				"elasticsearch.k8s.elastic.co/cluster-name": "evil",
				"team": "search",
			},
			wantPVC: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"elasticsearch.k8s.elastic.co/cluster-name": "es",
				"team": "search",
			}}},
			wantDiff: true,
		},
		{
			name:     "only-reserved expected: nil-labels PVC stays nil",
			pvc:      corev1.PersistentVolumeClaim{},
			expected: map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "evil"},
			wantPVC:  corev1.PersistentVolumeClaim{},
			wantDiff: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pvc := *tt.pvc.DeepCopy()
			got := syncPVCLabels(&pvc, tt.expected)
			require.Equal(t, tt.wantDiff, got)
			require.Equal(t, tt.wantPVC.Labels, pvc.Labels)
		})
	}
}

func Test_handleVolumeExpansionElasticsearch(t *testing.T) {
	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             ptr.To[int32](3),
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim},
		},
	}
	resizedSset := *sset.DeepCopy()
	resizedSset.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("3Gi")
	pvcsWithSize := func(size ...string) []corev1.PersistentVolumeClaim {
		pvcs := make([]corev1.PersistentVolumeClaim, 0, len(size))
		for i, s := range size {
			pvcs = append(pvcs, withStorageReq(corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: fmt.Sprintf("sample-claim-sample-sset-%d", i)},
				Spec:       sampleClaim.Spec,
			}, s))
		}
		return pvcs
	}
	pvcPtrs := func(pvcs []corev1.PersistentVolumeClaim) []client.Object {
		ptrs := make([]client.Object, 0, len(pvcs))
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
		{
			// label-only VCT change: no storage diff, no recreate, but PVCs should pick up the new labels.
			name: "label-only VCT change is propagated to existing PVCs",
			args: args{
				expectedSset: func() appsv1.StatefulSet {
					labeled := *sset.DeepCopy()
					labeled.Spec.VolumeClaimTemplates[0] = withLabels(sampleClaim, map[string]string{"team": "search"})
					return labeled
				}(),
				actualSset:           sset,
				validateStorageClass: true,
			},
			runtimeObjs: append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: func() []corev1.PersistentVolumeClaim {
				out := pvcsWithSize("1Gi", "1Gi", "1Gi")
				for i := range out {
					out[i].Labels = map[string]string{"team": "search"}
				}
				return out
			}(),
			wantRecreate: false,
			wantErr:      false,
		},
		{
			// Scale-up eventual-consistency: ReconcileStatefulSet preserves the existing
			// (immutable) VolumeClaimTemplates, so the StatefulSet controller will create
			// a new PVC from the stale (label-less) template on a replica increase. The
			// next reconcile pass through HandleVolumeExpansion must label the new PVC
			// alongside the pre-existing ones.
			name: "scale-up: HandleVolumeExpansion labels PVCs created from a stale template",
			args: args{
				expectedSset: func() appsv1.StatefulSet {
					labeled := *sset.DeepCopy()
					labeled.Spec.Replicas = ptr.To[int32](4)
					labeled.Spec.VolumeClaimTemplates[0] = withLabels(sampleClaim, map[string]string{"team": "search"})
					return labeled
				}(),
				// actualSset reflects the persisted StatefulSet after scale-up: 4 replicas
				// but the VCT is still the original label-less template (it is immutable
				// on an existing StatefulSet, see ReconcileStatefulSet).
				actualSset: func() appsv1.StatefulSet {
					stale := *sset.DeepCopy()
					stale.Spec.Replicas = ptr.To[int32](4)
					return stale
				}(),
				validateStorageClass: true,
			},
			runtimeObjs: append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: func() []corev1.PersistentVolumeClaim {
				out := pvcsWithSize("1Gi", "1Gi", "1Gi", "1Gi")
				for i := range out {
					out[i].Labels = map[string]string{"team": "search"}
				}
				return out
			}(),
			wantRecreate: false,
			wantErr:      false,
		},
		{
			// combined storage increase + label change: both propagate to existing PVCs and sset is recreated.
			name: "storage increase with VCT label change propagates both",
			args: args{
				expectedSset: func() appsv1.StatefulSet {
					labeled := *resizedSset.DeepCopy()
					labeled.Spec.VolumeClaimTemplates[0] = withLabels(
						withStorageReq(sampleClaim, "3Gi"),
						map[string]string{"team": "search"},
					)
					return labeled
				}(),
				actualSset:           sset,
				validateStorageClass: true,
			},
			runtimeObjs: append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: func() []corev1.PersistentVolumeClaim {
				out := pvcsWithSize("3Gi", "3Gi", "3Gi")
				for i := range out {
					out[i].Labels = map[string]string{"team": "search"}
				}
				return out
			}(),
			wantRecreate: true,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
				TypeMeta:   metav1.TypeMeta{Kind: esv1.Kind},
			}

			k8sClient := k8s.NewFakeClient(append(tt.runtimeObjs, &es)...)
			recreate, err := HandleVolumeExpansion(context.Background(), k8sClient, &es,
				tt.args.expectedSset, tt.args.actualSset, tt.args.validateStorageClass)
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
			var retrievedES esv1.Elasticsearch
			err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&es), &retrievedES)
			require.NoError(t, err)
			if tt.wantRecreate {
				require.Len(t, retrievedES.Annotations, 1)
				wantUpdatedSset := tt.args.actualSset.DeepCopy()
				// should have the expected claims
				wantUpdatedSset.Spec.VolumeClaimTemplates = tt.args.expectedSset.Spec.VolumeClaimTemplates

				// test ssetsToRecreate along the way
				gvk, err := apiutil.GVKForObject(&retrievedES, clientgoscheme.Scheme)
				if err != nil {
					t.Fatal(err)
				}
				toRecreate, err := ssetsToRecreate(&retrievedES, gvk.Kind)
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

// Test_handleVolumeExpansion_scaleUpSequence simulates the time-ordered scale-up scenario
// flagged in PR review: the first reconcile labels existing PVCs, the StatefulSet
// controller then creates a new PVC from the still-stale (immutable) VCT, and the next
// reconcile pass picks up that new PVC and labels it. This locks in the eventual
// consistency contract documented on ReconcileStatefulSet's UpdateReconciled.
func Test_handleVolumeExpansion_scaleUpSequence(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
		TypeMeta:   metav1.TypeMeta{Kind: esv1.Kind},
	}

	// initial state: 3 replicas, label-less VCT, 3 unlabeled PVCs.
	initialSset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             ptr.To[int32](3),
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim},
		},
	}
	pvc := func(idx int, labels map[string]string) corev1.PersistentVolumeClaim {
		return corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      fmt.Sprintf("sample-claim-sample-sset-%d", idx),
				Labels:    labels,
			},
			Spec: sampleClaim.Spec,
		}
	}
	initialPVCs := []corev1.PersistentVolumeClaim{pvc(0, nil), pvc(1, nil), pvc(2, nil)}

	k8sClient := k8s.NewFakeClient(
		&es,
		withVolumeExpansion(sampleStorageClass),
		&initialPVCs[0], &initialPVCs[1], &initialPVCs[2],
	)

	// === Pass 1 ===
	// User adds a label to the VCT. ReconcileStatefulSet preserves the existing VCT in
	// the apiserver but HandleVolumeExpansion still labels the existing PVCs.
	expectedSset := *initialSset.DeepCopy()
	expectedSset.Spec.VolumeClaimTemplates[0] = withLabels(sampleClaim, map[string]string{"team": "search"})

	recreate, err := HandleVolumeExpansion(context.Background(), k8sClient, &es, expectedSset, initialSset, true)
	require.NoError(t, err)
	require.False(t, recreate)

	for i := range 3 {
		var got corev1.PersistentVolumeClaim
		require.NoError(t, k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&initialPVCs[i]), &got))
		require.Equal(t, map[string]string{"team": "search"}, got.Labels, "existing PVC %d should be labeled after pass 1", i)
	}

	// === Time passes, then the StatefulSet is scaled up to 4 replicas ===
	// ReconcileStatefulSet preserves the immutable label-less VCT on the apiserver, so
	// the StatefulSet controller creates the 4th PVC from the stale template (no label).
	scaledSset := *initialSset.DeepCopy()
	scaledSset.Spec.Replicas = ptr.To[int32](4)
	freshPVC := pvc(3, nil)
	require.NoError(t, k8sClient.Create(context.Background(), &freshPVC))

	// === Pass 2 ===
	// On the next reconcile pass, expectedSset reflects the new replica count and the
	// labeled VCT; HandleVolumeExpansion must label the freshly-created PVC.
	expectedSset.Spec.Replicas = ptr.To[int32](4)
	recreate, err = HandleVolumeExpansion(context.Background(), k8sClient, &es, expectedSset, scaledSset, true)
	require.NoError(t, err)
	require.False(t, recreate)

	var allPVCs corev1.PersistentVolumeClaimList
	require.NoError(t, k8sClient.List(context.Background(), &allPVCs))
	require.Len(t, allPVCs.Items, 4)
	for i := range allPVCs.Items {
		require.Equal(t, map[string]string{"team": "search"}, allPVCs.Items[i].Labels,
			"PVC %s should be labeled after pass 2 (eventual consistency)", allPVCs.Items[i].Name)
	}
}

// Test_handleVolumeExpansion_labelRemovalAndRename locks in the additive-only contract
// documented on syncPVCLabels: removing a label from the VCT does not remove it from
// existing PVCs, and renaming a label adds the new key without removing the old one.
// This guards against silent product behavior changes if the additive-only design ever
// shifts to full sync.
func Test_handleVolumeExpansion_labelRemovalAndRename(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
		TypeMeta:   metav1.TypeMeta{Kind: esv1.Kind},
	}
	initialSset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             ptr.To[int32](2),
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim},
		},
	}
	pvc := func(idx int) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      fmt.Sprintf("sample-claim-sample-sset-%d", idx),
			},
			Spec: sampleClaim.Spec,
		}
	}

	k8sClient := k8s.NewFakeClient(&es, withVolumeExpansion(sampleStorageClass), pvc(0), pvc(1))

	// === Pass 1: add label "team=search" ===
	step1 := *initialSset.DeepCopy()
	step1.Spec.VolumeClaimTemplates[0] = withLabels(sampleClaim, map[string]string{"team": "search"})
	_, err := HandleVolumeExpansion(context.Background(), k8sClient, &es, step1, initialSset, true)
	require.NoError(t, err)

	// === Pass 2: remove "team" entirely ===
	// actualSset is now step1 (the VCT was previously updated logically; in reality the
	// apiserver VCT stays stale because of immutability, but HandleVolumeExpansion
	// compares storage only, so what we pass as actual does not matter for labels).
	step2 := *initialSset.DeepCopy() // VCT has no labels
	_, err = HandleVolumeExpansion(context.Background(), k8sClient, &es, step2, step1, true)
	require.NoError(t, err)

	// PVCs should retain "team=search" — additive-only, removal does not propagate.
	for i := range 2 {
		var got corev1.PersistentVolumeClaim
		require.NoError(t, k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(pvc(i)), &got))
		require.Equal(t, map[string]string{"team": "search"}, got.Labels,
			"PVC %d must retain stale label after VCT label removal", i)
	}

	// === Pass 3: rename to "squad=search" (different key entirely) ===
	step3 := *initialSset.DeepCopy()
	step3.Spec.VolumeClaimTemplates[0] = withLabels(sampleClaim, map[string]string{"squad": "search"})
	_, err = HandleVolumeExpansion(context.Background(), k8sClient, &es, step3, step2, true)
	require.NoError(t, err)

	// PVCs should now carry BOTH the stale "team" key and the new "squad" key — the
	// rename adds the new key but does not remove the old one.
	for i := range 2 {
		var got corev1.PersistentVolumeClaim
		require.NoError(t, k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(pvc(i)), &got))
		require.Equal(t, map[string]string{"team": "search", "squad": "search"}, got.Labels,
			"PVC %d must carry both stale and new label after rename (additive-only)", i)
	}
}

func Test_handleVolumeExpansionLogstash(t *testing.T) {
	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             ptr.To[int32](3),
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim},
		},
	}
	resizedSset := *sset.DeepCopy()
	resizedSset.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("3Gi")
	pvcsWithSize := func(size ...string) []corev1.PersistentVolumeClaim {
		pvcs := make([]corev1.PersistentVolumeClaim, 0, len(size))
		for i, s := range size {
			pvcs = append(pvcs, withStorageReq(corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: fmt.Sprintf("sample-claim-sample-sset-%d", i)},
				Spec:       sampleClaim.Spec,
			}, s))
		}
		return pvcs
	}
	pvcPtrs := func(pvcs []corev1.PersistentVolumeClaim) []client.Object {
		ptrs := make([]client.Object, 0, len(pvcs))
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
		{
			// label-only VCT change: no storage diff, no recreate, but PVCs should pick up the new labels.
			name: "label-only VCT change is propagated to existing PVCs",
			args: args{
				expectedSset: func() appsv1.StatefulSet {
					labeled := *sset.DeepCopy()
					labeled.Spec.VolumeClaimTemplates[0] = withLabels(sampleClaim, map[string]string{"team": "search"})
					return labeled
				}(),
				actualSset:           sset,
				validateStorageClass: true,
			},
			runtimeObjs: append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: func() []corev1.PersistentVolumeClaim {
				out := pvcsWithSize("1Gi", "1Gi", "1Gi")
				for i := range out {
					out[i].Labels = map[string]string{"team": "search"}
				}
				return out
			}(),
			wantRecreate: false,
			wantErr:      false,
		},
		{
			// combined storage increase + label change: both propagate to existing PVCs and sset is recreated.
			name: "storage increase with VCT label change propagates both",
			args: args{
				expectedSset: func() appsv1.StatefulSet {
					labeled := *resizedSset.DeepCopy()
					labeled.Spec.VolumeClaimTemplates[0] = withLabels(
						withStorageReq(sampleClaim, "3Gi"),
						map[string]string{"team": "search"},
					)
					return labeled
				}(),
				actualSset:           sset,
				validateStorageClass: true,
			},
			runtimeObjs: append(pvcPtrs(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: func() []corev1.PersistentVolumeClaim {
				out := pvcsWithSize("3Gi", "3Gi", "3Gi")
				for i := range out {
					out[i].Labels = map[string]string{"team": "search"}
				}
				return out
			}(),
			wantRecreate: true,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ls"},
				TypeMeta:   metav1.TypeMeta{Kind: logstashv1alpha1.Kind}}
			k8sClient := k8s.NewFakeClient(append(tt.runtimeObjs, &ls)...)
			recreate, err := HandleVolumeExpansion(context.Background(), k8sClient, &ls,
				tt.args.expectedSset, tt.args.actualSset, tt.args.validateStorageClass)
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

			// Logstash should be annotated with the sset to recreate
			var retrievedLS logstashv1alpha1.Logstash
			err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&ls), &retrievedLS)
			require.NoError(t, err)
			if tt.wantRecreate {
				require.Len(t, retrievedLS.Annotations, 1)
				wantUpdatedSset := tt.args.actualSset.DeepCopy()
				// should have the expected claims
				wantUpdatedSset.Spec.VolumeClaimTemplates = tt.args.expectedSset.Spec.VolumeClaimTemplates

				// test ssetsToRecreate along the way
				gvk, err := apiutil.GVKForObject(&retrievedLS, clientgoscheme.Scheme)
				if err != nil {
					t.Fatal(err)
				}
				toRecreate, err := ssetsToRecreate(&retrievedLS, gvk.Kind)
				require.NoError(t, err)
				require.Equal(t,
					map[string]appsv1.StatefulSet{
						"logstash.k8s.elastic.co/recreate-" + tt.args.actualSset.Name: *wantUpdatedSset},
					toRecreate)
			} else {
				require.Empty(t, retrievedLS.Annotations)
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
	es := func() *esv1.Elasticsearch {
		return &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es", UID: "es-uid"}, TypeMeta: metav1.TypeMeta{Kind: esv1.Kind}}
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
	require.NoError(t, controllerutil.SetOwnerReference(es(), pod1WithOwnerRef, clientgoscheme.Scheme))

	sset2 := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset2", UID: "sset2-uid"}}
	sset2Bytes, _ := json.Marshal(sset2)
	sset2JSON := string(sset2Bytes)

	type args struct {
		runtimeObjs []client.Object
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
				runtimeObjs: []client.Object{sset1, pod1},
				es:          *es(),
			},
			wantES:          *es(),
			wantPods:        []corev1.Pod{*pod1},
			wantRecreations: 0,
		},
		{
			name: "StatefulSet to delete",
			args: args{
				runtimeObjs: []client.Object{sset1, pod1}, // sset exists with the same UID
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
				runtimeObjs: []client.Object{pod1}, // sset doesn't exist
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
				runtimeObjs: []client.Object{sset1DifferentUID, pod1WithOwnerRef}, // sset recreated
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
				runtimeObjs: []client.Object{sset1, sset2, pod1},
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
				runtimeObjs: []client.Object{sset1DifferentUID, pod1}, // sset recreated
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
			es := tt.args.es
			k8sClient := k8s.NewFakeClient(append(tt.args.runtimeObjs, &es)...)

			got, err := RecreateStatefulSets(context.Background(), k8sClient, &es)
			require.NoError(t, err)
			require.Equal(t, tt.wantRecreations, got)

			var retrievedES esv1.Elasticsearch
			err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&es), &retrievedES)
			require.NoError(t, err)
			comparison.RequireEqual(t, &tt.wantES, &retrievedES)

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
	sampleEs = esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es", UID: "es-uid"}, TypeMeta: metav1.TypeMeta{Kind: esv1.Kind}}
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
	if err := controllerutil.SetOwnerReference(&sampleEs, &pod1WithOwnerRef, clientgoscheme.Scheme); err != nil {
		panic(err)
	}
	if err := controllerutil.SetOwnerReference(&sampleEs, &pod2WithOwnerRef, clientgoscheme.Scheme); err != nil {
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
				k8sClient:   k8s.NewFakeClient(&pod1, &pod2),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1WithOwnerRef, pod2WithOwnerRef},
		},
		{
			name: "owner ref already set: the function is idempotent",
			args: args{
				k8sClient:   k8s.NewFakeClient(&pod1WithOwnerRef, &pod2WithOwnerRef),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1WithOwnerRef, pod2WithOwnerRef},
		},
		{
			name: "one owner ref already set, one missing",
			args: args{
				k8sClient:   k8s.NewFakeClient(&pod1WithOwnerRef, &pod2),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1WithOwnerRef, pod2WithOwnerRef},
		},
		{
			name: "no Pods: nothing to do",
			args: args{
				k8sClient:   k8s.NewFakeClient(),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := tt.args.es
			err := updatePodOwners(context.Background(), tt.args.k8sClient, &es, es.Kind, tt.args.statefulSet)
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

func Test_removePodOwner(t *testing.T) {
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
				k8sClient:   k8s.NewFakeClient(&pod1WithOwnerRef, &pod2WithOwnerRef),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, pod2},
		},
		{
			name: "owner refs already removed: function is idempotent",
			args: args{
				k8sClient:   k8s.NewFakeClient(&pod1, &pod2),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, pod2},
		},
		{
			name: "one owner ref already removed, the other not yet removed",
			args: args{
				k8sClient:   k8s.NewFakeClient(&pod1WithOwnerRef, &pod2),
				es:          sampleEs,
				statefulSet: sset1,
			},
			wantPods: []corev1.Pod{pod1, pod2},
		},
		{
			name: "no Pods: nothing to do",
			args: args{
				k8sClient:   k8s.NewFakeClient(),
				es:          sampleEs,
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
			es := tt.args.es
			err := removePodOwner(context.Background(), tt.args.k8sClient, &es, es.Kind, tt.args.statefulSet)
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
