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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	priority = []esv1.NodeRole{esv1.DataRole, esv1.MasterRole, esv1.DataFrozenRole, esv1.IngestRole, esv1.MLRole, esv1.TransformRole, esv1.CoordinatingRole}
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
		group, ok := groups[roleName]
		if !ok {
			continue
		}
		if len(group) == 0 {
			continue
		}

		pdb, err := createPDBForStatefulSets(es, roleName, string(roleName), group, statefulSets, meta)
		if err != nil {
			return nil, err
		}
		if pdb != nil {
			pdbs = append(pdbs, pdb)
		}
	}

	return pdbs, nil
}

func groupBySharedRoles(statefulSets sset.StatefulSetList, resources nodespec.ResourcesList, v version.Version) (map[esv1.NodeRole][]appsv1.StatefulSet, error) {
	n := len(statefulSets)
	if n == 0 {
		return map[esv1.NodeRole][]appsv1.StatefulSet{}, nil
	}

	rolesToIndices := make(map[esv1.NodeRole][]int)
	indicesToRoles := make(map[int]set.StringSet)
	for i, sset := range statefulSets {
		roles, err := getRolesForStatefulSet(sset, resources, v)
		if err != nil {
			return nil, err
		}
		if len(roles) == 0 {
			// StatefulSets with no roles are coordinating nodes - group them together
			rolesToIndices[esv1.CoordinatingRole] = append(rolesToIndices[esv1.CoordinatingRole], i)
			indicesToRoles[i] = set.Make(string(esv1.CoordinatingRole))
			continue
		}
		for _, role := range roles {
			// Ensure that the data* roles are grouped together.
			normalizedRole := toGenericDataRole(role)
			rolesToIndices[normalizedRole] = append(rolesToIndices[normalizedRole], i)
			if _, ok := indicesToRoles[i]; !ok {
				indicesToRoles[i] = set.Make()
			}
			indicesToRoles[i].Add(string(normalizedRole))
		}
	}

	// This keeps track of which roles have been assigned to a PDB to avoid assigning the same role to multiple PDBs.
	roleToTargetPDB := map[esv1.NodeRole]esv1.NodeRole{}
	grouped := map[esv1.NodeRole][]int{}
	visited := make([]bool, n)
	for _, role := range priority {
		indices, ok := rolesToIndices[role]
		if !ok {
			continue
		}
		for _, idx := range indices {
			if visited[idx] {
				continue
			}
			targetPDBRole := role
			// if we already assigned a PDB for this role, use that instead
			if target, ok := roleToTargetPDB[role]; ok {
				targetPDBRole = target
			}
			grouped[targetPDBRole] = append(grouped[targetPDBRole], idx)
			for _, r := range indicesToRoles[idx].AsSlice() {
				roleToTargetPDB[esv1.NodeRole(r)] = targetPDBRole
			}
			visited[idx] = true
		}
	}
	// transform into the expected format
	res := make(map[esv1.NodeRole][]appsv1.StatefulSet)
	for role, indices := range grouped {
		group := make([]appsv1.StatefulSet, 0, len(indices))
		for _, idx := range indices {
			group = append(group, statefulSets[idx])
		}
		res[role] = group
	}
	return res, nil
}

