// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var errMergeConflict = errors.New("merge conflict")

// configPolicy is a generic container for merged StackConfigPolicy specifications.
// It holds the merged spec of type T (either ElasticsearchConfigPolicySpec or KibanaConfigPolicySpec),
// along with metadata about which policies were merged, any conflicts encountered, and aggregated
// secret sources. The extractFunc and mergeFunc callbacks allow customization of how specs are
// extracted from policies and merged together.
type configPolicy[T any] struct {
	// Spec is the merged config policy specification
	Spec T
	// extractFunc extracts the relevant spec (ES or Kibana) from a StackConfigPolicy
	extractFunc func(p *policyv1alpha1.StackConfigPolicy) (spec T)
	// mergeFunc merges a source spec into the destination spec, handling conflicts
	mergeFunc func(dstSpec *T, srcSpec T, srcPolicy *policyv1alpha1.StackConfigPolicy) error
	// SecretSources contains aggregated secure settings secret sources, keyed by StackConfigPolicy namespace
	SecretSources []commonv1.NamespacedSecretSource
	// PolicyRefs contains references to all policies that targeted and were merged for this object
	PolicyRefs []policyv1alpha1.StackConfigPolicy
}

// merge processes all provided policies, filters those targeting the given object, and merges them
// in order of their weight (highest weight first). Policies with the same weight are flagged as conflicts.
// The merge operation is customized through the configPolicy's extractFunc and mergeFunc callbacks.
func merge[T any](
	c *configPolicy[T],
	obj metav1.Object,
	allPolicies []policyv1alpha1.StackConfigPolicy,
	operatorNamespace string,
) error {
	if len(allPolicies) == 0 {
		return nil
	}

	policiesByWeight := make(map[int32]policyv1alpha1.StackConfigPolicy)
	for _, p := range allPolicies {
		matches, err := DoesPolicyMatchObject(&p, obj, operatorNamespace)
		if err != nil {
			return err
		}
		if !matches {
			// policy does not target the given k8s object
			continue
		}
		pWeight := p.Spec.Weight
		if pExisting, exists := policiesByWeight[pWeight]; exists {
			pNsn := k8s.ExtractNamespacedName(&p)
			pExistingNsn := k8s.ExtractNamespacedName(&pExisting)
			err := fmt.Errorf("%w: policies %q and %q have the same weight %d", errMergeConflict, pNsn, pExistingNsn, pWeight)
			return err
		}

		policiesByWeight[pWeight] = p
		c.PolicyRefs = append(c.PolicyRefs, p)
	}

	slices.SortFunc(c.PolicyRefs, func(p1, p2 policyv1alpha1.StackConfigPolicy) int {
		return cmp.Compare(p2.Spec.Weight, p1.Spec.Weight)
	})

	for _, p := range c.PolicyRefs {
		srcSpec := c.extractFunc(&p)
		if err := c.mergeFunc(&c.Spec, srcSpec, &p); err != nil {
			return err
		}
	}

	return nil
}

// getConfigPolicyForElasticsearch builds a merged stack config policy for the given Elasticsearch cluster.
// It processes all provided policies, filtering those that target the Elasticsearch cluster, and merges them
// in order of their weight (lowest to highest). Policies with the same weight are flagged as conflicts.
// Returns an esPolicyConfig containing the merged configuration and any error occurred during merging.
func getConfigPolicyForElasticsearch(es *esv1.Elasticsearch, allPolicies []policyv1alpha1.StackConfigPolicy, params operator.Parameters) (*configPolicy[policyv1alpha1.ElasticsearchConfigPolicySpec], error) {
	mergedPolicies := 0
	sMntAggr := secretMountsAggregator{}
	sSrcAggr := secretSourceAggregator{}
	cfgPolicy := &configPolicy[policyv1alpha1.ElasticsearchConfigPolicySpec]{
		extractFunc: func(p *policyv1alpha1.StackConfigPolicy) policyv1alpha1.ElasticsearchConfigPolicySpec {
			return p.Spec.Elasticsearch
		},
		mergeFunc: func(dst *policyv1alpha1.ElasticsearchConfigPolicySpec, src policyv1alpha1.ElasticsearchConfigPolicySpec, srcPolicy *policyv1alpha1.StackConfigPolicy) error {
			var err error
			if mergedPolicies == 0 {
				// First policy: copy directly without merging/canonicalizing to avoid unnecessary differences
				specCopy := src.DeepCopy()
				// SecureSettings are aggregated by sSrcAggr
				specCopy.SecureSettings = nil
				// SecretMounts are aggregated by sMntAggr
				specCopy.SecretMounts = nil
				*dst = *specCopy
			} else {
				if err = mergeElasticsearchSpecs(dst, &src); err != nil {
					return err
				}
			}

			if dst.SecretMounts, err = sMntAggr.aggregate(dst.SecretMounts, srcPolicy.Spec.Elasticsearch.SecretMounts, srcPolicy); err != nil {
				return err
			}

			sSrcAggr.aggregate(srcPolicy.Spec.Elasticsearch.SecureSettings, srcPolicy)
			mergedPolicies++
			return nil
		},
	}

	if err := merge(cfgPolicy, es, allPolicies, params.OperatorNamespace); err != nil {
		return cfgPolicy, err
	}
	cfgPolicy.SecretSources = sSrcAggr.namespacedSecretSources
	return cfgPolicy, nil
}

