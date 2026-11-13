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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
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

// transitiveESClientCertLabel marks the client certificate an agent uses against the Elasticsearch referenced by its
// Fleet Server, and the trust source published for it into the Elasticsearch namespace.
const transitiveESClientCertLabel = "agentassociation.k8s.elastic.co/transitive-es-client-cert"

// agentFleetServerAssociationLabels returns the labels which identify the Agent and its association with a Fleet
// Server, on the resources created for them.
func agentFleetServerAssociationLabels(associated types.NamespacedName) map[string]string {
	return map[string]string{
		AgentAssociationLabelName:      associated.Name,
		AgentAssociationLabelNamespace: associated.Namespace,
		AgentAssociationLabelType:      commonv1.FleetServerAssociationType,
	}
}

func agentFleetServerAssociationInfo() association.AssociationInfo {
	return association.AssociationInfo{
		AssociatedObjTemplate:                 func() commonv1.Associated { return &agentv1alpha1.Agent{} },
		AssociatedObjListTemplate:             func() client.ObjectList { return &agentv1alpha1.AgentList{} },
		ReferencedObjTemplate:                 func() client.Object { return &agentv1alpha1.Agent{} },
		ExternalServiceURL:                    getFleetServerExternalURL,
		ReferencedResourceVersion:             referencedFleetServerStatusVersion,
		ReferencedResourceNamer:               agent.Namer,
		AssociationName:                       "agent-fleetserver",
		AssociatedShortName:                   "agent",
		AssociationType:                       commonv1.FleetServerAssociationType,
		AdditionalSecrets:                     additionalSecrets,
		ElasticsearchRef:                      agentFleetServerESRef,
		ReconcileTransitiveESSecrets:          fleetManagedAgentTransitiveESRef,
		Labels:                                agentFleetServerAssociationLabels,
		AssociationConfAnnotationNameBase:     commonv1.FleetServerConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      agent.NameLabelName,
		AssociationResourceNamespaceLabelName: agent.NamespaceLabelName,

		ElasticsearchUserCreation: nil,
	}
}

func AddAgentFleetServer(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, agentFleetServerAssociationInfo())
}

// getFleetServerESAssociation fetches the fleet server at nsn and returns its single
// Elasticsearch association. Returns (nil, nil) when the fleet server has no
// ElasticsearchRefs. Returns (nil, err) on any lookup or ambiguity error.
func getFleetServerESAssociation(ctx context.Context, c k8s.Client, nsn types.NamespacedName) (commonv1.Association, error) {
	var fleetServer agentv1alpha1.Agent
	if err := c.Get(ctx, nsn, &fleetServer); err != nil {
		return nil, err
	}
	if len(fleetServer.Spec.ElasticsearchRefs) == 0 {
		return nil, nil
	}
	return association.SingleAssociationOfType(fleetServer.GetAssociations(), commonv1.ElasticsearchAssociationType)
}

// agentFleetServerESRef resolves the Elasticsearch reference transitively via the Fleet Server
// (Agent -> Fleet Server -> Elasticsearch) for RBAC enforcement purposes.
// Returns (false, _, nil) whenever no transitive ES ref can be resolved: the fleet server ref is
// not set on the agent, the fleet server does not exist yet, or the fleet server has no ES association.
func agentFleetServerESRef(ctx context.Context, c k8s.Client, assoc commonv1.Association) (bool, commonv1.AssociationRef, error) {
	fleetServerRef := assoc.AssociationRef()
	if !fleetServerRef.IsSet() {
		return false, commonv1.ObjectSelector{}, nil
	}
	esAssociation, err := getFleetServerESAssociation(ctx, c, fleetServerRef.NamespacedName())
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, commonv1.ObjectSelector{}, nil
		}
		return false, commonv1.ObjectSelector{}, err
	}
	if esAssociation == nil {
		return false, commonv1.ObjectSelector{}, nil
	}
	return true, esAssociation.AssociationRef(), nil
}

