// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonapikey "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/apikey"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	esuser "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

const (
	// PolicyNameLabelKey is the label key for the AutoOpsAgentPolicy name.
	// This is exported as its used in the remotecluster controller to identify API keys managed by the autoops controller.
	PolicyNameLabelKey = "autoops.k8s.elastic.co/policy-name"
	// policyNamespaceLabelKey is the label key for the AutoOpsAgentPolicy namespace.
	policyNamespaceLabelKey = "autoops.k8s.elastic.co/policy-namespace"
	// policySecretTypeLabelKey is the label key that identifies the type of secret for the AutoOpsAgentPolicy.
	policySecretTypeLabelKey = "autoops.k8s.elastic.co/secret-type" //nolint:gosec
	apiKeySecretKey          = "api_key"
)

// apiKeySpec represents the specification for an autoops API key
type apiKeySpec struct {
	roleDescriptors map[string]esclient.Role
}

// reconcileAutoOpsESAPIKey reconciles the API key and secret for a specific Elasticsearch cluster.
// The secret is returned to the caller to avoid an additional call to the cache to retrieve the secret
// which delays the initial deployment.
func (r *AgentPolicyReconciler) reconcileAutoOpsESAPIKey(
	ctx context.Context,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	es esv1.Elasticsearch,
) (*corev1.Secret, error) {
	log := ulog.FromContext(ctx).WithValues(
		"es_namespace", es.Namespace,
		"es_name", es.Name,
	)
	log.V(1).Info("Reconciling AutoOps ES API key")

	esClient, err := r.esClientProvider(ctx, r.Client, r.params.Dialer, es)
	if err != nil {
		return nil, fmt.Errorf("while creating Elasticsearch client for %s/%s: %w", es.Namespace, es.Name, err)
	}
	defer esClient.Close()

	autoOpsUserRole, ok := esuser.PredefinedRoles[esuser.AutoOpsUserRole].(esclient.Role)
	if !ok {
		return nil, fmt.Errorf("autoOpsUserRole could not be converted to esclient.Role")
	}

	apiKeySpec := apiKeySpec{
		roleDescriptors: map[string]esclient.Role{
			"eck_autoops_role": autoOpsUserRole,
		},
	}

	// Calculate expected hash
	expectedHash := hash.HashObject(apiKeySpec)

	apiKeyName := apiKeyNameFor(policy, es)

	// Check if API key exists
	activeAPIKeys, err := esClient.GetAPIKeysByName(ctx, apiKeyName)
	if err != nil {
		return nil, fmt.Errorf("while getting API keys by name %s: %w", apiKeyName, err)
	}

	var activeAPIKey *esclient.APIKey
	for i := range activeAPIKeys.APIKeys {
		if activeAPIKeys.APIKeys[i].Name == apiKeyName {
			activeAPIKey = &activeAPIKeys.APIKeys[i]
			break
		}
	}

	if activeAPIKey == nil {
		return r.createAPIKey(ctx, log, esClient, apiKeyName, apiKeySpec, expectedHash, policy, es)
	}

	return r.maybeUpdateAPIKey(ctx, log, activeAPIKey, apiKeyName, apiKeySpec, expectedHash, policy, es)
}

