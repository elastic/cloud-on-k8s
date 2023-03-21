// Copyright Logstash B.V. and/or licensed to Logstash B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	//"reflect"
	"testing"
	//"k8s.io/utils/pointer"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"


	//controllerscheme "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	//"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"


	//"github.com/stretchr/testify/require"


	//"k8s.io/apimachinery/pkg/api/meta"
	//"k8s.io/client-go/kubernetes/scheme"
	//"k8s.io/utils/pointer"
	//"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

)

func newReconcileLogstash(objs ...runtime.Object) *ReconcileLogstash {
	r := &ReconcileLogstash{
		Client:         k8s.NewFakeClient(objs...),
		recorder:       record.NewFakeRecorder(100),
		dynamicWatches: watches.NewDynamicWatches(),
	}
	return r
}

func TestReconcileLogstash_Reconcile(t *testing.T) {
	defaultLabels := (&logstashv1alpha1.Logstash{ObjectMeta: metav1.ObjectMeta{Name: "testLogstash"}}).GetIdentityLabels()
	tests := []struct {
		name     string
		objs     []runtime.Object
		request  reconcile.Request
		want     reconcile.Result
		expected logstashv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name: "valid unmanaged Logstash does not increment observedGeneration",
			objs: []runtime.Object{
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
						Version: "8.6.1",
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
					Version: "8.6.1",
				},
				Status: logstashv1alpha1.LogstashStatus{
					ObservedGeneration: 1,
				},
			},
			wantErr: false,
		},
		{
			name: "too long name fails validation, and updates observedGeneration",
			objs: []runtime.Object{
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
			wantErr: true,
		},
		{
			name: "Logstash with ready stateful set and pod updates status properly",
			objs: []runtime.Object{
				&logstashv1alpha1.Logstash{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash",
						Namespace:  "test",
						Generation: 2,
					},
					Spec: logstashv1alpha1.LogstashSpec{
						Version: "8.6.1",
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
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testLogstash-ls",
						Namespace:  "test",
						Generation: 2,
						Labels:     map[string]string{NameLabelName: "testLogstash", VersionLabelName: "8.6.1"},
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
			expected: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testLogstash",
					Namespace:  "test",
					Generation: 2,
				},
				Spec: logstashv1alpha1.LogstashSpec{
					Version: "8.6.1",
					Count:   1,
				},
				Status: logstashv1alpha1.LogstashStatus{
					Version:            "8.6.1",
					ExpectedNodes:      1,
					AvailableNodes:     1,
					ObservedGeneration: 2,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconcileLogstash(tt.objs...)
			//got, err := r.Reconcile(context.Background(), tt.request)
			//if (err != nil) != tt.wantErr {
			//	t.Errorf("ReconcileLogstash.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			//	return
			//}
			//if !reflect.DeepEqual(got, tt.want) {
			//	t.Errorf("ReconcileLogstash.Reconcile() = %v, want %v", got, tt.want)
			//}

			var Logstash logstashv1alpha1.Logstash
			if err := r.Client.Get(context.Background(), tt.request.NamespacedName, &Logstash); err != nil {
				t.Error(err)
				return
			}

			comparison.AssertEqual(t, &Logstash, &tt.expected)
		})
	}
}