// additionalSecrets returns secrets from the Fleet Server's Elasticsearch association that
// need to be copied into the fleet-managed agent's namespace: the CA secret (when present)
// and the client certificate secret (when the user has provided one on the Fleet Server's ES ref).
// For same-namespace deployments (agent and Fleet Server share a namespace) the active entries
// (CA and client cert) are still returned so that their content is hashed into AdditionalSecretsHash;
// copySecret detects the shared namespace and skips creating a copy while still writing the hash.
// Legacy copies (created by older ECK with the source secret's original name) are collected by
// GarbageCollectLegacyFleetServerAdditionalSecrets at startup.
func additionalSecrets(ctx context.Context, c k8s.Client, assoc commonv1.Association) ([]association.AdditionalSecret, error) {
	log := ulog.FromContext(ctx)
	fleetServerRef := assoc.AssociationRef()
	if !fleetServerRef.IsSet() {
		return nil, nil
	}
	esAssociation, err := getFleetServerESAssociation(ctx, c, fleetServerRef.NamespacedName())
	if err != nil {
		return nil, err
	}
	if esAssociation == nil {
		return nil, nil
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

	secrets := make([]association.AdditionalSecret, 0, 2)

	if conf.CACertProvided {
		log.V(1).Info("additional secret because CA provided")
		secrets = append(secrets, association.AdditionalSecret{
			Source: types.NamespacedName{Namespace: fleetServerRef.NamespacedName().Namespace, Name: conf.CASecretName},
			// Always restrict the copy to ca.crt. For external ES references, CASecretName ==
			// AuthSecretName, so the source secret also contains url/username/password — filtering
			// to ca.crt prevents credential leakage. For managed refs the CA secret also contains
			// tls.crt; agents only need ca.crt for TLS verification, so the narrower copy is correct
			// in both cases.
			Keys:       []string{certificates.CAFileName},
			TargetName: transitiveESCATargetName(assoc, conf),
		})
	}

	if conf.ClientCertIsConfigured() && esAssociation.AssociationRef().GetClientCertificateSecretName() != "" {
		log.V(1).Info("additional secret because user client certificate is provided")
		secrets = append(secrets, association.AdditionalSecret{
			Source:     types.NamespacedName{Namespace: fleetServerRef.NamespacedName().Namespace, Name: conf.GetClientCertSecretName()},
			TargetName: transitiveESClientCertTargetName(assoc, conf),
		})
	}

	return secrets, nil
}

// clientCertSecretName returns a deterministic secret name for a transitive client certificate.
// The name includes a hash of the referenced resource's namespace and name to avoid collisions.
func clientCertSecretName(associated commonv1.Associated, ref commonv1.AssociationRef, associationName string) string {
	associatedName := associated.GetName()
	refHash := hash.HashObject(ref.NamespacedName())
	return association.ClientCertNamer.Suffix(associatedName, associationName, refHash, "client-cert")
}

// transitiveSecretTargetName returns the name a transitive ES secret should have in the
// fleet-managed agent's namespace. For same-namespace deployments this is origName (the secret
// already lives in the shared namespace, no copy needed); for cross-namespace deployments it is a
// deterministic-hashed name that embeds the fleet server's NamespacedName to avoid collisions.
// Used by both additionalSecrets and fleetManagedAgentTransitiveESRef so the two code paths always
// agree on the target name.
func transitiveSecretTargetName(assoc commonv1.Association, origName, suffix string) string {
	agentNS := assoc.Associated().GetNamespace()
	refNS := assoc.AssociationRef().GetNamespace()
	switch refNS {
	case "":
		// Omitting namespace on the FleetServerRef means same namespace as the agent.
		return origName
	case agentNS:
		return origName
	}
	refHash := hash.HashObject(assoc.AssociationRef().NamespacedName())
	return association.AdditionalSecretNamer.Suffix(assoc.Associated().GetName(), "agent-fleetserver", refHash, suffix)
}

func transitiveESCATargetName(assoc commonv1.Association, conf *commonv1.AssociationConf) string {
	return transitiveSecretTargetName(assoc, conf.CASecretName, "es-ca")
}

func transitiveESClientCertTargetName(assoc commonv1.Association, conf *commonv1.AssociationConf) string {
	return transitiveSecretTargetName(assoc, conf.GetClientCertSecretName(), "es-client-cert")
}

// fleetManagedAgentTransitiveESRef resolves the transitive Elasticsearch association
// (Agent -> Fleet Server -> Elasticsearch) and reconciles a client certificate for the
// fleet-managed agent when the Elasticsearch requires client authentication.
// Orphaned client certificate secrets from previous ES references are cleaned up.
func fleetManagedAgentTransitiveESRef(ctx context.Context, c k8s.Client, assoc commonv1.Association, assocMeta metadata.Metadata) (*commonv1.TransitiveESRef, *reconciler.Results) {
	results := reconciler.NewResult(ctx)

	log := ulog.FromContext(ctx)
	associated := assoc.Associated()
	fleetServerRef := assoc.AssociationRef()
	if !fleetServerRef.IsSet() {
		if err := deleteOrphanedTransitiveESClientCertSecrets(ctx, c, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return nil, nil
	}
	esAssociation, err := getFleetServerESAssociation(ctx, c, fleetServerRef.NamespacedName())
	if err != nil {
		return nil, results.WithError(err)
	}
	if esAssociation == nil {
		if err := deleteOrphanedTransitiveESClientCertSecrets(ctx, c, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return nil, nil
	}
	conf, err := esAssociation.AssociationConf()
	if err != nil {
		log.V(1).Info("Transitive ES association conf not available")
		return nil, results.WithError(err)
	}
	if conf == nil {
		log.V(1).Info("Transitive ES association conf is nil")
		if err := deleteOrphanedTransitiveESClientCertSecrets(ctx, c, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return nil, nil
	}

	esAssocRef := esAssociation.AssociationRef()

	var caSecretName string
	if conf.CACertProvided {
		caSecretName = transitiveESCATargetName(assoc, conf)
	}

	// Compute the user-provided client cert name up front so the external-ES branch can include
	// it in the TransitiveESRef and preserve its copy in the agent namespace.
	var userClientCertName string
	if esAssociation.AssociationRef().GetClientCertificateSecretName() != "" && conf.ClientCertIsConfigured() {
		userClientCertName = transitiveESClientCertTargetName(assoc, conf)
	}

	if esAssocRef.IsExternal() {
		// For external ES references, the operator cannot determine whether client
		// authentication is required. Propagate the user-provided cert copy if present
		// but skip ECK-managed client cert reconciliation.
		if err := deleteOrphanedTransitiveESClientCertSecrets(ctx, c, associated, userClientCertName); err != nil {
			return nil, results.WithError(err)
		}
		return transitiveESRefOrNil(caSecretName, userClientCertName), nil
	}

	var es esv1.Elasticsearch
	if err := c.Get(ctx, esAssocRef.NamespacedName(), &es); err != nil {
		return nil, results.WithError(err)
	}

	if !annotation.HasClientAuthenticationRequired(&es) {
		if err := deleteOrphanedTransitiveESClientCertSecrets(ctx, c, associated, ""); err != nil {
			return nil, results.WithError(err)
		}
		return transitiveESRefOrNil(caSecretName, ""), nil
	}

	// If the Fleet Server has a user-provided client certificate for its ES association, reuse
	// it: for cross-namespace associations this is the per-agent copy created by
	// additionalSecrets/copySecret; for same-namespace associations it is the original secret.
	if userClientCertName != "" {
		// Pass userClientCertName so the user-provided copy is never deleted, even if it inherited the
		// selector's labels from the source secret.
		if err := deleteOrphanedTransitiveESClientCertSecrets(ctx, c, associated, userClientCertName); err != nil {
			return nil, results.WithError(err)
		}
		// The copy keeps the server soft-owner labels older operator versions copied from the Fleet Server client
		// certificate. They can point to a previously referenced ES, whose deletion would delete the copy.
		if err := removeServerSoftOwnerLabelsIfExists(ctx, c, types.NamespacedName{Namespace: associated.GetNamespace(), Name: userClientCertName}); err != nil {
			return nil, results.WithError(err)
		}
		return transitiveESRefOrNil(caSecretName, userClientCertName), results
	}

	// The transitive label tells this certificate apart from the agent's own Fleet Server client certificate,
	// which has the same association labels. The trust source published below inherits it.
	extraLabels := map[string]string{
		transitiveESClientCertLabel:       "true",
		labels.ClientCertificateLabelName: "true",
	}
	trustSourceMeta := assocMeta.Merge(metadata.Metadata{Labels: map[string]string{transitiveESClientCertLabel: "true"}})

	secretName := clientCertSecretName(associated, esAssocRef, "agent-es")

	clientCertSecret, certResults := association.ReconcileManagedClientCert(ctx, c, assoc, assocMeta, secretName, extraLabels)
	results.WithResults(certResults)
	if results.HasError() {
		return nil, results
	}

	// Older operator versions set server soft-owner labels, which are no longer used.
	if err := certificates.RemoveServerSoftOwnerLabels(ctx, c, &clientCertSecret.Secret); err != nil {
		return nil, results.WithError(err)
	}

	// Publish the public client certificate into the Elasticsearch namespace so it is included in its trust bundle.
	if err := certificates.ReconcileClientCertTrustSource(
		ctx, c, &es, k8s.ExtractNamespacedName(&clientCertSecret.Secret),
		clientCertSecret.CertPem(), trustSourceMeta,
	); err != nil {
		return nil, results.WithError(err)
	}

	if err := deleteOrphanedTransitiveESClientCertSecrets(ctx, c, associated, secretName); err != nil {
		return nil, results.WithError(err)
	}

	return transitiveESRefOrNil(caSecretName, secretName), results
}

// removeServerSoftOwnerLabelsIfExists removes the server soft-owner labels from the given secret, if it exists.
func removeServerSoftOwnerLabelsIfExists(ctx context.Context, c k8s.Client, nsn types.NamespacedName) error {
	secret, err := k8s.GetSecretIfExists(ctx, c, nsn)
	if err != nil || secret == nil {
		return err
	}
	return certificates.RemoveServerSoftOwnerLabels(ctx, c, secret)
}

// transitiveESRefOrNil returns a *TransitiveESRef populated with the provided names, or nil if
// both are empty (indicating no transitive ES state to propagate).
func transitiveESRefOrNil(caSecretName, clientCertSecretName string) *commonv1.TransitiveESRef {
	if caSecretName == "" && clientCertSecretName == "" {
		return nil
	}
	return &commonv1.TransitiveESRef{
		CASecretName:         caSecretName,
		ClientCertSecretName: clientCertSecretName,
	}
}

// deleteOrphanedTransitiveESClientCertSecrets deletes the transitive ES client cert secrets of the agent, and their
// trust sources, other than currentSecretName. Pass an empty string to delete all.
// They are selected with the labels identifying the agent only: the labels propagated from the agent, or the ones of
// its Fleet Server, can change along with the Elasticsearch reference.
func deleteOrphanedTransitiveESClientCertSecrets(ctx context.Context, c k8s.Client, associated commonv1.Associated, currentSecretName string) error {
	agentLabels := agentFleetServerAssociationLabels(k8s.ExtractNamespacedName(associated))

	transitiveSelector := make(client.MatchingLabels, len(agentLabels)+2)
	maps.Copy(transitiveSelector, agentLabels)
	transitiveSelector[labels.ClientCertificateLabelName] = "true"
	transitiveSelector[transitiveESClientCertLabel] = "true"

	// Transitive client certificates created by older operator versions have no transitive label: they are
	// told apart from the Fleet Server client certificate by the Elasticsearch kind of their server soft-owner
	// labels. They are migrated, and stop matching, once they are reconciled as the current certificate.
	// TODO: drop the legacy selector once the versions which set the soft-owner labels are out of support.
	legacySelector := make(client.MatchingLabels, len(agentLabels)+2)
	maps.Copy(legacySelector, agentLabels)
	legacySelector[labels.ClientCertificateLabelName] = "true"
	legacySelector[reconciler.SoftOwnerKindLabel] = esv1.Kind

	for _, selector := range []client.MatchingLabels{transitiveSelector, legacySelector} {
		var secrets corev1.SecretList
		if err := c.List(ctx, &secrets, client.InNamespace(associated.GetNamespace()), selector); err != nil {
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
	}

	// Keep only the trust source of currentSecretName. A user-provided client certificate is the Fleet Server's
	// own, already trusted through the Fleet Server's Elasticsearch association.
	trustSourceLabels := make(client.MatchingLabels, len(agentLabels)+1)
	maps.Copy(trustSourceLabels, agentLabels)
	trustSourceLabels[transitiveESClientCertLabel] = "true"
	trustSources, err := certificates.ListClientCertTrustSources(ctx, c, trustSourceLabels)
	if err != nil {
		return err
	}
	current := types.NamespacedName{Namespace: associated.GetNamespace(), Name: currentSecretName}
	for i := range trustSources {
		if origin, _ := certificates.ClientCertTrustSourceOrigin(trustSources[i]); origin == current {
			continue
		}
		if err := k8s.DeleteSecretIfExists(ctx, c, k8s.ExtractNamespacedName(&trustSources[i])); err != nil {
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
