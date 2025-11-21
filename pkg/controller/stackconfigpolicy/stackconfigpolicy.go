// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var errMergeConflict = errors.New("merge conflict")

// esPolicyConfig represents the merged configuration from multiple StackConfigPolicies
// that apply to a specific Elasticsearch cluster.
type esPolicyConfig struct {
	// Spec holds the merged Elasticsearch configuration from all applicable policies
	Spec policyv1alpha1.ElasticsearchConfigPolicySpec
	// PoliciesWithConflictErrors maps policy namespaced names to conflict errors when multiple policies
	// have the same weight
	PoliciesWithConflictErrors map[types.NamespacedName]error
	// PoliciesRefs contains all StackConfigPolicies that target this Elasticsearch cluster
	PoliciesRefs []policyv1alpha1.StackConfigPolicy
}

// getPolicyConfigForElasticsearch builds a merged stack config policy for the given Elasticsearch cluster.
// It processes all provided policies, filtering those that target the Elasticsearch cluster, and merges them
// in order of their weight (lowest to highest). Policies with the same weight are flagged as conflicts.
// Returns an esPolicyConfig containing the merged configuration and any error occurred during merging.
func getPolicyConfigForElasticsearch(es *esv1.Elasticsearch, allPolicies []policyv1alpha1.StackConfigPolicy, params operator.Parameters) (*esPolicyConfig, error) {
	esPolicy := esPolicyConfig{
		PoliciesWithConflictErrors: make(map[types.NamespacedName]error),
	}
	if len(allPolicies) == 0 {
		return &esPolicy, nil
	}

	// Group policies by weight
	var weights []int32
	weightKeyStackPolicies := make(map[int32][]*policyv1alpha1.StackConfigPolicy)
	for _, p := range allPolicies {
		isRef, err := doesPolicyRefsObject(&p, es, params.OperatorNamespace)
		if err != nil {
			return nil, err
		}
		if !isRef {
			// policy does not target the given Elasticsearch
			continue
		}

		if _, exists := weightKeyStackPolicies[p.Spec.Weight]; !exists {
			weights = append(weights, p.Spec.Weight)
		}

		weightKeyStackPolicies[p.Spec.Weight] = append(weightKeyStackPolicies[p.Spec.Weight], &p)
		esPolicy.PoliciesRefs = append(esPolicy.PoliciesRefs, p)
	}

	if len(esPolicy.PoliciesRefs) == 1 {
		// Since we have only one policy avoid merging (including canonicalise)
		// and thus avoid any reconciliation storm caused by unnecessary
		// secret changes
		esConfigPolicy := esPolicy.PoliciesRefs[0].Spec.Elasticsearch.DeepCopy()
		esPolicy.Spec = *esConfigPolicy
		return &esPolicy, nil
	}

	// Process policies in order of weight (lowest first)
	slices.Sort(weights)

	// Reverse the weights so that we process policies in order of weight (highest first)
	slices.Reverse(weights)

	var previouslyAppliedPolicy *policyv1alpha1.StackConfigPolicy
	for _, weight := range weights {
		policiesWithSameWeight := weightKeyStackPolicies[weight]
		if len(policiesWithSameWeight) > 1 {
			// Multiple policies with the same weight - this is a conflict
			conflictErr := getPolicyConflictError(policiesWithSameWeight, weight)
			for _, p := range policiesWithSameWeight {
				esPolicy.PoliciesWithConflictErrors[k8s.ExtractNamespacedName(p)] = conflictErr
			}
			return &esPolicy, conflictErr
		}
		policy := policiesWithSameWeight[0]
		// Merge the single policy at this weight level
		if err := mergeElasticsearchConfig(&esPolicy.Spec, policy.Spec.Elasticsearch); err != nil {
			if errors.Is(err, errMergeConflict) {
				policyNsn := k8s.ExtractNamespacedName(policy)
				esPolicy.PoliciesWithConflictErrors[policyNsn] = err

				if previouslyAppliedPolicy != nil {
					previouslyAppliedPolicyNsn := k8s.ExtractNamespacedName(previouslyAppliedPolicy)
					esPolicy.PoliciesWithConflictErrors[previouslyAppliedPolicyNsn] = err
				}
				return &esPolicy, err
			}
			return nil, err
		}
		previouslyAppliedPolicy = policy
	}

	return &esPolicy, nil
}

