// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	defaultResources = commonv1alpha1.AutoscalingResources{
		CPURange:       &commonv1alpha1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
		MemoryRange:    &commonv1alpha1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("2Gi")},
		StorageRange:   &commonv1alpha1.QuantityRange{Min: resource.MustParse("5Gi"), Max: resource.MustParse("10Gi")},
		NodeCountRange: commonv1alpha1.CountRange{Min: 1, Max: 2},
	}
)

func TestValidateElasticsearchAutoscaler(t *testing.T) {
	type args struct {
		esa     v1alpha1.ElasticsearchAutoscaler
		es      *esv1.Elasticsearch
		checker license.Checker
	}
	tests := []struct {
		name                string
		args                args
		wantValidationError *string
		wantRuntimeError    *string
	}{
		{
			name: "Using the autoscaling annotation",
			args: args{
				es: es(map[string]string{esv1.ElasticsearchAutoscalingSpecAnnotationName: ""}, nil, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("Autoscaling annotation is no longer supported"),
		},
		{
			name: "ML must be in a dedicated autoscaling policy",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-ml": {"data", "ml"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_ml_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data", "ml"}},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("ML nodes must be in a dedicated autoscaling policy"),
		},
		{
			name: "ML is in a dedicated autoscaling policy",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-ml": {"ml"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "ml_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"ml"}},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
		},
		{
			name: "Autoscaling policy with no NodeSet",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}, "nodeset-data-2": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: defaultResources,
							},
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "ml_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"ml"}},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("Invalid value: []string{\"ml\"}: roles must be used in at least one nodeSet"),
		},
		{
			name: "Policy name is duplicated",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}, "nodeset-data-2": {"ml"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "ml_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: defaultResources,
							},
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "ml_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"ml"}},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("Invalid value: \"ml_policy\": policy is duplicated"),
		},
		{
			name: "nodeSet with no roles",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": nil, "nodeset-data-2": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("cannot parse nodeSet configuration: node.roles must be set"),
		},
		{
			name: "Min memory is 2G",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": nil, "nodeset-data-2": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: commonv1alpha1.AutoscalingResources{
									CPURange:       &commonv1alpha1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
									MemoryRange:    &commonv1alpha1.QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("2Gi")},
									StorageRange:   &commonv1alpha1.QuantityRange{Min: resource.MustParse("5Gi"), Max: resource.MustParse("10Gi")},
									NodeCountRange: commonv1alpha1.CountRange{Min: 1, Max: 2},
								},
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("min quantity must be greater than 2G"),
		},
		{
			name: "No name",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("name: Required value: name is mandatory"),
		},
		{
			name: "No roles",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{}},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("roles: Required value: roles field is mandatory and must not be empty"),
		},
		{
			name: "Max count should not be 0",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: commonv1alpha1.AutoscalingResources{
									CPURange:       &commonv1alpha1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
									MemoryRange:    &commonv1alpha1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("2Gi")},
									StorageRange:   &commonv1alpha1.QuantityRange{Min: resource.MustParse("5Gi"), Max: resource.MustParse("10Gi")},
									NodeCountRange: commonv1alpha1.CountRange{Min: 0, Max: 0},
								},
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("spec.policies[0].resources.nodeCount.max: Invalid value: 0: max count must be greater than 0"),
		},
		{
			name: "Min. count should be equal or greater than 0",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: commonv1alpha1.AutoscalingResources{
									CPURange:       &commonv1alpha1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
									MemoryRange:    &commonv1alpha1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("2Gi")},
									StorageRange:   &commonv1alpha1.QuantityRange{Min: resource.MustParse("5Gi"), Max: resource.MustParse("10Gi")},
									NodeCountRange: commonv1alpha1.CountRange{Min: -1, Max: 2},
								},
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("spec.policies[0].resources.nodeCount.min: Invalid value: -1: min count must be equal or greater than 0"),
		},
		{
			name: "Min. count is 0 max count must be greater than 0",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: commonv1alpha1.AutoscalingResources{
									CPURange:       &commonv1alpha1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
									MemoryRange:    &commonv1alpha1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("2Gi")},
									StorageRange:   &commonv1alpha1.QuantityRange{Min: resource.MustParse("5Gi"), Max: resource.MustParse("10Gi")},
									NodeCountRange: commonv1alpha1.CountRange{Min: 0, Max: 0},
								},
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("spec.policies[0].resources.nodeCount.max: Invalid value: 0: max count must be greater than 0"),
		},
		{
			name: "Min. count and max count are equal",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: commonv1alpha1.AutoscalingResources{
									CPURange:       &commonv1alpha1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
									MemoryRange:    &commonv1alpha1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("2Gi")},
									StorageRange:   &commonv1alpha1.QuantityRange{Min: resource.MustParse("5Gi"), Max: resource.MustParse("10Gi")},
									NodeCountRange: commonv1alpha1.CountRange{Min: 2, Max: 2},
								},
							},
						},
					},
				},
				checker: yesCheck,
			},
		},
		{
			name: "Min. count is greater than max",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: commonv1alpha1.AutoscalingResources{
									CPURange:       &commonv1alpha1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
									MemoryRange:    &commonv1alpha1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("2Gi")},
									StorageRange:   &commonv1alpha1.QuantityRange{Min: resource.MustParse("5Gi"), Max: resource.MustParse("10Gi")},
									NodeCountRange: commonv1alpha1.CountRange{Min: 5, Max: 4},
								},
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("spec.policies[0].resources.nodeCount.max: Invalid value: 4: max node count must be an integer greater or equal than the min node count"),
		},
		{
			name: "Min. CPU is greater than max",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: commonv1alpha1.AutoscalingResources{
									CPURange:       &commonv1alpha1.QuantityRange{Min: resource.MustParse("3"), Max: resource.MustParse("2")},
									MemoryRange:    &commonv1alpha1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("2Gi")},
									StorageRange:   &commonv1alpha1.QuantityRange{Min: resource.MustParse("5Gi"), Max: resource.MustParse("10Gi")},
									NodeCountRange: commonv1alpha1.CountRange{Min: 2, Max: 4},
								},
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("spec.policies[0].cpu.max: Invalid value: \"2\": max quantity must be greater or equal than min quantity"),
		},
		// Volumes validations
		{
			name: "Not the default volume claim",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, []string{"volume1"}, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: nil, // we do support configurations in which the volume claim is not the default (as long as there's only one)
		},
		{
			name: "More than one volume claim",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data"}}, []string{"volume1", "volume2"}, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name:              "data_policy",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("ElasticsearchAutoscaler.autoscaling.k8s.elastic.co \"esa\" is invalid: Elasticsearch.spec.nodeSets[0]: Invalid value: []string{\"volume1\", \"volume2\"}: autoscaling supports only one volume claim"),
		},
		{
			name: "ML policy with roles [ml, remote_cluster_client] succeeds",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data", "remote_cluster_client"}, "ml": {"ml", "remote_cluster_client"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name: "data",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{
										Roles:    []string{"data", "remote_cluster_client"},
										Deciders: nil,
									},
								},
								AutoscalingResources: defaultResources,
							},
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name: "ml",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{
										Roles:    []string{"ml", "remote_cluster_client"},
										Deciders: nil,
									},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: nil,
		},
		{
			name: "2 ML policies with roles [ml, remote_cluster_client] and [ml] fails",
			args: args{
				es: es(map[string]string{}, map[string][]string{"nodeset-data-1": {"data", "remote_cluster_client"}, "ml1": {"ml", "remote_cluster_client"}, "ml2": {"ml"}}, nil, "8.0.0"),
				esa: v1alpha1.ElasticsearchAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "esa", Namespace: "ns"},
					Spec: v1alpha1.ElasticsearchAutoscalerSpec{
						ElasticsearchRef: v1alpha1.ElasticsearchRef{
							Name: "es",
						},
						AutoscalingPolicySpecs: commonv1alpha1.AutoscalingPolicySpecs{
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name: "data",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{
										Roles:    []string{"data", "remote_cluster_client"},
										Deciders: nil,
									},
								},
								AutoscalingResources: defaultResources,
							},
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name: "ml1",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{
										Roles:    []string{"ml", "remote_cluster_client"},
										Deciders: nil,
									},
								},
								AutoscalingResources: defaultResources,
							},
							{
								NamedAutoscalingPolicy: commonv1alpha1.NamedAutoscalingPolicy{
									Name: "ml2",
									AutoscalingPolicy: commonv1alpha1.AutoscalingPolicy{
										Roles:    []string{"ml"},
										Deciders: nil,
									},
								},
								AutoscalingResources: defaultResources,
							},
						},
					},
				},
				checker: yesCheck,
			},
			wantValidationError: ptr.To[string]("ElasticsearchAutoscaler.autoscaling.k8s.elastic.co \"esa\" is invalid: spec.policies[2].name: Invalid value: \"ml\": ML nodes must be in a dedicated NodeSet"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var k8sClient k8s.Client
			if tt.args.es != nil {
				k8sClient = k8s.NewFakeClient(tt.args.es)
			} else {
				k8sClient = k8s.NewFakeClient()
			}
			validationError, runtimeError := ValidateElasticsearchAutoscaler(context.TODO(), k8sClient, tt.args.esa, tt.args.checker)
			if (validationError != nil) != (tt.wantValidationError != nil) {
				t.Errorf("ValidateElasticsearchAutoscaler() validationError = %v, wantValidationError %v", validationError, tt.wantValidationError)
			}
			if tt.wantValidationError != nil && validationError != nil && !strings.Contains(validationError.Error(), *tt.wantValidationError) {
				assert.ErrorContains(t, validationError, *tt.wantValidationError)
			}
			if (runtimeError != nil) != (tt.wantRuntimeError != nil) {
				t.Errorf("ValidateElasticsearchAutoscaler() runtimeError = %v, wantRuntimeError %v", runtimeError, tt.wantRuntimeError)
			}
		})
	}
}

