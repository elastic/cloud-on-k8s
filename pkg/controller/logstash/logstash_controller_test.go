// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	sampleStorageClass = storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "fixed"},
	}
	fixedStorageClass = storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "fixed"},
	}
	resizableStorageClass = storagev1.StorageClass{
		ObjectMeta:           metav1.ObjectMeta{Name: "resizable"},
		AllowVolumeExpansion: ptr.To(true),
	}
)

func newReconcileLogstash(objs ...client.Object) *ReconcileLogstash {
	client := k8s.NewFakeClient(objs...)
	r := &ReconcileLogstash{
		Client:         client,
		recorder:       record.NewFakeRecorder(100),
		dynamicWatches: watches.NewDynamicWatches(),
		expectations:   expectations.NewClustersExpectations(client),
	}
	return r
}

func TestReconcileLogstash_Reconcile(t *testing.T) {
	defaultLabels := (&logstashv1alpha1.Logstash{ObjectMeta: metav1.ObjectMeta{Name: "testLogstash"}}).GetIdentityLabels()
	tests := []struct {
		name            string
		objs            []client.Object
		request         reconcile.Request
		want            reconcile.Result
		expected        logstashv1alpha1.Logstash
		expectedObjects expectedObjects
		wantErr         bool
	}{
		{
			name: "valid unmanaged Logstash does not increment observedGeneration",
			objs: []client.Object{
				&logstashv1alpha1.Logstash{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash",
						Namespace:  "test",
						Generation: 1,
						Annotations: map[string]string{
							common.ManagedAnnotation: "false",
						},
					},
					Spec: logstashv1alpha1.LogstashSpec{
						Version: "8.12.0",
					},
					Status: logstashv1alpha1.LogstashStatus{
						ObservedGeneration: 1,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "testLogstash",
				},
			},
			want: reconcile.Result{},
			expected: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testLogstash",
					Namespace:  "test",
					Generation: 1,
					Annotations: map[string]string{
						common.ManagedAnnotation: "false",
					},
				},
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.12.0",
				},
				Status: logstashv1alpha1.LogstashStatus{
					ObservedGeneration: 1,
				},
			},
			expectedObjects: []expectedObject{},
			wantErr:         false,
		},
		{
			name: "too long name fails validation, and updates observedGeneration",
			objs: []client.Object{
				&logstashv1alpha1.Logstash{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstashwithtoolongofanamereallylongname",
						Namespace:  "test",
						Generation: 2,
					},
					Status: logstashv1alpha1.LogstashStatus{
						ObservedGeneration: 1,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "testLogstashwithtoolongofanamereallylongname",
				},
			},
			want: reconcile.Result{},
			expected: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testLogstashwithtoolongofanamereallylongname",
					Namespace:  "test",
					Generation: 2,
				},
				Status: logstashv1alpha1.LogstashStatus{
					ObservedGeneration: 2,
				},
			},
			expectedObjects: []expectedObject{},
			wantErr:         true,
		},
		{
			name: "Logstash with ready StatefulSet and Pod updates status and creates secrets and service",
			objs: []client.Object{
				&logstashv1alpha1.Logstash{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash",
						Namespace:  "test",
						Generation: 2,
					},
					Spec: logstashv1alpha1.LogstashSpec{
						Version: "8.12.0",
						Count:   1,
					},
					Status: logstashv1alpha1.LogstashStatus{
						ObservedGeneration: 1,
					},
				},
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testLogstash-ls",
						Namespace: "test",
						Labels:    addLabel(defaultLabels, hash.TemplateHashLabelName, "3145706383"),
					},
					Status: appsv1.StatefulSetStatus{
						AvailableReplicas: 1,
						Replicas:          1,
						ReadyReplicas:     1,
					},
					Spec: appsv1.StatefulSetSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "logstash-data",
									Namespace: "test",
								},
								Spec: corev1.PersistentVolumeClaimSpec{
									StorageClassName: ptr.To[string](sampleStorageClass.Name),
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("1.5Gi"),
										},
									},
								},
							},
						},
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash-ls",
						Namespace:  "test",
						Generation: 2,
						Labels:     map[string]string{labels.NameLabelName: "testLogstash", VersionLabelName: "8.12.0"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "testLogstash",
				},
			},
			want: reconcile.Result{},
			expectedObjects: []expectedObject{
				{
					t:    &corev1.Service{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-api"},
				},
				{
					t:    &corev1.Secret{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-config"},
				},
				{
					t:    &corev1.Secret{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-pipeline"},
				},
			},

			expected: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testLogstash",
					Namespace:  "test",
					Generation: 2,
				},
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.12.0",
					Count:   1,
				},
				Status: logstashv1alpha1.LogstashStatus{
					Version:            "8.12.0",
					ExpectedNodes:      1,
					AvailableNodes:     1,
					Health:             logstashv1alpha1.LogstashGreenHealth,
					ObservedGeneration: 2,
					Selector:           "common.k8s.elastic.co/type=logstash,logstash.k8s.elastic.co/name=testLogstash",
				},
			},
			wantErr: false,
		},
		{
			name: "Logstash with a custom service creates secrets and service",
			objs: []client.Object{
				&logstashv1alpha1.Logstash{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash",
						Namespace:  "test",
						Generation: 2,
					},
					Spec: logstashv1alpha1.LogstashSpec{
						Version: "8.12.0",
						Count:   1,
						Services: []logstashv1alpha1.LogstashService{{
							Name: "test",
							Service: commonv1.ServiceTemplate{
								Spec: corev1.ServiceSpec{
									Ports: []corev1.ServicePort{
										{Protocol: "TCP", Port: 9500},
									},
								},
							},
						}},
					},
					Status: logstashv1alpha1.LogstashStatus{
						ObservedGeneration: 1,
					},
				},
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testLogstash-ls",
						Namespace: "test",
						Labels:    addLabel(defaultLabels, hash.TemplateHashLabelName, "3145706383"),
					},
					Status: appsv1.StatefulSetStatus{
						AvailableReplicas: 1,
						Replicas:          1,
						ReadyReplicas:     1,
					},
					Spec: appsv1.StatefulSetSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "logstash-data",
									Namespace: "test",
								},
								Spec: corev1.PersistentVolumeClaimSpec{
									StorageClassName: ptr.To[string](sampleStorageClass.Name),
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("1.5Gi"),
										},
									},
								},
							},
						},
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash-ls",
						Namespace:  "test",
						Generation: 2,
						Labels:     map[string]string{labels.NameLabelName: "testLogstash", VersionLabelName: "8.12.0"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "testLogstash",
				},
			},
			want: reconcile.Result{},
			expectedObjects: []expectedObject{
				{
					t:    &corev1.Service{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-api"},
				},
				{
					t:    &corev1.Service{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-test"},
				},
				{
					t:    &corev1.Secret{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-config"},
				},
				{
					t:    &corev1.Secret{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-pipeline"},
				},
			},

			expected: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testLogstash",
					Namespace:  "test",
					Generation: 2,
				},
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.12.0",
					Count:   1,
					Services: []logstashv1alpha1.LogstashService{{
						Name: "test",
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: []corev1.ServicePort{
									{Protocol: "TCP", Port: 9500},
								},
							},
						},
					}},
				},
				Status: logstashv1alpha1.LogstashStatus{
					Version:            "8.12.0",
					ExpectedNodes:      1,
					AvailableNodes:     1,
					Health:             logstashv1alpha1.LogstashGreenHealth,
					ObservedGeneration: 2,
					Selector:           "common.k8s.elastic.co/type=logstash,logstash.k8s.elastic.co/name=testLogstash",
				},
			},
			wantErr: false,
		},
		{
			name: "Logstash with a service with no port creates secrets and service",
			objs: []client.Object{
				&logstashv1alpha1.Logstash{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash",
						Namespace:  "test",
						Generation: 2,
					},
					Spec: logstashv1alpha1.LogstashSpec{
						Version: "8.12.0",
						Count:   1,
						Services: []logstashv1alpha1.LogstashService{{
							Name: "api",
							Service: commonv1.ServiceTemplate{
								Spec: corev1.ServiceSpec{
									Ports: nil,
								},
							},
						}},
					},
					Status: logstashv1alpha1.LogstashStatus{
						ObservedGeneration: 1,
					},
				},
				&storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default-sc",
					},
				},
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testLogstash-ls",
						Namespace: "test",
						Labels:    addLabel(defaultLabels, hash.TemplateHashLabelName, "3145706383"),
					},
					Status: appsv1.StatefulSetStatus{
						AvailableReplicas: 1,
						Replicas:          1,
						ReadyReplicas:     1,
					},
					Spec: appsv1.StatefulSetSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "logstash-data",
									Namespace: "test",
								},
								Spec: corev1.PersistentVolumeClaimSpec{
									StorageClassName: ptr.To[string](sampleStorageClass.Name),
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("1.5Gi"),
										},
									},
								},
							},
						},
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash-ls",
						Namespace:  "test",
						Generation: 2,
						Labels:     map[string]string{labels.NameLabelName: "testLogstash", VersionLabelName: "8.12.0"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "testLogstash",
				},
			},
			want: reconcile.Result{},
			expectedObjects: []expectedObject{
				{
					t:    &corev1.Service{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-api"},
				},
				{
					t:    &corev1.Secret{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-config"},
				},
				{
					t:    &corev1.Secret{},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls-pipeline"},
				},
			},

			expected: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testLogstash",
					Namespace:  "test",
					Generation: 2,
				},
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.12.0",
					Count:   1,
					Services: []logstashv1alpha1.LogstashService{{
						Name: "api",
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: nil,
							},
						},
					}},
				},
				Status: logstashv1alpha1.LogstashStatus{
					Version:            "8.12.0",
					ExpectedNodes:      1,
					AvailableNodes:     1,
					Health:             logstashv1alpha1.LogstashGreenHealth,
					ObservedGeneration: 2,
					Selector:           "common.k8s.elastic.co/type=logstash,logstash.k8s.elastic.co/name=testLogstash",
				},
			},
			wantErr: false,
		},
		{
			name: "Logstash with UpdateStrategy creates StatefulSet with UpdateStrategy",
			objs: []client.Object{
				&logstashv1alpha1.Logstash{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash",
						Namespace:  "test",
						Generation: 2,
					},
					Spec: logstashv1alpha1.LogstashSpec{
						Version: "8.12.0",
						Count:   1,
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.RollingUpdateStatefulSetStrategyType,
							RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
								MaxUnavailable: &intstr.IntOrString{
									IntVal: 1,
								},
							},
						},
					},
					Status: logstashv1alpha1.LogstashStatus{
						ObservedGeneration: 1,
					},
				},
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testLogstash-ls",
						Namespace: "test",
						Labels:    addLabel(defaultLabels, hash.TemplateHashLabelName, "3145706383"),
					},
					Status: appsv1.StatefulSetStatus{
						AvailableReplicas: 1,
						Replicas:          1,
						ReadyReplicas:     1,
					},
					Spec: appsv1.StatefulSetSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "logstash-data",
									Namespace: "test",
								},
								Spec: corev1.PersistentVolumeClaimSpec{
									StorageClassName: ptr.To[string](sampleStorageClass.Name),
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("1.5Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "testLogstash",
				},
			},
			want: reconcile.Result{},
			expectedObjects: []expectedObject{
				{
					t: &appsv1.StatefulSet{
						Spec: appsv1.StatefulSetSpec{
							UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
								Type: appsv1.RollingUpdateStatefulSetStrategyType,
								RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
									MaxUnavailable: &intstr.IntOrString{
										IntVal: 1,
									},
								},
							},
						},
					},
					name: types.NamespacedName{Namespace: "test", Name: "testLogstash-ls"},
				},
			},

			expected: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testLogstash",
					Namespace:  "test",
					Generation: 2,
				},
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.12.0",
					Count:   1,
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
							MaxUnavailable: &intstr.IntOrString{
								IntVal: 1,
							},
						},
					},
				},
				Status: logstashv1alpha1.LogstashStatus{
					ExpectedNodes:      1,
					AvailableNodes:     1,
					Health:             logstashv1alpha1.LogstashGreenHealth,
					ObservedGeneration: 2,
					Selector:           "common.k8s.elastic.co/type=logstash,logstash.k8s.elastic.co/name=testLogstash",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconcileLogstash(tt.objs...)
			got, err := r.Reconcile(context.Background(), tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileLogstash.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileLogstash.Reconcile() = %v, want %v", got, tt.want)
			}

			var Logstash logstashv1alpha1.Logstash
			if err := r.Client.Get(context.Background(), tt.request.NamespacedName, &Logstash); err != nil {
				t.Error(err)
				return
			}
			tt.expectedObjects.assertExist(t, r.Client)
			comparison.AssertEqual(t, &Logstash, &tt.expected)
		})
	}
}