// kbnPolicyConfig represents the merged configuration from multiple StackConfigPolicies
// that apply to a specific Kibana instance.
type kbnPolicyConfig struct {
	// Spec contains the merged Kibana configuration from all applicable policies
	Spec policyv1alpha1.KibanaConfigPolicySpec
	// PoliciesWithConflictErrors maps policy namespaced names to conflict errors when multiple policies
	// have the same weight
	PoliciesWithConflictErrors map[types.NamespacedName]error
	// PoliciesRefs contains all StackConfigPolicies that target this Kibana instance
	PoliciesRefs []policyv1alpha1.StackConfigPolicy
}

// getPolicyConfigForKibana builds a merged stack config policy for the given Kibana instance.
// It processes all provided policies, filtering those that target the Kibana instance, and merges them
// in order of their weight (lowest to highest). Policies with the same weight are flagged as conflicts.
// Returns an kbnPolicyConfig containing the merged configuration and any error occurred during merging.
func getPolicyConfigForKibana(kb *kbv1.Kibana, allPolicies []policyv1alpha1.StackConfigPolicy, params operator.Parameters) (*kbnPolicyConfig, error) {
	kbPolicy := kbnPolicyConfig{
		PoliciesWithConflictErrors: make(map[types.NamespacedName]error),
	}
	if len(allPolicies) == 0 {
		return &kbPolicy, nil
	}

	// Group policies by weight
	var weights []int32
	weightKeyStackPolicies := make(map[int32][]*policyv1alpha1.StackConfigPolicy)
	for _, p := range allPolicies {
		isRef, err := doesPolicyRefsObject(&p, kb, params.OperatorNamespace)
		if err != nil {
			return nil, err
		}
		if !isRef {
			// policy does not target the given Kibana instance
			continue
		}

		if _, exists := weightKeyStackPolicies[p.Spec.Weight]; !exists {
			weights = append(weights, p.Spec.Weight)
		}

		weightKeyStackPolicies[p.Spec.Weight] = append(weightKeyStackPolicies[p.Spec.Weight], &p)
		kbPolicy.PoliciesRefs = append(kbPolicy.PoliciesRefs, p)
	}

	if len(kbPolicy.PoliciesRefs) == 1 {
		// Since we have only one policy avoid merging (including canonicalise)
		// and thus don't cause a reconciliation storm by unnecessary
		// secret changes
		kbConfigPolicy := kbPolicy.PoliciesRefs[0].Spec.Kibana.DeepCopy()
		kbPolicy.Spec = *kbConfigPolicy
		return &kbPolicy, nil
	}

	// Process policies in order of weight (lowest first)
	slices.Sort(weights)

	// Reverse the weights so that we process policies in order of weight (highest first)
	slices.Reverse(weights)

	for _, weight := range weights {
		policiesWithWeight := weightKeyStackPolicies[weight]
		if len(policiesWithWeight) > 1 {
			// Multiple policies with the same weight - this is a conflict
			conflictErr := getPolicyConflictError(policiesWithWeight, weight)
			for _, p := range policiesWithWeight {
				kbPolicy.PoliciesWithConflictErrors[k8s.ExtractNamespacedName(p)] = conflictErr
			}
			return &kbPolicy, conflictErr
		}

		// Merge the single policy at this weight level
		if err := mergeKibanaConfig(&kbPolicy.Spec, policiesWithWeight[0].Spec.Kibana); err != nil {
			return nil, err
		}
	}

	return &kbPolicy, nil
}