// mergeElasticsearchSpecs merges src policyv1alpha1.ElasticsearchConfigPolicySpec into dst.
func mergeElasticsearchSpecs(dst, src *policyv1alpha1.ElasticsearchConfigPolicySpec) error {
	var err error
	fields := []struct {
		dst   **commonv1.Config
		src   *commonv1.Config
		merge func(*commonv1.Config, *commonv1.Config) (*commonv1.Config, error)
	}{
		{&dst.ClusterSettings, src.ClusterSettings, deepMergeConfig},
		{&dst.SnapshotRepositories, src.SnapshotRepositories, mergeConfig},
		{&dst.SnapshotLifecyclePolicies, src.SnapshotLifecyclePolicies, deepMergeConfig},
		{&dst.SecurityRoleMappings, src.SecurityRoleMappings, deepMergeConfig},
		{&dst.IndexLifecyclePolicies, src.IndexLifecyclePolicies, deepMergeConfig},
		{&dst.IngestPipelines, src.IngestPipelines, deepMergeConfig},
		{&dst.IndexTemplates.ComposableIndexTemplates, src.IndexTemplates.ComposableIndexTemplates, deepMergeConfig},
		{&dst.IndexTemplates.ComponentTemplates, src.IndexTemplates.ComponentTemplates, deepMergeConfig},
		{&dst.Config, src.Config, deepMergeConfig},
	}
	for _, f := range fields {
		*f.dst, err = f.merge(*f.dst, f.src)
		if err != nil {
			return err
		}
	}
	return nil
}

// getConfigPolicyForKibana builds a merged stack config policy for the given Kibana instance.
// It processes all provided policies, filtering those that target the Kibana instance, and merges them
// in order of their weight (lowest to highest). Policies with the same weight are flagged as conflicts.
// Returns an kbnPolicyConfig containing the merged configuration and any error occurred during merging.
func getConfigPolicyForKibana(kbn *kbv1.Kibana, allPolicies []policyv1alpha1.StackConfigPolicy, params operator.Parameters) (*configPolicy[policyv1alpha1.KibanaConfigPolicySpec], error) {
	mergedPolicies := 0
	sSrcAggr := secretSourceAggregator{}
	cfgPolicy := &configPolicy[policyv1alpha1.KibanaConfigPolicySpec]{
		extractFunc: func(p *policyv1alpha1.StackConfigPolicy) policyv1alpha1.KibanaConfigPolicySpec {
			return p.Spec.Kibana
		},
		mergeFunc: func(dst *policyv1alpha1.KibanaConfigPolicySpec, src policyv1alpha1.KibanaConfigPolicySpec, srcPolicy *policyv1alpha1.StackConfigPolicy) error {
			if mergedPolicies == 0 {
				// First policy: copy directly without merging/canonicalizing to avoid unnecessary differences
				specCopy := src.DeepCopy()
				// SecureSettings are aggregated by sSrcAggr
				specCopy.SecureSettings = nil
				*dst = *specCopy
			} else {
				var err error
				if dst.Config, err = deepMergeConfig(dst.Config, src.Config); err != nil {
					return err
				}
			}

			sSrcAggr.aggregate(srcPolicy.Spec.Kibana.SecureSettings, srcPolicy)
			mergedPolicies++
			return nil
		},
	}

	if err := merge(cfgPolicy, kbn, allPolicies, params.OperatorNamespace); err != nil {
		return cfgPolicy, err
	}
	cfgPolicy.SecretSources = sSrcAggr.namespacedSecretSources
	return cfgPolicy, nil
}

// DoesPolicyMatchObject checks if the given StackConfigPolicy targets the given object (e.g., Elasticsearch or Kibana).
// A policy targets an object if both following conditions are met:
// 1. The policy is in either the operator namespace or the same namespace as the object
// 2. The policy's label selector matches the object's labels
// Returns true if the policy targets the object, false otherwise, and an error if the label selector is invalid.
func DoesPolicyMatchObject(policy *policyv1alpha1.StackConfigPolicy, obj metav1.Object, operatorNamespace string) (bool, error) {
	// Check namespace restrictions; the policy must be in operator namespace or same namespace as the target object.
	// This enforces the scoping rules: policies in the operator namespace are global,
	// policies in other namespaces can only target resources in their own namespace.
	if policy.Namespace != operatorNamespace && policy.Namespace != obj.GetNamespace() {
		return false, nil
	}

	// Convert the label selector from the policy spec into a labels.Selector that can be used for matching
	selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.ResourceSelector)
	if err != nil {
		// Return error if the label selector syntax is invalid (e.g., malformed expressions)
		return false, err
	}

	// Check if the label selector matches the object's labels.
	// This is the actual matching logic - does this policy's selector match this object's labels?
	if !selector.Matches(labels.Set(obj.GetLabels())) {
		return false, nil
	}

	// Both conditions met: namespace is valid and labels match
	return true, nil
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

	if err = dstCanonicalConfig.MergeWith(srcCanonicalConfig); err != nil {
		return nil, err
	}

	dst.Data = nil
	if err = dstCanonicalConfig.Unpack(&dst.Data); err != nil {
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
	if err = dstCanonicalConfig.Unpack(&dst.Data); err != nil {
		return nil, err
	}

	srcCfg := &commonv1.Config{}
	if err = srcCanonicalConfig.Unpack(&srcCfg.Data); err != nil {
		return nil, err
	}

	maps.Copy(dst.Data, srcCfg.Data)

	return dst, nil
}