func TestReconcileLogstash_Resize(t *testing.T) {
	ctx := context.Background()
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "test", Name: "testLogstash"}}

	tests := []struct {
		name            string
		extraVerify     func(r ReconcileLogstash, desiredCapacity string) (reconcile.Result, error)
		initialCapacity string
		desiredCapacity string
		storageClass    storagev1.StorageClass
		wantErr         bool
	}{
		{
			name:            "Cannot increase storage with fixed storage class",
			initialCapacity: "1.5Gi",
			desiredCapacity: "3Gi",
			storageClass:    fixedStorageClass,
			wantErr:         true,
		},
		{
			name:            "Cannot decrease storage with resizable storage class",
			initialCapacity: "3Gi",
			desiredCapacity: "1.5Gi",
			storageClass:    resizableStorageClass,
			wantErr:         true,
		},
		{
			name:            "Nothing happens when keeping the storage the same with fixed storage class",
			initialCapacity: "3Gi",
			desiredCapacity: "3Gi",
			storageClass:    fixedStorageClass,
			wantErr:         false,
		},
		{
			name:            "Nothing happens when keeping the storage the same with resizable storage class",
			initialCapacity: "1.5Gi",
			desiredCapacity: "1.5Gi",
			storageClass:    resizableStorageClass,
			wantErr:         false,
		},
		{
			name:            "Can successfully resize the storage with resizable storage class",
			initialCapacity: "1.5Gi",
			desiredCapacity: "3Gi",
			storageClass:    resizableStorageClass,
			extraVerify: func(r ReconcileLogstash, desiredCapacity string) (reconcile.Result, error) {
				// When performing an actual volume resize, the first pass of the reconciler adds annotations to
				// Logstash with details of the StatefulSet to be replaced.
				updatedls := logstashv1alpha1.Logstash{}
				_ = r.Client.Get(ctx, request.NamespacedName, &updatedls)
				require.Equal(t, 1, len(updatedls.Annotations))

				// Second pass of the reconciler removes the StatefulSet, and requeues the request
				result, err := r.Reconcile(ctx, request)
				require.NoError(t, err)
				require.NotZero(t, result.RequeueAfter)

				ssNamespacedName := types.NamespacedName{Namespace: "test", Name: "testLogstash-ls"}
				ss := appsv1.StatefulSet{}
				require.Error(t, r.Client.Get(ctx, ssNamespacedName, &ss))

				// Third pass of the reconciler should recreate the StatefulSet
				result, err = r.Reconcile(ctx, request)

				require.NoError(t, err)
				require.NotZero(t, result.RequeueAfter)

				ss = appsv1.StatefulSet{}
				require.NoError(t, r.Client.Get(ctx, ssNamespacedName, &ss))
				require.Equal(t, resource.MustParse("3Gi"), ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage])

				// The K8s fake client does not supply a UID in created objects, which breaks the logic
				// in the state machine used to determine the end of the cycle, where the annotations can be deleted
				// and the pod owner reset
				ss.UID = uuid.NewUUID()
				_ = r.Client.Update(ctx, &ss)

				// Final pass of the reconciler  deletes the logstash annotations
				result, err = r.Reconcile(ctx, request)
				require.NoError(t, err)
				require.Equal(t, reconcile.Result{Requeue: false}, result)

				updatedls = logstashv1alpha1.Logstash{}
				_ = r.Client.Get(ctx, request.NamespacedName, &updatedls)

				// Verify that the annotations are gone
				require.Equal(t, 0, len(updatedls.Annotations))
				require.NoError(t, r.Client.Get(ctx, ssNamespacedName, &ss))

				// Verify that the resize is successful
				require.Equal(t, resource.MustParse(desiredCapacity), ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage])
				return result, err
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the reconciler
			reconciler := *setupFixtures(tt.initialCapacity, tt.storageClass)
			_, err := reconciler.Reconcile(ctx, request)
			require.NoError(t, err)

			// Retrieve created logstash and update the storage size requirements
			updatedLs := logstashv1alpha1.Logstash{}
			_ = reconciler.Client.Get(ctx, request.NamespacedName, &updatedLs)
			logstashResized := *updatedLs.DeepCopy()
			logstashResized.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(tt.desiredCapacity)
			_ = reconciler.Client.Update(ctx, &logstashResized)

			// Reconcile with updated storage size requirements
			_, err = reconciler.Reconcile(ctx, request)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.extraVerify != nil {
				_, err = tt.extraVerify(reconciler, tt.desiredCapacity)
				if (err != nil) != tt.wantErr {
					t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
			}
		})
	}
}

