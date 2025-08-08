// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	"context"
	"fmt"
	"slices"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

var (
	// group the statefulsets by the priority of their roles.
	// master, data_*, ingest, ml, transform, coordinating, and we ignore remote_cluster_client as it has no impact on availability
	priority = []esv1.NodeRole{esv1.MasterRole, esv1.DataRole, esv1.DataFrozenRole, esv1.IngestRole, esv1.MLRole, esv1.TransformRole, esv1.CoordinatingRole}
	// All data role variants should be treated as a generic data role for PDB purposes
	dataRoles = []esv1.NodeRole{
		esv1.DataRole,
		esv1.DataHotRole,
		esv1.DataWarmRole,
		esv1.DataColdRole,
		esv1.DataContentRole,
		// Note: DataFrozenRole is excluded as it has different disruption rules (yellow+ health)
	}
)

// toGenericDataRole returns the normalized form of a role where any data role
// is normalized to the same data role.
func toGenericDataRole(role esv1.NodeRole) esv1.NodeRole {
	if slices.Contains(dataRoles, role) {
		return esv1.DataRole
	}
	return role
}

// reconcileRoleSpecificPDBs creates and reconciles PodDisruptionBudgets per nodeSet roles for enterprise-licensed clusters.
func reconcileRoleSpecificPDBs(
	ctx context.Context,
	k8sClient k8s.Client,
	es esv1.Elasticsearch,
	statefulSets sset.StatefulSetList,
	resources nodespec.ResourcesList,
	meta metadata.Metadata,
) error {
	// Check if PDB is disabled in the ES spec, and if so delete all existing PDBs (both default and role-specific)
	// that have a proper owner reference.
	if es.Spec.PodDisruptionBudget != nil && es.Spec.PodDisruptionBudget.IsDisabled() {
		if err := deleteDefaultPDB(ctx, k8sClient, es); err != nil {
			return err
		}
		return deleteAllRoleSpecificPDBs(ctx, k8sClient, es)
	}

	// Retrieve the expected list of PDBs.
	pdbs, err := expectedRolePDBs(es, statefulSets, resources, meta)
	if err != nil {
		return fmt.Errorf("while retrieving expected role-specific PDBs: %w", err)
	}

	// Reconcile and delete unnecessary role-specific PDBs that could have been created
	// by a previous reconciliation with a different set of StatefulSets.
	if err := reconcileAndDeleteUnnecessaryPDBs(ctx, k8sClient, es, pdbs); err != nil {
		return err
	}

	// Always ensure any existing default PDB is removed.
	if err := deleteDefaultPDB(ctx, k8sClient, es); err != nil {
		return fmt.Errorf("while deleting the default PDB: %w", err)
	}

	return nil
}

// expectedRolePDBs returns a slice of PDBs to reconcile based on statefulSet roles.
func expectedRolePDBs(
	es esv1.Elasticsearch,
	statefulSets sset.StatefulSetList,
	resources nodespec.ResourcesList,
	meta metadata.Metadata,
) ([]*policyv1.PodDisruptionBudget, error) {
	pdbs := make([]*policyv1.PodDisruptionBudget, 0, len(statefulSets))

	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, fmt.Errorf("while parsing Elasticsearch version: %w", err)
	}

	// Group StatefulSets by their connected roles.
	groups, err := groupBySharedRoles(statefulSets, resources, v)
	if err != nil {
		return nil, fmt.Errorf("while grouping StatefulSets by roles: %w", err)
	}

	// Create one PDB per group
	// Maps order isn't guaranteed so process in order of defined priority.
	for _, roleName := range priority {
		group, ok := groups[string(roleName)]
		if !ok {
			continue
		}
		if len(group) == 0 {
			continue
		}

		// Determine the roles for this group
		groupRoles := sets.New[esv1.NodeRole]()
		for _, sset := range group {
			roles, err := getRolesForStatefulSet(sset, resources, v)
			if err != nil {
				return nil, fmt.Errorf("while getting roles for StatefulSet %s: %w", sset.Name, err)
			}
			for _, role := range roles {
				groupRoles.Insert(esv1.NodeRole(role))
			}
		}

		// Determine the most conservative role to use when determining the maxUnavailable setting.
		// If group has no roles, it's a coordinating ES role.
		primaryRole := getPrimaryRoleForPDB(groupRoles)

		pdb, err := createPDBForStatefulSets(es, primaryRole, string(roleName), group, statefulSets, meta)
		if err != nil {
			return nil, err
		}
		if pdb != nil {
			pdbs = append(pdbs, pdb)
		}
	}

	return pdbs, nil
}

