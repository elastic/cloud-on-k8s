// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"reflect"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
)

func Test_checkNodeSetNameUniqueness(t *testing.T) {
	type args struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}
	tests := []args{
		{
			name: "several duplicate nodeSets",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.4.0",
					NodeSets: []esv1.NodeSet{
						{Name: "foo", Count: 1},
						{Name: "foo", Count: 1},
						{Name: "bar", Count: 1},
						{Name: "bar", Count: 1},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "good spec with 1 nodeSet",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []esv1.NodeSet{{Name: "foo", Count: 1}},
				},
			},
			expectErrors: false,
		},
		{
			name: "good spec with 2 nodeSets",
			es: esv1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1"},
				Spec: esv1.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []esv1.NodeSet{{Name: "foo", Count: 1}, {Name: "bar", Count: 1}},
				},
			},
			expectErrors: false,
		},
		{
			name: "duplicate nodeSet",
			es: esv1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1"},
				Spec: esv1.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []esv1.NodeSet{{Name: "foo", Count: 1}, {Name: "foo", Count: 1}},
				},
			},
			expectErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := checkNodeSetNameUniqueness(tt.es)
			actualErrors := len(actual) > 0

			if tt.expectErrors != actualErrors {
				t.Errorf("failed checkNodeSetNameUniqueness(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec.NodeSets)
			}
		})
	}
}

func Test_hasCorrectNodeRoles(t *testing.T) {
	type m map[string]interface{}

	esWithRoles := func(version string, count int32, nodeSetRoles ...m) esv1.Elasticsearch {
		x := es(version)
		for _, nsc := range nodeSetRoles {
			data := nsc
			var cfg *commonv1.Config
			if data != nil {
				cfg = &commonv1.Config{Data: data}
			}

			x.Spec.NodeSets = append(x.Spec.NodeSets, esv1.NodeSet{
				Count:  count,
				Config: cfg,
			})
		}

		return x
	}

	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name:         "no topology",
			es:           esWithRoles("6.8.0", 1),
			expectErrors: true,
		},
		{
			name:         "one nodeset with no config",
			es:           esWithRoles("7.6.0", 1, nil),
			expectErrors: false,
		},
		{
			name:         "no master defined (node attributes)",
			es:           esWithRoles("7.6.0", 1, m{esv1.NodeMaster: "false", esv1.NodeData: "true"}, m{esv1.NodeMaster: "true", esv1.NodeVotingOnly: "true"}),
			expectErrors: true,
		},
		{
			name:         "no master defined (node roles)",
			es:           esWithRoles("7.9.0", 1, m{esv1.NodeRoles: []string{esv1.DataRole}}, m{esv1.NodeRoles: []string{esv1.MasterRole, esv1.VotingOnlyRole}}),
			expectErrors: true,
		},
		{
			name:         "zero master nodes (node attributes)",
			es:           esWithRoles("7.6.0", 0, m{esv1.NodeMaster: "true", esv1.NodeData: "true"}, m{esv1.NodeData: "true"}),
			expectErrors: true,
		},
		{
			name:         "zero master nodes (node roles)",
			es:           esWithRoles("7.9.0", 0, m{esv1.NodeRoles: []string{esv1.MasterRole, esv1.DataRole}}, m{esv1.NodeRoles: []string{esv1.DataRole}}),
			expectErrors: true,
		},
		{
			name:         "mixed node attributes and node roles",
			es:           esWithRoles("7.9.0", 1, m{esv1.NodeMaster: "true", esv1.NodeRoles: []string{esv1.DataRole}}, m{esv1.NodeRoles: []string{esv1.DataRole, esv1.TransformRole}}),
			expectErrors: true,
		},
		{
			name:         "node roles on older version",
			es:           esWithRoles("7.6.0", 1, m{esv1.NodeRoles: []string{esv1.MasterRole}}, m{esv1.NodeRoles: []string{esv1.DataRole}}),
			expectErrors: true,
		},
		{
			name: "valid configuration (node attributes)",
			es:   esWithRoles("7.6.0", 3, m{esv1.NodeMaster: "true", esv1.NodeData: "true"}, m{esv1.NodeData: "true"}),
		},
		{
			name: "valid configuration (node roles)",
			es:   esWithRoles("7.9.0", 3, m{esv1.NodeRoles: []string{esv1.MasterRole, esv1.DataRole}}, m{esv1.NodeRoles: []string{esv1.DataRole}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCorrectNodeRoles(tt.es)
			hasErrors := len(result) > 0
			if tt.expectErrors != hasErrors {
				t.Errorf("expectedErrors=%t hasErrors=%t result=%+v", tt.expectErrors, hasErrors, result)
			}
		})
	}
}

