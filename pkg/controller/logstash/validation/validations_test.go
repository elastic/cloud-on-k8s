// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestCheckNameLength(t *testing.T) {
	testCases := []struct {
		name         string
		logstashName string
		wantErr      bool
		wantErrMsg   string
	}{
		{
			name:         "valid configuration",
			logstashName: "test-logstash",
			wantErr:      false,
		},
		{
			name:         "long Logstash name",
			logstashName: "extremely-long-winded-and-unnecessary-name-for-logstash",
			wantErr:      true,
			wantErrMsg:   "name exceeds maximum allowed length",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ls := lsv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.logstashName,
					Namespace: "test",
				},
				Spec: lsv1alpha1.LogstashSpec{},
			}

			errList := checkNameLength(&ls)
			assert.Equal(t, tc.wantErr, len(errList) > 0)
		})
	}
}

func TestCheckNoUnknownFields(t *testing.T) {
	type args struct {
		prev *lsv1alpha1.Logstash
		curr *lsv1alpha1.Logstash
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			name: "No downgrade",
			args: args{
				prev: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "7.17.0"}},
				curr: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.6.1"}},
			},
			want: nil,
		},
		{
			name: "Downgrade NOK",
			args: args{
				prev: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.6.1"}},
				curr: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.5.0"}},
			},
			want: field.ErrorList{&field.Error{Type: field.ErrorTypeForbidden, Field: "spec.version", BadValue: "", Detail: "Version downgrades are not supported"}},
		},
		{
			name: "Downgrade with override OK",
			args: args{
				prev: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.6.1"}},
				curr: &lsv1alpha1.Logstash{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
					commonv1.DisableDowngradeValidationAnnotation: "true",
				}}, Spec: lsv1alpha1.LogstashSpec{Version: "8.5.0"}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, checkNoDowngrade(tt.args.prev, tt.args.curr), "checkNoDowngrade(%v, %v)", tt.args.prev, tt.args.curr)
		})
	}
}

func Test_checkSingleConfigSource(t *testing.T) {
	tests := []struct {
		name     string
		logstash lsv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name: "configRef absent, config present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Config: &commonv1.Config{},
				},
			},
			wantErr: false,
		},
		{
			name: "config absent, configRef present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					ConfigRef: &commonv1.ConfigSource{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{},
			},
			wantErr: false,
		},
		{
			name: "both present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Config:    &commonv1.Config{},
					ConfigRef: &commonv1.ConfigSource{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSingleConfigSource(&tc.logstash)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkSinglePipelineSource(t *testing.T) {
	tests := []struct {
		name     string
		logstash lsv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name: "pipelinesRef absent, pipelines present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Pipelines: []commonv1.Config{},
				},
			},
			wantErr: false,
		},
		{
			name: "pipelines absent, pipelinesRef present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					PipelinesRef: &commonv1.ConfigSource{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{},
			},
			wantErr: false,
		},
		{
			name: "both present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Pipelines:    []commonv1.Config{},
					PipelinesRef: &commonv1.ConfigSource{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSinglePipelineSource(&tc.logstash)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkSupportedVersion(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "below min supported",
			version: "8.5.0",
			wantErr: true,
		},
		{
			name:    "above max supported",
			version: "42.0.0",
			wantErr: true,
		},
		{
			name:    "above min supported",
			version: "8.12.0",
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Version: tt.version,
				},
			}
			got := checkSupportedVersion(&a)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkEsRefsAssociations(t *testing.T) {
	type args struct {
		b *lsv1alpha1.Logstash
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &lsv1alpha1.Logstash{},
			},
			wantErr: false,
		},
		{
			name: "mix secret named and named refs: OK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "bla"}},
								ClusterName:           "test",
							},
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "test2",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "secret named ref with a name: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "bla", Name: "bla"}},
								ClusterName:           "test",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no name or secret name with namespace: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Namespace: "blub"}},
								ClusterName:           "test",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no name or secret name with serviceName: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{ServiceName: "ble"}},
								ClusterName:           "test",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkAssociations(tt.args.b)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkPVCchanges(t *testing.T) {
	storageClass := storagev1.StorageClass{
		ObjectMeta:           metav1.ObjectMeta{Name: "sample-sc"},
		AllowVolumeExpansion: ptr.To[bool](true),
	}
	claim := func(name, size string, labels map[string]string) corev1.PersistentVolumeClaim {
		c := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To[string](storageClass.Name),
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(size)},
				},
			},
		}
		return c
	}
	ls := func(claims ...corev1.PersistentVolumeClaim) *lsv1alpha1.Logstash {
		return &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{VolumeClaimTemplates: claims}}
	}

	tests := []struct {
		name      string
		current   *lsv1alpha1.Logstash
		proposed  *lsv1alpha1.Logstash
		k8sClient k8s.Client
		wantErr   bool
	}{
		{
			name:      "no change: ok",
			current:   ls(claim("data", "1Gi", nil)),
			proposed:  ls(claim("data", "1Gi", nil)),
			k8sClient: k8s.NewFakeClient(&storageClass),
			wantErr:   false,
		},
		{
			name:      "storage increase: ok",
			current:   ls(claim("data", "1Gi", nil)),
			proposed:  ls(claim("data", "3Gi", nil)),
			k8sClient: k8s.NewFakeClient(&storageClass),
			wantErr:   false,
		},
		{
			name:      "label-only change: ok",
			current:   ls(claim("data", "1Gi", nil)),
			proposed:  ls(claim("data", "1Gi", map[string]string{"team": "search"})),
			k8sClient: k8s.NewFakeClient(&storageClass),
			wantErr:   false,
		},
		{
			name:      "label removal: ok",
			current:   ls(claim("data", "1Gi", map[string]string{"team": "search"})),
			proposed:  ls(claim("data", "1Gi", nil)),
			k8sClient: k8s.NewFakeClient(&storageClass),
			wantErr:   false,
		},
		{
			name:      "label change combined with storage increase: ok",
			current:   ls(claim("data", "1Gi", nil)),
			proposed:  ls(claim("data", "3Gi", map[string]string{"team": "search"})),
			k8sClient: k8s.NewFakeClient(&storageClass),
			wantErr:   false,
		},
		{
			name:    "storageClassName change: error",
			current: ls(claim("data", "1Gi", nil)),
			proposed: func() *lsv1alpha1.Logstash {
				c := claim("data", "1Gi", nil)
				c.Spec.StorageClassName = ptr.To[string]("other-sc")
				return ls(c)
			}(),
			k8sClient: k8s.NewFakeClient(&storageClass),
			wantErr:   true,
		},
		{
			name:      "storage decrease: error",
			current:   ls(claim("data", "3Gi", nil)),
			proposed:  ls(claim("data", "1Gi", nil)),
			k8sClient: k8s.NewFakeClient(&storageClass),
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := checkPVCchanges(context.Background(), tt.current, tt.proposed, tt.k8sClient, true)
			if tt.wantErr {
				require.NotEmpty(t, errs)
			} else {
				require.Empty(t, errs)
			}
		})
	}
}