//func TestReconcileLogstash_ReconcileStatefulSet(t *testing.T) {
//	defaultLabels := (&logstashv1alpha1.Logstash{ObjectMeta: metav1.ObjectMeta{Name: "testLogstash"}}).GetIdentityLabels()
//	tests := []struct {
//		name     string
//		objs     []runtime.Object
//		request  reconcile.Request
//		want     reconcile.Result
//		expected appsv1.StatefulSet
//		wantErr  bool
//	}{
//		{
//			name: "Logstash with ready stateful set and pod updates status properly",
//			objs: []runtime.Object{
//				&logstashv1alpha1.Logstash{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:       "testLogstash",
//						Namespace:  "test",
//						Generation: 2,
//					},
//					Spec: logstashv1alpha1.LogstashSpec{
//						Version: "8.6.1",
//						Count:   1,
//					},
//					Status: logstashv1alpha1.LogstashStatus{
//						ObservedGeneration: 1,
//					},
//				},
//				&appsv1.StatefulSet{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "testLogstash-ls",
//						Namespace: "test",
//						Labels:    addLabel(defaultLabels, hash.TemplateHashLabelName, "3145706383"),
//					},
//					Status: appsv1.StatefulSetStatus{
//						AvailableReplicas: 1,
//						Replicas:          1,
//						ReadyReplicas:     1,
//					},
//				},
//				&corev1.Pod{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:       "testLogstash-ls",
//						Namespace:  "test",
//						Generation: 2,
//						Labels:     map[string]string{NameLabelName: "testLogstash", VersionLabelName: "8.6.1"},
//					},
//					Status: corev1.PodStatus{
//						Phase: corev1.PodRunning,
//					},
//				},
//			},
//			request: reconcile.Request{
//				NamespacedName: types.NamespacedName{
//					Namespace: "test",
//					Name:      "testLogstash-ls",
//				},
//			},
//			want: reconcile.Result{},
//			expected: appsv1.StatefulSet{
//					ObjectMeta: metav1.ObjectMeta{
//						Namespace: "test",
//						Name:      "testLogstash-ls",
//						Labels: map[string]string{
//							hash.TemplateHashLabelName: "3145706383",
//						},
//					},
//					Spec: appsv1.StatefulSetSpec{
//						Replicas: pointer.Int32(1),
//					},
//			},
//			wantErr: false,
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			r := newReconcileLogstash(tt.objs...)
//			//got, err := r.Reconcile(context.Background(), tt.request)
//			//if (err != nil) != tt.wantErr {
//			//	t.Errorf("ReconcileLogstash.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
//			//	return
//			//}
//			//if !reflect.DeepEqual(got, tt.want) {
//			//	t.Errorf("ReconcileLogstash.Reconcile() = %v, want %v", got, tt.want)
//			//}
//
//			var retrieved appsv1.StatefulSet
//
//			err := r.Client.Get(context.Background(), tt.request.NamespacedName, &retrieved)
//			//err = r.Client.Get(context.Background(), k8s.ExtractNamespacedName(&tt.expected), &retrieved)
//			require.NoError(t, err)
//			comparison.AssertEqual(t, &retrieved, &tt.expected)
//
//			//var Logstash logstashv1alpha1.Logstash
//			//if err := r.Client.Get(context.Background(), tt.request.NamespacedName, &Logstash); err != nil {
//			//	t.Error(err)
//			//	return
//			//}
//			//
//			//comparison.AssertEqual(t, &Logstash, &tt.expected)
//		})
//	}
//}



