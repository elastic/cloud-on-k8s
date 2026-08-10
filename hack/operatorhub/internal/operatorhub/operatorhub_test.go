// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operatorhub

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	gyaml "github.com/ghodss/yaml"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSplitRBACRules(t *testing.T) {
	tests := []struct {
		name                   string
		scopeMap               map[schema.GroupResource]bool
		rules                  []rbacv1.PolicyRule
		wantPermissions        []rbacv1.PolicyRule
		wantClusterPermissions []rbacv1.PolicyRule
		wantErr                bool
	}{
		{
			name: "namespaced resource",
			scopeMap: map[schema.GroupResource]bool{
				{Resource: "pods"}: false,
			},
			rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
			wantPermissions: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
		},
		{
			name: "cluster-scoped resources",
			scopeMap: map[schema.GroupResource]bool{
				{Resource: "namespaces"}:                              true,
				{Resource: "nodes"}:                                   true,
				{Group: "storage.k8s.io", Resource: "storageclasses"}: true,
				{Group: "admissionregistration.k8s.io", Resource: "validatingwebhookconfigurations"}: true,
				{Group: "authorization.k8s.io", Resource: "subjectaccessreviews"}:                    true,
			},
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
			scopeMap: map[schema.GroupResource]bool{
				{Resource: "pods"}:  false,
				{Resource: "nodes"}: true,
			},
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
		{
			name: "empty apiGroups with resources returns error",
			rules: []rbacv1.PolicyRule{
				{APIGroups: []string{}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
			wantErr: true,
		},
		{
			name: "nonResourceURLs and resources together returns error",
			rules: []rbacv1.PolicyRule{
				{NonResourceURLs: []string{"/metrics"}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
			wantErr: true,
		},
		{
			// A rule with N apiGroups fans out into N separate rules, one per group.
			name: "multiple apiGroups fans out one rule per group",
			scopeMap: map[schema.GroupResource]bool{
				{Group: "", Resource: "pods"}:            false,
				{Group: "apps", Resource: "pods"}:        false,
				{Group: "", Resource: "deployments"}:     false,
				{Group: "apps", Resource: "deployments"}: false,
			},
			rules: []rbacv1.PolicyRule{
				{APIGroups: []string{"", "apps"}, Resources: []string{"pods", "deployments"}, Verbs: []string{"get"}},
			},
			wantPermissions: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods", "deployments"}, Verbs: []string{"get"}},
				{APIGroups: []string{"apps"}, Resources: []string{"pods", "deployments"}, Verbs: []string{"get"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			permissions, clusterPermissions, err := splitRBACRules(tt.rules, tt.scopeMap)
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
// from config/operator.yaml and verifies that every resource is covered by either the CRD
// scopes derived from config/crds.yaml or the built-in knownResources map.
// If this test fails a new resource was added to the ClusterRole without a matching CRD or
// knownResources entry — add a CRD to config/crds.yaml or add the built-in resource to
// knownResources in operatorhub.go.
func TestSplitRBACRulesAgainstRealOperatorRBAC(t *testing.T) {
	crdsYAML := filepath.Join("..", "..", "..", "..", "config", "crds.yaml")
	crdsFile, err := os.Open(crdsYAML)
	if err != nil {
		t.Fatalf("opening %s: %v", crdsYAML, err)
	}
	defer crdsFile.Close()

	crdExtracts, err := extractYAMLParts(crdsFile)
	if err != nil {
		t.Fatalf("extracting CRD YAML parts: %v", err)
	}

	// Build the same combined scope map the production code uses.
	allScopes := make(map[schema.GroupResource]bool, len(knownResources)+len(crdExtracts.crds))
	maps.Copy(allScopes, knownResources)
	for _, crd := range crdExtracts.crds {
		allScopes[schema.GroupResource{Group: crd.Group, Resource: crd.Plural}] = crd.Scope == apiextv1.ClusterScoped
	}

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

	permissions, clusterPermissions, err := splitRBACRules(extracts.operatorRBAC, allScopes)
	if err != nil {
		t.Fatalf("splitRBACRules failed: %v", err)
	}

	// Assert that the exact set of cluster-scoped Group/Resource pairs lands in clusterPermissions.
	// A resource mis-categorised as namespaced (the bug #9654 described) would be absent here,
	// turning this into a regression guard for the split itself, not just for a missing map entry.
	wantClusterResources := map[schema.GroupResource]struct{}{
		{Resource: "namespaces"}: {},
		{Resource: "nodes"}:      {},
		{Group: "admissionregistration.k8s.io", Resource: "validatingwebhookconfigurations"}: {},
		{Group: "authorization.k8s.io", Resource: "subjectaccessreviews"}:                    {},
		{Group: "storage.k8s.io", Resource: "storageclasses"}:                                {},
	}
	gotClusterResources := make(map[schema.GroupResource]struct{})
	for _, rule := range clusterPermissions {
		for _, group := range rule.APIGroups {
			for _, resource := range rule.Resources {
				gotClusterResources[schema.GroupResource{Group: group, Resource: resource}] = struct{}{}
			}
		}
	}
	if !reflect.DeepEqual(gotClusterResources, wantClusterResources) {
		t.Errorf("unexpected cluster-scoped resources in clusterPermissions:\n got  %v\nwant %v", gotClusterResources, wantClusterResources)
	}

	// Verify every entry in allScopes (built-ins + CRDs) is covered by at least one RBAC rule.
	// This is not a generation-correctness check (splitRBACRules already errors on unknown resources);
	// it guards against stale entries accumulating in knownResources after an RBAC rule is removed.
	covered := make(map[schema.GroupResource]struct{})
	for _, rules := range [][]rbacv1.PolicyRule{permissions, clusterPermissions} {
		for _, rule := range rules {
			for _, group := range rule.APIGroups {
				for _, resource := range rule.Resources {
					base, _, _ := strings.Cut(resource, "/")
					covered[schema.GroupResource{Group: group, Resource: base}] = struct{}{}
				}
			}
		}
	}
	for gr := range allScopes {
		if _, ok := covered[gr]; !ok {
			t.Errorf("resource %v is in allScopes but has no RBAC rule in operator.yaml", gr)
		}
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
