// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

const (
	// serviceAccountNameTemplate is the template to be used with Beat name to obtain the name of a ServiceAccount.
	// Note that users might depend on it.
	serviceAccountNameTemplate = "elastic-operator-beat-%s"

	// clusterRoleBindingNameTemplate is the template to be used with Beat namespace and name to obtain the name of
	// a ClusterRoleBinding.
	clusterRoleBindingNameTemplate = "elastic-operator-beat-autodiscover-%s-%s"

	// clusterRoleName is the name of the ClusterRole. If autodiscover RBAC management is enabled, operator assumes
	// that this role already exists in the cluster.
	clusterRoleName = "elastic-operator-beat-autodiscover"

	// autodiscoverBeatNameLabelName is a label name that is applied to ClusterRoleBinding for autodiscover
	// permissions. Label value is the name of the Beat resource that the binding is for.
	autodiscoverBeatNameLabelName = "autodiscover.beat.k8s.elastic.co/name"

	// autodiscoverBeatNamespaceLabelName is a label name that is applied to ClusterRoleBinding for autodiscover
	// permissions. Label value is the namespace of the Beat resource that the binding is for.
	autodiscoverBeatNamespaceLabelName = "autodiscover.beat.k8s.elastic.co/namespace"
)

var (
	shouldManageRBAC = false
)

// EnableAutodiscoverRBACManagement enables setting up autodiscover RBAC.
func EnableAutodiscoverRBACManagement() {
	shouldManageRBAC = true
}

// ShouldManageAutodiscoverRBAC returns true if autodiscover RBAC is expected to be set up by the operator.
func ShouldManageAutodiscoverRBAC() bool {
	return shouldManageRBAC
}

// SetupAutodiscoveryRBAC reconciles all resources needed for the default RBAC setup.
func ReconcileAutodiscoverRBAC(ctx context.Context, log logr.Logger, client k8s.Client, beat beatv1beta1.Beat) error {
	if !ShouldManageAutodiscoverRBAC() {
		return nil
	}

	err := reconcileAutodiscoverRBAC(ctx, client, beat)
	if err != nil {
		log.V(1).Info(
			"autodiscovery rbac setup failed",
			"namespace", beat.Namespace,
			"beat_name", beat.Name)
	}

	return err
}

func CleanUp(client k8s.Client, nsName types.NamespacedName) error {
	if ShouldManageAutodiscoverRBAC() {
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
	labels := meta.GetLabels()
	if labels == nil {
		return false, types.NamespacedName{}
	}

	name, okName := labels[autodiscoverBeatNameLabelName]
	ns, okNs := labels[autodiscoverBeatNamespaceLabelName]
	if okName && okNs {
		return true, types.NamespacedName{
			Name:      name,
			Namespace: ns,
		}
	}

	return false, types.NamespacedName{}
}

func reconcileAutodiscoverRBAC(ctx context.Context, client k8s.Client, beat beatv1beta1.Beat) error {
	span, _ := apm.StartSpan(ctx, "reconcile_autodiscover_rbac", tracing.SpanTypeApp)
	defer span.End()

	if err := reconcileServiceAccount(client, beat); err != nil {
		return err
	}

	if err := reconcileClusterRoleBinding(client, beat); err != nil {
		return err
	}

	return nil
}

func reconcileServiceAccount(client k8s.Client, beat beatv1beta1.Beat) error {
	expected := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceAccountName(beat.Name),
			Namespace: beat.Namespace,
			Labels:    addLabels(NewLabels(beat), beat),
		},
	}
	expected.Labels = hash.SetTemplateHashLabel(nil, expected)

	reconciled := &corev1.ServiceAccount{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     client,
		Owner:      &beat,
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

func reconcileClusterRoleBinding(client k8s.Client, beat beatv1beta1.Beat) error {
	expected := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ClusterRoleBindingName(beat.Namespace, beat.Name),
			Labels: addLabels(nil, beat),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ServiceAccountName(beat.Name),
				Namespace: beat.Namespace,
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

func addLabels(labels map[string]string, beat beatv1beta1.Beat) map[string]string {
	return maps.Merge(labels, map[string]string{
		autodiscoverBeatNameLabelName:      beat.Name,
		autodiscoverBeatNamespaceLabelName: beat.Namespace,
	})
}

func ClusterRoleBindingName(namespace, name string) string {
	return fmt.Sprintf(clusterRoleBindingNameTemplate, namespace, name)
}

func ServiceAccountName(name string) string {
	return fmt.Sprintf(serviceAccountNameTemplate, name)
}
