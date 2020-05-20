// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
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
	serviceAccountNameTemplate     = "elastic-operator-autodiscover-%s"
	clusterRoleBindingNameTemplate = "elastic-operator-autodiscover-%s-%s"
	clusterRoleName                = "elastic-operator-autodiscover"
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

// SetupAutodiscoveryRBAC reconciles all resources needed for default RBAC setup
func SetupAutodiscoverRBAC(ctx context.Context, log logr.Logger, client k8s.Client, owner metav1.Object, labels map[string]string) error {
	if ShouldSetupAutodiscoverRBAC() {
		if err := setupAutodiscoverRBAC(ctx, client, owner, labels); err != nil {
			log.V(1).Info(
				"autodiscovery rbac setup failed",
				"namespace", owner.GetNamespace(),
				"beat_name", owner.GetName())
			return err
		}
	}
	return nil
}

func setupAutodiscoverRBAC(ctx context.Context, client k8s.Client, owner metav1.Object, labels map[string]string) error {
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
			Name:      ServiceAccountName(owner.GetName()),
			Namespace: owner.GetNamespace(),
			Labels:    labels,
		},
	}

	return doReconcile(client, sa, owner)
}

func reconcileClusterRole(client k8s.Client) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Verbs:     []string{"get", "watch", "list"},
				Resources: []string{"namespaces", "pods"},
			},
		},
	}

	return doReconcile(client, role, nil)
}

func reconcileClusterRoleBinding(client k8s.Client, owner metav1.Object) error {
	binding := &rbacv1.ClusterRoleBinding{
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

	return doReconcile(client, binding, nil)
}

func doReconcile(client k8s.Client, obj runtime.Object, owner metav1.Object) error {
	// labels set here must be exactly the same for all callers in particular namespace
	// otherwise they'll just keep trying to override each other
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	objMeta.SetLabels(maps.Merge(objMeta.GetLabels(), hash.SetTemplateHashLabel(nil, obj)))

	reconciled := obj.DeepCopyObject()
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     client,
		Owner:      owner,
		Expected:   obj,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			objMeta2, err := meta.Accessor(reconciled)

			// compare hash of the deployment at the time it was built
			return err != nil || hash.GetTemplateHashLabel(objMeta.GetLabels()) != hash.GetTemplateHashLabel(objMeta2.GetLabels())
		},
		UpdateReconciled: func() {
			reflect.ValueOf(reconciled).Elem().Set(reflect.ValueOf(obj).Elem())
		},
	})
}

func ClusterRoleBindingName(namespace, name string) string {
	return fmt.Sprintf(clusterRoleBindingNameTemplate, namespace, name)
}

func ServiceAccountName(name string) string {
	return fmt.Sprintf(serviceAccountNameTemplate, name)
}
