// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	esav1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoscaling/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var (
	sampleStorageClass = storagev1.StorageClass{
		Name: "sample-sc"}

	sampleClaim = corev1.PersistentVolumeClaim{
		Name: "sample-claim",
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: new(sampleStorageClass.Name),
			Resources: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}
	sampleClaim2 = corev1.PersistentVolumeClaim{
		Name: "sample-claim-2",
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: new(sampleStorageClass.Name),
			Resources: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}

	// claimWithoutStorageReq declares a storage class but no size, leaving Resources.Requests nil.
	// That is the shape the autoscaling contract produces, and it is nil rather than empty because
	// the field is omitempty all the way down.
	claimWithoutStorageReq = corev1.PersistentVolumeClaim{
		Name: "sample-claim",
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: new(sampleStorageClass.Name),
		}}
)

func withStorageReq(claim corev1.PersistentVolumeClaim, size string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(size)
	return *c
}

func withStorageClass(claim corev1.PersistentVolumeClaim, storageClassName string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.StorageClassName = new(storageClassName)
	return *c
}

func withAccessMode(claim corev1.PersistentVolumeClaim, mode corev1.PersistentVolumeAccessMode) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{mode}
	return *c
}

// nodeSetWithRoles returns a NodeSet with the given name, roles, and VolumeClaimTemplates.
func nodeSetWithRoles(name string, roles []string, vcts []corev1.PersistentVolumeClaim) esv1.NodeSet {
	return esv1.NodeSet{
		Name:                 name,
		Config:               &commonv1.Config{Data: map[string]any{"node.roles": roles}},
		VolumeClaimTemplates: vcts,
	}
}

// autoscalerWithStoragePolicy returns an ElasticsearchAutoscaler that targets "cluster" and has a
// single policy covering the given roles with a storage range.
func autoscalerWithStoragePolicy(roles []string) *esav1alpha1.ElasticsearchAutoscaler {
	return &esav1alpha1.ElasticsearchAutoscaler{
		Namespace: "ns", Name: "autoscaler",
		Spec: esav1alpha1.ElasticsearchAutoscalerSpec{
			ElasticsearchRef: esav1alpha1.ElasticsearchRef{Name: "cluster"},
			AutoscalingPolicySpecs: v1alpha1.AutoscalingPolicySpecs{
				{
					Name:  "storage-policy",
					Roles: roles,
					StorageRange: &v1alpha1.QuantityRange{
						Min: resource.MustParse("1Gi"),
						Max: resource.MustParse("100Gi"),
					},
				},
			},
		},
	}
}

