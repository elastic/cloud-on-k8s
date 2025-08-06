// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	"context"
	"fmt"

	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	lic "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// Reconcile ensures that PodDisruptionBudget(s) exists for this cluster, inheriting the spec content.
//  1. For non-enterprise users: The default PDB we setup dynamically adapts MinAvailable to the number of nodes in the cluster.
//  2. For enterprise users: We optimize the PDBs that we setup to speed up Kubernetes cluster operations such as upgrades as much
//     as safely possible by grouping statefulSets by associated Elasticsearch node roles into the same PDB, and then dynamically setting
//     maxUnavailable according to whatever cluster health is optimal for the set of roles.
//
// If the spec has disabled the default PDB, it will ensure none exist.
func Reconcile(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch, statefulSets sset.StatefulSetList, meta metadata.Metadata) error {
	licenseChecker := lic.NewLicenseChecker(k8sClient, es.Namespace)
	enterpriseEnabled, err := licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return fmt.Errorf("while checking license during pdb reconciliation: %w", err)
	}
	if enterpriseEnabled {
		return reconcileRoleSpecificPDBs(ctx, k8sClient, es, statefulSets, meta)
	}

	return reconcileDefaultPDB(ctx, k8sClient, es, statefulSets, meta)
}

// reconcileDefaultPDB reconciles the default PDB for non-enterprise users.
func reconcileDefaultPDB(
	ctx context.Context,
	k8sClient k8s.Client,
	es esv1.Elasticsearch,
	statefulSets sset.StatefulSetList,
	meta metadata.Metadata,
) error {
	expected, err := expectedPDB(es, statefulSets, meta)
	if err != nil {
		return err
	}
	if expected == nil {
		return deleteDefaultPDB(ctx, k8sClient, es)
	}

	return reconcilePDB(ctx, k8sClient, es, expected)
}

// reconcilePDB reconciles a single PDB, handling both v1 and v1beta1 versions.
func reconcilePDB(
	ctx context.Context,
	k8sClient k8s.Client,
	es esv1.Elasticsearch,
	expected *policyv1.PodDisruptionBudget,
) error {
	// label the PDB with a hash of its content, for comparison purposes
	expected.Labels = hash.SetTemplateHashLabel(expected.Labels, expected)

	v1Available, err := isPDBV1Available(k8sClient)
	if err != nil {
		return err
	}

	if v1Available {
		reconciled := &policyv1.PodDisruptionBudget{}
		return reconciler.ReconcileResource(
			reconciler.Params{
				Context:    ctx,
				Client:     k8sClient,
				Owner:      &es,
				Expected:   expected,
				Reconciled: reconciled,
				NeedsUpdate: func() bool {
					return hash.GetTemplateHashLabel(expected.Labels) != hash.GetTemplateHashLabel(reconciled.Labels)
				},
				UpdateReconciled: func() {
					expected.DeepCopyInto(reconciled)
				},
			},
		)
	}

	// Fall back to v1beta1
	reconciled := &policyv1beta1.PodDisruptionBudget{}
	converted := convert(expected)
	return reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     k8sClient,
			Owner:      &es,
			Expected:   converted,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return hash.GetTemplateHashLabel(converted.Labels) != hash.GetTemplateHashLabel(reconciled.Labels)
			},
			UpdateReconciled: func() {
				converted.DeepCopyInto(reconciled)
			},
		},
	)
}

// deleteDefaultPDB deletes the default pdb if it exists.
func deleteDefaultPDB(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch) error {
	// we do this by getting first because that is a local cache read,
	// versus a Delete call, which would hit the API.

	v1Available, err := isPDBV1Available(k8sClient)
	if err != nil {
		return err
	}
	var pdb client.Object
	if v1Available {
		pdb = &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: es.Namespace,
				Name:      esv1.DefaultPodDisruptionBudget(es.Name),
			},
		}
	} else {
		pdb = &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: es.Namespace,
				Name:      esv1.DefaultPodDisruptionBudget(es.Name),
			},
		}
	}

	if err := k8sClient.Get(ctx, k8s.ExtractNamespacedName(pdb), pdb); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if apierrors.IsNotFound(err) {
		// already deleted, which is fine
		return nil
	}
	if err := k8sClient.Delete(ctx, pdb); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// expectedPDB returns a PDB according to the given ES spec.
