// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/rolemapping"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// roleMappingManagedByKey, roleMappingManagedByECK, roleMappingResourceUIDKey and roleMappingAssociationKey
	// are the metadata keys and value ECK sets on the role mappings it generates.
	roleMappingManagedByKey   = "managed_by"
	roleMappingManagedByECK   = "eck"
	roleMappingResourceUIDKey = "eck_resource_uid"
	roleMappingAssociationKey = "eck_association"

	// pkiFingerprintMetadataField and pkiDelegatedByRealmMetadataField are the user fields matched by the role mapping
	// rules. Elasticsearch sets both on users authenticated by any PKI realm, the latter only for delegated PKI.
	pkiFingerprintMetadataField      = "metadata.pki_public_key_fingerprint"
	pkiDelegatedByRealmMetadataField = "metadata.pki_delegated_by_realm"
)

// ReconcilePKIRoleMappings rebuilds the ECK-generated PKI role mappings in the file-settings Secret from the request
// Secrets, so mappings of deleted requests are removed. A mapping is withheld when its fingerprint is shared by
// requests asking for different roles, or when its name is used by a user-defined role mapping. Its request Secret
// then gets PKIConflictAnnotation, and the association controller keeps the association Pending.
// No mapping is generated when the ECK-managed PKI realm or mTLS is disabled, so the mappings are removed when PKI is
// disabled: their rules match the fingerprint from any PKI realm, so a user-defined PKI realm would still grant the roles.
func ReconcilePKIRoleMappings(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
) error {
	requests, err := rolemapping.RetrieveRequests(ctx, c, es)
	if err != nil {
		return err
	}
	var pkiMappings *commonv1.Config
	var conflictMessages map[string]string
	if es.Spec.Auth.PKI.Enabled && es.Spec.HTTP.TLS.Client.Authentication {
		pkiMappings, conflictMessages = buildPKIRoleMappings(ctx, requests)
	}

	meta := metadata.Propagate(&es, metadata.Metadata{Labels: eslabel.NewLabels(k8s.ExtractNamespacedName(&es))})
	esNsn := k8s.ExtractNamespacedName(&es)

	var nameCollisions map[string]string
	if err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		fs, err := Load(ctx, c, esNsn, meta)
		if err != nil {
			return err
		}
		var desired *commonv1.Config
		desired, nameCollisions = withholdNameCollisions(ctx, pkiMappings, fs.currentUserRoleMappings())
		// Skip write when the incoming mappings are identical to those already stored.
		// Covers both the no-PKI case (nil == nil) and the unchanged-entries case.
		if fs.pkiMappingsUnchanged(desired) {
			return nil
		}
		if err := fs.SetPKIRoleMappings(desired); err != nil {
			return err
		}
		return fs.Save(ctx, c, &es, WithAdditiveMetadata())
	}); err != nil {
		return err
	}

	return patchConflictAnnotations(ctx, c, requests, conflictMessages, nameCollisions)
}

// withholdNameCollisions drops the entries of pkiMappings whose name is already used by a user-defined role
// mapping, e.g. from a StackConfigPolicy created before the eck_managed_pki_ prefix was reserved. It returns the
// remaining mappings (nil if none) and a role mapping name→conflict-message map of the dropped entries.
func withholdNameCollisions(ctx context.Context, pkiMappings *commonv1.Config, userMappings map[string]any) (*commonv1.Config, map[string]string) {
	if pkiMappings == nil {
		return nil, nil
	}
	collisions := make(map[string]string)
	data := make(map[string]any, len(pkiMappings.Data))
	for name, mapping := range pkiMappings.Data {
		if _, exists := userMappings[name]; !exists {
			data[name] = mapping
			continue
		}
		collisions[name] = fmt.Sprintf(
			"the PKI role mapping %s generated for this association has the same name as a role mapping defined in a StackConfigPolicy. "+
				"Rename the role mapping in the StackConfigPolicy: names beginning with %q are reserved for ECK",
			name, commonv1.ECKPKIRoleMappingPrefix,
		)
		ulog.FromContext(ctx).V(1).Info("PKI role mapping withheld: name already used by a user-defined role mapping", "role_mapping", name)
	}
	if len(data) == 0 {
		return nil, collisions
	}
	return &commonv1.Config{Data: data}, collisions
}

// patchConflictAnnotations sets PKIConflictAnnotation on each request Secret whose role mapping is withheld, and
// removes it from the others. conflictMessages is keyed by public key fingerprint, nameCollisions by role mapping
// name. Patch errors are aggregated so the caller can retry.
func patchConflictAnnotations(
	ctx context.Context,
	c k8s.Client,
	requests []rolemapping.Request,
	conflictMessages map[string]string,
	nameCollisions map[string]string,
) error {
	var errs []error
	for _, req := range requests {
		desiredMsg := conflictMessages[req.PublicKeyFingerprint]
		if desiredMsg == "" {
			desiredMsg = nameCollisions[pkiRoleMappingName(req)]
		}
		if req.ConflictAnnotation == desiredMsg {
			continue // annotation already reflects the desired state
		}
		// annotationVal is nil (JSON null) when there is no conflict, which removes the annotation.
		var annotationVal any
		if desiredMsg != "" {
			annotationVal = desiredMsg
		}
		patch, err := json.Marshal(map[string]any{
			"metadata": map[string]any{
				"annotations": map[string]any{rolemapping.PKIConflictAnnotation: annotationVal},
			},
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("building PKI conflict annotation patch for Secret %s/%s: %w", req.SecretNamespace, req.SecretName, err))
			continue
		}
		secret := &corev1.Secret{}
		secret.Namespace = req.SecretNamespace
		secret.Name = req.SecretName
		if err := c.Patch(ctx, secret, client.RawPatch(types.MergePatchType, patch)); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("patching PKI conflict annotation on Secret %s/%s: %w", req.SecretNamespace, req.SecretName, err))
		}
	}
	return errors.Join(errs...)
}