// secretMountsAggregator aggregates secret mounts from multiple policies while detecting conflicts.
// It tracks which policy defines each secret name and mount path to ensure no two policies
// attempt to mount different secrets to the same path or mount the same secret name twice.
type secretMountsAggregator struct {
	policiesByMountPath  map[string]*policyv1alpha1.StackConfigPolicy
	policiesBySecretName map[string]*policyv1alpha1.StackConfigPolicy
	appliedPolicies      int
}

// aggregate merges source secret mounts into destination, checking for conflicts on secret names
// and mount paths. Returns the merged slice of secret mounts sorted deterministically when
// multiple policies have been applied, or an error if conflicts are detected.
func (s *secretMountsAggregator) aggregate(dst []policyv1alpha1.SecretMount, src []policyv1alpha1.SecretMount, srcPolicy *policyv1alpha1.StackConfigPolicy) ([]policyv1alpha1.SecretMount, error) {
	if src == nil {
		return dst, nil
	}

	s.appliedPolicies++

	if s.policiesBySecretName == nil {
		s.policiesBySecretName = make(map[string]*policyv1alpha1.StackConfigPolicy)
	}
	if s.policiesByMountPath == nil {
		s.policiesByMountPath = make(map[string]*policyv1alpha1.StackConfigPolicy)
	}

	srcPolicyNsn := k8s.ExtractNamespacedName(srcPolicy)

	// Merge in source entries, checking for conflicts
	for _, secretMount := range src {
		if existingPolicy, exists := s.policiesBySecretName[secretMount.SecretName]; exists {
			existingPolicyNsn := k8s.ExtractNamespacedName(existingPolicy)
			err := fmt.Errorf("%w: secret with name %q is defined in policies %q, %q", errMergeConflict, secretMount.SecretName,
				srcPolicyNsn.String(), existingPolicyNsn.String())
			return nil, err
		}
		if existingPolicy, exists := s.policiesByMountPath[secretMount.MountPath]; exists {
			existingPolicyNsn := k8s.ExtractNamespacedName(existingPolicy)
			err := fmt.Errorf("%w: secret mount path %q is defined in policies %q, %q", errMergeConflict, secretMount.MountPath,
				srcPolicyNsn.String(), existingPolicyNsn.String())
			return nil, err
		}
		s.policiesBySecretName[secretMount.SecretName] = srcPolicy
		s.policiesByMountPath[secretMount.MountPath] = srcPolicy
		dst = append(dst, secretMount)
	}

	if s.appliedPolicies > 1 {
		// we want sort only when we have applied more than one policy to guarantee deterministic order, otherwise
		// leave namespacedSecretSources as they came to not cause undesired differences
		slices.SortFunc(dst, func(a, b policyv1alpha1.SecretMount) int {
			return strings.Compare(a.SecretName, b.SecretName)
		})
	}
	return dst, nil
}

// secretSourceAggregator aggregates secure settings secret sources from multiple policies.
// It organizes secret sources by policy namespace and ensures deterministic ordering when
// multiple policies contribute sources.
type secretSourceAggregator struct {
	appliedPolicies         int
	namespacedSecretSources []commonv1.NamespacedSecretSource
}

// aggregate merges source secure settings into the aggregator, organizing them by the source
// policy's namespace. Secret sources are sorted deterministically when multiple policies have
// been applied to ensure consistent results.
func (s *secretSourceAggregator) aggregate(src []commonv1.SecretSource, srcPolicy *policyv1alpha1.StackConfigPolicy) {
	if src == nil {
		return
	}
	s.appliedPolicies++

	srcPolicyNamespace := srcPolicy.GetNamespace()
	for _, ss := range src {
		s.namespacedSecretSources = append(s.namespacedSecretSources, commonv1.NamespacedSecretSource{
			Namespace:  srcPolicyNamespace,
			SecretName: ss.SecretName,
			Entries:    ss.Entries,
		})
	}

	if s.appliedPolicies > 1 {
		// we want sort only when we have applied more than one policy to guarantee deterministic order, otherwise
		// leave namespacedSecretSources as they came to not cause undesired differences
		slices.SortFunc(s.namespacedSecretSources, func(a, b commonv1.NamespacedSecretSource) int {
			return strings.Compare(a.SecretName, b.SecretName)
		})
	}
}
