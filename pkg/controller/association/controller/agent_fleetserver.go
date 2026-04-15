// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"maps"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	ver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

func AddAgentFleetServer(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:        func() commonv1.Associated { return &agentv1alpha1.Agent{} },
		ReferencedObjTemplate:        func() client.Object { return &agentv1alpha1.Agent{} },
		ExternalServiceURL:           getFleetServerExternalURL,
		ReferencedResourceVersion:    referencedFleetServerStatusVersion,
		ReferencedResourceNamer:      agent.Namer,
		AssociationName:              "agent-fleetserver",
		AssociatedShortName:          "agent",
		AssociationType:              commonv1.FleetServerAssociationType,
		AdditionalSecrets:            additionalSecrets,
		ReconcileTransitiveESSecrets: fleetManagedAgentTransitiveESRef,
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				AgentAssociationLabelName:      associated.Name,
				AgentAssociationLabelNamespace: associated.Namespace,
				AgentAssociationLabelType:      commonv1.FleetServerAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.FleetServerConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      agent.NameLabelName,
		AssociationResourceNamespaceLabelName: agent.NamespaceLabelName,

		ElasticsearchUserCreation: nil,
	})
}

// additionalSecrets returns secrets from the Fleet Server's Elasticsearch association that
// need to be copied into the fleet-managed agent's namespace: the CA secret (when present)
// and the client certificate secret (when the user has provided one on the Fleet Server's ES ref).
func additionalSecrets(ctx context.Context, c k8s.Client, assoc commonv1.Association) ([]types.NamespacedName, error) {
	log := ulog.FromContext(ctx)
	associated := assoc.Associated()
	var agent agentv1alpha1.Agent
	nsn := types.NamespacedName{Namespace: associated.GetNamespace(), Name: associated.GetName()}
	if err := c.Get(ctx, nsn, &agent); err != nil {
		return nil, err
	}
	fleetServerRef := assoc.AssociationRef()
	if !fleetServerRef.IsSet() {
		return nil, nil
	}
	fleetServer := agentv1alpha1.Agent{}
	if err := c.Get(ctx, fleetServerRef.NamespacedName(), &fleetServer); err != nil {
		return nil, err
	}

	// If the Fleet Server Agent is not associated with an Elasticsearch cluster
	// (potentially because of a manual setup) we should do nothing.
	if len(fleetServer.Spec.ElasticsearchRefs) == 0 {
		return nil, nil
	}
	esRef := fleetServer.Spec.ElasticsearchRefs[0]
	esAssociation, err := association.SingleAssociationOfType(fleetServer.GetAssociations(), commonv1.ElasticsearchAssociationType)
	if err != nil {
		return nil, err
	}

	conf, err := esAssociation.AssociationConf()
	if err != nil {
		log.V(1).Info("no additional secrets because no assoc conf")
		return nil, err
	}
	if conf == nil {
		log.V(1).Info("no additional secrets because conf is nil")
		return nil, nil
	}

	secrets := make([]types.NamespacedName, 0, 2)
	if conf.CACertProvided {
		log.V(1).Info("additional secret because CA provided")
		secrets = append(secrets, types.NamespacedName{
			Namespace: fleetServer.Namespace,
			Name:      conf.CASecretName,
		})
	}
	if conf.ClientCertIsConfigured() && esRef.GetClientCertificateSecretName() != "" {
		log.V(1).Info("additional secret because user client certificate is provided")
		secrets = append(secrets, types.NamespacedName{
			Namespace: fleetServer.Namespace,
			Name:      conf.GetClientCertSecretName(),
		})
	}
	return secrets, nil
}

// clientCertSecretName returns a deterministic secret name for a transitive client certificate.
// The name includes a hash of the referenced resource's namespace and name to avoid collisions.
func clientCertSecretName(associated commonv1.Associated, ref commonv1.AssociationRef, associationName string) string {
	associatedName := associated.GetName()
	refHash := hash.HashObject(ref.NamespacedName())
	return associatedName + "-" + associationName + "-" + refHash + "-client-cert"
}