// buildPKIRoleMappings returns one role mapping per request, bound to its public key fingerprint (see
// pkiRoleMappingRules). Requests without roles are skipped. Requests that share a fingerprint but ask
// for different roles are all withheld, as the identity is ambiguous. The second return value maps each conflicted
// fingerprint to its message.
func buildPKIRoleMappings(ctx context.Context, requests []rolemapping.Request) (*commonv1.Config, map[string]string) {
	if len(requests) == 0 {
		return nil, nil
	}
	log := ulog.FromContext(ctx)

	// First pass: find the fingerprints whose requests ask for different roles, so that all their requests are
	// withheld, whatever the request order.
	type fpState struct {
		roles    []string
		sources  []string
		conflict bool
	}
	// describe identifies the owner of a request, falling back to the request Secret when the associated
	// resource is unknown.
	describe := func(req rolemapping.Request, roles []string) string {
		owner := req.SecretNamespace + "/" + req.SecretName
		if req.ResourceName != "" {
			owner = fmt.Sprintf("%s/%s (association %s)", req.ResourceNamespace, req.ResourceName, req.AssociationName)
		}
		return fmt.Sprintf("%s requests roles [%s]", owner, strings.Join(roles, ", "))
	}
	byFingerprint := make(map[string]*fpState)
	for _, req := range requests {
		roles := normalizedPKIRoles(req.Roles)
		if len(roles) == 0 {
			continue
		}
		source := describe(req, roles)
		if s, seen := byFingerprint[req.PublicKeyFingerprint]; seen {
			s.sources = append(s.sources, source)
			if !slices.Equal(s.roles, roles) {
				s.conflict = true
			}
		} else {
			byFingerprint[req.PublicKeyFingerprint] = &fpState{roles: roles, sources: []string{source}}
		}
	}

	// Build one message per conflicted fingerprint.
	conflictMessages := make(map[string]string)
	for fp, s := range byFingerprint {
		if !s.conflict {
			continue
		}
		slices.Sort(s.sources)
		conflictMessages[fp] = fmt.Sprintf(
			"the client certificate with public key fingerprint %s is shared by associations that request different roles: %s. "+
				"Use a distinct client certificate (elasticsearchRef.clientCertificateSecretName) for each of them",
			fp, strings.Join(s.sources, "; "),
		)
		log.V(1).Info("PKI role mappings withheld: requests for the same key fingerprint disagree on roles",
			"fingerprint", fp, "sources", s.sources)
	}

	data := make(map[string]any)
	for _, request := range requests {
		roleNames := normalizedPKIRoles(request.Roles)
		if len(roleNames) == 0 {
			log.V(1).Info("PKI role mapping skipped: empty roles", "request", request.SecretNamespace+"/"+request.SecretName)
			continue
		}
		if _, conflicted := conflictMessages[request.PublicKeyFingerprint]; conflicted {
			continue
		}

		roles := make([]any, len(roleNames))
		for i := range roleNames {
			roles[i] = roleNames[i]
		}

		meta := map[string]any{
			roleMappingManagedByKey:   roleMappingManagedByECK,
			roleMappingResourceUIDKey: request.ResourceUID,
			roleMappingAssociationKey: request.AssociationName,
		}

		data[pkiRoleMappingName(request)] = map[string]any{
			"enabled":  true,
			"roles":    roles,
			"rules":    pkiRoleMappingRules(request.PublicKeyFingerprint),
			"metadata": meta,
		}
	}

	if len(data) == 0 {
		return nil, conflictMessages
	}
	return &commonv1.Config{Data: data}, conflictMessages
}

// pkiRoleMappingRules returns the rules of a generated role mapping: users authenticated by any PKI realm, through a
// TLS connection, with a certificate whose public key has the given fingerprint.
//
// The rules do not name the eck_managed_pki realm. It is last in the realm chain, so a user-defined PKI realm that
// trusts the certificate authenticates the component first, and the chain stops there. Binding to the key alone still
// grants the component its roles. Delegated PKI is excluded: there, Elasticsearch does not see the client prove it
// holds the private key.
//
// The values use the types JSON decoding produces, so that rules read back from the file-settings Secret compare
// equal to newly built ones.
func pkiRoleMappingRules(fingerprint string) map[string]any {
	return map[string]any{
		"all": []any{
			map[string]any{"field": map[string]any{pkiFingerprintMetadataField: fingerprint}},
			map[string]any{"except": map[string]any{"field": map[string]any{pkiDelegatedByRealmMetadataField: "*"}}},
		},
	}
}

// pkiRoleMappingName returns the role mapping name generated for request, suffixed with the request Secret
// namespace and name. All requests of a cluster live in the Elasticsearch namespace, so names are unique
// because request Secret names are.
func pkiRoleMappingName(request rolemapping.Request) string {
	return commonv1.ECKPKIRoleMappingPrefix + request.SecretNamespace + "." + request.SecretName
}

// normalizedPKIRoles returns a sorted set of nonempty role names for comparison and output.
func normalizedPKIRoles(roles string) []string {
	var names []string
	for role := range strings.SplitSeq(roles, ",") {
		if role = strings.TrimSpace(role); role != "" {
			names = append(names, role)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}
