// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	common "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
)

func TestNewValidationContext(t *testing.T) {
	type args struct {
		current  *estype.Elasticsearch
		proposed estype.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    *Context
		wantErr bool
	}{
		{
			name: "garbage version FAIL",
			args: args{
				current:  nil,
				proposed: *es("garbage"),
			},
			wantErr: true,
		},
		{
			name: "current version garbage SHOULD NEVER HAPPEN",
			args: args{
				current:  es("garbage"),
				proposed: *es("6.0.0"),
			},
			wantErr: true,
		},
		{
			name: "create current is nil OK",
			args: args{
				current:  nil,
				proposed: *es("7.0.0"),
			},
			want: &Context{
				Proposed: ElasticsearchVersion{
					Elasticsearch: *es("7.0.0"),
					Version:       version.MustParse("7.0.0"),
				},
			},
			wantErr: false,
		},
		{
			name: "update both OK",
			args: args{
				current:  es("6.5.0"),
				proposed: *es("7.0.0"),
			},
			want: &Context{
				Current: &ElasticsearchVersion{
					Elasticsearch: *es("6.5.0"),
					Version:       version.MustParse("6.5.0"),
				},
				Proposed: ElasticsearchVersion{
					Elasticsearch: *es("7.0.0"),
					Version:       version.MustParse("7.0.0"),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewValidationContext(tt.args.current, tt.args.proposed)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewValidationContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewValidationContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	invalidIP := "notanip"
	type args struct {
		es estype.Elasticsearch
	}
	tests := []struct {
		name        string
		args        args
		wantErr     bool
		errContains []string
	}{
		{
			name: "happy path",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						Nodes: []estype.NodeSpec{
							{
								Config: &estype.Config{
									Data: map[string]interface{}{
										estype.NodeMaster: "true",
										estype.NodeData:   "false",
										estype.NodeIngest: "false",
										estype.NodeML:     "false",
									},
								},
								NodeCount: 1,
							}},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "single failure",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{Version: "7.0.0"},
				},
			},
			wantErr: false,
			errContains: []string{
				masterRequiredMsg,
			},
		},
		{
			name: "multiple failures",
			args: args{
				es: estype.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "that-is-a-very-long-name-with-37chars",
					},
					Spec: estype.ElasticsearchSpec{
						Version: "1.0.0",
						HTTP: common.HTTPConfig{
							TLS: common.TLSOptions{
								SelfSignedCertificate: &common.SelfSignedCertificate{
									SubjectAlternativeNames: []common.SubjectAlternativeName{
										{
											IP: invalidIP,
										},
									},
								},
							},
						},
						Nodes: []estype.NodeSpec{
							{
								NodeCount: 1,
								Config: &estype.Config{
									Data: map[string]interface{}{
										estype.NodeMaster: false,
										settings.XPackSecurityTransportSslCertificate: "blacklisted setting",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
			errContains: []string{
				"name length cannot exceed the limit",
				masterRequiredMsg,
				"unsupported version",
				"is not user configurable",
				invalidSanIPErrMsg,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validations, err := Validate(tt.args.es)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			require.Equal(t, len(tt.errContains), len(validations))
			for _, errStr := range tt.errContains {
				found := false
				for _, v := range validations {
					if strings.Contains(v.Reason, errStr) {
						found = true
						break
					}
				}
				assert.True(t, found, "wanted %v, but not found", errStr)
			}

		})

	}
}
