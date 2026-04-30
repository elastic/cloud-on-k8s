// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validations

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
)

func withVolumeExpansion(sc storagev1.StorageClass) *storagev1.StorageClass {
	sc.AllowVolumeExpansion = ptr.To[bool](true)
	return &sc
}

func withStorageReq(claim corev1.PersistentVolumeClaim, size string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(size)
	return *c
}

func TestIsReservedLabelKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "elasticsearch subdomain reserved", key: "elasticsearch.k8s.elastic.co/cluster-name", want: true},
		{name: "common subdomain reserved", key: "common.k8s.elastic.co/type", want: true},
		{name: "association subdomain reserved", key: "association.k8s.elastic.co/es-conf", want: true},
		{name: "k8s.elastic.co subdomain reserved", key: "k8s.elastic.co/foo", want: true},
		{name: "empty string not reserved", key: "", want: false},
		{name: "no slash not reserved", key: "elasticsearch.k8s.elastic.co", want: false},
		{name: "lookalike suffix not reserved", key: "notk8s.elastic.co/foo", want: false},
		{name: "third-party label not reserved", key: "velero.io/exclude-from-backup", want: false},
		{name: "user label not reserved", key: "team", want: false},
		{name: "user label with prefix not reserved", key: "example.com/team", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReservedLabelKey(tt.key); got != tt.want {
				t.Errorf("IsReservedLabelKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestStripReservedLabelKeys(t *testing.T) {
	claim := func(name string, labels map[string]string) corev1.PersistentVolumeClaim {
		return corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		}
	}
	tests := []struct {
		name string
		in   []corev1.PersistentVolumeClaim
		want []corev1.PersistentVolumeClaim
	}{
		{
			name: "nil input: nil output",
			in:   nil,
			want: nil,
		},
		{
			name: "empty slice: empty slice (input returned unchanged)",
			in:   []corev1.PersistentVolumeClaim{},
			want: []corev1.PersistentVolumeClaim{},
		},
		{
			name: "no labels at all: no-op",
			in:   []corev1.PersistentVolumeClaim{claim("data", nil)},
			want: []corev1.PersistentVolumeClaim{claim("data", nil)},
		},
		{
			name: "only non-reserved labels: no-op (input returned unchanged)",
			in:   []corev1.PersistentVolumeClaim{claim("data", map[string]string{"team": "search", "velero.io/exclude-from-backup": "true"})},
			want: []corev1.PersistentVolumeClaim{claim("data", map[string]string{"team": "search", "velero.io/exclude-from-backup": "true"})},
		},
		{
			name: "single reserved key removed",
			in:   []corev1.PersistentVolumeClaim{claim("data", map[string]string{"common.k8s.elastic.co/type": "evil"})},
			// reserved key removed; resulting empty label map is dropped to nil so the
			// produced StatefulSet is byte-equivalent to one that never carried labels.
			want: []corev1.PersistentVolumeClaim{claim("data", nil)},
		},
		{
			name: "reserved key removed, non-reserved keys preserved",
			in: []corev1.PersistentVolumeClaim{claim("data", map[string]string{
				"team": "search",
				"elasticsearch.k8s.elastic.co/cluster-name": "evil",
				"velero.io/exclude-from-backup":             "true",
			})},
			want: []corev1.PersistentVolumeClaim{claim("data", map[string]string{
				"team":                          "search",
				"velero.io/exclude-from-backup": "true",
			})},
		},
		{
			name: "multiple claims: only the offending one is rewritten",
			in: []corev1.PersistentVolumeClaim{
				claim("clean", map[string]string{"team": "search"}),
				claim("dirty", map[string]string{"common.k8s.elastic.co/type": "evil", "team": "search"}),
			},
			want: []corev1.PersistentVolumeClaim{
				claim("clean", map[string]string{"team": "search"}),
				claim("dirty", map[string]string{"team": "search"}),
			},
		},
		{
			name: "k8s.elastic.co subdomain reserved key removed",
			in:   []corev1.PersistentVolumeClaim{claim("data", map[string]string{"k8s.elastic.co/foo": "bar"})},
			want: []corev1.PersistentVolumeClaim{claim("data", nil)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripReservedLabelKeys(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StripReservedLabelKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripReservedLabelKeys_doesNotMutateInput(t *testing.T) {
	in := []corev1.PersistentVolumeClaim{{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Labels: map[string]string{
			"common.k8s.elastic.co/type": "evil",
			"team":                       "search",
		}},
	}}

	_ = StripReservedLabelKeys(in)

	// input must still carry both labels — defense-in-depth helper deep-copies before mutating.
	if _, ok := in[0].ObjectMeta.Labels["common.k8s.elastic.co/type"]; !ok {
		t.Errorf("StripReservedLabelKeys mutated its input: reserved label was removed from the source")
	}
	if _, ok := in[0].ObjectMeta.Labels["team"]; !ok {
		t.Errorf("StripReservedLabelKeys mutated its input: non-reserved label disappeared from the source")
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
			if err := EnsureClaimSupportsExpansion(context.Background(), tt.k8sClient, tt.claim, tt.validateStoragClass); (err != nil) != tt.wantErr {
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
			sc:   storagev1.StorageClass{AllowVolumeExpansion: ptr.To[bool](true)},
			want: true,
		},
		{
			name: "allow volume expansion: false",
			sc:   storagev1.StorageClass{AllowVolumeExpansion: ptr.To[bool](false)},
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
			claim:     corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: ptr.To[string](sampleStorageClass.Name)}},
			want:      sampleStorageClass,
			wantErr:   false,
		},
		{
			name:      "error out if not found",
			k8sClient: k8s.NewFakeClient(&defaultStorageClass),
			claim:     corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: ptr.To[string](sampleStorageClass.Name)}},
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
		{
			// label-only diff is invisible to storage validation: changing labels (with no
			// storage diff) must not regress into a storage validation error.
			name: "label-only change with no storage diff: ok",
			args: args{
				k8sClient: k8s.NewFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:   []corev1.PersistentVolumeClaim{sampleClaim},
				updated: func() []corev1.PersistentVolumeClaim {
					c := *sampleClaim.DeepCopy()
					c.Labels = map[string]string{"team": "search"}
					return []corev1.PersistentVolumeClaim{c}
				}(),
				validateStorageClass: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateClaimsStorageUpdate(context.Background(), tt.args.k8sClient, tt.args.initial, tt.args.updated, tt.args.validateStorageClass); (err != nil) != tt.wantErr {
				t.Errorf("ValidateClaimsStorageUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClaimsWithoutAdjustableFields_nilRequestsDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ClaimsWithoutAdjustableFields panicked: %v", r)
		}
	}()
	claims := []corev1.PersistentVolumeClaim{{ObjectMeta: metav1.ObjectMeta{Name: "data"}}}
	_ = ClaimsWithoutAdjustableFields(claims)
}

func TestValidateReservedLabelsOnCreate(t *testing.T) {
	templatesPath := field.NewPath("spec").Child("volumeClaimTemplates")
	claim := func(name string, labels map[string]string) corev1.PersistentVolumeClaim {
		return corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
	}
	tests := []struct {
		name     string
		proposed []corev1.PersistentVolumeClaim
		want     int
	}{
		{name: "no labels", proposed: []corev1.PersistentVolumeClaim{claim("data", nil)}, want: 0},
		{name: "non-reserved label", proposed: []corev1.PersistentVolumeClaim{claim("data", map[string]string{"team": "a"})}, want: 0},
		{name: "reserved label", proposed: []corev1.PersistentVolumeClaim{claim("data", map[string]string{"common.k8s.elastic.co/type": "evil"})}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateReservedLabelsOnCreate(tt.proposed, templatesPath)
			if len(errs) != tt.want {
				t.Fatalf("len(errs)=%d want %d", len(errs), tt.want)
			}
		})
	}
}

func TestValidateReservedLabelsOnUpdate(t *testing.T) {
	templatesPath := field.NewPath("spec").Child("volumeClaimTemplates")
	data := func(labels map[string]string) corev1.PersistentVolumeClaim {
		return corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Labels: labels}}
	}
	tests := []struct {
		name     string
		current  []corev1.PersistentVolumeClaim
		proposed []corev1.PersistentVolumeClaim
		want     int
	}{
		{
			name:     "grandfather unchanged reserved",
			current:  []corev1.PersistentVolumeClaim{data(map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})},
			proposed: []corev1.PersistentVolumeClaim{data(map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})},
			want:     0,
		},
		{
			name:     "new reserved key",
			current:  []corev1.PersistentVolumeClaim{data(nil)},
			proposed: []corev1.PersistentVolumeClaim{data(map[string]string{"common.k8s.elastic.co/type": "x"})},
			want:     1,
		},
		{
			name:     "change reserved value",
			current:  []corev1.PersistentVolumeClaim{data(map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})},
			proposed: []corev1.PersistentVolumeClaim{data(map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "new"})},
			want:     1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateReservedLabelsOnUpdate(tt.current, tt.proposed, templatesPath)
			if len(errs) != tt.want {
				t.Fatalf("len(errs)=%d want %d: %v", len(errs), tt.want, errs)
			}
		})
	}
}

func TestClaimMatchingName(t *testing.T) {
	claims := []corev1.PersistentVolumeClaim{
		{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
	}
	if ClaimMatchingName(claims, "a") == nil {
		t.Fatal("expected claim a")
	}
	if ClaimMatchingName(claims, "missing") != nil {
		t.Fatal("expected nil for missing")
	}
	if ClaimMatchingName(nil, "a") != nil {
		t.Fatal("nil slice should yield nil")
	}
}