func groupBySharedRoles(statefulSets sset.StatefulSetList, resources nodespec.ResourcesList, v version.Version) (map[string][]appsv1.StatefulSet, error) {
	n := len(statefulSets)
	if n == 0 {
		return map[string][]appsv1.StatefulSet{}, nil
	}
	rolesToIndices := make(map[string][]int)
	indicesToRoles := make(map[int]set.StringSet)
	for i, sset := range statefulSets {
		roles, err := getRolesForStatefulSet(sset, resources, v)
		if err != nil {
			return nil, err
		}
		if len(roles) == 0 {
			// StatefulSets with no roles are coordinating nodes - group them together
			rolesToIndices[string(esv1.CoordinatingRole)] = append(rolesToIndices[string(esv1.CoordinatingRole)], i)
			indicesToRoles[i] = set.Make(string(esv1.CoordinatingRole))
			continue
		}
		for _, role := range roles {
			// Ensure that the data* roles are grouped together.
			normalizedRole := string(toGenericDataRole(esv1.NodeRole(role)))
			rolesToIndices[normalizedRole] = append(rolesToIndices[normalizedRole], i)
			if _, ok := indicesToRoles[i]; !ok {
				indicesToRoles[i] = set.Make()
			}
			indicesToRoles[i].Add(normalizedRole)
		}
	}

	// This keeps track of which roles have been assigned to a PDB to avoid assigning the same role to multiple PDBs.
	roleToTargetPDB := map[string]string{}
	grouped := map[string][]int{}
	visited := make([]bool, n)
	for _, role := range priority {
		indices, ok := rolesToIndices[string(role)]
		if !ok {
			continue
		}
		for _, idx := range indices {
			if visited[idx] {
				continue
			}
			targetPDBRole := string(role)
			// if we already assigned a PDB for this role, use that instead
			if target, ok := roleToTargetPDB[string(role)]; ok {
				targetPDBRole = target
			}
			grouped[targetPDBRole] = append(grouped[targetPDBRole], idx)
			for _, r := range indicesToRoles[idx].AsSlice() {
				roleToTargetPDB[r] = targetPDBRole
			}
			visited[idx] = true
		}
	}
	// transform into the expected format
	res := make(map[string][]appsv1.StatefulSet)
	for role, indices := range grouped {
		group := make([]appsv1.StatefulSet, 0, len(indices))
		for _, idx := range indices {
			group = append(group, statefulSets[idx])
		}
		res[role] = group
	}
	return res, nil
}