func Test_validPVCModification(t *testing.T) {
	es := func(nodeSets []esv1.NodeSet) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			Namespace: "ns", Name: "cluster",
			Spec: esv1.ElasticsearchSpec{NodeSets: nodeSets},
		}
	}
	esV := func(nodeSets []esv1.NodeSet) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			Namespace: "ns", Name: "cluster",
			Spec: esv1.ElasticsearchSpec{Version: "8.0.0", NodeSets: nodeSets},
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
						Namespace: "ns", Name: "cluster-es-set1",
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
						Namespace: "ns", Name: "cluster-es-set1",
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
						Namespace: "ns", Name: "cluster-es-set1",
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
						Namespace: "ns", Name: "cluster-es-set1",
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
						Namespace: "ns", Name: "cluster-es-set1",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, withStorageReq(sampleClaim2, "0.5Gi"),
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			// https://github.com/elastic/cloud-on-k8s/issues/6796
			name: "reusing nodeSet name while StatefulSet still exists (quick rename back scenario): error",
			args: args{
				// Current state: nodeSet was renamed from "default" to "default-new" with different storageClass
				current: es([]esv1.NodeSet{
					{Name: "default-new", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{withStorageClass(sampleClaim, "invalid")}},
				}),
				// Proposed state: trying to rename back to "default" while old StatefulSet still exists
				proposed: es([]esv1.NodeSet{
					{Name: "default", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				// The old StatefulSet "cluster-es-default" still exists with the original storageClass
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-default",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, // original storageClass "sample-sc"
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "claim with no resource requests at all: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{claimWithoutStorageReq}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{claimWithoutStorageReq}},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-set1",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			// Moving the size out of the claim and into the shorthand must not read as a
			// modification to the claim, which is what the storage-request normalisation is for.
			name: "storage size moved out of the claim into the shorthand: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				proposed: es([]esv1.NodeSet{
					{
						Name:                 "set1",
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{claimWithoutStorageReq},
						Resources:            esv1.NodeSetResources{Storage: new(resource.MustParse("1Gi"))},
					},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-set1",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "storage decrease via shorthand vs. existing statefulset: error",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				proposed: es([]esv1.NodeSet{
					{
						Name:                 "set1",
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{claimWithoutStorageReq},
						Resources:            esv1.NodeSetResources{Storage: new(resource.MustParse("500Mi"))},
					},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-set1",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			// Regression: when a nodeSet omits volumeClaimTemplates entirely, BuildStatefulSet
			// injects the default elasticsearch-data claim via AppendDefaultPVCs before applying
			// the storage shorthand. The validator must mirror that sequence; without it,
			// proposedClaims is empty and the decrease check is silently skipped.
			name: "storage decrease via shorthand on nodeSet without explicit VCTs: error",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1"},
				}),
				proposed: es([]esv1.NodeSet{
					{
						Name:      "set1",
						Resources: esv1.NodeSetResources{Storage: new(resource.MustParse("500Mi"))},
					},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-set1",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								Name: "elasticsearch-data",
								Spec: corev1.PersistentVolumeClaimSpec{
									Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("5Gi"),
									}},
								},
							},
						}},
					}),
				validateStorageClass: false,
			},
			wantErr: true,
		},
		{
			name: "autoscaler active + stale storage size in proposed: ok (autoscaler controls size)",
			args: args{
				current: esV([]esv1.NodeSet{
					nodeSetWithRoles("data", []string{"data"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				proposed: esV([]esv1.NodeSet{
					// autoscaler already bumped the STS to 10Gi; user re-applies the old manifest with stale 1Gi - must be accepted
					nodeSetWithRoles("data", []string{"data"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				k8sClient: k8s.NewFakeClient(
					autoscalerWithStoragePolicy([]string{"data"}),
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-data",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							withStorageReq(sampleClaim, "10Gi"), // autoscaler already bumped it
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "autoscaler active + proposed size matches STS: ok",
			args: args{
				current: esV([]esv1.NodeSet{
					nodeSetWithRoles("data", []string{"data"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				proposed: esV([]esv1.NodeSet{
					nodeSetWithRoles("data", []string{"data"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				k8sClient: k8s.NewFakeClient(
					autoscalerWithStoragePolicy([]string{"data"}),
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-data",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "autoscaler active + NodeSet not covered by storage policy: storage decrease rejected",
			args: args{
				current: esV([]esv1.NodeSet{
					nodeSetWithRoles("master", []string{"master"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				proposed: esV([]esv1.NodeSet{
					nodeSetWithRoles("master", []string{"master"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				k8sClient: k8s.NewFakeClient(
					autoscalerWithStoragePolicy([]string{"data"}), // policy covers "data", not "master"
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-master",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							withStorageReq(sampleClaim, "10Gi"),
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "autoscaler active + NodeSet missing node.roles config: storage decrease still rejected",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "data", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "data", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				k8sClient: k8s.NewFakeClient(
					autoscalerWithStoragePolicy([]string{"data"}),
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-data",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							withStorageReq(sampleClaim, "10Gi"),
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "mixed cluster: autoscaled NodeSet accepts stale size, unmanaged NodeSet rejects decrease",
			args: args{
				current: esV([]esv1.NodeSet{
					nodeSetWithRoles("data", []string{"data"}, []corev1.PersistentVolumeClaim{sampleClaim}),
					nodeSetWithRoles("master", []string{"master"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				proposed: esV([]esv1.NodeSet{
					nodeSetWithRoles("data", []string{"data"}, []corev1.PersistentVolumeClaim{sampleClaim}),
					nodeSetWithRoles("master", []string{"master"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				k8sClient: k8s.NewFakeClient(
					autoscalerWithStoragePolicy([]string{"data"}),
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-data",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							withStorageReq(sampleClaim, "10Gi"),
						}},
					},
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-master",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							withStorageReq(sampleClaim, "10Gi"),
						}},
					},
				),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "autoscaler active + storage class changed: error (immutable even with autoscaler)",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "data", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "data", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{withStorageClass(sampleClaim, "other-sc")}},
				}),
				k8sClient: k8s.NewFakeClient(
					&esav1alpha1.ElasticsearchAutoscaler{
						Namespace: "ns", Name: "autoscaler",
						Spec: esav1alpha1.ElasticsearchAutoscalerSpec{ElasticsearchRef: esav1alpha1.ElasticsearchRef{Name: "cluster"}},
					},
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-data",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "autoscaler active + access mode changed: error (immutable even with autoscaler)",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "data", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "data", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{withAccessMode(sampleClaim, corev1.ReadWriteMany)}},
				}),
				k8sClient: k8s.NewFakeClient(
					&esav1alpha1.ElasticsearchAutoscaler{
						Namespace: "ns", Name: "autoscaler",
						Spec: esav1alpha1.ElasticsearchAutoscalerSpec{ElasticsearchRef: esav1alpha1.ElasticsearchRef{Name: "cluster"}},
					},
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-data",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "autoscaler active + nodeSet name reuse while STS exists: error",
			args: args{
				// NodeSet "default" is not in current (renamed away), but old STS still exists.
				current: es([]esv1.NodeSet{
					{Name: "default-new", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "default", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				k8sClient: k8s.NewFakeClient(
					&esav1alpha1.ElasticsearchAutoscaler{
						Namespace: "ns", Name: "autoscaler",
						Spec: esav1alpha1.ElasticsearchAutoscalerSpec{ElasticsearchRef: esav1alpha1.ElasticsearchRef{Name: "cluster"}},
					},
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-default",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "autoscaler active + claim removed: error (identity immutable even with autoscaler)",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "data", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "data", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				k8sClient: k8s.NewFakeClient(
					&esav1alpha1.ElasticsearchAutoscaler{
						Namespace: "ns", Name: "autoscaler",
						Spec: esav1alpha1.ElasticsearchAutoscalerSpec{ElasticsearchRef: esav1alpha1.ElasticsearchRef{Name: "cluster"}},
					},
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-data",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
		},
		{
			name: "autoscaler active + new NodeSet no STS yet: ok (initial creation path)",
			args: args{
				current: es([]esv1.NodeSet{}),
				proposed: es([]esv1.NodeSet{
					{Name: "data", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim}},
				}),
				k8sClient: k8s.NewFakeClient(
					&esav1alpha1.ElasticsearchAutoscaler{
						Namespace: "ns", Name: "autoscaler",
						Spec: esav1alpha1.ElasticsearchAutoscalerSpec{ElasticsearchRef: esav1alpha1.ElasticsearchRef{Name: "cluster"}},
					},
					// no StatefulSet: this is the initial creation case
				),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "autoscaling lookup error: storage size validation skipped",
			args: args{
				current: esV([]esv1.NodeSet{
					nodeSetWithRoles("data", []string{"data"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				proposed: esV([]esv1.NodeSet{
					nodeSetWithRoles("data", []string{"data"}, []corev1.PersistentVolumeClaim{sampleClaim}),
				}),
				k8sClient: k8s.NewFakeClientBuilder(
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-data",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							withStorageReq(sampleClaim, "10Gi"), // autoscaler had bumped it
						}},
					},
				).WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if _, ok := list.(*esav1alpha1.ElasticsearchAutoscalerList); ok {
							return errors.New("transient list error")
						}
						return c.List(ctx, list, opts...)
					},
				}).Build(),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			name: "storage increase via shorthand on nodeSet without explicit VCTs: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1"},
				}),
				proposed: es([]esv1.NodeSet{
					{
						Name:      "set1",
						Resources: esv1.NodeSetResources{Storage: new(resource.MustParse("5Gi"))},
					},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						Namespace: "ns", Name: "cluster-es-set1",
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								Name: "elasticsearch-data",
								Spec: corev1.PersistentVolumeClaimSpec{
									Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("1Gi"),
									}},
								},
							},
						}},
					}),
				validateStorageClass: false,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validPVCModification(context.Background(), tt.args.current, tt.args.proposed, tt.args.k8sClient, tt.args.validateStorageClass)
			if tt.wantErr {
				require.NotEmpty(t, errs)
			} else {
				require.Empty(t, errs)
			}
		})
	}
}

func Test_validPVCNaming(t *testing.T) {
	esFixture := func() esv1.Elasticsearch {
		return esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{
			{Name: "default"},
		}}}
	}
	esWithClaim := func(claimName string, es esv1.Elasticsearch) esv1.Elasticsearch {
		es.Spec.NodeSets[0].VolumeClaimTemplates = append(es.Spec.NodeSets[0].VolumeClaimTemplates, corev1.PersistentVolumeClaim{
			Name: claimName,
		})
		return es
	}
	esWithVolumeMount := func(mountName string, es esv1.Elasticsearch) esv1.Elasticsearch {
		if es.Spec.NodeSets[0].PodTemplate.Spec.Containers == nil {
			es.Spec.NodeSets[0].PodTemplate.Spec.Containers = []corev1.Container{
				{Name: "elasticsearch"},
			}
		}
		es.Spec.NodeSets[0].PodTemplate.Spec.Containers[0].VolumeMounts = append(
			es.Spec.NodeSets[0].PodTemplate.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      mountName,
				MountPath: "/something/we/cannot/check/as/it/is/customizable/in/elasticsearch.yml",
			},
		)
		return es
	}
	esWithSidecar := esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name: "default",
					PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
						{
							Name: "sidecar",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "my-data"},
							},
						}}}},
				},
			},
		},
	}
	tests := []struct {
		name    string
		es      esv1.Elasticsearch
		wantErr bool
	}{
		{
			name:    "no claims is OK",
			es:      esFixture(),
			wantErr: false,
		},
		{
			name:    "default volume claim name is OK",
			es:      esWithClaim("elasticsearch-data", esFixture()),
			wantErr: false,
		},
		{
			name:    "custom claim name not mounted is NOK",
			es:      esWithClaim("my-data", esFixture()),
			wantErr: true,
		},
		{
			name:    "custom claim name but mounted is OK",
			es:      esWithVolumeMount("my-data", esWithClaim("my-data", esFixture())),
			wantErr: false,
		},
		{
			name:    "multiple custom claims but one not mounted is NOK",
			es:      esWithVolumeMount("my-data", esWithClaim("yet-another", esWithClaim("my-data", esFixture()))),
			wantErr: true,
		},
		{
			name:    "multiple custom claims is OK",
			es:      esWithVolumeMount("yet-another", esWithVolumeMount("my-data", esWithClaim("yet-another", esWithClaim("my-data", esFixture())))),
			wantErr: false,
		},
		{
			name: "custom claims for sidecars if all are mounted is OK",
			// this example has no valid data volume but if we want to allow data path customization there is no easy way to validate that
			es:      esWithClaim("my-data", esWithSidecar),
			wantErr: false,
		},
		{
			name:    "elasticsearch-cache claim name not mounted is NOK",
			es:      esWithClaim("elasticsearch-cache", esFixture()),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validPVCNaming(tt.es)
			if tt.wantErr {
				require.NotEmpty(t, got)
			} else {
				require.Empty(t, got)
			}
		})
	}
}