// createAPIKey creates a new API key in Elasticsearch and stores it in a secret.
func (r *AgentPolicyReconciler) createAPIKey(
	ctx context.Context,
	log logr.Logger,
	esClient esclient.Client,
	apiKeyName string,
	apiKeySpec apiKeySpec,
	expectedHash string,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	es esv1.Elasticsearch,
) (*corev1.Secret, error) {
	log.Info("Creating API key", "key", apiKeyName)

	metadata := newMetadataFor(&policy, &es, expectedHash)
	// Unfortunately we need to convert the metadata to a map[string]any to satisfy the APIKeyCreateRequest type.
	// We return map[string]string because this is also used when storing the API key in a secret.
	metadataAny := make(map[string]any, len(metadata))
	for k, v := range metadata {
		metadataAny[k] = v
	}

	apiKeyResp, err := esClient.CreateAPIKey(ctx, esclient.APIKeyCreateRequest{
		Name: apiKeyName,
		APIKeyUpdateRequest: esclient.APIKeyUpdateRequest{
			RoleDescriptors: apiKeySpec.roleDescriptors,
			Metadata:        metadataAny,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("while creating API key %s: %w", apiKeyName, err)
	}

	return r.storeAPIKeyInSecret(ctx, policy, es, apiKeyResp.Encoded, expectedHash)
}

// maybeUpdateAPIKey checks if the API key needs to be updated and handles it.
func (r *AgentPolicyReconciler) maybeUpdateAPIKey(
	ctx context.Context,
	log logr.Logger,
	activeAPIKey *esclient.APIKey,
	apiKeyName string,
	apiKeySpec apiKeySpec,
	expectedHash string,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	es esv1.Elasticsearch,
) (*corev1.Secret, error) {
	// If the key isn't managed by ECK or it's hash is incorrect, invalidate and recreate the api key.
	if !commonapikey.IsManagedByECK(activeAPIKey.Metadata) || commonapikey.NeedsUpdate(activeAPIKey.Metadata, expectedHash) {
		return r.invalidateAndCreateAPIKey(ctx, log, activeAPIKey, apiKeyName, apiKeySpec, expectedHash, policy, es)
	}

	// The API key is seemingly up to date, so we need to ensure the secret exists with correct value
	secretName := autoopsv1alpha1.APIKeySecret(policy.GetName(), k8s.ExtractNamespacedName(&es))
	var secret corev1.Secret
	nsn := types.NamespacedName{
		Namespace: policy.Namespace,
		Name:      secretName,
	}
	if err := r.Client.Get(ctx, nsn, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret doesn't exist so again we need to invalidate and recreate the API key.
			log.Info("API key secret not found, recreating key", "key", apiKeyName)
			return r.invalidateAndCreateAPIKey(ctx, log, activeAPIKey, apiKeyName, apiKeySpec, expectedHash, policy, es)
		}
		return nil, fmt.Errorf("while retrieving API key secret %s: %w", secretName, err)
	}

	// Since the secret exists, we just need to verify the data is correct.
	if encodedKey, ok := secret.Data[apiKeySecretKey]; !ok || string(encodedKey) == "" {
		log.Info("API key secret exists but is missing api_key, recreating key", "key", apiKeyName)
		return r.invalidateAndCreateAPIKey(ctx, log, activeAPIKey, apiKeyName, apiKeySpec, expectedHash, policy, es)
	}

	log.V(1).Info("API key is up to date", "key", apiKeyName)
	return &secret, nil
}

func (r *AgentPolicyReconciler) invalidateAndCreateAPIKey(
	ctx context.Context,
	log logr.Logger,
	activeAPIKey *esclient.APIKey,
	apiKeyName string,
	apiKeySpec apiKeySpec,
	expectedHash string,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	es esv1.Elasticsearch,
) (*corev1.Secret, error) {
	esClient, err := r.esClientProvider(ctx, r.Client, r.params.Dialer, es)
	if err != nil {
		return nil, fmt.Errorf("while creating Elasticsearch client for %s/%s: %w", es.Namespace, es.Name, err)
	}
	defer esClient.Close()
	if err := invalidateAPIKey(ctx, esClient, activeAPIKey.ID); err != nil {
		return nil, err
	}
	return r.createAPIKey(ctx, log, esClient, apiKeyName, apiKeySpec, expectedHash, policy, es)
}

// invalidateAPIKey invalidates an API key by its key ID by calling the Elasticsearch API.
func invalidateAPIKey(ctx context.Context, esClient esclient.Client, keyID string) error {
	_, err := esClient.InvalidateAPIKeys(ctx, esclient.APIKeysInvalidateRequest{
		IDs: []string{keyID},
	})
	return err
}

// storeAPIKeyInSecret stores the API key in a Kubernetes secret.
func (r *AgentPolicyReconciler) storeAPIKeyInSecret(
	ctx context.Context,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	es esv1.Elasticsearch,
	encodedKey string,
	expectedHash string,
) (*corev1.Secret, error) {
	secretName := autoopsv1alpha1.APIKeySecret(policy.GetName(), k8s.ExtractNamespacedName(&es))
	expected := buildAutoOpsESAPIKeySecret(policy, es, secretName, encodedKey, expectedHash)

	reconciled := &corev1.Secret{}
	err := reconciler.ReconcileResource(
		reconciler.Params{
			Context: ctx,
			Client:  r.Client,
			// explicitly setting the owner to nil to allow the controller to clean up
			// the api keys in Elasticsearch when the policy is deleted.
			Owner:      nil,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
					!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
					!reflect.DeepEqual(expected.Data, reconciled.Data)
			},
			UpdateReconciled: func() {
				reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
				reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
				reconciled.Data = expected.Data
			},
		},
	)
	if err != nil {
		return nil, err
	}

	watcher := k8s.ExtractNamespacedName(&policy)

	// Add a watch for the AutoOps API key secret
	err = watches.WatchUserProvidedSecrets(
		watcher,
		r.dynamicWatches,
		secretName,
		[]string{secretName},
	)
	if err != nil {
		return nil, err
	}

	return reconciled, nil
}