func setupFixtures(initialCapacity string, storage storagev1.StorageClass) *ReconcileLogstash {
	ls := createLogstash(initialCapacity, storage.Name)
	pod := createPod()
	return newReconcileLogstash(&ls, &pod, &storage)
}

func createPod() corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "testLogstash-ls-0",
			Namespace:  "test",
			UID:        uuid.NewUUID(),
			Generation: 1,
			Labels: map[string]string{
				"common.k8s.elastic.co/type":               "logstash",
				"logstash.k8s.elastic.co/name":             "testLogstash",
				"logstash.k8s.elastic.co/statefulset-name": "testLogstash-ls",
				"logstash.k8s.elastic.co/version":          "8.12.0",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	return pod
}

func createLogstash(capacity string, storageClassName string) logstashv1alpha1.Logstash {
	ls := logstashv1alpha1.Logstash{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "testLogstash",
			UID:       uuid.NewUUID(),
		},
		Spec: logstashv1alpha1.LogstashSpec{
			Version: "8.12.0",
			Count:   1,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{
					Name: "test-pq",
				},
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(capacity),
							},
						},
						StorageClassName: &storageClassName,
					},
				},
			},
		},
	}
	return ls
}

func addLabel(labels map[string]string, key, value string) map[string]string {
	newLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		newLabels[k] = v
	}
	newLabels[key] = value
	return newLabels
}

type expectedObject struct {
	t    client.Object
	name types.NamespacedName
}

type expectedObjects []expectedObject

func (e expectedObjects) assertExist(t *testing.T, k8s client.Client) {
	t.Helper()
	for _, o := range e {
		obj := o.t.DeepCopyObject().(client.Object) //nolint:forcetypeassert
		assert.NoError(t, k8s.Get(context.Background(), o.name, obj), "Expected object not found: %s", o.name)
	}
}