func Test_checkPVCReservedLabels(t *testing.T) {
	withClaims := func(claims ...corev1.PersistentVolumeClaim) *lsv1alpha1.Logstash {
		return &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{VolumeClaimTemplates: claims}}
	}
	claim := func(name string, labels map[string]string) corev1.PersistentVolumeClaim {
		return corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
	}

	tests := []struct {
		name     string
		current  *lsv1alpha1.Logstash
		proposed *lsv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name:     "nil current/proposed: ok",
			current:  nil,
			proposed: nil,
			wantErr:  false,
		},
		{
			name:     "non-reserved label added: ok",
			current:  withClaims(claim("data", nil)),
			proposed: withClaims(claim("data", map[string]string{"team": "search"})),
			wantErr:  false,
		},
		{
			name:     "reserved label newly added: error",
			current:  withClaims(claim("data", nil)),
			proposed: withClaims(claim("data", map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "evil"})),
			wantErr:  true,
		},
		{
			name:     "reserved label already present and unchanged: grandfathered",
			current:  withClaims(claim("data", map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})),
			proposed: withClaims(claim("data", map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})),
			wantErr:  false,
		},
		{
			name:     "reserved label already present but value changed: error",
			current:  withClaims(claim("data", map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})),
			proposed: withClaims(claim("data", map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "new"})),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := checkPVCReservedLabels(tt.current, tt.proposed)
			if tt.wantErr {
				require.NotEmpty(t, errs)
			} else {
				require.Empty(t, errs)
			}
		})
	}
}

func Test_claimsWithoutAdjustableFieldsLogstash(t *testing.T) {
	// Ensures the function does not panic when Resources.Requests is nil.
	claims := []corev1.PersistentVolumeClaim{
		{ObjectMeta: metav1.ObjectMeta{Name: "data"}},
	}
	require.NotPanics(t, func() {
		_ = claimsWithoutAdjustableFields(claims)
	})
}

func Test_checkESRefsNamed(t *testing.T) {
	type args struct {
		b *lsv1alpha1.Logstash
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &lsv1alpha1.Logstash{},
			},
			wantErr: false,
		},
		{
			name: "one ref, missing clusterName: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple refs, each with clusterName: OK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "bla",
							},
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "blub",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple refs, one missing clusterName: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "",
							},
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "default",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkESRefsNamed(tt.args.b)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}
