// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// +build integration

package common_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/utils/test"
)

const (
	managedNamespace   = "managed"
	unManagedNamespace = "unmanaged"
)

// testReconciler is a fake reconciler to just test whether the Reconcile function is called or not.
type testReconciler struct {
	reconcileCallCount int
}

func (r *testReconciler) Reconcile(context.Context, reconcile.Request) (reconcile.Result, error) {
	r.reconcileCallCount++
	return reconcile.Result{}, nil
}

func TestMain(m *testing.M) {
	test.RunWithK8s(m)
}

func TestManagedNamespacesPredicate(t *testing.T) {
	require.NoError(t, corev1.AddToScheme(scheme.Scheme))
	require.NoError(t, appsv1.AddToScheme(scheme.Scheme))

	reconciler := &testReconciler{}
	mgr, err := manager.New(test.Config, manager.Options{
		Scheme: scheme.Scheme,
	})
	require.NoError(t, err)

	bldr := builder.ControllerManagedBy(mgr).
		For(&appsv1.Deployment{}, builder.
			WithPredicates(common.ManagedNamespacesPredicate([]string{managedNamespace})))
	require.NoError(t, bldr.Complete(reconciler))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		require.NoError(t, mgr.Start(ctx))
	}()

	require.True(t, mgr.GetCache().WaitForCacheSync(ctx))

	tests := []struct {
		name                        string
		objects                     []client.Object
		expectedReconcilerCallCount int
	}{
		{
			"Reconcile is not called for deployment in un-managed namespace",
			[]client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: unManagedNamespace,
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testdeployment",
						Namespace: unManagedNamespace,
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"key": "value",
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"key": "value",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "nginx",
										Image: "nginx",
									},
								},
							},
						},
					},
				},
			},
			0,
		},
		{
			"Reconcile is called for deployment in managed namespace",
			[]client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: managedNamespace,
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testmanageddeployment",
						Namespace: managedNamespace,
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"key": "value",
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"key": "value",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "nginx",
										Image: "nginx",
									},
								},
							},
						},
					},
				},
			},
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := mgr.GetClient()
			for _, object := range tt.objects {
				require.NoError(t, client.Create(context.TODO(), object))
				assert.Eventually(t, func() bool {
					err := client.Get(context.TODO(), types.NamespacedName{Namespace: object.GetNamespace(), Name: object.GetName()}, object)
					return err == nil
				}, 30*time.Second, 2*time.Second)
			}
			assert.Equal(t, tt.expectedReconcilerCallCount, reconciler.reconcileCallCount)
		})
	}
}
