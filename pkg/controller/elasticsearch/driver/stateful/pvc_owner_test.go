// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_reconcilePVCOwnerRefs(t *testing.T) {
	type args struct {
		c     k8s.Client
		es    esv1.Elasticsearch
		ssets es_sset.StatefulSetList
	}

	esFixture := func(policy esv1.VolumeClaimDeletePolicy) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			Name: "es", Namespace: "ns",
			Spec: esv1.ElasticsearchSpec{VolumeClaimDeletePolicy: policy},
		}
	}

	pvcFixture := func(name string, ownerRefs ...string) corev1.PersistentVolumeClaim {
		pvc := corev1.PersistentVolumeClaim{
			Namespace: "ns",
			Name:      name,
			Labels: map[string]string{
				label.ClusterNameLabelName: "es",
			},
		}
		for _, ref := range ownerRefs {
			pvc.OwnerReferences = append(pvc.OwnerReferences, metav1.OwnerReference{
				Name:       ref,
				Kind:       "Elasticsearch",
				APIVersion: "elasticsearch.k8s.elastic.co/v1",
			})
		}
		return pvc
	}

	pvcFixturePtr := func(name string, ownerRefs ...string) *corev1.PersistentVolumeClaim {
		pvc := pvcFixture(name, ownerRefs...)
		return &pvc
	}

	// pvcFixturePtrWithControllerRef creates a PVC whose ES ownerRef has Controller=true.
	pvcFixturePtrWithControllerRef := func(name string) *corev1.PersistentVolumeClaim {
		pvc := pvcFixturePtr(name, "es")
		controllerTrue := true
		pvc.OwnerReferences[0].Controller = &controllerTrue
		return pvc
	}

	// withSSetOwnerRef appends a StatefulSet ownerRef to the given PVC.
	withSSetOwnerRef := func(pvc corev1.PersistentVolumeClaim, ssetName string) corev1.PersistentVolumeClaim {
		pvc.OwnerReferences = append(pvc.OwnerReferences, metav1.OwnerReference{
			Name:       ssetName,
			Kind:       "StatefulSet",
			APIVersion: "apps/v1",
		})
		return pvc
	}

	// ssetFixture builds a minimal StatefulSet whose PVCNames() includes "{claimName}-{ssetName}-{0..replicas-1}".
	// Use sset name "data" and claim name "es" to generate PVC names matching the "es-data-N" fixture pattern.
	ssetFixture := func(ssetName, claimName string, replicas int32) appsv1.StatefulSet {
		return appsv1.StatefulSet{
			Name: ssetName, Namespace: "ns",
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{Name: claimName},
				},
			},
		}
	}

	tests := []struct {
		name            string
		args            args
		want            []corev1.PersistentVolumeClaim
		wantUpdate      bool
		wantErr         bool
		wantWaitingPVCs []string
	}{
		{
			name: "remove references on DeleteOnScaledownOnlyPolicy",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0", "es")),
				es: esFixture(esv1.DeleteOnScaledownOnlyPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "avoid unnecessary updates when reference already removed",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0")),
				es: esFixture(esv1.DeleteOnScaledownOnlyPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0")},
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name: "no update when no ES ownerRef on DeleteOnScaledownAndClusterDeletion",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0")),
				es: esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0")},
			wantErr:    false,
			wantUpdate: false,
		},
		{
			name: "no update when ES ownerRef present but upstream has not yet stamped its own ref",
			args: args{
				c:     k8s.NewFakeClient(pvcFixturePtr("es-data-0", "es")),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)}, // "es-data-0" is in-range
			},
			want:            []corev1.PersistentVolumeClaim{pvcFixture("es-data-0", "es")},
			wantErr:         false,
			wantUpdate:      false,
			wantWaitingPVCs: []string{"es-data-0"},
		},
		{
			// Out-of-range PVCs keep their ES ref so cluster deletion still GCs them via
			// cascading ownership. We just don't requeue waiting for an upstream ref that
			// will never arrive for a pod that no longer exists.
			name: "leave out-of-range PVC untouched and do not wait (pre-upgrade scale-down leftover)",
			args: args{
				c:     k8s.NewFakeClient(pvcFixturePtr("es-data-2", "es")),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 2)}, // only "es-data-0" and "es-data-1" in range
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-2", "es")},
			wantErr:    false,
			wantUpdate: false,
		},
		{
			name: "downgrade Controller=true ES ref and wait for upstream to take over",
			args: args{
				c:     k8s.NewFakeClient(pvcFixturePtrWithControllerRef("es-data-0")),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want: []corev1.PersistentVolumeClaim{func() corev1.PersistentVolumeClaim {
				f := false
				pvc := pvcFixture("es-data-0")
				pvc.OwnerReferences = []metav1.OwnerReference{{
					Name:       "es",
					Kind:       "Elasticsearch",
					APIVersion: "elasticsearch.k8s.elastic.co/v1",
					Controller: &f,
				}}
				return pvc
			}()},
			wantErr:         false,
			wantUpdate:      true,
			wantWaitingPVCs: []string{"es-data-0"},
		},
		{
			name: "downgrade Controller=true ES ref on out-of-range PVC without waiting",
			args: args{
				c:     k8s.NewFakeClient(pvcFixturePtrWithControllerRef("es-data-2")),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 2)}, // only "es-data-0" and "es-data-1" in range
			},
			want: []corev1.PersistentVolumeClaim{func() corev1.PersistentVolumeClaim {
				f := false
				pvc := pvcFixture("es-data-2")
				pvc.OwnerReferences = []metav1.OwnerReference{{
					Name:       "es",
					Kind:       "Elasticsearch",
					APIVersion: "elasticsearch.k8s.elastic.co/v1",
					Controller: &f,
				}}
				return pvc
			}()},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "no update for unowned in-range PVC after switching back to DeleteOnScaledownAndClusterDeletion",
			args: args{
				c:     k8s.NewFakeClient(pvcFixturePtr("es-data-0")),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0")},
			wantErr:    false,
			wantUpdate: false,
		},
		{
			name: "keep upstream StatefulSet ownerRef after switching back to DeleteOnScaledownAndClusterDeletion",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := withSSetOwnerRef(pvcFixture("es-data-0"), "data")
					return &p
				}()),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want:       []corev1.PersistentVolumeClaim{withSSetOwnerRef(pvcFixture("es-data-0"), "data")},
			wantErr:    false,
			wantUpdate: false,
		},
		{
			name: "remove legacy ES ownerRef once StatefulSet ownerRef is present",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := withSSetOwnerRef(pvcFixture("es-data-0", "es"), "data")
					return &p
				}()),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want:       []corev1.PersistentVolumeClaim{withSSetOwnerRef(pvcFixture("es-data-0"), "data")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "remove legacy ES ownerRef once Pod ownerRef is present (condemned pod)",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := pvcFixture("es-data-0", "es")
					p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
						Name: "data-0", Kind: "Pod", APIVersion: "v1",
					})
					return &p
				}()),
				es: esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
			},
			want: []corev1.PersistentVolumeClaim{func() corev1.PersistentVolumeClaim {
				p := pvcFixture("es-data-0")
				p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
					Name: "data-0", Kind: "Pod", APIVersion: "v1",
				})
				return p
			}()},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "keep ES ownerRef and wait when the StatefulSet ownerRef points to another StatefulSet",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := withSSetOwnerRef(pvcFixture("es-data-0", "es"), "other")
					return &p
				}()),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want:            []corev1.PersistentVolumeClaim{withSSetOwnerRef(pvcFixture("es-data-0", "es"), "other")},
			wantErr:         false,
			wantUpdate:      false,
			wantWaitingPVCs: []string{"es-data-0"},
		},
		{
			name: "keep ES ownerRef and wait when the StatefulSet ownerRef has an unexpected APIVersion",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := pvcFixture("es-data-0", "es")
					p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
						Name: "data", Kind: "StatefulSet", APIVersion: "apps/v1beta2",
					})
					return &p
				}()),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want: []corev1.PersistentVolumeClaim{func() corev1.PersistentVolumeClaim {
				p := pvcFixture("es-data-0", "es")
				p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
					Name: "data", Kind: "StatefulSet", APIVersion: "apps/v1beta2",
				})
				return p
			}()},
			wantErr:         false,
			wantUpdate:      false,
			wantWaitingPVCs: []string{"es-data-0"},
		},
		{
			name: "keep ES ownerRef and wait when the Pod ownerRef is not the PVC's own Pod",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := pvcFixture("es-data-0", "es")
					p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
						Name: "data-1", Kind: "Pod", APIVersion: "v1",
					})
					return &p
				}()),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want: []corev1.PersistentVolumeClaim{func() corev1.PersistentVolumeClaim {
				p := pvcFixture("es-data-0", "es")
				p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{
					Name: "data-1", Kind: "Pod", APIVersion: "v1",
				})
				return p
			}()},
			wantErr:         false,
			wantUpdate:      false,
			wantWaitingPVCs: []string{"es-data-0"},
		},
		{
			name: "keep non-ES ownerRefs when removing ES ownerRef on DeleteOnScaledownOnly",
			args: args{
				c:  k8s.NewFakeClient(pvcFixturePtr("es-data-0", "es", "some-other-ref")),
				es: esFixture(esv1.DeleteOnScaledownOnlyPolicy),
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0", "some-other-ref")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "remove stale StatefulSet ownerRef on DeleteOnScaledownOnlyPolicy",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := withSSetOwnerRef(pvcFixture("es-data-0"), "data")
					return &p
				}()),
				es:    esFixture(esv1.DeleteOnScaledownOnlyPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "remove both ES and StatefulSet ownerRefs on DeleteOnScaledownOnlyPolicy",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := withSSetOwnerRef(pvcFixture("es-data-0", "es"), "data")
					return &p
				}()),
				es:    esFixture(esv1.DeleteOnScaledownOnlyPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "keep non-StatefulSet ownerRefs when removing stale StatefulSet ownerRef",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := withSSetOwnerRef(pvcFixture("es-data-0", "some-other-ref"), "data")
					return &p
				}()),
				es:    esFixture(esv1.DeleteOnScaledownOnlyPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want:       []corev1.PersistentVolumeClaim{pvcFixture("es-data-0", "some-other-ref")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "keep ownerRefs to other StatefulSets on DeleteOnScaledownOnlyPolicy",
			args: args{
				c: k8s.NewFakeClient(func() *corev1.PersistentVolumeClaim {
					p := withSSetOwnerRef(pvcFixture("es-data-0", "es"), "other")
					return &p
				}()),
				es:    esFixture(esv1.DeleteOnScaledownOnlyPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 1)},
			},
			want:       []corev1.PersistentVolumeClaim{withSSetOwnerRef(pvcFixture("es-data-0"), "other")},
			wantErr:    false,
			wantUpdate: true,
		},
		{
			name: "one PVC waiting and one PVC updated in the same call",
			args: args{
				c: k8s.NewFakeClient(
					pvcFixturePtr("es-data-0", "es"), // ES ownerRef only: no upstream ref yet → waiting
					func() *corev1.PersistentVolumeClaim {
						p := withSSetOwnerRef(pvcFixture("es-data-1", "es"), "data") // ES + SSet → remove ES ref
						return &p
					}(),
				),
				es:    esFixture(esv1.DeleteOnScaledownAndClusterDeletionPolicy),
				ssets: es_sset.StatefulSetList{ssetFixture("data", "es", 2)}, // "es-data-0" and "es-data-1" in-range
			},
			want: []corev1.PersistentVolumeClaim{
				pvcFixture("es-data-0", "es"),                     // unchanged: still waiting
				withSSetOwnerRef(pvcFixture("es-data-1"), "data"), // ES ref removed, SSet kept
			},
			wantErr:         false,
			wantUpdate:      true,
			wantWaitingPVCs: []string{"es-data-0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trackedClient := trackingK8sClient{Client: tt.args.c}
			waiting, err := reconcilePVCOwnerRefs(t.Context(), &trackedClient, tt.args.es, tt.args.ssets)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcilePVCOwnerRefs() error = %v, wantErr %v", err, tt.wantErr)
			}
			require.Equal(t, tt.wantWaitingPVCs, waiting, "unexpected PVCs waiting for upstream ownerRefs")
			var pvcs corev1.PersistentVolumeClaimList
			if err := tt.args.c.List(t.Context(), &pvcs); err != nil {
				t.Errorf("reconcilePVCOwnerRefs(), failed to list pvcs: %v", err)
			}
			require.Equal(t, len(tt.want), len(pvcs.Items), "unexpected number of pvcs")
			for i := 0; i < len(tt.want); i++ {
				comparison.AssertEqual(t, &pvcs.Items[i], &tt.want[i])
			}
			require.Equal(t, tt.wantUpdate, trackedClient.updateCalled, "unexpected client interaction: update called")
		})
	}
}

