// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"reflect"
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
	"k8s.io/utils/pointer"
)

var (
	sampleStorageClass = storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name: "sample-sc"}}
	defaultStorageClass = storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "default-sc",
			Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}}
	defaultBetaStorageClass = storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name:        "default-beta-sc",
		Annotations: map[string]string{"storageclass.beta.kubernetes.io/is-default-class": "true"}}}

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
)

func withVolumeExpansion(sc storagev1.StorageClass) *storagev1.StorageClass {
	sc.AllowVolumeExpansion = pointer.BoolPtr(true)
	return &sc
}

func withStorageReq(claim corev1.PersistentVolumeClaim, size string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(size)
	return *c
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
			k8sClient:           k8s.NewFakeClient(withVolumeExpansion(sampleStorageClass)),
			claim:               sampleClaim,
			validateStoragClass: true,
			wantErr:             false,
		},
		{
			name:                "specified storage class does not support volume expansion",
			k8sClient:           k8s.NewFakeClient(&sampleStorageClass),
			claim:               sampleClaim,
			validateStoragClass: true,
			wantErr:             true,
		},
		{
			name:                "default storage class supports volume expansion",
			k8sClient:           k8s.NewFakeClient(withVolumeExpansion(defaultStorageClass)),
			claim:               corev1.PersistentVolumeClaim{},
			validateStoragClass: true,
			wantErr:             false,
		},
		{
			name:                "default storage class does not support volume expansion",
			k8sClient:           k8s.NewFakeClient(&defaultStorageClass),
			claim:               corev1.PersistentVolumeClaim{},
			validateStoragClass: true,
			wantErr:             true,
		},
		{
			name:                "storage class validation disabled: no-op",
			k8sClient:           k8s.NewFakeClient(&sampleStorageClass), // would otherwise be refused: no expansion
			claim:               sampleClaim,
			validateStoragClass: false,
			wantErr:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := EnsureClaimSupportsExpansion(tt.k8sClient, tt.claim, tt.validateStoragClass); (err != nil) != tt.wantErr {
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
			k8sClient: k8s.NewFakeClient(&sampleStorageClass, &defaultStorageClass),
			want:      defaultStorageClass,
		},
		{
			name:      "default storage class not found",
			k8sClient: k8s.NewFakeClient(&sampleStorageClass),
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
			k8sClient: k8s.NewFakeClient(&sampleStorageClass, &defaultStorageClass),
			claim:     corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: pointer.StringPtr(sampleStorageClass.Name)}},
			want:      sampleStorageClass,
			wantErr:   false,
		},
		{
			name:      "error out if not found",
			k8sClient: k8s.NewFakeClient(&defaultStorageClass),
			claim:     corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: pointer.StringPtr(sampleStorageClass.Name)}},
			want:      storagev1.StorageClass{},
			wantErr:   true,
		},
		{
			name:      "fallback to the default storage class if unspecified",
			k8sClient: k8s.NewFakeClient(&sampleStorageClass, &defaultStorageClass),
			claim:     corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{}},
			want:      defaultStorageClass,
			wantErr:   false,
		},
		{
			name:      "error out if unspecified and default storage class not found",
			k8sClient: k8s.NewFakeClient(&sampleStorageClass),
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

func TestValidateClaimsUpdate(t *testing.T) {
	type args struct {
		k8sClient            k8s.Client
		initial              []corev1.PersistentVolumeClaim
		updated              []corev1.PersistentVolumeClaim
		validateStorageClass bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "same claims: ok",
			args: args{
				k8sClient:            k8s.NewFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2},
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "no claims: ok",
			args: args{
				k8sClient:            k8s.NewFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              nil,
				updated:              nil,
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "claim in updated does not exist in initial: error",
			args: args{
				k8sClient:            k8s.NewFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2},
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "storage increase: ok",
			args: args{
				k8sClient:            k8s.NewFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim, "3Gi")},
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "storage increase but volume expansion not supported: error",
			args: args{
				k8sClient:            k8s.NewFakeClient(&sampleStorageClass),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim, "3Gi")},
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "storage increase, volume expansion not supported, but no storage class check: ok",
			args: args{
				k8sClient:            k8s.NewFakeClient(&sampleStorageClass),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim, "3Gi")},
				validateStorageClass: false,
			},
			wantErr: false,
		},
		{
			name: "storage decrease: error",
			args: args{
				k8sClient:            k8s.NewFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim, "0.5Gi")},
				validateStorageClass: true,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateClaimsStorageUpdate(tt.args.k8sClient, tt.args.initial, tt.args.updated, tt.args.validateStorageClass); (err != nil) != tt.wantErr {
				t.Errorf("ValidateClaimsStorageUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validPVCModification(t *testing.T) {
	es := func(nodeSets []esv1.NodeSet) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster"},
			Spec:       esv1.ElasticsearchSpec{NodeSets: nodeSets},
		}
	}
	type args struct {
		current              esv1.Elasticsearch
		proposed             esv1.Elasticsearch
		k8sClient            k8s.Client
		validateStorageClass bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no changes in the claims: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "new nodeSet: ok",
			args: args{
				current: es([]esv1.NodeSet{}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				k8sClient:            k8s.NewFakeClient(),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "statefulSet does not exist: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				k8sClient: k8s.NewFakeClient(),
			},
			wantErr: false,
		},
		{
			name: "modified claims (one less) in the proposed Elasticsearch: error",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "modified claims (new name) in the proposed Elasticsearch: error",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim}},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "storage decrease in the proposed elasticsearch vs. existing statefulset: error",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim2, "0.5Gi")}}, // decrease
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "storage decrease in the proposed elasticsearch vs. current elasticsearch, but matches current sset: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim2, "0.5Gi")}}, // revert to previous size
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, withStorageReq(sampleClaim2, "0.5Gi"),
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validPVCModification(tt.args.current, tt.args.proposed, tt.args.k8sClient, tt.args.validateStorageClass)
			if tt.wantErr {
				require.NotEmpty(t, errs)
			} else {
				require.Empty(t, errs)
			}
		})
	}
}
