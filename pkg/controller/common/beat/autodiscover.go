// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"context"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

const (
	AutodiscoverServiceAccountName     = "elastic-operator-autodiscover"
	autodiscoverClusterRoleBindingName = "elastic-operator-autodiscover"
	autodiscoverClusterRoleName        = "elastic-operator-autodiscover"
)

var (
	shouldSetupRBAC = false
)

// EnableAutodiscoverRBACSetup enables setting up autodiscover RBAC
func EnableAutodiscoverRBACSetup() {
	shouldSetupRBAC = true
}

// ShouldSetupAutodiscoverRBAC returns true if autodiscover RBAC is expected to be set up by the operator
func ShouldSetupAutodiscoverRBAC() bool {
	return shouldSetupRBAC
}

// SetupAutodiscoverRBAC reconciles all resources needed for default RBAC setup
func SetupAutodiscoverRBAC(ctx context.Context, client k8s.Client, owner metav1.Object, labels map[string]string) error {
	// this is the source of truth and should be respected at all times
	if !ShouldSetupAutodiscoverRBAC() {
		return nil
	}

	span, _ := apm.StartSpan(ctx, "reconcile_autodiscover_rbac", tracing.SpanTypeApp)
	defer span.End()

	if err := reconcileServiceAccount(client, owner, labels); err != nil {
		return err
	}

	if err := reconcileClusterRole(client); err != nil {
		return err
	}

	if err := reconcileClusterRoleBinding(client, owner); err != nil {
		return err
	}

	return nil
}

func reconcileServiceAccount(client k8s.Client, owner metav1.Object, labels map[string]string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AutodiscoverServiceAccountName,
			Namespace: owner.GetNamespace(),
			Labels:    labels,
		},
	}

	return reconcile(client, sa, owner)
}

func reconcileClusterRole(client k8s.Client) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: autodiscoverClusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Verbs:     []string{"get", "watch", "list"},
				Resources: []string{"namespaces", "pods"},
			},
		},
	}

	return reconcile(client, role, nil)
}

func reconcileClusterRoleBinding(client k8s.Client, owner metav1.Object) error {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: autodiscoverClusterRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      AutodiscoverServiceAccountName,
				Namespace: owner.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     autodiscoverClusterRoleName,
		},
	}

	return reconcile(client, binding, nil)
}

func reconcile(client k8s.Client, obj runtime.Object, owner metav1.Object) error {
	// labels set here must have be exact the same for all callers in particular namespace
	// otherwise they'll just keep trying to override each other
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	objMeta.SetLabels(maps.Merge(objMeta.GetLabels(), hash.SetTemplateHashLabel(nil, obj)))

	obj2 := obj.DeepCopyObject()
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     client,
		Owner:      owner,
		Expected:   obj,
		Reconciled: obj2,
		NeedsUpdate: func() bool {
			objMeta2, err := meta.Accessor(obj2)

			// compare hash of the deployment at the time it was built
			return err != nil || hash.GetTemplateHashLabel(objMeta.GetLabels()) != hash.GetTemplateHashLabel(objMeta2.GetLabels())
		},
		UpdateReconciled: func() {
		},
	})
}