// fleetManagedAgentTransitiveESRef resolves the transitive Elasticsearch association
// (Agent -> Fleet Server -> Elasticsearch) and reconciles a client certificate for the
// fleet-managed agent when the Elasticsearch requires client authentication.
// Orphaned client certificate secrets from previous ES references are cleaned up.
func fleetManagedAgentTransitiveESRef(ctx context.Context, c k8s.Client, assoc commonv1.Association, assocMeta metadata.Metadata) (*commonv1.TransitiveESRef, *reconciler.Results) {
	results := reconciler.NewResult(ctx)

	log := ulog.FromContext(ctx)
	associated := assoc.Associated()
	var agent agentv1alpha1.Agent
	nsn := types.NamespacedName{Namespace: associated.GetNamespace(), Name: associated.GetName()}
	if err := c.Get(ctx, nsn, &agent); err != nil {
		return nil, results.WithError(err)
	}
	fleetServerRef := assoc.AssociationRef()
	if !fleetServerRef.IsSet() {
		if err := deleteOrphanedTransitiveClientCertSecrets(ctx, c, assocMeta, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return nil, nil
	}
	fleetServer := agentv1alpha1.Agent{}
	if err := c.Get(ctx, fleetServerRef.NamespacedName(), &fleetServer); err != nil {
		return nil, results.WithError(err)
	}

	// Fleet Server Agent is not associated with an Elasticsearch cluster
	if len(fleetServer.Spec.ElasticsearchRefs) == 0 {
		if err := deleteOrphanedTransitiveClientCertSecrets(ctx, c, assocMeta, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return nil, nil
	}
	esRef := fleetServer.Spec.ElasticsearchRefs[0]
	esAssociation, err := association.SingleAssociationOfType(fleetServer.GetAssociations(), commonv1.ElasticsearchAssociationType)
	if err != nil {
		return nil, results.WithError(err)
	}

	conf, err := esAssociation.AssociationConf()
	if err != nil {
		log.V(1).Info("Transitive ES association conf not available")
		return nil, results.WithError(err)
	}
	if conf == nil {
		log.V(1).Info("Transitive ES association conf is nil")
		if err := deleteOrphanedTransitiveClientCertSecrets(ctx, c, assocMeta, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return nil, nil
	}

	ref := esAssociation.AssociationRef()
	if ref.IsExternal() {
		// For external ES references, the operator cannot determine whether client
		// authentication is required. Skip client cert reconciliation.
		if err := deleteOrphanedTransitiveClientCertSecrets(ctx, c, assocMeta, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return nil, nil
	}

	var es esv1.Elasticsearch
	if err := c.Get(ctx, ref.NamespacedName(), &es); err != nil {
		return nil, results.WithError(err)
	}

	if !annotation.HasClientAuthenticationRequired(&es) {
		if err := deleteOrphanedTransitiveClientCertSecrets(ctx, c, assocMeta, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return nil, nil
	}

	// If the Fleet Server has a user-provided client certificate for its ES association, reuse it.
	// The secret is already copied into the agent's namespace by additionalSecrets/copySecret.
	if esRef.GetClientCertificateSecretName() != "" && conf.ClientCertIsConfigured() {
		copiedSecretName := conf.GetClientCertSecretName()
		if err := deleteOrphanedTransitiveClientCertSecrets(ctx, c, assocMeta, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return &commonv1.TransitiveESRef{
			ClientCertSecretName: copiedSecretName,
		}, results
	}

	// Build soft-owner labels pointing to the referenced server resource.
	extraLabels := map[string]string{
		reconciler.SoftOwnerNameLabel:      ref.GetName(),
		reconciler.SoftOwnerNamespaceLabel: ref.GetNamespace(),
		reconciler.SoftOwnerKindLabel:      esv1.Kind,
		labels.ClientCertificateLabelName:  "true",
	}

	secretName := clientCertSecretName(associated, ref, "agent-es")

	_, certResults := association.ReconcileManagedClientCert(ctx, c, assoc, assocMeta, secretName, extraLabels)
	results.WithResults(certResults)
	if results.HasError() {
		return nil, results
	}

	if err := deleteOrphanedTransitiveClientCertSecrets(ctx, c, assocMeta, associated, secretName); err != nil {
		return nil, results.WithError(err)
	}

	return &commonv1.TransitiveESRef{
		ClientCertSecretName: secretName,
	}, results
}

// deleteOrphanedTransitiveClientCertSecrets lists client cert secrets scoped to this agent
// (via assocMeta labels) and deletes any that don't match currentSecretName. Pass empty
// string to delete all.
func deleteOrphanedTransitiveClientCertSecrets(ctx context.Context, c k8s.Client, assocMeta metadata.Metadata, associated commonv1.Associated, currentSecretName string) error {
	matchingLabels := make(client.MatchingLabels, len(assocMeta.Labels)+1)
	maps.Copy(matchingLabels, assocMeta.Labels)
	matchingLabels[labels.ClientCertificateLabelName] = "true"

	var secrets corev1.SecretList
	if err := c.List(ctx, &secrets, client.InNamespace(associated.GetNamespace()), matchingLabels); err != nil {
		return err
	}
	for i := range secrets.Items {
		if secrets.Items[i].Name == currentSecretName {
			continue
		}
		if err := c.Delete(ctx, &secrets.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func getFleetServerExternalURL(c k8s.Client, assoc commonv1.Association) (string, error) {
	fleetServerRef := assoc.AssociationRef()
	if !fleetServerRef.IsSet() {
		return "", nil
	}
	fleetServer := agentv1alpha1.Agent{}
	if err := c.Get(context.Background(), fleetServerRef.NamespacedName(), &fleetServer); err != nil {
		return "", err
	}
	serviceName := fleetServerRef.GetServiceName()
	if serviceName == "" {
		serviceName = agent.HTTPServiceName(fleetServer.Name)
	}
	nsn := types.NamespacedName{Namespace: fleetServer.Namespace, Name: serviceName}
	return association.ServiceURL(c, nsn, fleetServer.Spec.HTTP.Protocol(), "")
}

type fleetVersionResponse struct {
	Version struct {
		Number string `json:"number"`
	} `json:"version"`
}

func (fvr fleetVersionResponse) IsServerless() bool {
	return false
}

func (fvr fleetVersionResponse) GetVersion() (string, error) {
	if _, err := ver.Parse(fvr.Version.Number); err != nil {
		return "", err
	}
	return fvr.Version.Number, nil
}

// referencedFleetServerStatusVersion returns the currently running version of Agent
// reported in its status.
func referencedFleetServerStatusVersion(c k8s.Client, fsAssociation commonv1.Association) (string, bool, error) {
	fsRef := fsAssociation.AssociationRef()
	if fsRef.IsExternal() {
		info, err := association.GetUnmanagedAssociationConnectionInfoFromSecret(c, fsAssociation)
		if err != nil {
			return "", false, err
		}
		fleetVersionResponse := &fleetVersionResponse{}
		ver, isServerless, err := info.Version("/api/status", fleetVersionResponse)
		if err != nil {
			// version is in the status API from version 8.0
			if err.Error() == "version is not found" {
				return association.UnknownVersion, false, nil
			}
			return "", false, err
		}
		return ver, isServerless, nil
	}

	var fleetServer agentv1alpha1.Agent
	err := c.Get(context.Background(), fsRef.NamespacedName(), &fleetServer)
	if err != nil {
		return "", false, err
	}
	return fleetServer.Status.Version, false, nil
}