// getPrimaryRoleForPDB returns the primary role from a set of roles for PDB naming and grouping.
// Data roles are most restrictive (require green health), so they take priority.
// All other roles have similar disruption rules (require yellow+ health).
func getPrimaryRoleForPDB(roles sets.Set[esv1.NodeRole]) esv1.NodeRole {
	if len(roles) == 0 {
		return "" // coordinating role
	}

	// Data roles are most restrictive (require green health), so they take priority.
	// Check if any data role variant is present (excluding data_frozen)
	if slices.ContainsFunc(dataRoles, func(dataRole esv1.NodeRole) bool {
		return roles.Has(dataRole)
	}) {
		// Return generic data role for all data role variants
		return esv1.DataRole
	}

	// Master role comes next in priority
	if _, ok := roles[esv1.MasterRole]; ok {
		return esv1.MasterRole
	}

	// Data frozen role (has different disruption rules than other data roles)
	if _, ok := roles[esv1.DataFrozenRole]; ok {
		return esv1.DataFrozenRole
	}

	// Return the first role we encounter in a deterministic order
	// Define a priority order for non-data roles
	nonDataRoles := []esv1.NodeRole{
		esv1.IngestRole,
		esv1.MLRole,
		esv1.TransformRole,
		esv1.RemoteClusterClientRole,
	}

	// Check non-data roles in priority order
	for _, role := range nonDataRoles {
		if _, ok := roles[role]; ok {
			return role
		}
	}

	// If no known role found, return any role from the map
	for role := range roles {
		return role
	}

	// Should never reach here if roles is not empty
	return ""
}

// getRolesForStatefulSet gets the roles from a StatefulSet's expected configuration.
func getRolesForStatefulSet(
	statefulSet appsv1.StatefulSet,
	expectedResources nodespec.ResourcesList,
	v version.Version,
) ([]string, error) {
	forStatefulSet, err := expectedResources.ForStatefulSet(statefulSet.Name)
	if err != nil {
		return nil, err
	}
	cfg, err := forStatefulSet.Config.Unpack(v)
	if err != nil {
		return nil, err
	}
	return cfg.Node.Roles, nil
}

// createPDBForStatefulSets creates a PDB for a group of StatefulSets with shared roles.
func createPDBForStatefulSets(
	es esv1.Elasticsearch,
	// role is the role used to determine the maxUnavailable value.
	role esv1.NodeRole,
	// roleName is used to determine the name of the PDB.
	roleName string,
	// statefulSets are the statefulSets grouped into this pdb.
	statefulSets []appsv1.StatefulSet,
	// allStatefulSets are all statefulsets in the whole ES cluster.
	allStatefulSets sset.StatefulSetList,
	meta metadata.Metadata,
) (*policyv1.PodDisruptionBudget, error) {
	if len(statefulSets) == 0 {
		return nil, nil
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.PodDisruptionBudgetNameForRole(es.Name, roleName),
			Namespace: es.Namespace,
		},
		Spec: buildRoleSpecificPDBSpec(es, role, statefulSets, allStatefulSets),
	}

	mergedMeta := meta.Merge(metadata.Metadata{
		Labels:      pdb.Labels,
		Annotations: pdb.Annotations,
	})
	pdb.Labels = mergedMeta.Labels
	pdb.Annotations = mergedMeta.Annotations

	// Set owner reference
	if err := controllerutil.SetControllerReference(&es, pdb, scheme.Scheme); err != nil {
		return nil, err
	}

	return pdb, nil
}

// buildRoleSpecificPDBSpec returns a PDBSpec for a specific node role.
func buildRoleSpecificPDBSpec(
	es esv1.Elasticsearch,
	role esv1.NodeRole,
	// statefulSets are the statefulSets grouped into this pdb.
	statefulSets sset.StatefulSetList,
	// allStatefulSets are all statefulsets in the whole ES cluster.
	allStatefulSets sset.StatefulSetList,
) policyv1.PodDisruptionBudgetSpec {
	// Get the allowed disruptions for this role based on cluster health and role type
	allowedDisruptions := allowedDisruptionsForRole(es, role, allStatefulSets)

	spec := policyv1.PodDisruptionBudgetSpec{
		MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: allowedDisruptions},
	}

	// Get StatefulSet names for the selector
	ssetNames := make([]string, 0, len(statefulSets))
	for _, sset := range statefulSets {
		ssetNames = append(ssetNames, sset.Name)
	}

	// Sort for consistency
	sort.Strings(ssetNames)

	spec.Selector = selectorForStatefulSets(es, ssetNames)
	return spec
}