// buildAutoOpsESAPIKeySecret builds the expected Secret for autoops ES API key.
func buildAutoOpsESAPIKeySecret(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch, secretName string, encodedKey string, expectedHash string) corev1.Secret {
	baseLabels := resourceLabelsFor(policy, es)
	apiKeyLabels := maps.Merge(baseLabels, map[string]string{
		policySecretTypeLabelKey:           apiKeySecretType,
		commonapikey.MetadataKeyConfigHash: expectedHash,
	})

	meta := metadata.Propagate(&policy, metadata.Metadata{
		Labels:      apiKeyLabels,
		Annotations: policy.GetAnnotations(),
	})

	// The 'managed-by' is only needed/wanted for the API key in
	// Elasticsearch so we remove it from the secret.
	delete(meta.Labels, commonapikey.MetadataKeyManagedBy)

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        secretName,
			Namespace:   policy.GetNamespace(),
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: map[string][]byte{
			apiKeySecretKey: []byte(encodedKey),
		},
	}
}

// IsManagedByAutoOps checks if an API key is managed by the autoops controller.
func IsManagedByAutoOps(metadata map[string]any) bool {
	if metadata == nil {
		return false
	}
	_, exists := metadata[PolicyNameLabelKey]
	return exists
}

// apiKeyNameFor generates a unique name for the API key according to the policy, and ES instance.
func apiKeyNameFor(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) string {
	return fmt.Sprintf("autoops-%s-%s-%s-%s", policy.Namespace, policy.Name, es.Namespace, es.Name)
}

// newMetadataFor returns the metadata to be set in the Elasticsearch API key.
func newMetadataFor(policy *autoopsv1alpha1.AutoOpsAgentPolicy, es *esv1.Elasticsearch, expectedHash string) map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:              autoOpsAgentType,
		commonapikey.MetadataKeyConfigHash:  expectedHash,
		commonapikey.MetadataKeyESName:      es.Name,
		commonapikey.MetadataKeyESNamespace: es.Namespace,
		commonapikey.MetadataKeyManagedBy:   commonapikey.MetadataValueECK,
		PolicyNameLabelKey:                  policy.Name,
		policyNamespaceLabelKey:             policy.Namespace,
	}
}

// cleanupAutoOpsESAPIKey invalidates the API key and removes the secret when autoops should not exist.
func cleanupAutoOpsESAPIKey(
	ctx context.Context,
	c k8s.Client,
	esClientProvider commonesclient.Provider,
	dialer net.Dialer,
	policyNamespace, policyName string,
	es esv1.Elasticsearch,
) error {
	log := ulog.FromContext(ctx).WithValues(
		"es_namespace", es.Namespace,
		"es_name", es.Name,
	)
	log.V(1).Info("Cleaning up AutoOps ES API key")

	if es.Status.Phase != esv1.ElasticsearchReadyPhase {
		log.V(1).Info("Skipping ES cluster that is not ready")
		return nil
	}

	// Get Elasticsearch client
	esClient, err := esClientProvider(ctx, c, dialer, es)
	if err != nil {
		return fmt.Errorf("failed to create Elasticsearch client for %s/%s: %w", es.Namespace, es.Name, err)
	}
	defer esClient.Close()

	apiKeyName := apiKeyNameFor(autoopsv1alpha1.AutoOpsAgentPolicy{ObjectMeta: metav1.ObjectMeta{Namespace: policyNamespace, Name: policyName}}, es)

	// Check if API key exists
	activeAPIKeys, err := esClient.GetAPIKeysByName(ctx, apiKeyName)
	if err != nil {
		return fmt.Errorf("while getting API keys by name %s: %w", apiKeyName, err)
	}

	// Invalidate all matching API keys
	for _, key := range activeAPIKeys.APIKeys {
		if key.Name == apiKeyName {
			log.Info("Invalidating API key", "key", apiKeyName, "id", key.ID)
			if err := invalidateAPIKey(ctx, esClient, key.ID); err != nil {
				log.Error(err, "while invalidating API key", "key", apiKeyName, "id", key.ID)
			}
		}
	}

	return deleteESAPIKeySecret(ctx, c, log,
		types.NamespacedName{Namespace: policyNamespace, Name: policyName},
		k8s.ExtractNamespacedName(&es))
}

func deleteESAPIKeySecret(ctx context.Context, c k8s.Client, log logr.Logger, policy types.NamespacedName, es types.NamespacedName) error {
	secretName := autoopsv1alpha1.APIKeySecret(policy.Name, es)
	secretKey := types.NamespacedName{
		Namespace: policy.Namespace,
		Name:      secretName,
	}
	var secret corev1.Secret
	if err := c.Get(ctx, secretKey, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret already deleted, nothing to do
			return nil
		}
		return fmt.Errorf("while getting API key secret %s: %w", secretName, err)
	}

	log.Info("Deleting API key secret", "secret", secretName)
	if err := c.Delete(ctx, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("while deleting API key secret %s: %w", secretName, err)
	}
	return nil
}
