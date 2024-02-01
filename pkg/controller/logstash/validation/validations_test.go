// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/validation/field"

	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
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
			version: "9.0.0",
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
								ObjectSelector: commonv1.ObjectSelector{SecretName: "bla"},
								ClusterName:    "test",
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								ClusterName:    "test2",
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
								ObjectSelector: commonv1.ObjectSelector{SecretName: "bla", Name: "bla"},
								ClusterName:    "test",
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
								ObjectSelector: commonv1.ObjectSelector{Namespace: "blub"},
								ClusterName:    "test",
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
								ObjectSelector: commonv1.ObjectSelector{ServiceName: "ble"},
								ClusterName:    "test",
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
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
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
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								ClusterName:    "bla",
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								ClusterName:    "blub",
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
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								ClusterName:    "",
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								ClusterName:    "default",
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