// allowedDisruptionsForRole returns the maximum number of pods that can be disrupted for a given role.
func allowedDisruptionsForRole(
	es esv1.Elasticsearch,
	role esv1.NodeRole,
	statefulSets sset.StatefulSetList,
) int32 {
	if es.Status.Health == esv1.ElasticsearchUnknownHealth || es.Status.Health == esv1.ElasticsearchHealth("") {
		return 0
	}
	// In a single node cluster (not highly-available) always allow 1 disruption
	// to ensure K8s nodes operations can be performed.
	if statefulSets.ExpectedNodeCount() == 1 {
		return 1
	}
	// There's a risk the single master of the cluster gets removed, don't allow it.
	if role == esv1.MasterRole && statefulSets.ExpectedMasterNodesCount() == 1 {
		return 0
	}
	// There's a risk the single data node of the cluster gets removed, don't allow it.
	if role == esv1.DataRole && statefulSets.ExpectedDataNodesCount() == 1 {
		return 0
	}
	// There's a risk the single ingest node of the cluster gets removed, don't allow it.
	if role == esv1.IngestRole && statefulSets.ExpectedIngestNodesCount() == 1 {
		return 0
	}

	// Check if this is a data role (any of the data variants)
	isDataRole := role == esv1.DataRole ||
		role == esv1.DataHotRole ||
		role == esv1.DataWarmRole ||
		role == esv1.DataColdRole ||
		role == esv1.DataContentRole

	// For data roles, only allow disruption if cluster is green
	if isDataRole && es.Status.Health != esv1.ElasticsearchGreenHealth {
		return 0
	}

	// For data_frozen, master, ingest, ml, transform, and coordinating (no roles) nodes, allow disruption if cluster is at least yellow
	if role == esv1.DataFrozenRole || role == esv1.MasterRole || role == esv1.IngestRole || role == esv1.MLRole || role == esv1.TransformRole || role == "" {
		if es.Status.Health != esv1.ElasticsearchGreenHealth && es.Status.Health != esv1.ElasticsearchYellowHealth {
			return 0
		}
	}

	// Allow one pod to be disrupted for all other cases
	return 1
}

// selectorForStatefulSets returns a label selector that matches pods from specific StatefulSets.
func selectorForStatefulSets(es esv1.Elasticsearch, ssetNames []string) *metav1.LabelSelector {
	// For simplicity both single and multi-statefulsets use matchExpressions with In operator
	return &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      label.ClusterNameLabelName,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{es.Name},
			},
			{
				Key:      label.StatefulSetNameLabelName,
				Operator: metav1.LabelSelectorOpIn,
				Values:   ssetNames,
			},
		},
	}
}

// reconcileAndDeleteUnnecessaryPDBs reconciles the PDBs that are expected to exist and deletes any that exist but are not expected.
func reconcileAndDeleteUnnecessaryPDBs(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch, expectedPDBs []*policyv1.PodDisruptionBudget) error {
	existingPDBs, err := listAllRoleSpecificPDBs(ctx, k8sClient, es)
	if err != nil {
		return fmt.Errorf("while listing existing role-specific PDBs: %w", err)
	}

	toDelete := make(map[string]client.Object)

	// Populate the toDelete map with existing PDBs
	for _, pdb := range existingPDBs {
		toDelete[pdb.GetName()] = pdb
	}

	// Remove expected PDBs from the toDelete map
	for _, pdb := range expectedPDBs {
		delete(toDelete, pdb.Name)
		// Ensure that the expected PDB is reconciled.
		if err := reconcilePDB(ctx, k8sClient, es, pdb); err != nil {
			return fmt.Errorf("while reconciling role-specific PDB %s: %w", pdb.Name, err)
		}
	}

	// Delete unnecessary PDBs
	for name, pdb := range toDelete {
		if err := deletePDB(ctx, k8sClient, pdb); err != nil {
			return fmt.Errorf("while deleting role-specific PDB %s: %w", name, err)
		}
	}

	return nil
}

