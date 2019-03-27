// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func Test_compareResources(t *testing.T) {
	type args struct {
		actual   corev1.ResourceRequirements
		expected corev1.ResourceRequirements
	}
	tests := []struct {
		name      string
		args      args
		wantMatch bool
	}{
		{
			name: "same memory",
			args: args{
				actual: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"memory": resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						"memory": resource.MustParse("1Gi"),
					},
				},
				expected: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"memory": resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						"memory": resource.MustParse("1Gi")},
				},
			},
			wantMatch: true,
		},
		{
			name: "different memory",
			args: args{
				actual: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"memory": resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						"memory": resource.MustParse("1Gi"),
					},
				},
				expected: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"memory": resource.MustParse("2Gi"),
					},
					Requests: corev1.ResourceList{
						"memory": resource.MustParse("2Gi")},
				},
			},
			wantMatch: false,
		},
		{
			name: "same memory expressed differently",
			args: args{
				actual: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"memory": resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						"memory": resource.MustParse("1Gi"),
					},
				},
				expected: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"memory": resource.MustParse("1024Mi"),
					},
					Requests: corev1.ResourceList{
						"memory": resource.MustParse("1024Mi")},
				},
			},
			wantMatch: true,
		},
		{
			name: "same cpu",
			args: args{
				actual: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu": resource.MustParse("500m"),
					},
					Requests: corev1.ResourceList{
						"cpu": resource.MustParse("500m"),
					},
				},
				expected: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu": resource.MustParse("500m"),
					},
					Requests: corev1.ResourceList{
						"cpu": resource.MustParse("500m")},
				},
			},
			wantMatch: true,
		},
		{
			name: "different cpu",
			args: args{
				actual: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu": resource.MustParse("500m"),
					},
					Requests: corev1.ResourceList{
						"cpu": resource.MustParse("500m"),
					},
				},
				expected: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu": resource.MustParse("400m"),
					},
					Requests: corev1.ResourceList{
						"cpu": resource.MustParse("500m")},
				},
			},
			wantMatch: false,
		},
		{
			name: "same cpu, different memory",
			args: args{
				actual: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    resource.MustParse("500m"),
						"memory": resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    resource.MustParse("500m"),
						"memory": resource.MustParse("1Gi"),
					},
				},
				expected: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    resource.MustParse("500m"),
						"memory": resource.MustParse("2Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    resource.MustParse("500m"),
						"memory": resource.MustParse("2Gi"),
					},
				},
			},
			wantMatch: false,
		},
		{
			name: "defaulted memory",
			args: args{
				actual: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"memory": resource.MustParse("1Gi"), // defaulted
					},
					Requests: corev1.ResourceList{
						"memory": resource.MustParse("1Gi"), // defaulted
					},
				},
				expected: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{}, // use default
					Requests: corev1.ResourceList{}, // use default
				},
			},
			wantMatch: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := compareResources(tt.args.actual, tt.args.expected)
			assert.Equal(t, tt.wantMatch, res.Match)
		})
	}
}

func Test_equalResourceList(t *testing.T) {
	type args struct {
		resListA corev1.ResourceList
		resListB corev1.ResourceList
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "same A and B",
			args: args{
				resListA: corev1.ResourceList{
					"key": resource.MustParse("100m"),
				},
				resListB: corev1.ResourceList{
					"key": resource.MustParse("100m"),
				},
			},
			want: true,
		},
		{
			name: "different A and B",
			args: args{
				resListA: corev1.ResourceList{
					"key": resource.MustParse("100m"),
				},
				resListB: corev1.ResourceList{
					"key": resource.MustParse("200m"),
				},
			},
			want: false,
		},
		{
			name: "more values in A",
			args: args{
				resListA: corev1.ResourceList{
					"key":  resource.MustParse("100m"),
					"key2": resource.MustParse("100m"),
				},
				resListB: corev1.ResourceList{
					"key": resource.MustParse("100m"),
				},
			},
			want: false,
		},
		{
			name: "more values in B",
			args: args{
				resListA: corev1.ResourceList{
					"key": resource.MustParse("100m"),
				},
				resListB: corev1.ResourceList{
					"key":  resource.MustParse("100m"),
					"key2": resource.MustParse("100m"),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, equalResourceList(tt.args.resListA, tt.args.resListB))
		})
	}
}
