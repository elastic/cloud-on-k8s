// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"context"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/go-logr/logr"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	serviceAccountNameTemplate     = "elastic-operator-autodiscover-%s"
	clusterRoleBindingNameTemplate = "elastic-operator-autodiscover-%s-%s"
	clusterRoleName                = "elastic-operator-beat"
)

var (
	shouldSetupRBAC = false
)

// EnableAutodiscoverRBACSetup enables setting up autodiscover RBAC.
func EnableAutodiscoverRBACSetup() {
	shouldSetupRBAC = true
}

// ShouldSetupAutodiscoverRBAC returns true if autodiscover RBAC is expected to be set up by the operator.
func ShouldSetupAutodiscoverRBAC() bool {
	return shouldSetupRBAC
}

// SetupAutodiscoveryRBAC reconciles all resources needed for the default RBAC setup.
func SetupAutodiscoverRBAC(ctx context.Context, log logr.Logger, client k8s.Client, owner metav1.Object, labels map[string]string) error {
	// this is the source of truth and should be respected at all times
	if !ShouldSetupAutodiscoverRBAC() {
		return nil
	}

	err := setupAutodiscoverRBAC(ctx, client, owner, labels)
	if err != nil {
		log.V(1).Info(
			"autodiscovery rbac setup failed",
			"namespace", owner.GetNamespace(),
			"beat_name", owner.GetName())
	}

	return err
}

func setupAutodiscoverRBAC(ctx context.Context, client k8s.Client, owner metav1.Object, labels map[string]string) error {
	span, _ := apm.StartSpan(ctx, "reconcile_autodiscover_rbac", tracing.SpanTypeApp)
	defer span.End()

	if err := reconcileServiceAccount(client, owner, labels); err != nil {
		return err
	}

	if err := reconcileClusterRoleBinding(client, owner); err != nil {
		return err
	}

	return nil
}

func reconcileServiceAccount(client k8s.Client, owner metav1.Object, labels map[string]string) error {
	expected := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceAccountName(owner.GetName()),
			Namespace: owner.GetNamespace(),
			Labels:    labels,
		},
	}
	expected.Labels = hash.SetTemplateHashLabel(nil, expected)

	reconciled := &corev1.ServiceAccount{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     client,
		Owner:      owner,
		Expected:   expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// compare hash of the deployment at the time it was built
			return hash.GetTemplateHashLabel(expected.Labels) != hash.GetTemplateHashLabel(reconciled.Labels)
		},
		UpdateReconciled: func() {
			expected.DeepCopyInto(reconciled)
		},
	})
}

func reconcileClusterRoleBinding(client k8s.Client, owner metav1.Object) error {
	expected := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: ClusterRoleBindingName(owner.GetNamespace(), owner.GetName()),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ServiceAccountName(owner.GetName()),
				Namespace: owner.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
	}

	reconciled := &rbacv1.ClusterRoleBinding{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     client,
		Expected:   expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// compare hash of the deployment at the time it was built
			return hash.GetTemplateHashLabel(expected.Labels) != hash.GetTemplateHashLabel(reconciled.Labels)
		},
		UpdateReconciled: func() {
			expected.DeepCopyInto(reconciled)
		},
	})
}

func ClusterRoleBindingName(namespace, name string) string {
	return fmt.Sprintf(clusterRoleBindingNameTemplate, namespace, name)
}

func ServiceAccountName(name string) string {
	return fmt.Sprintf(serviceAccountNameTemplate, name)
}
