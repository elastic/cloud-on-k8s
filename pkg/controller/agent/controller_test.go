// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func newReconcileAgent(objs ...client.Object) *ReconcileAgent {
	r := &ReconcileAgent{
		Client:         k8s.NewFakeClient(objs...),
		recorder:       record.NewFakeRecorder(100),
		dynamicWatches: watches.NewDynamicWatches(),
	}
	return r
}

func TestReconcileAgent_Reconcile(t *testing.T) {
	defaultLabels := (&agentv1alpha1.Agent{ObjectMeta: metav1.ObjectMeta{Name: "testAgent"}}).GetIdentityLabels()
	tests := []struct {
		name     string
		objs     []client.Object
		request  reconcile.Request
		want     reconcile.Result
		expected agentv1alpha1.Agent
		wantErr  bool
	}{
		{
			name: "valid unmanaged agent does not increment observedGeneration",
			objs: []client.Object{
				&agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testAgent",
						Namespace:  "test",
						Generation: 1,
						Annotations: map[string]string{
							common.ManagedAnnotation: "false",
						},
					},
					Spec: agentv1alpha1.AgentSpec{
						Version:    "8.0.1",
						Deployment: &agentv1alpha1.DeploymentSpec{},
					},
					Status: agentv1alpha1.AgentStatus{
						ObservedGeneration: 1,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "testAgent",
				},
			},
			want: reconcile.Result{},
			expected: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testAgent",
					Namespace:  "test",
					Generation: 1,
					Annotations: map[string]string{
						common.ManagedAnnotation: "false",
					},
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:    "8.0.1",
					Deployment: &agentv1alpha1.DeploymentSpec{},
				},
				Status: agentv1alpha1.AgentStatus{
					ObservedGeneration: 1,
				},
			},
			wantErr: false,
		},
		{
			name: "too long name fails validation, and updates observedGeneration",
			objs: []client.Object{
				&agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testAgentwithtoolongofanamereallylongname",
						Namespace:  "test",
						Generation: 2,
					},
					Status: agentv1alpha1.AgentStatus{
						ObservedGeneration: 1,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "testAgentwithtoolongofanamereallylongname",
				},
			},
			want: reconcile.Result{},
			expected: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testAgentwithtoolongofanamereallylongname",
					Namespace:  "test",
					Generation: 2,
				},
				Status: agentv1alpha1.AgentStatus{
					ObservedGeneration: 2,
				},
			},
			wantErr: true,
		},
		{
			name: "agent with ready deployment+pod updates status.health properly",
			objs: []client.Object{
				&agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testAgent",
						Namespace:  "test",
						Generation: 2,
					},
					Spec: agentv1alpha1.AgentSpec{
						Version: "8.0.1",
						Deployment: &agentv1alpha1.DeploymentSpec{
							Replicas: ptr.To[int32](1),
						},
					},
					Status: agentv1alpha1.AgentStatus{
						ObservedGeneration: 1,
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testAgent-agent",
						Namespace: "test",
						Labels:    addLabel(defaultLabels, hash.TemplateHashLabelName, "3145706383"),
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 1,
						Replicas:          1,
						ReadyReplicas:     1,
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "testAgent",
						Namespace:  "test",
						Generation: 2,
						Labels:     map[string]string{NameLabelName: "testAgent", VersionLabelName: "8.0.1"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "testAgent",
				},
			},
			want: reconcile.Result{},
			expected: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "testAgent",
					Namespace:  "test",
					Generation: 2,
				},
				Spec: agentv1alpha1.AgentSpec{
					Version: "8.0.1",
					Deployment: &agentv1alpha1.DeploymentSpec{
						Replicas: ptr.To[int32](1),
					},
				},
				Status: agentv1alpha1.AgentStatus{
					Version:            "8.0.1",
					ExpectedNodes:      1,
					AvailableNodes:     1,
					ObservedGeneration: 2,
					Health:             agentv1alpha1.AgentGreenHealth,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconcileAgent(tt.objs...)
			got, err := r.Reconcile(context.Background(), tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileAgent.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileAgent.Reconcile() = %v, want %v", got, tt.want)
			}

			var agent agentv1alpha1.Agent
			if err := r.Client.Get(context.Background(), tt.request.NamespacedName, &agent); err != nil {
				t.Error(err)
				return
			}
			// AllowUnexported required because of *AssocConf on the agent.
			comparison.AssertEqual(t, &agent, &tt.expected, cmp.AllowUnexported(agentv1alpha1.Agent{}))
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