func Test_supportedVersion(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name: "unsupported minor version should fail",
			es:   es("6.0.0"),

			expectErrors: true,
		},
		{
			name:         "unsupported major should fail",
			es:           es("1.0.0"),
			expectErrors: true,
		},
		{
			name:         "supported OK",
			es:           es("6.8.0"),
			expectErrors: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := supportedVersion(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed supportedVersion(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec.Version)
			}
		})
	}
}

func Test_validName(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name: "name length too long",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "that-is-a-very-long-name-with-37chars",
				},
			},
			expectErrors: true,
		},
		{
			name: "name length OK",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "that-is-a-very-long-name-with-36char",
				},
			},
			expectErrors: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := validName(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed validName(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Name)
			}
		})
	}
}

func Test_validSanIP(t *testing.T) {
	validIP := "3.4.5.6"
	validIP2 := "192.168.12.13"
	validIPv6 := "2001:db8:0:85a3:0:0:ac1f:8001"
	invalidIP := "notanip"

	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name: "no SAN IP: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{},
			},
			expectErrors: false,
		},
		{
			name: "valid SAN IPs: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					HTTP: commonv1.HTTPConfig{
						TLS: commonv1.TLSOptions{
							SelfSignedCertificate: &commonv1.SelfSignedCertificate{
								SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
									{
										IP: validIP,
									},
									{
										IP: validIP2,
									},
									{
										IP: validIPv6,
									},
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "invalid SAN IPs: NOT OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					HTTP: commonv1.HTTPConfig{
						TLS: commonv1.TLSOptions{
							SelfSignedCertificate: &commonv1.SelfSignedCertificate{
								SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
									{
										IP: invalidIP,
									},
									{
										IP: validIP2,
									},
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := validSanIP(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed validSanIP(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec)
			}
		})
	}
}

func TestValidation_noDowngrades(t *testing.T) {
	tests := []struct {
		name         string
		current      esv1.Elasticsearch
		proposed     esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name:         "prevent downgrade",
			current:      es("2.0.0"),
			proposed:     es("1.0.0"),
			expectErrors: true,
		},
		{
			name:         "allow upgrades",
			current:      es("1.0.0"),
			proposed:     es("1.2.0"),
			expectErrors: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := noDowngrades(tt.current, tt.proposed)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed noDowngrades(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.proposed)
			}
		})
	}
}

func Test_validUpgradePath(t *testing.T) {
	tests := []struct {
		name         string
		current      esv1.Elasticsearch
		proposed     esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name:     "unsupported version rejected",
			current:  es("1.0.0"),
			proposed: es("2.0.0"),

			expectErrors: true,
		},
		{
			name:         "too old version rejected",
			current:      es("6.5.0"),
			proposed:     es("7.0.0"),
			expectErrors: true,
		},
		{
			name:         "too new rejected",
			current:      es("7.0.0"),
			proposed:     es("6.5.0"),
			expectErrors: true,
		},
		{
			name:         "in range accepted",
			current:      es("6.8.0"),
			proposed:     es("7.1.0"),
			expectErrors: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := validUpgradePath(tt.current, tt.proposed)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed validUpgradePath(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.proposed)
			}
		})
	}
}

