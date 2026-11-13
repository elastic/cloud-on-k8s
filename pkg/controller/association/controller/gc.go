// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// GarbageCollectLegacyFleetServerAdditionalSecrets removes secret copies that were placed in
// fleet-managed agent namespaces by older ECK versions. Those copies used the source secret's
// original name as the target name and carried the fleet server's Elasticsearch association labels
// verbatim (agentassociation.k8s.elastic.co/name=<fleet-server>, type=elasticsearch). Current
// ECK overrides those labels with agent-level labels (type=fleetserver), so the label selector
// below exclusively matches legacy copies and never touches new-scheme copies or original secrets.
//
// Detection relies on a namespace-mismatch invariant: a secret that carries an association
// namespace label (agentassociation.k8s.elastic.co/namespace) whose value differs from the
// secret's own namespace is by definition a cross-namespace copy and not the original. Original
// secrets always live in the same namespace as the resource they describe, so their association
// namespace label equals their actual namespace.
//
// An empty managedNamespaces slice means the operator manages all namespaces.
func GarbageCollectLegacyFleetServerAdditionalSecrets(ctx context.Context, c k8s.Client, managedNamespaces []string) error {
	var errs error
	log := ulog.FromContext(ctx)
	if len(managedNamespaces) == 0 {
		// Empty means all namespaces; use the empty string understood by client.InNamespace.
		managedNamespaces = []string{""}
	}
	for _, ns := range managedNamespaces {
		var secrets corev1.SecretList
		if err := c.List(ctx, &secrets,
			client.InNamespace(ns),
			client.MatchingLabels{AgentAssociationLabelType: commonv1.ElasticsearchAssociationType},
			client.HasLabels{AgentAssociationLabelName, AgentAssociationLabelNamespace, eslabel.ClusterNameLabelName, eslabel.ClusterNamespaceLabelName},
		); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to list secrets in namespace %q: %w", ns, err))
			continue
		}
		for i := range secrets.Items {
			s := &secrets.Items[i]
			// New-scheme copies carry an owner reference to the agent so that Kubernetes GC
			// removes them on agent deletion. Legacy copies have no owner references.
			if len(s.OwnerReferences) > 0 {
				continue
			}
			// If the association namespace label matches the secret's actual namespace, this is
			// the original secret in its home namespace - leave it alone.
			if s.Labels[AgentAssociationLabelNamespace] == s.Namespace {
				continue
			}
			if err := k8s.DeleteSecretIfExists(ctx, c, k8s.ExtractNamespacedName(s)); err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			log.Info("Deleted legacy fleet-server additional secret copy", "secret_name", s.Name, "secret_namespace", s.Namespace)
		}
	}
	return errs
}
