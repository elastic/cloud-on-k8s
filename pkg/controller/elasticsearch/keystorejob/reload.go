// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystorejob

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// ReloadResult represents the result of a keystore reload operation.
type ReloadResult struct {
	// Converged is true if all expected nodes have the expected keystore digest.
	Converged bool
	// Message provides additional context about the reload status.
	Message string
}

// ReloadSecureSettings calls the Elasticsearch reload_secure_settings API and checks
// if all expected nodes have converged to the expected keystore digest.
// expectedNodeCount is the number of nodes expected based on the ES spec (sum of all NodeSet replicas).
// Returns a ReloadResult indicating whether convergence is complete.
func ReloadSecureSettings(
	ctx context.Context,
	c k8s.Client,
	esClient esclient.Client,
	es esv1.Elasticsearch,
	expectedNodeCount int32,
) (ReloadResult, error) {
	log := ulog.FromContext(ctx)

	// Get the keystore Secret to read the expected digest
	keystoreSecretName := esv1.KeystoreSecretName(es.Name)
	var keystoreSecret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: es.Namespace, Name: keystoreSecretName}, &keystoreSecret); err != nil {
		return ReloadResult{}, err
	}

	// Get the expected digest from the Secret's annotation
	expectedDigest, hasDigest := keystoreSecret.Annotations[esv1.KeystoreDigestAnnotation]
	if !hasDigest || expectedDigest == "" {
		// No digest annotation means the keystore Secret hasn't been created by our job yet
		log.V(1).Info("Keystore secret missing digest annotation, skipping reload",
			"secret", keystoreSecretName)
		return ReloadResult{
			Converged: false,
			Message:   "keystore secret missing digest annotation",
		}, nil
	}

	// Call the reload API
	log.V(1).Info("Calling reload_secure_settings API",
		"expectedDigest", expectedDigest,
		"expectedNodeCount", expectedNodeCount)
	response, err := esClient.ReloadSecureSettings(ctx)
	if err != nil {
		return ReloadResult{}, err
	}

	// Check if all expected nodes responded and have the expected digest
	respondedNodes := int32(len(response.Nodes))
	if respondedNodes < expectedNodeCount {
		log.V(1).Info("Not all expected nodes responded to reload",
			"responded", respondedNodes,
			"expected", expectedNodeCount)
		return ReloadResult{
			Converged: false,
			Message:   "not all expected nodes responded to reload",
		}, nil
	}

	var nodesWithExpectedDigest int32
	for nodeID, node := range response.Nodes {
		if node.KeystoreDigest == expectedDigest {
			nodesWithExpectedDigest++
		} else {
			log.V(1).Info("Node has different keystore digest",
				"node", node.Name,
				"nodeID", nodeID,
				"expected", expectedDigest,
				"actual", node.KeystoreDigest)
		}
	}

	if nodesWithExpectedDigest >= expectedNodeCount {
		log.Info("All expected nodes have reloaded keystore with expected digest",
			"nodesConverged", nodesWithExpectedDigest,
			"expectedNodes", expectedNodeCount,
			"digest", expectedDigest)
		return ReloadResult{
			Converged: true,
			Message:   "all expected nodes have expected keystore digest",
		}, nil
	}

	log.Info("Keystore reload not yet converged",
		"nodesWithExpectedDigest", nodesWithExpectedDigest,
		"expectedNodeCount", expectedNodeCount,
		"expectedDigest", expectedDigest)
	return ReloadResult{
		Converged: false,
		Message:   "waiting for all nodes to reload keystore",
	}, nil
}
