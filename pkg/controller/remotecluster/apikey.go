// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remotecluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// reconcileAPIKeys creates or updates the API Keys for the remote/client cluster,
// which may have several references (.Spec.RemoteClusters) to the cluster being reconciled.
func reconcileAPIKeys(
	ctx context.Context,
	c k8s.Client,
	activeAPIKeys esclient.CrossClusterAPIKeyList, // all the API Keys in the reconciled/local cluster
	reconciledES *esv1.Elasticsearch, // the Elasticsearch cluster being reconciled, where the API keys must be created/invalidated
	clientES *esv1.Elasticsearch, // the remote Elasticsearch cluster which is going to act as the client, where the API keys are going to be store in the keystore Secret
	remoteClusters []esv1.RemoteCluster, // the expected API keys for that client cluster
	esClient esclient.Client, // ES client for the remote cluster which is going to act as the client
) error {
	// clientClusterAPIKeyStore is used to reconcile encoded API keys in the client cluster, to inject new API keys
	// or to delete the ones which are no longer needed.
	clientClusterAPIKeyStore, err := LoadAPIKeyStore(ctx, c, clientES)
	if err != nil {
		return err
	}

	log := ulog.FromContext(ctx).WithValues(
		"local_namespace", reconciledES.Namespace,
		"local_name", reconciledES.Name,
		"remote_namespace", clientES.Namespace,
		"remote_name", clientES.Name,
	)

	// Maintain a list of the expected API keys for that specific client cluster, to detect the ones which are no longer expected in the reconciled cluster.
	expectedKeysInReconciledES := sets.New[string]()
	// Same for the aliases
	expectedAliases := sets.New[string]()
	activeAPIKeysNames := activeAPIKeys.KeyNames()
	for _, remoteCluster := range remoteClusters {
		apiKeyName := fmt.Sprintf("eck-%s-%s-%s", clientES.Namespace, clientES.Name, remoteCluster.Name)
		expectedKeysInReconciledES.Insert(apiKeyName)
		expectedAliases.Insert(remoteCluster.Name)
		if remoteCluster.APIKey == nil {
			if activeAPIKeysNames.Has(apiKeyName) {
				// We found an API key for that client cluster while it is not expected to have one.
				// It may happen when the user switched back from API keys to the legacy remote cluster.
				log.Info("Invalidating API key as remote cluster is not configured to use it", "alias", remoteCluster.Name)
				if err := esClient.InvalidateCrossClusterAPIKey(ctx, apiKeyName); err != nil {
					return err
				}
			}
			continue
		}

		// Attempt to get an existing API Key with that key name.
		activeAPIKey := activeAPIKeys.GetActiveKeyWithName(apiKeyName)
		expectedHash := hash.HashObject(remoteCluster.APIKey)
		if activeAPIKey == nil {
			// Active API key not found, let's create a new one.
			log.Info("Creating API key", "alias", remoteCluster.Name, "key", apiKeyName)
			apiKey, err := esClient.CreateCrossClusterAPIKey(ctx, esclient.CrossClusterAPIKeyCreateRequest{
				Name: apiKeyName,
				CrossClusterAPIKeyUpdateRequest: esclient.CrossClusterAPIKeyUpdateRequest{
					RemoteClusterAPIKey: *remoteCluster.APIKey,
					Metadata:            newMetadataFor(clientES, expectedHash),
				},
			})
			if err != nil {
				return err
			}
			clientClusterAPIKeyStore.Update(reconciledES.Name, reconciledES.Namespace, remoteCluster.Name, apiKey.ID, apiKey.Encoded)
		}
		// If an API key already exists ensure that the access field is the expected one using the hash
		if activeAPIKey != nil {
			// Ensure that the API key is in the keystore
			if clientClusterAPIKeyStore.KeyIDFor(remoteCluster.Name) != activeAPIKey.ID {
				// We have a problem here, the API Key ID in Elasticsearch does not match the API Key recorded in the Secret.
				// Invalidate the API Key in ES and requeue
				log.Info("Invalidating API key as it does not match the one in keystore", "alias", remoteCluster.Name, "key", apiKeyName)
				if err := esClient.InvalidateCrossClusterAPIKey(ctx, activeAPIKey.Name); err != nil {
					return err
				}
				return fmt.Errorf(
					"cluster key id for alias %s %s (%s), does not match the one stored in the keystore of %s/%s",
					remoteCluster.Name, activeAPIKey.Name, activeAPIKey.ID, clientES.Namespace, clientES.Name,
				)
			}
			currentHash := activeAPIKey.Metadata["elasticsearch.k8s.elastic.co/config-hash"]
			if currentHash != expectedHash {
				log.Info("Updating API key", "alias", remoteCluster.Name)
				// Update the Key
				_, err := esClient.UpdateCrossClusterAPIKey(ctx, activeAPIKey.ID, esclient.CrossClusterAPIKeyUpdateRequest{
					RemoteClusterAPIKey: *remoteCluster.APIKey,
					Metadata:            newMetadataFor(clientES, expectedHash),
				})
				if err != nil {
					return err
				}
			}
		}
	}

	// Get all the active API keys which have been created for that client cluster.
	activeAPIKeysForClientCluster, err := activeAPIKeys.ForCluster(clientES.Namespace, clientES.Name)
	if err != nil {
		return err
	}
	// Invalidate all the keys related to that local cluster which are not expected.
	for keyName := range activeAPIKeysForClientCluster.KeyNames() {
		if !expectedKeysInReconciledES.Has(keyName) {
			// Unexpected key, let's invalidate it.
			log.Info("Invalidating unexpected API key", "key", keyName)
			if err := esClient.InvalidateCrossClusterAPIKey(ctx, keyName); err != nil {
				return err
			}
		}
	}

	// Delete all the keys in the keystore which are not expected.
	aliases := clientClusterAPIKeyStore.ForCluster(reconciledES.Namespace, reconciledES.Name)
	for existingAlias := range aliases {
		if expectedAliases.Has(existingAlias) {
			continue
		}
		clientClusterAPIKeyStore.Delete(existingAlias)
	}

	// Save the generated keys in the keystore.
	if err := clientClusterAPIKeyStore.Save(ctx, c, clientES); err != nil {
		return err
	}
	return nil
}

// newMetadataFor returns the metadata to be set in the Elasticsearch API keys metadata in the Elasticsearch cluster
// state, not on a Kubernetes object.
func newMetadataFor(clientES *esv1.Elasticsearch, expectedHash string) map[string]interface{} {
	return map[string]interface{}{
		"elasticsearch.k8s.elastic.co/config-hash": expectedHash,
		"elasticsearch.k8s.elastic.co/name":        clientES.Name,
		"elasticsearch.k8s.elastic.co/namespace":   clientES.Namespace,
		"elasticsearch.k8s.elastic.co/uid":         clientES.UID,
		"elasticsearch.k8s.elastic.co/managed-by":  "eck",
	}
}