// doesPolicyRefsObject checks if the given StackConfigPolicy targets the given Elasticsearch cluster.
// A policy targets an Elasticsearch cluster if both following conditions are met:
// 1. The policy is in either the operator namespace or the same namespace as the Elasticsearch cluster
// 2. The policy's label selector matches the Elasticsearch cluster's labels
// Returns true or false depending on whether the given policy targets the Elasticsearch cluster and
// an error if the label selector is invalid.
func doesPolicyRefsObject(policy *policyv1alpha1.StackConfigPolicy, obj metav1.Object, operatorNamespace string) (bool, error) {
	// Convert the label selector to a selector object
	selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.ResourceSelector)
	if err != nil {
		return false, err
	}

	// Check namespace restrictions; the policy must be in operator namespace or same namespace as the target object
	if policy.Namespace != operatorNamespace && policy.Namespace != obj.GetNamespace() {
		return false, nil
	}

	// Check if the label selector matches the Elasticsearch labels
	if !selector.Matches(labels.Set(obj.GetLabels())) {
		return false, nil
	}

	return true, nil
}

// getPolicyConflictError creates an error message listing all policies that have conflicting weights.
// The error message includes the namespaced names of all conflicting policies.
// Returns an error with a message listing all conflicting policy names.
func getPolicyConflictError(policies []*policyv1alpha1.StackConfigPolicy, weight int32) error {
	strBuilder := strings.Builder{}

	strBuilder.WriteString("multiple stack config policies ")
	for idx, p := range policies {
		if idx > 0 {
			strBuilder.WriteString(", ")
		}
		strBuilder.WriteString(`"`)
		strBuilder.WriteString(k8s.ExtractNamespacedName(p).String())
		strBuilder.WriteString(`"`)
	}
	strBuilder.WriteString(" with the same weight ")
	strBuilder.WriteString(strconv.Itoa(int(weight)))
	return fmt.Errorf("%w: %s", errMergeConflict, strBuilder.String())
}

// mergeKibanaConfig merges the source KibanaConfigPolicySpec into the destination.
// For configuration fields (Config, SecureSettings), it performs a deep merge
// where source values override destination values at the field level.
// For SecretMounts and SecureSettings, it merges by name/key, with source values taking precedence.
// Returns any error occurred during configuration merges.
func mergeKibanaConfig(dst *policyv1alpha1.KibanaConfigPolicySpec, src policyv1alpha1.KibanaConfigPolicySpec) error {
	var err error
	if dst.Config, err = deepMergeConfig(dst.Config, src.Config); err != nil {
		return err
	}
	dst.SecureSettings = mergeSecretSources(dst.SecureSettings, src.SecureSettings)
	return nil
}

// mergeElasticsearchConfig merges the source ElasticsearchConfigPolicySpec into the destination.
// For configuration fields (ClusterSettings, SnapshotRepositories, etc.), it performs a deep merge
// where source values override destination values at the field level.
// For SecretMounts, conflicts are detected when the same SecretName or MountPath exists in both
// dst and src. An error is returned to prevent duplicate secret references or mount path collisions.
// For SecureSettings, it merges by SecretName/Key with source values taking precedence.
// Returns any error occurred during configuration merges.
func mergeElasticsearchConfig(dst *policyv1alpha1.ElasticsearchConfigPolicySpec, src policyv1alpha1.ElasticsearchConfigPolicySpec) error {
	var err error
	if dst.ClusterSettings, err = deepMergeConfig(dst.ClusterSettings, src.ClusterSettings); err != nil {
		return err
	}
	if dst.SnapshotRepositories, err = mergeConfig(dst.SnapshotRepositories, src.SnapshotRepositories); err != nil {
		return err
	}
	if dst.SnapshotLifecyclePolicies, err = deepMergeConfig(dst.SnapshotLifecyclePolicies, src.SnapshotLifecyclePolicies); err != nil {
		return err
	}
	if dst.SecurityRoleMappings, err = deepMergeConfig(dst.SecurityRoleMappings, src.SecurityRoleMappings); err != nil {
		return err
	}
	if dst.IndexLifecyclePolicies, err = deepMergeConfig(dst.IndexLifecyclePolicies, src.IndexLifecyclePolicies); err != nil {
		return err
	}
	if dst.IngestPipelines, err = deepMergeConfig(dst.IngestPipelines, src.IngestPipelines); err != nil {
		return err
	}
	if dst.IndexTemplates.ComposableIndexTemplates, err = deepMergeConfig(dst.IndexTemplates.ComposableIndexTemplates, src.IndexTemplates.ComposableIndexTemplates); err != nil {
		return err
	}
	if dst.IndexTemplates.ComponentTemplates, err = deepMergeConfig(dst.IndexTemplates.ComponentTemplates, src.IndexTemplates.ComponentTemplates); err != nil {
		return err
	}
	if dst.Config, err = deepMergeConfig(dst.Config, src.Config); err != nil {
		return err
	}

	if dst.SecretMounts, err = mergeSecretMounts(dst.SecretMounts, src.SecretMounts); err != nil {
		return err
	}
	dst.SecureSettings = mergeSecretSources(dst.SecureSettings, src.SecureSettings)
	return nil
}

