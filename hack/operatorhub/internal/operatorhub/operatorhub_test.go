// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operatorhub

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	gyaml "github.com/ghodss/yaml"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestSplitRBACRules(t *testing.T) {
	tests := []struct {
		name                   string
		rules                  []rbacv1.PolicyRule
		wantPermissions        []rbacv1.PolicyRule
		wantClusterPermissions []rbacv1.PolicyRule
		wantErr                bool
	}{
		{
			name: "namespaced resource",
			rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
			wantPermissions: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
		},
		{
			name: "cluster-scoped resources",
			rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"namespaces", "nodes"}, Verbs: []string{"get", "list", "watch"}},
				{APIGroups: []string{"storage.k8s.io"}, Resources: []string{"storageclasses"}, Verbs: []string{"get", "list", "watch"}},
				{APIGroups: []string{"admissionregistration.k8s.io"}, Resources: []string{"validatingwebhookconfigurations"}, Verbs: []string{"get"}},
				{APIGroups: []string{"authorization.k8s.io"}, Resources: []string{"subjectaccessreviews"}, Verbs: []string{"create"}},
			},
			wantClusterPermissions: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"namespaces", "nodes"}, Verbs: []string{"get", "list", "watch"}},
				{APIGroups: []string{"storage.k8s.io"}, Resources: []string{"storageclasses"}, Verbs: []string{"get", "list", "watch"}},
				{APIGroups: []string{"admissionregistration.k8s.io"}, Resources: []string{"validatingwebhookconfigurations"}, Verbs: []string{"get"}},
				{APIGroups: []string{"authorization.k8s.io"}, Resources: []string{"subjectaccessreviews"}, Verbs: []string{"create"}},
			},
		},
		{
			name: "mixed resource scope",
			rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods", "nodes"}, ResourceNames: []string{"example"}, Verbs: []string{"get"}},
			},
			wantPermissions: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, ResourceNames: []string{"example"}, Verbs: []string{"get"}},
			},
			wantClusterPermissions: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"nodes"}, ResourceNames: []string{"example"}, Verbs: []string{"get"}},
			},
		},
		{
			name: "non-resource URL",
			rules: []rbacv1.PolicyRule{
				{NonResourceURLs: []string{"/metrics"}, Verbs: []string{"get"}},
			},
			wantClusterPermissions: []rbacv1.PolicyRule{
				{NonResourceURLs: []string{"/metrics"}, Verbs: []string{"get"}},
			},
		},
		{
			name: "unknown resource returns error",
			rules: []rbacv1.PolicyRule{
				{APIGroups: []string{"example.io"}, Resources: []string{"widgets"}, Verbs: []string{"get"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			permissions, clusterPermissions, err := splitRBACRules(tt.rules)
			if (err != nil) != tt.wantErr {
				t.Fatalf("splitRBACRules() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(tt.wantPermissions, permissions) {
				t.Fatalf("unexpected permissions: want %#v, got %#v", tt.wantPermissions, permissions)
			}
			if !reflect.DeepEqual(tt.wantClusterPermissions, clusterPermissions) {
				t.Fatalf("unexpected cluster permissions: want %#v, got %#v", tt.wantClusterPermissions, clusterPermissions)
			}
		})
	}
}

// TestSplitRBACRulesAgainstRealOperatorRBAC parses the actual elastic-operator ClusterRole
// from config/operator.yaml and verifies that every resource is listed in knownResources.
// If this test fails a new cluster-scoped or namespaced resource was added to the ClusterRole
// without being categorized — add it to knownResources in operatorhub.go.
func TestSplitRBACRulesAgainstRealOperatorRBAC(t *testing.T) {
	operatorYAML := filepath.Join("..", "..", "..", "..", "config", "operator.yaml")
	f, err := os.Open(operatorYAML)
	if err != nil {
		t.Fatalf("opening %s: %v", operatorYAML, err)
	}
	defer f.Close()

	extracts, err := extractYAMLParts(f)
	if err != nil {
		t.Fatalf("extracting YAML parts: %v", err)
	}
	if len(extracts.operatorRBAC) == 0 {
		t.Fatal("no RBAC rules extracted from operator.yaml")
	}

	if _, _, err := splitRBACRules(extracts.operatorRBAC); err != nil {
		t.Fatalf("splitRBACRules failed: %v\nAdd the unknown resource(s) to knownResources in operatorhub.go", err)
	}
}

func TestCSVTemplateRendersScopedPermissions(t *testing.T) {
	permissions := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	}
	clusterPermissions := []rbacv1.PolicyRule{
		{APIGroups: []string{"storage.k8s.io"}, Resources: []string{"storageclasses"}, Verbs: []string{"get"}},
	}
	permissionsYAML, err := gyaml.Marshal(permissions)
	if err != nil {
		t.Fatal(err)
	}
	clusterPermissionsYAML, err := gyaml.Marshal(clusterPermissions)
	if err != nil {
		t.Fatal(err)
	}

	params := &RenderParams{
		NewVersion:                 "1.0.0",
		ShortVersion:               "1.0",
		PrevVersion:                "0.9.0",
		StackVersion:               "9.0.0",
		OperatorRepo:               "example.com/eck-operator",
		OperatorPermissions:        string(permissionsYAML),
		OperatorClusterPermissions: string(clusterPermissionsYAML),
		OperatorWebhooks:           "[]\n",
		PackageName:                "elastic-cloud-eck",
		Tag:                        ":1.0.0",
	}
	outPath := filepath.Join(t.TempDir(), "csv.yaml")
	if err := renderTemplate(params, filepath.Join("..", "..", "templates", csvTemplateFile), outPath); err != nil {
		t.Fatal(err)
	}
	rendered, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	type permissionSet struct {
		Rules              []rbacv1.PolicyRule `json:"rules"`
		ServiceAccountName string              `json:"serviceAccountName"`
	}
	var csv struct {
		Spec struct {
			Install struct {
				Spec struct {
					Permissions        []permissionSet `json:"permissions"`
					ClusterPermissions []permissionSet `json:"clusterPermissions"`
				} `json:"spec"`
			} `json:"install"`
		} `json:"spec"`
	}
	if err := gyaml.Unmarshal(rendered, &csv); err != nil {
		t.Fatal(err)
	}

	if len(csv.Spec.Install.Spec.Permissions) != 1 {
		t.Fatalf("expected one namespaced permission set, got %d", len(csv.Spec.Install.Spec.Permissions))
	}
	if !reflect.DeepEqual(permissions, csv.Spec.Install.Spec.Permissions[0].Rules) {
		t.Fatalf("unexpected permissions: want %#v, got %#v", permissions, csv.Spec.Install.Spec.Permissions[0].Rules)
	}
	if csv.Spec.Install.Spec.Permissions[0].ServiceAccountName != operatorName {
		t.Fatalf("unexpected permissions service account: %s", csv.Spec.Install.Spec.Permissions[0].ServiceAccountName)
	}
	if len(csv.Spec.Install.Spec.ClusterPermissions) != 1 {
		t.Fatalf("expected one cluster permission set, got %d", len(csv.Spec.Install.Spec.ClusterPermissions))
	}
	if !reflect.DeepEqual(clusterPermissions, csv.Spec.Install.Spec.ClusterPermissions[0].Rules) {
		t.Fatalf("unexpected cluster permissions: want %#v, got %#v", clusterPermissions, csv.Spec.Install.Spec.ClusterPermissions[0].Rules)
	}
	if csv.Spec.Install.Spec.ClusterPermissions[0].ServiceAccountName != operatorName {
		t.Fatalf("unexpected cluster permissions service account: %s", csv.Spec.Install.Spec.ClusterPermissions[0].ServiceAccountName)
	}
}