func es(
	annotations map[string]string,
	nodeSets map[string][]string,
	volumeClaims []string,
	version string,
) *esv1.Elasticsearch {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "es",
			Namespace:   "ns",
			Annotations: annotations,
		},
		Spec: esv1.ElasticsearchSpec{Version: version},
	}
	for nodeSetName, roles := range nodeSets {
		cfg := commonv1.NewConfig(map[string]interface{}{})
		if roles != nil {
			cfg = commonv1.NewConfig(map[string]interface{}{"node.roles": roles})
		}
		volumeClaimTemplates := volumeClaimTemplates(volumeClaims)
		nodeSet := esv1.NodeSet{
			Name:                 nodeSetName,
			Config:               &cfg,
			VolumeClaimTemplates: volumeClaimTemplates,
		}
		es.Spec.NodeSets = append(es.Spec.NodeSets, nodeSet)
	}
	return &es
}

func volumeClaimTemplates(volumeClaims []string) []corev1.PersistentVolumeClaim {
	volumeClaimTemplates := make([]corev1.PersistentVolumeClaim, len(volumeClaims))
	for i := range volumeClaims {
		volumeClaimTemplates[i] = corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: volumeClaims[i],
			},
		}
	}
	return volumeClaimTemplates
}

// -- Fake license checker

var (
	yesCheck = &fakeChecker{
		enterpriseFeaturesEnabled: true,
		valid:                     true,
		operatorLicenseType:       license.LicenseTypeEnterprise,
	}
)

type fakeChecker struct {
	enterpriseFeaturesEnabled bool
	valid                     bool
	operatorLicenseType       license.OperatorLicenseType
}

func (f fakeChecker) CurrentEnterpriseLicense(_ context.Context) (*license.EnterpriseLicense, error) {
	panic("not implemented")
}

func (f fakeChecker) EnterpriseFeaturesEnabled(_ context.Context) (bool, error) {
	return f.enterpriseFeaturesEnabled, nil
}

func (f fakeChecker) Valid(_ context.Context, _ license.EnterpriseLicense) (bool, error) {
	return f.valid, nil
}

func (f fakeChecker) ValidOperatorLicenseKeyType(_ context.Context) (license.OperatorLicenseType, error) {
	return f.operatorLicenseType, nil
}

var _ license.Checker = &fakeChecker{}