// getRolesForStatefulSet gets the roles from a StatefulSet's expected configuration.
func getRolesForStatefulSet(
	statefulSet appsv1.StatefulSet,
	expectedResources nodespec.ResourcesList,
	v version.Version,
) ([]esv1.NodeRole, error) {
	forStatefulSet, err := expectedResources.ForStatefulSet(statefulSet.Name)
	if err != nil {
		return nil, err
	}
	cfg, err := forStatefulSet.Config.Unpack(v)
	if err != nil {
		return nil, err
	}
	var nodeRoles []esv1.NodeRole
	// Special case of no roles specified, which results in all roles being valid for this sts.
	if cfg.Node.Roles == nil {
		// since the priority slice contains all the roles that we are interested in
		// when creating a pdb for a sts, we can use the priority slice as the roles.
		nodeRoles = priority
		// remove Coordinating role from the end of the slice.
		nodeRoles = nodeRoles[:len(nodeRoles)-1]
		return nodeRoles, nil
	}
	// Special case of empty roles being specified, which indicates the coordinating role for this sts.
	if len(cfg.Node.Roles) == 0 {
		nodeRoles = append(nodeRoles, esv1.CoordinatingRole)
		return nodeRoles, nil
	}
	nodeRoles = make([]esv1.NodeRole, len(cfg.Node.Roles))
	// Otherwise, use the list of roles from the configuration.
	for i, role := range cfg.Node.Roles {
		nodeRoles[i] = esv1.NodeRole(role)
	}
	return nodeRoles, nil
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
	// allStatefulSets are all statefulSets in the whole ES cluster.
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
	// allStatefulSets are all statefulSets in the whole ES cluster.
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
	allStatefulSets sset.StatefulSetList,
) int32 {
	// Disallow disruptions when health is unknown, empty or red.
	if es.Status.Health == esv1.ElasticsearchUnknownHealth ||
		es.Status.Health == esv1.ElasticsearchHealth("") ||
		es.Status.Health == esv1.ElasticsearchRedHealth {
		return 0
	}

	// In a single node cluster (not highly-available) always allow 1 disruption
	// to ensure K8s nodes operations can be performed.
	if allStatefulSets.ExpectedNodeCount() == 1 {
		return 1
	}

	// Allow 1 disruption for data roles only if health is green.
	if role == esv1.DataRole {
		if es.Status.Health == esv1.ElasticsearchGreenHealth {
			return 1
		}
		return 0
	}

	// Allow a single disruption for non-data roles when health is green or yellow.
	if es.Status.Health == esv1.ElasticsearchGreenHealth || es.Status.Health == esv1.ElasticsearchYellowHealth {
		return 1
	}

	// In all other cases, we want to allow no disruptions.
	return 0
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

	toDelete := make(map[string]policyv1.PodDisruptionBudget)

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
		if err := deletePDB(ctx, k8sClient, &pdb); err != nil {
			return fmt.Errorf("while deleting role-specific PDB %s: %w", name, err)
		}
	}

	return nil
}

// listAllRoleSpecificPDBs lists all role-specific PDBs for the cluster by retrieving
// all PDBs in the namespace with the cluster label and verifying the owner reference.
func listAllRoleSpecificPDBs(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch) ([]policyv1.PodDisruptionBudget, error) {
	// List all PDBs in the namespace with the cluster label
	pdbList := &policyv1.PodDisruptionBudgetList{}

	if err := k8sClient.List(ctx, pdbList, client.InNamespace(es.Namespace), client.MatchingLabels{
		label.ClusterNameLabelName: es.Name,
	}); err != nil {
		return nil, err
	}

	// Filter only PDBs that are owned by this Elasticsearch controller
	var roleSpecificPDBs []policyv1.PodDisruptionBudget
	for _, pdb := range pdbList.Items {
		// Check if this PDB is owned by the Elasticsearch resource
		if k8s.HasOwner(&pdb, &es) {
			roleSpecificPDBs = append(roleSpecificPDBs, pdb)
		}
	}
	return roleSpecificPDBs, nil
}

// deleteAllRoleSpecificPDBs deletes all existing role-specific PDBs for the cluster by retrieving
// all PDBs in the namespace with the cluster label and verifying the owner reference.
func deleteAllRoleSpecificPDBs(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch) error {
	// List all PDBs in the namespace with the cluster label
	var pdbList policyv1.PodDisruptionBudgetList
	if err := k8sClient.List(ctx, &pdbList, client.InNamespace(es.Namespace), client.MatchingLabels{
		label.ClusterNameLabelName: es.Name,
	}); err != nil {
		return err
	}

	// Delete PDBs owned by this Elasticsearch resource
	for _, pdb := range pdbList.Items {
		// Ensure we do not delete the default PDB if it exists.
		if k8s.HasOwner(&pdb, &es) && pdb.GetName() != esv1.DefaultPodDisruptionBudget(es.Name) {
			if err := k8sClient.Delete(ctx, &pdb); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}
