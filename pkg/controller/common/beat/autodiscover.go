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
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	"github.com/go-logr/logr"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	serviceAccountNameTemplate     = "elastic-operator-beat-autodiscover-%s"
	clusterRoleBindingNameTemplate = "elastic-operator-beat-autodiscover-%s-%s"
	clusterRoleName                = "elastic-operator-beat-autodiscover"

	autodiscoverBeatNameLabelName      = "autodiscover.beat.k8s.elastic.co/name"
	autodiscoverBeatNamespaceLabelName = "autodiscover.beat.k8s.elastic.co/namespace"
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

func CleanUp(client k8s.Client, nsName types.NamespacedName) error {
	if ShouldSetupAutodiscoverRBAC() {
		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: ClusterRoleBindingName(nsName.Namespace, nsName.Name),
			},
		}

		if err := client.Delete(clusterRoleBinding); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func IsAutodiscoverResource(meta metav1.Object) (bool, types.NamespacedName) {
	name, okName := meta.GetLabels()[autodiscoverBeatNameLabelName]
	ns, okNs := meta.GetLabels()[autodiscoverBeatNamespaceLabelName]

	if okName && okNs {
		return true, types.NamespacedName{
			Name:      name,
			Namespace: ns,
		}
	}

	return false, types.NamespacedName{}
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
			Labels:    addLabels(labels, owner),
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
			// compare hash of the service account at the time it was built
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
			Name:   ClusterRoleBindingName(owner.GetNamespace(), owner.GetName()),
			Labels: addLabels(nil, owner),
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
			// compare hash of the cluster role binding at the time it was built
			return hash.GetTemplateHashLabel(expected.Labels) != hash.GetTemplateHashLabel(reconciled.Labels)
		},
		UpdateReconciled: func() {
			expected.DeepCopyInto(reconciled)
		},
	})
}

func addLabels(labels map[string]string, owner metav1.Object) map[string]string {
	return maps.Merge(labels, map[string]string{
		autodiscoverBeatNameLabelName:      owner.GetName(),
		autodiscoverBeatNamespaceLabelName: owner.GetNamespace(),
	})
}

func ClusterRoleBindingName(namespace, name string) string {
	return fmt.Sprintf(clusterRoleBindingNameTemplate, namespace, name)
}

func ServiceAccountName(name string) string {
	return fmt.Sprintf(serviceAccountNameTemplate, name)
}