//func TestReconcileStatefulSet(t *testing.T) {
//	controllerscheme.SetupScheme()
//	ls := logstashv1alpha1.Logstash{
//		ObjectMeta: metav1.ObjectMeta{
//			Namespace: "ns",
//			Name:      "ls",
//			UID:       types.UID("uid"),
//		},
//	}
//	ssetSample := appsv1.StatefulSet{
//		ObjectMeta: metav1.ObjectMeta{
//			Namespace: ls.Namespace,
//			Name:      "sset",
//			Labels: map[string]string{
//				hash.TemplateHashLabelName: "hash-value",
//			},
//		},
//		Spec: appsv1.StatefulSetSpec{
//			Replicas: pointer.Int32(3),
//		},
//	}
//	metaObj, err := meta.Accessor(&ssetSample)
//	require.NoError(t, err)
//	err = controllerutil.SetControllerReference(&ls, metaObj, scheme.Scheme)
//	require.NoError(t, err)
//
//	// simulate updated replicas & template hash label
//	updatedSset := *ssetSample.DeepCopy()
//	updatedSset.Spec.Replicas = pointer.Int32(4)
//	updatedSset.Labels[hash.TemplateHashLabelName] = "updated"
//
//	tests := []struct {
//		name                    string
//		client                  func() k8s.Client
//		expected                func() appsv1.StatefulSet
//		want                    func() appsv1.StatefulSet
//		wantExpectationsUpdated bool
//	}{
//		{
//			name:                    "create new sset",
//			client:                  func() k8s.Client { return k8s.NewFakeClient() },
//			expected:                func() appsv1.StatefulSet { return ssetSample },
//			want:                    func() appsv1.StatefulSet { return ssetSample },
//			wantExpectationsUpdated: false,
//		},
//		{
//			name:                    "no update when expected == actual",
//			client:                  func() k8s.Client { return k8s.NewFakeClient(&ssetSample) },
//			expected:                func() appsv1.StatefulSet { return ssetSample },
//			want:                    func() appsv1.StatefulSet { return ssetSample },
//			wantExpectationsUpdated: false,
//		},
//		{
//			name:                    "update sset with different template hash",
//			client:                  func() k8s.Client { return k8s.NewFakeClient(&ssetSample) },
//			expected:                func() appsv1.StatefulSet { return updatedSset },
//			want:                    func() appsv1.StatefulSet { return updatedSset },
//			wantExpectationsUpdated: true,
//		},
//		{
//			name: "update sset with missing template hash label",
//			client: func() k8s.Client {
//				ssetSampleWithMissingLabel := ssetSample.DeepCopy()
//				ssetSampleWithMissingLabel.Labels = map[string]string{}
//				return k8s.NewFakeClient(ssetSampleWithMissingLabel)
//			},
//			expected:                func() appsv1.StatefulSet { return ssetSample },
//			want:                    func() appsv1.StatefulSet { return ssetSample },
//			wantExpectationsUpdated: true,
//		},
//		{
//			name: "sset update should preserve existing annotations and labels",
//			client: func() k8s.Client {
//				ssetSampleWithExtraMetadata := ssetSample.DeepCopy()
//				// simulate annotations and labels manually set by the user
//				ssetSampleWithExtraMetadata.Annotations = map[string]string{"a": "b"}
//				ssetSampleWithExtraMetadata.Labels["a"] = "b"
//				return k8s.NewFakeClient(ssetSampleWithExtraMetadata)
//			},
//			expected: func() appsv1.StatefulSet { return updatedSset },
//			want: func() appsv1.StatefulSet {
//				// we want the expected sset + extra metadata from the existing one
//				expectedWithExtraMetadata := *updatedSset.DeepCopy()
//				expectedWithExtraMetadata.Annotations = map[string]string{"a": "b"}
//				expectedWithExtraMetadata.Labels["a"] = "b"
//				return expectedWithExtraMetadata
//			},
//			wantExpectationsUpdated: true,
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			client := tt.client()
//			//expected := tt.expected()
//			want := tt.want()
//			exp := expectations.NewExpectations(client)
//
//			params := Params{
//				Context:       context.Background(),
//				Client:        client,
//			}
//
//			returned, err := reconcileStatefulSet(params, nil)
//			require.NoError(t, err)
//
//			// returned sset should be the one we want
//			comparison.AssertEqual(t, &want, &returned)
//			// and be stored in the apiserver
//			var retrieved appsv1.StatefulSet
//			err = client.Get(context.Background(), k8s.ExtractNamespacedName(&want), &retrieved)
//			require.NoError(t, err)
//			comparison.AssertEqual(t, &want, &retrieved)
//
//			// check expectations were updated
//			require.Equal(t, tt.wantExpectationsUpdated, len(exp.GetGenerations()) != 0)
//		})
//	}
//}

func addLabel(labels map[string]string, key, value string) map[string]string {
	newLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		newLabels[k] = v
	}
	newLabels[key] = value
	return newLabels
}
