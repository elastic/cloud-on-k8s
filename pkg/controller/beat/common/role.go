// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"fmt"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// serviceAccountNameTemplate is the template to be used with Beat name to obtain the name of a ServiceAccount.
	// Note that users might depend on it.
	serviceAccountNameTemplate = "elastic-beat-%s"

	// clusterRoleBindingNameTemplate is the template to be used with role name, Beat namespace and Beat name to
	// obtain the name of a ClusterRoleBinding.
	clusterRoleBindingNameTemplate = "%s-binding-%s-%s"

	// autodiscoverRoleName is the name of the autodiscover ClusterRole. Operator assumes that this role
	// already exists in the cluster.
	autodiscoverRoleName = "elastic-beat-autodiscover"

	presetRoleNamePrefix = "elastic-beat-preset-"

	// MetricbeatK8sHostsPresetRole is the name of the role holding permissions needed by metricbeat-k8s-hosts preset.
	// Operator assumes that this role already exists in the cluster.
	MetricbeatK8sHostsPresetRole = presetRoleNamePrefix + string(beatv1beta1.MetricbeatK8sHostsPreset)

	// beatRoleBindingNameLabelName is a label name that is applied to ClusterRoleBinding of ECK-managed Beat
	//roles. Label value is the name of the Beat resource that the binding is for.
	beatRoleBindingNameLabelName = "role.beat.k8s.elastic.co/name"

	// beatRoleBindingNamespaceLabelName is a label name that is applied to ClusterRoleBinding of ECK-managed Beat
	// roles. Label value is the namespace of the Beat resource that the binding is for.
	beatRoleBindingNamespaceLabelName = "role.beat.k8s.elastic.co/namespace"
)

var (
	shouldManageRBAC = false
	roleRefs         = map[string]rbacv1.RoleRef{
		autodiscoverRoleName: {
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     autodiscoverRoleName,
		},
		MetricbeatK8sHostsPresetRole: {
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     MetricbeatK8sHostsPresetRole,
		},
	}
)

// EnableRBACManagement enables setting up autodiscover RBAC
func EnableRBACManagement() {
	shouldManageRBAC = true
}

// ShouldManageRBAC returns true if autodiscover RBAC is expected to be set up by the operator
func ShouldManageRBAC() bool {
	return shouldManageRBAC
}

func IsManagedRBACResource(meta metav1.Object) (bool, types.NamespacedName) {
	labels := meta.GetLabels()
	if labels == nil {
		return false, types.NamespacedName{}
	}

	name, okName := labels[beatRoleBindingNameLabelName]
	ns, okNs := labels[beatRoleBindingNamespaceLabelName]
	if okName && okNs {
		return true, types.NamespacedName{
			Name:      name,
			Namespace: ns,
		}
	}

	return false, types.NamespacedName{}
}

func DeleteRBACResources(client k8s.Client, nsName types.NamespacedName) error {
	if !ShouldManageRBAC() {
		return nil
	}

	for roleName := range roleRefs {
		clusterRoleBindingName := clusterRoleBindingName(roleName, nsName.Namespace, nsName.Name)
		clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		err := client.Get(types.NamespacedName{Name: clusterRoleBindingName}, clusterRoleBinding)
		if !apierrors.IsNotFound(err) {
			clusterRoleBinding = &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterRoleBindingName,
				},
			}
			if err := client.Delete(clusterRoleBinding); err != nil {
				if !apierrors.IsNotFound(err) {
					return err
				}
			}
		}
	}

	saName := fmt.Sprintf(serviceAccountNameTemplate, nsName.Name)
	sa := &corev1.ServiceAccount{}
	err := client.Get(types.NamespacedName{Name: saName, Namespace: nsName.Namespace}, sa)
	if !apierrors.IsNotFound(err) {
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: nsName.Namespace,
			},
		}
		if err := client.Delete(sa); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func ServiceAccountName(name string) string {
	return fmt.Sprintf(serviceAccountNameTemplate, name)
}

func shouldManageRBACFor(podSpec corev1.PodSpec) bool {
	return ShouldManageRBAC() && podSpec.ServiceAccountName == ""
}

// reconcileRBAC reconciles all resources needed for the Beat RBAC setup
func reconcileRBAC(cfgBytes []byte, roles []string, params DriverParams) error {
	if !shouldManageRBACFor(params.GetPodTemplate().Spec) {
		return nil
	}

	// in addition to roles from the preset, we bind autodiscover role if the feature is used in the config
	if required, err := isAutodiscoverRoleRequired(cfgBytes); err != nil {
		return err
	} else if required {
		roles = append(roles, autodiscoverRoleName)
	}

	err := internalReconcileRBAC(params.Context, params.Client, params.Beat, roles)
	if err != nil {
		params.Logger.V(1).Info(
			"beat rbac setup failed",
			"namespace", params.Beat.Namespace,
			"beat_name", params.Beat.Name)
	}

	return err
}

func isAutodiscoverRoleRequired(cfgBytes []byte) (bool, error) {
	cfg, err := uyaml.NewConfig(cfgBytes)
	if err != nil {
		return false, err
	}

	for _, field := range cfg.GetFields() {
		child, err := cfg.Child(field, -1)
		if err != nil {
			return false, err
		}
		if child.HasField("autodiscover") {
			return true, nil
		}
	}

	return false, nil
}

func internalReconcileRBAC(ctx context.Context, client k8s.Client, beat beatv1beta1.Beat, roles []string) error {
	span, _ := apm.StartSpan(ctx, "reconcile_autodiscover_rbac", tracing.SpanTypeApp)
	defer span.End()

	if err := reconcileServiceAccount(client, beat); err != nil {
		return err
	}

	for _, roleName := range roles {
		roleRef, ok := roleRefs[roleName]
		if !ok {
			return fmt.Errorf("role name %s not known", roleName)
		}
		if err := reconcileClusterRoleBinding(client, beat, roleRef); err != nil {
			return err
		}
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

func reconcileClusterRoleBinding(client k8s.Client, beat beatv1beta1.Beat, roleRef rbacv1.RoleRef) error {
	expected := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clusterRoleBindingName(roleRef.Name, beat.Namespace, beat.Name),
			Labels: addLabels(nil, beat),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ServiceAccountName(beat.Name),
				Namespace: beat.Namespace,
			},
		},
		RoleRef: roleRef,
	}
	expected.Labels = hash.SetTemplateHashLabel(expected.Labels, expected)

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
		beatRoleBindingNameLabelName:      beat.Name,
		beatRoleBindingNamespaceLabelName: beat.Namespace,
	})
}

func clusterRoleBindingName(role, namespace, name string) string {
	return fmt.Sprintf(clusterRoleBindingNameTemplate, role, namespace, name)
}
