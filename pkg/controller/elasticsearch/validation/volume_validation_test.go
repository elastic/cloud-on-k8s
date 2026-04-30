// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
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
)

func withStorageReq(claim corev1.PersistentVolumeClaim, size string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(size)
	return *c
}

func withStorageClass(claim corev1.PersistentVolumeClaim, storageClassName string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.StorageClassName = ptr.To[string](storageClassName)
	return *c
}

func withLabels(claim corev1.PersistentVolumeClaim, labels map[string]string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.ObjectMeta.Labels = labels
	return *c
}

func withVolumeExpansion(sc storagev1.StorageClass) *storagev1.StorageClass {
	sc.AllowVolumeExpansion = ptr.To[bool](true)
	return &sc
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
		{
			// https://github.com/elastic/cloud-on-k8s/issues/7910
			name: "add label to claim: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						withLabels(sampleClaim, map[string]string{"velero.io/exclude-from-backup": "true"}),
						sampleClaim2,
					}},
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
			// https://github.com/elastic/cloud-on-k8s/issues/7910
			name: "modify existing label on claim: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						withLabels(sampleClaim, map[string]string{"team": "platform"}),
						sampleClaim2,
					}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						withLabels(sampleClaim, map[string]string{"team": "search"}),
						sampleClaim2,
					}},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							withLabels(sampleClaim, map[string]string{"team": "platform"}),
							sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			// https://github.com/elastic/cloud-on-k8s/issues/7910
			name: "remove all labels from claim: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						withLabels(sampleClaim, map[string]string{"team": "platform"}),
						sampleClaim2,
					}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				k8sClient: k8s.NewFakeClient(
					&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-set1"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							withLabels(sampleClaim, map[string]string{"team": "platform"}),
							sampleClaim2,
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: false,
		},
		{
			// https://github.com/elastic/cloud-on-k8s/issues/7910
			name: "add label and increase storage in the same change: ok",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						withLabels(withStorageReq(sampleClaim, "3Gi"), map[string]string{"velero.io/exclude-from-backup": "true"}),
						sampleClaim2,
					}},
				}),
				k8sClient: k8s.NewFakeClient(
					withVolumeExpansion(sampleStorageClass),
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
			// https://github.com/elastic/cloud-on-k8s/issues/7910
			name: "label change combined with forbidden storageClassName change: error",
			args: args{
				current: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim, sampleClaim2}},
				}),
				proposed: es([]esv1.NodeSet{
					{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						withLabels(withStorageClass(sampleClaim, "different-sc"), map[string]string{"team": "platform"}),
						sampleClaim2,
					}},
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
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster-es-default"},
						Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							sampleClaim, // original storageClass "sample-sc"
						}},
					}),
				validateStorageClass: true,
			},
			wantErr: true,
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

func Test_validPVCReservedLabelsOnCreate(t *testing.T) {
	es := func(claims ...corev1.PersistentVolumeClaim) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster"},
			Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{
				{Name: "set1", VolumeClaimTemplates: claims},
			}},
		}
	}
	tests := []struct {
		name    string
		es      esv1.Elasticsearch
		wantErr bool
	}{
		{
			name:    "no labels: ok",
			es:      es(sampleClaim),
			wantErr: false,
		},
		{
			name:    "non-reserved label: ok",
			es:      es(withLabels(sampleClaim, map[string]string{"team": "search"})),
			wantErr: false,
		},
		{
			name:    "reserved cluster-name label: rejected (no grandfathering on create)",
			es:      es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "evil"})),
			wantErr: true,
		},
		{
			name:    "reserved statefulset-name label: rejected",
			es:      es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/statefulset-name": "evil"})),
			wantErr: true,
		},
		{
			name:    "reserved common.k8s.elastic.co label: rejected",
			es:      es(withLabels(sampleClaim, map[string]string{"common.k8s.elastic.co/type": "evil"})),
			wantErr: true,
		},
		{
			name:    "third-party look-alike key: ok",
			es:      es(withLabels(sampleClaim, map[string]string{"velero.io/exclude-from-backup": "true"})),
			wantErr: false,
		},
		{
			name: "reserved key in second claim is also detected",
			es: es(
				sampleClaim,
				withLabels(sampleClaim2, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "evil"}),
			),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validPVCReservedLabelsOnCreate(tt.es)
			if tt.wantErr {
				require.NotEmpty(t, errs)
			} else {
				require.Empty(t, errs)
			}
		})
	}
}