// deepMergeConfig merges the source Config into the destination Config using canonical configuration merging.
// The merge is performed at the field level, with source values overriding destination values.
// If src is nil, dst is returned unchanged. If dst is nil, it is initialized before merging.
// Returns the merged config and any error occurred during config parsing or merging.
func deepMergeConfig(dst *commonv1.Config, src *commonv1.Config) (*commonv1.Config, error) {
	if src == nil {
		return dst, nil
	}

	var dstCanonicalConfig *settings.CanonicalConfig
	var err error
	if dst == nil {
		dst = &commonv1.Config{}
		dstCanonicalConfig = settings.NewCanonicalConfig()
	} else {
		dstCanonicalConfig, err = settings.NewCanonicalConfigFrom(dst.DeepCopy().Data)
		if err != nil {
			return nil, err
		}
	}

	srcCanonicalConfig, err := settings.NewCanonicalConfigFrom(src.DeepCopy().Data)
	if err != nil {
		return nil, err
	}

	err = dstCanonicalConfig.MergeWith(srcCanonicalConfig)
	if err != nil {
		return nil, err
	}

	dst.Data = nil
	err = dstCanonicalConfig.Unpack(&dst.Data)
	if err != nil {
		return nil, err
	}

	return dst, nil
}

// mergeConfig merges the source Config into the destination Config by replacing entire top-level keys.
// Unlike deepMergeConfig which performs recursive merging, this function replaces each top-level key
// in dst with the corresponding value from src. Both configs are first canonicalized to ensure
// consistent structure. If src is nil, dst is returned unchanged. If dst is nil, it is initialized.
// Returns the merged config and any error occurred during config parsing or unpacking.
func mergeConfig(dst *commonv1.Config, src *commonv1.Config) (*commonv1.Config, error) {
	if src == nil {
		return dst, nil
	}

	var dstCanonicalConfig *settings.CanonicalConfig
	var err error
	if dst == nil {
		dst = &commonv1.Config{}
		dstCanonicalConfig = settings.NewCanonicalConfig()
	} else {
		dstCanonicalConfig, err = settings.NewCanonicalConfigFrom(dst.DeepCopy().Data)
		if err != nil {
			return nil, err
		}
	}

	srcCanonicalConfig, err := settings.NewCanonicalConfigFrom(src.DeepCopy().Data)
	if err != nil {
		return nil, err
	}

	dst.Data = nil
	err = dstCanonicalConfig.Unpack(&dst.Data)
	if err != nil {
		return nil, err
	}

	srcCfg := &commonv1.Config{}
	err = srcCanonicalConfig.Unpack(&srcCfg.Data)
	if err != nil {
		return nil, err
	}

	for k, v := range srcCfg.Data {
		dst.Data[k] = v
	}

	return dst, nil
}