// It may return nil if the PDB has been explicitly disabled in the ES spec.
func expectedPDB(es esv1.Elasticsearch, statefulSets sset.StatefulSetList, meta metadata.Metadata) (*policyv1.PodDisruptionBudget, error) {
	template := es.Spec.PodDisruptionBudget.DeepCopy()
	if template.IsDisabled() {
		return nil, nil
	}
	if template == nil {
		template = &commonv1.PodDisruptionBudgetTemplate{}
	}

	expected := policyv1.PodDisruptionBudget{
		ObjectMeta: template.ObjectMeta,
	}

	// inherit user-provided ObjectMeta, but set our own name & namespace
	expected.Name = esv1.DefaultPodDisruptionBudget(es.Name)
	expected.Namespace = es.Namespace
	// Add labels and annotations
	mergedMeta := meta.Merge(metadata.Metadata{
		Labels:      expected.Labels,
		Annotations: expected.Annotations,
	})
	expected.Labels = mergedMeta.Labels
	expected.Annotations = mergedMeta.Annotations
	// set owner reference for deletion upon ES resource deletion
	if err := controllerutil.SetControllerReference(&es, &expected, scheme.Scheme); err != nil {
		return nil, err
	}

	if template.Spec.Selector != nil || template.Spec.MaxUnavailable != nil || template.Spec.MinAvailable != nil {
		// use the user-defined spec
		expected.Spec = template.Spec
	} else {
		// set our default spec
		expected.Spec = buildPDBSpec(es, statefulSets)
	}

	return &expected, nil
}

// buildPDBSpec returns a PDBSpec computed from the current StatefulSets,
// considering the cluster health and topology.
func buildPDBSpec(es esv1.Elasticsearch, statefulSets sset.StatefulSetList) policyv1.PodDisruptionBudgetSpec {
	// compute MinAvailable based on the maximum number of Pods we're supposed to have
	nodeCount := statefulSets.ExpectedNodeCount()
	// maybe allow some Pods to be disrupted
	minAvailable := nodeCount - allowedDisruptionsForSinglePDB(es, statefulSets)

	minAvailableIntStr := intstr.IntOrString{Type: intstr.Int, IntVal: minAvailable}

	return policyv1.PodDisruptionBudgetSpec{
		// match all pods for this cluster
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				label.ClusterNameLabelName: es.Name,
			},
		},
		MinAvailable: &minAvailableIntStr,
		// MaxUnavailable can only be used if the selector matches a builtin controller selector
		// (eg. Deployments, StatefulSets, etc.). We cannot use it with our own cluster-name selector.
		MaxUnavailable: nil,
	}
}

// allowedDisruptionsForSinglePDB returns the number of Pods that we allow to be disrupted while keeping the cluster healthy
// when there is a single PodDisruptionBudget that encompasses a whole Elasticsearch cluster.
func allowedDisruptionsForSinglePDB(es esv1.Elasticsearch, actualSsets sset.StatefulSetList) int32 {
	if actualSsets.ExpectedNodeCount() == 1 {
		// single node cluster (not highly-available)
		// allow the node to be disrupted to ensure K8s nodes operations can be performed
		return 1
	}
	if es.Status.Health != esv1.ElasticsearchGreenHealth {
		// A non-green cluster may become red if we disrupt one node, don't allow it.
		// The health information we're using here may be out-of-date, that's best effort.
		return 0
	}
	if actualSsets.ExpectedMasterNodesCount() == 1 {
		// There's a risk the single master of the cluster gets removed, don't allow it.
		return 0
	}
	if actualSsets.ExpectedDataNodesCount() == 1 {
		// There's a risk the single data node of the cluster gets removed, don't allow it.
		return 0
	}
	if actualSsets.ExpectedIngestNodesCount() == 1 {
		// There's a risk the single ingest node of the cluster gets removed, don't allow it.
		return 0
	}
	// Allow one pod (only) to be disrupted on a healthy cluster.
	// We could technically allow more, but the cluster health freshness would become a bigger problem.
	return 1
}