func Test_validPVCAnnotations_areStillImmutable(t *testing.T) {
	// Locks in the contract that VCT *annotations* (unlike labels) are still rejected
	// when modified on update. This guards against confusion since users frequently
	// conflate metadata.labels and metadata.annotations.
	es := func(claim corev1.PersistentVolumeClaim) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster"},
			Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{
				{Name: "set1", VolumeClaimTemplates: []corev1.PersistentVolumeClaim{claim}},
			}},
		}
	}
	withAnnotations := func(claim corev1.PersistentVolumeClaim, annotations map[string]string) corev1.PersistentVolumeClaim {
		c := claim.DeepCopy()
		c.ObjectMeta.Annotations = annotations
		return *c
	}

	current := es(sampleClaim)
	proposed := es(withAnnotations(sampleClaim, map[string]string{"team": "search"}))
	k8sClient := k8s.NewFakeClient()

	errs := validPVCModification(context.Background(), current, proposed, k8sClient, true)
	require.NotEmpty(t, errs, "adding an annotation to a VCT must be rejected (annotations are not in the adjustable-fields list)")
}

func Test_validPVCReservedLabels(t *testing.T) {
	es := func(claims ...corev1.PersistentVolumeClaim) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster"},
			Spec: esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{
				{Name: "set1", VolumeClaimTemplates: claims},
			}},
		}
	}
	tests := []struct {
		name     string
		current  esv1.Elasticsearch
		proposed esv1.Elasticsearch
		wantErr  bool
	}{
		{
			name:     "no labels: ok",
			current:  es(sampleClaim),
			proposed: es(sampleClaim),
			wantErr:  false,
		},
		{
			name:     "non-reserved label added: ok",
			current:  es(sampleClaim),
			proposed: es(withLabels(sampleClaim, map[string]string{"team": "search"})),
			wantErr:  false,
		},
		{
			name:     "third-party label that looks similar: ok",
			current:  es(sampleClaim),
			proposed: es(withLabels(sampleClaim, map[string]string{"velero.io/exclude-from-backup": "true"})),
			wantErr:  false,
		},
		{
			name:     "elasticsearch reserved cluster-name label newly added: error",
			current:  es(sampleClaim),
			proposed: es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "evil"})),
			wantErr:  true,
		},
		{
			name:     "elasticsearch reserved statefulset-name label newly added: error",
			current:  es(sampleClaim),
			proposed: es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/statefulset-name": "evil"})),
			wantErr:  true,
		},
		{
			name:     "common reserved type label newly added: error",
			current:  es(sampleClaim),
			proposed: es(withLabels(sampleClaim, map[string]string{"common.k8s.elastic.co/type": "evil"})),
			wantErr:  true,
		},
		{
			name:     "k8s.elastic.co subdomain label newly added: error",
			current:  es(sampleClaim),
			proposed: es(withLabels(sampleClaim, map[string]string{"k8s.elastic.co/foo": "bar"})),
			wantErr:  true,
		},
		{
			name:    "reserved key in second claim is also detected",
			current: es(sampleClaim, sampleClaim2),
			proposed: es(
				sampleClaim,
				withLabels(sampleClaim2, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "evil"}),
			),
			wantErr: true,
		},
		{
			// regression guard: existing CRs with a reserved label already on the VCT
			// must not be bricked at upgrade time. The check grandfathers unchanged
			// (key, value) pairs.
			name:     "reserved label already present and unchanged: grandfathered",
			current:  es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})),
			proposed: es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})),
			wantErr:  false,
		},
		{
			name:     "reserved label already present but value changed: error",
			current:  es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})),
			proposed: es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "new"})),
			wantErr:  true,
		},
		{
			name:     "newly added reserved label alongside grandfathered one: error only on the new one",
			current:  es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old"})),
			proposed: es(withLabels(sampleClaim, map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": "old", "common.k8s.elastic.co/type": "evil"})),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validPVCReservedLabels(tt.current, tt.proposed)
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
			ObjectMeta: metav1.ObjectMeta{Name: claimName},
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
			name: "stateless: elasticsearch-cache claim name is OK",
			es: func() esv1.Elasticsearch {
				es := esWithClaim("elasticsearch-cache", esFixture())
				es.Spec.Mode = esv1.ElasticsearchModeStateless
				return es
			}(),
			wantErr: false,
		},
		{
			name:    "stateful: elasticsearch-cache claim name not mounted is NOK",
			es:      esWithClaim("elasticsearch-cache", esFixture()),
			wantErr: true,
		},
		{
			name: "stateless: elasticsearch-data claim name not mounted is NOK",
			es: func() esv1.Elasticsearch {
				es := esWithClaim("elasticsearch-data", esFixture())
				es.Spec.Mode = esv1.ElasticsearchModeStateless
				return es
			}(),
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
