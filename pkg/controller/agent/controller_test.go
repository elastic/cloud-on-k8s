// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func newReconcileAgent(objs ...runtime.Object) *ReconcileAgent {
	r := &ReconcileAgent{
		Client:   k8s.NewFakeClient(objs...),
		recorder: record.NewFakeRecorder(100),
	}
	return r
}

func TestReconcileAgent_Reconcile(t *testing.T) {
	type k8sFields struct {
		objs []runtime.Object
	}
	tests := []struct {
		name      string
		k8sfields k8sFields
		request   reconcile.Request
		want      reconcile.Result
		expected  agentv1alpha1.Agent
		wantErr   bool
	}{
		{
			name: "valid unmanaged agent does not increment observedGeneration",
			k8sfields: k8sFields{
				objs: []runtime.Object{
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
			k8sfields: k8sFields{
				objs: []runtime.Object{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconcileAgent(tt.k8sfields.objs...)
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