// mergeSecretMounts merges source SecretMounts into destination SecretMounts.
// SecretMounts are keyed by SecretName and MountPath. Conflicts are detected when:
// - The same SecretName exists in both dst and src (prevents duplicate secret references)
// - The same MountPath exists in both dst and src (prevents mount path collisions)
// Returns a new slice containing the merged SecretMounts sorted by SecretName for
// deterministic output, or an error if conflicts are detected.
func mergeSecretMounts(dst []policyv1alpha1.SecretMount, src []policyv1alpha1.SecretMount) ([]policyv1alpha1.SecretMount, error) {
	secretMounts := make(map[string]policyv1alpha1.SecretMount)

	// Add all destination entries
	mountPoints := make(map[string]string)
	for _, secretMount := range dst {
		secretMounts[secretMount.SecretName] = secretMount
		mountPoints[secretMount.MountPath] = secretMount.SecretName
	}

	// Merge in source entries, checking for conflicts
	for _, secretMount := range src {
		if _, exists := secretMounts[secretMount.SecretName]; exists {
			return nil, fmt.Errorf("%w: secret with name %q is defined in multiple policies", errMergeConflict, secretMount.SecretName)
		}
		if _, exists := mountPoints[secretMount.MountPath]; exists {
			return nil, fmt.Errorf("%w: secret mount path %q is defined in multiple policies", errMergeConflict, secretMount.MountPath)
		}
		mountPoints[secretMount.MountPath] = secretMount.SecretName
		secretMounts[secretMount.SecretName] = secretMount
	}

	// Collect secret names and sort them for deterministic output
	secretNames := slices.Collect(maps.Keys(secretMounts))
	if len(secretNames) == 0 {
		return nil, nil
	}

	slices.Sort(secretNames)

	// Build the result in sorted order
	mergedSecretMounts := make([]policyv1alpha1.SecretMount, 0, len(secretNames))
	for _, secretName := range secretNames {
		mergedSecretMounts = append(mergedSecretMounts, secretMounts[secretName])
	}
	return mergedSecretMounts, nil
}

// mergeSecretSources merges source SecretSources into destination SecretSources.
// SecretSources are merged at two levels:
// 1. First level: keyed by SecretName
// 2. Second level: within each SecretName, entries are keyed by Key
// If the same SecretName and Key exist in both dst and src, the src entry overrides the dst entry.
// Returns a new slice containing the merged SecretSources, sorted by SecretName and
// Key for deterministic output.
func mergeSecretSources(dst []commonv1.SecretSource, src []commonv1.SecretSource) []commonv1.SecretSource {
	secureSettings := make(map[string]map[string]commonv1.KeyToPath)
	// Add all destination entries
	for _, secureSetting := range dst {
		secureSettings[secureSetting.SecretName] = make(map[string]commonv1.KeyToPath)
		for _, entry := range secureSetting.Entries {
			secureSettings[secureSetting.SecretName][entry.Key] = entry
		}
	}
	// Merge in source entries (overriding destination if same SecretName/Key)
	for _, secureSetting := range src {
		if _, exists := secureSettings[secureSetting.SecretName]; !exists {
			secureSettings[secureSetting.SecretName] = make(map[string]commonv1.KeyToPath)
		}
		for _, entry := range secureSetting.Entries {
			secureSettings[secureSetting.SecretName][entry.Key] = entry
		}
	}

	// Collect and sort secret names for deterministic output
	secretNames := slices.Collect(maps.Keys(secureSettings))
	if len(secretNames) == 0 {
		return nil
	}

	slices.Sort(secretNames)

	// Build the result in sorted order
	mergedSecureSettings := make([]commonv1.SecretSource, 0, len(secretNames))
	for _, secretName := range secretNames {
		entries := secureSettings[secretName]

		// Collect and sort entry keys for deterministic output
		keys := slices.Collect(maps.Keys(entries))
		slices.Sort(keys)

		secretSource := commonv1.SecretSource{
			SecretName: secretName,
		}
		for _, key := range keys {
			secretSource.Entries = append(secretSource.Entries, entries[key])
		}
		mergedSecureSettings = append(mergedSecureSettings, secretSource)
	}

	return mergedSecureSettings
}