func Test_noUnknownFields(t *testing.T) {
	GetEsWithLastApplied := func(lastApplied string) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					corev1.LastAppliedConfigAnnotation: lastApplied,
				},
			},
		}
	}

	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		errorOnField string
	}{
		{
			name: "good annotation",
			es: GetEsWithLastApplied(
				`{"apiVersion":"elasticsearch.k8s.elastic.co/v1","kind":"Elasticsearch"` +
					`,"metadata":{"annotations":{},"name":"quickstart","namespace":"default"},` +
					`"spec":{"nodeSets":[{"config":{"node.store.allow_mmap":false},"count":1,` +
					`"name":"default"}],"version":"7.5.1"}}`),
		},
		{
			name: "no annotation",
			es:   esv1.Elasticsearch{},
		},
		{
			name: "bad annotation",
			es: GetEsWithLastApplied(
				`{"apiVersion":"elasticsearch.k8s.elastic.co/v1","kind":"Elasticsearch"` +
					`,"metadata":{"annotations":{},"name":"quickstart","namespace":"default"},` +
					`"spec":{"nodeSets":[{"config":{"node.store.allow_mmap":false},"count":1,` +
					`"name":"default","wrongthing":true}],"version":"7.5.1"}}`),
			errorOnField: "wrongthing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := noUnknownFields(tt.es)
			actualErrors := len(actual) > 0
			expectErrors := tt.errorOnField != ""
			if expectErrors != actualErrors || (actualErrors && actual[0].Field != tt.errorOnField) {
				t.Errorf(
					"failed NoUnknownFields(). Name: %v, actual %v, wanted error on field: %v, es value: %v",
					tt.name,
					actual,
					tt.errorOnField,
					tt.es)
			}
		})
	}
}

// es returns an es fixture at a given version
func es(v string) esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
		},
		Spec: esv1.ElasticsearchSpec{Version: v},
	}
}

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
			name:                "storage class validation disabled: no-op",
			k8sClient:           k8s.WrappedFakeClient(&sampleStorageClass), // would otherwise be refused: no expansion
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
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2},
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "one claim removed: error",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim},
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "one claim added: error",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2, sampleClaim2},
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "one claim modified: error",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim},
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "storage increase: ok",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim, "3Gi")},
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "storage increase but volume expansion not supported: error",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(&sampleStorageClass),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim, "3Gi")},
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "storage increase, volume expansion not supported, but no storage class check: ok",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(&sampleStorageClass),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim, "3Gi")},
				validateStorageClass: false,
			},
			wantErr: false,
		},
		{
			name: "storage decrease: error",
			args: args{
				k8sClient:            k8s.WrappedFakeClient(withVolumeExpansion(sampleStorageClass)),
				initial:              []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim},
				updated:              []corev1.PersistentVolumeClaim{sampleClaim, withStorageReq(sampleClaim, "0.5Gi")},
				validateStorageClass: true,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateClaimsUpdate(tt.args.k8sClient, tt.args.initial, tt.args.updated, tt.args.validateStorageClass); (err != nil) != tt.wantErr {
				t.Errorf("ValidateClaimsUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_pvcModification(t *testing.T) {
	es := func(nodeSets []esv1.NodeSet) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster"},
			Spec:       esv1.ElasticsearchSpec{NodeSets: nodeSets},
		}
	}
	type args struct {
		proposed             esv1.Elasticsearch
		k8sClient            k8s.Client
		validateStorageClass bool
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			name: "no changes in the claims: ok",
			args: args{
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				k8sClient: k8s.WrappedFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			want: nil,
		},
		{
			name: "statefulSet does not exist: ok",
			args: args{
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				k8sClient: k8s.WrappedFakeClient(),
			},
			want: nil,
		},
		{
			name: "invalid claim update (one less claim): error", // other cases are checked in TestValidateClaimsUpdate
			args: args{
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				k8sClient: k8s.WrappedFakeClient(),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pvcModification(tt.args.proposed, tt.args.k8sClient, tt.args.validateStorageClass); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("pvcModification() = %v, want %v", got, tt.want)
			}
		})
	}
}
