// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func newReconcileLogstash(objs ...client.Object) *ReconcileLogstash {
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
					Version: "8.6.1",
					Count:   1,
				},
				Status: logstashv1alpha1.LogstashStatus{
					Version:            "8.6.1",
					ExpectedNodes:      1,
					AvailableNodes:     1,
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
						Version: "8.6.1",
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
					Version: "8.6.1",
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
					Version:            "8.6.1",
					ExpectedNodes:      1,
					AvailableNodes:     1,
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
						Version: "8.6.1",
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
					Version: "8.6.1",
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
					Version:            "8.6.1",
					ExpectedNodes:      1,
					AvailableNodes:     1,
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