type trackingK8sClient struct {
	k8s.Client
	updateCalled bool
}

func (t *trackingK8sClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	t.updateCalled = true
	return t.Client.Update(ctx, obj, opts...)
}

func Test_setPVCOwnerRefsForRecreation(t *testing.T) {
	esRef := metav1.OwnerReference{
		Name:       "es",
		Kind:       "Elasticsearch",
		APIVersion: "elasticsearch.k8s.elastic.co/v1",
	}
	ssetRef := func(name string) metav1.OwnerReference {
		return metav1.OwnerReference{Name: name, Kind: "StatefulSet", APIVersion: "apps/v1"}
	}
	pvcFixture := func(name string, refs ...metav1.OwnerReference) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			Namespace:       "ns",
			Name:            name,
			Labels:          map[string]string{label.ClusterNameLabelName: "es"},
			OwnerReferences: refs,
		}
	}
	// recreateAnnotation schedules the re-creation of a StatefulSet named "data" with a single "es" claim template.
	recreateAnnotation := func(t *testing.T, replicas int32) map[string]string {
		t.Helper()
		sset := appsv1.StatefulSet{
			Name: "data", Namespace: "ns",
			Spec: appsv1.StatefulSetSpec{
				Replicas:             &replicas,
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{Name: "es"}},
			},
		}
		bytes, err := json.Marshal(sset)
		require.NoError(t, err)
		return map[string]string{"elasticsearch.k8s.elastic.co/recreate-data": string(bytes)}
	}

	tests := []struct {
		name        string
		policy      esv1.VolumeClaimDeletePolicy
		annotations func(t *testing.T) map[string]string
		pvcs        []*corev1.PersistentVolumeClaim
		wantRefs    map[string][]metav1.OwnerReference
		wantUpdate  bool
	}{
		{
			name:        "add ES ownerRef to PVCs of the StatefulSet scheduled for re-creation",
			policy:      esv1.DeleteOnScaledownAndClusterDeletionPolicy,
			annotations: func(t *testing.T) map[string]string { t.Helper(); return recreateAnnotation(t, 3) },
			pvcs: []*corev1.PersistentVolumeClaim{
				pvcFixture("es-data-0", ssetRef("data")),
				pvcFixture("es-data-1", ssetRef("data"), esRef), // already owned: unchanged
				// es-data-2 does not exist: skipped
				pvcFixture("es-master-0", ssetRef("master")), // not scheduled for re-creation: unchanged
			},
			wantRefs: map[string][]metav1.OwnerReference{
				"es-data-0":   {ssetRef("data"), esRef},
				"es-data-1":   {ssetRef("data"), esRef},
				"es-master-0": {ssetRef("master")},
			},
			wantUpdate: true,
		},
		{
			name:        "no ES ownerRef on DeleteOnScaledownOnlyPolicy",
			policy:      esv1.DeleteOnScaledownOnlyPolicy,
			annotations: func(t *testing.T) map[string]string { t.Helper(); return recreateAnnotation(t, 1) },
			pvcs:        []*corev1.PersistentVolumeClaim{pvcFixture("es-data-0", ssetRef("data"))},
			wantRefs:    map[string][]metav1.OwnerReference{"es-data-0": {ssetRef("data")}},
			wantUpdate:  false,
		},
		{
			name:        "no update when no StatefulSet is scheduled for re-creation",
			policy:      esv1.DeleteOnScaledownAndClusterDeletionPolicy,
			annotations: func(t *testing.T) map[string]string { t.Helper(); return nil },
			pvcs:        []*corev1.PersistentVolumeClaim{pvcFixture("es-data-0", ssetRef("data"))},
			wantRefs:    map[string][]metav1.OwnerReference{"es-data-0": {ssetRef("data")}},
			wantUpdate:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{
				Name: "es", Namespace: "ns", Annotations: tt.annotations(t),
				Spec: esv1.ElasticsearchSpec{VolumeClaimDeletePolicy: tt.policy},
			}
			objs := make([]client.Object, 0, len(tt.pvcs))
			for _, pvc := range tt.pvcs {
				objs = append(objs, pvc)
			}
			trackedClient := trackingK8sClient{Client: k8s.NewFakeClient(objs...)}

			require.NoError(t, setPVCOwnerRefsForRecreation(t.Context(), &trackedClient, es))

			for name, wantRefs := range tt.wantRefs {
				var pvc corev1.PersistentVolumeClaim
				require.NoError(t, trackedClient.Get(t.Context(), types.NamespacedName{Namespace: "ns", Name: name}, &pvc))
				require.Equal(t, wantRefs, pvc.OwnerReferences, "unexpected ownerRefs on %s", name)
			}
			require.Equal(t, tt.wantUpdate, trackedClient.updateCalled, "unexpected client interaction: update called")
		})
	}
}