// listAllRoleSpecificPDBs lists all role-specific PDBs for the cluster by retrieving
// all PDBs in the namespace with the cluster label and verifying the owner reference.
func listAllRoleSpecificPDBs(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch) ([]client.Object, error) {
	// List all PDBs in the namespace with the cluster label
	var pdbList client.ObjectList

	v1Available, err := isPDBV1Available(k8sClient)
	if err != nil {
		return nil, err
	}

	if v1Available {
		pdbList = &policyv1.PodDisruptionBudgetList{}
	} else {
		pdbList = &policyv1beta1.PodDisruptionBudgetList{}
	}

	if err := k8sClient.List(ctx, pdbList, client.InNamespace(es.Namespace), client.MatchingLabels{
		label.ClusterNameLabelName: es.Name,
	}); err != nil {
		return nil, err
	}

	var items []client.Object
	switch list := pdbList.(type) {
	case *policyv1.PodDisruptionBudgetList:
		for i := range list.Items {
			items = append(items, &list.Items[i])
		}
	case *policyv1beta1.PodDisruptionBudgetList:
		for i := range list.Items {
			items = append(items, &list.Items[i])
		}
	}

	// Filter only PDBs that are owned by this Elasticsearch controller
	var roleSpecificPDBs []client.Object
	for _, pdb := range items {
		// Check if this PDB is owned by the Elasticsearch resource
		if k8s.HasOwner(pdb, &es) {
			roleSpecificPDBs = append(roleSpecificPDBs, pdb)
		}
	}
	return roleSpecificPDBs, nil
}

// deleteAllRoleSpecificPDBs deletes all existing role-specific PDBs for the cluster by retrieving
// all PDBs in the namespace with the cluster label and verifying the owner reference.
func deleteAllRoleSpecificPDBs(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch) error {
	v1Available, err := isPDBV1Available(k8sClient)
	if err != nil {
		return err
	}

	// List and process PDBs based on the available API version
	if v1Available {
		return deleteAllRoleSpecificPDBsWithVersion(ctx, k8sClient, es, &policyv1.PodDisruptionBudgetList{})
	} else {
		return deleteAllRoleSpecificPDBsWithVersion(ctx, k8sClient, es, &policyv1beta1.PodDisruptionBudgetList{})
	}
}

// deleteAllRoleSpecificPDBsWithVersion handles listing and deleting PDBs using a specific PDB version
func deleteAllRoleSpecificPDBsWithVersion(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch, pdbList client.ObjectList) error {
	// List all PDBs in the namespace with the cluster label
	if err := k8sClient.List(ctx, pdbList, client.InNamespace(es.Namespace), client.MatchingLabels{
		label.ClusterNameLabelName: es.Name,
	}); err != nil {
		return err
	}

	// Get items from the list and delete those owned by this Elasticsearch resource
	var items []client.Object

	// Extract items based on the concrete type
	switch list := pdbList.(type) {
	case *policyv1.PodDisruptionBudgetList:
		for i := range list.Items {
			items = append(items, &list.Items[i])
		}
	case *policyv1beta1.PodDisruptionBudgetList:
		for i := range list.Items {
			items = append(items, &list.Items[i])
		}
	}

	// Delete PDBs owned by this Elasticsearch resource
	for _, item := range items {
		if k8s.HasOwner(item, &es) {
			if err := k8sClient.Delete(ctx, item); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// podDisruptionBudgetName returns the name of the PDB.
func podDisruptionBudgetName(esName string, role string) string {
	name := esv1.DefaultPodDisruptionBudget(esName) + "-" + role
	// For coordinating nodes (no roles), append "coordinating" to the name
	if role == "" {
		name += string(esv1.CoordinatingRole)
	}
	return name
}
