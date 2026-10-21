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
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/rolemapping"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// roleMappingAssociationKey is the metadata key ECK sets to the association name on the role mappings it generates,
	// next to the ownership metadata described in commonv1.IsECKManagedPKIRoleMapping.
	roleMappingAssociationKey = "eck_association"

	// pkiFingerprintMetadataField and pkiDelegatedByRealmMetadataField are the user fields matched by the role mapping
	// rules. Elasticsearch sets both on users authenticated by any PKI realm, the latter only for delegated PKI.
	pkiFingerprintMetadataField      = "metadata.pki_public_key_fingerprint"
	pkiDelegatedByRealmMetadataField = "metadata.pki_delegated_by_realm"
)

// PKIEvaluations are the pending conflict-free evaluations of the request Secrets. They let associations switch to PKI,
// so ReconcilePKIRoleMappings returns them to be applied once the ECK-managed PKI realm runs on all the nodes and
// Elasticsearch applied the file-based settings holding the PKI role mappings.
type PKIEvaluations struct {
	updates []evaluationUpdate
	// fileSettingsVersion is the version of the file-based settings holding the PKI role mappings.
	fileSettingsVersion int64
}

// evaluationUpdate sets PKIEvaluationAnnotation of a request Secret to value, or removes it when value is empty.
type evaluationUpdate struct {
	namespace string
	name      string
	value     string
}

// Pending reports whether some request Secrets must be annotated with a conflict-free evaluation.
func (e PKIEvaluations) Pending() bool {
	return len(e.updates) > 0
}

// FileSettingsApplied reports whether Elasticsearch applied the file-based settings holding the PKI role mappings.
// A later version also holds them: the StackConfigPolicy controller preserves the ECK-generated PKI role mappings.
func (e PKIEvaluations) FileSettingsApplied(current esclient.FileSettings) bool {
	return current.Version >= e.fileSettingsVersion
}

// FileSettingsVersion returns the version of the file-based settings holding the PKI role mappings.
func (e PKIEvaluations) FileSettingsVersion() int64 {
	return e.fileSettingsVersion
}

// Apply annotates the request Secrets. Call it only once the ECK-managed PKI realm runs on all the nodes and
// Elasticsearch applied the file-based settings holding the PKI role mappings (see FileSettingsApplied). Patch errors
// are aggregated so the caller can retry.
func (e PKIEvaluations) Apply(ctx context.Context, c k8s.Client) error {
	return patchEvaluationAnnotations(ctx, c, e.updates)
}

// ReconcilePKIRoleMappings rebuilds the ECK-generated PKI role mappings in the file-settings Secret from the request
// Secrets, so mappings of deleted requests are removed. A mapping is withheld when its fingerprint is shared by
// requests asking for different roles, or when its name is used by a user-defined role mapping. Once the mappings are
// saved, each request Secret gets PKIEvaluationAnnotation, with the conflict if its mapping is withheld, and the
// association controller keeps the association Pending until the annotation matches the request, or while it reports
// a conflict.
// Conflicts and removals are annotated right away: they cannot switch an association to PKI, and keeping a previous
// conflict-free annotation would keep its association Established while its mapping is withheld. The conflict-free
// evaluations are returned instead, to be applied once the realm runs on all the nodes and Elasticsearch applied the
// file-based settings holding the mappings: the realm is a static setting, and until all the nodes load it and the
// mappings are applied an association switching to PKI must keep its credentials. Without the realm no user matches
// the mappings, so writing them early is harmless.
// No mapping is generated when the ECK-managed PKI realm is not configured (pkiRealmConfigured, see
// settings.PKIRealmConfigured), so the mappings are removed with the realm: their rules match the fingerprint from any
// PKI realm, so a user-defined PKI realm would still grant the roles. The annotations are removed then.
func ReconcilePKIRoleMappings(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	pkiRealmConfigured bool,
) (PKIEvaluations, error) {
	requests, err := rolemapping.RetrieveRequests(ctx, c, es)
	if err != nil {
		return PKIEvaluations{}, err
	}
	var pkiMappings *commonv1.Config
	var conflictMessages map[string]string
	if pkiRealmConfigured {
		pkiMappings, conflictMessages = buildPKIRoleMappings(ctx, requests)
	}

	meta := metadata.Propagate(&es, metadata.Metadata{Labels: eslabel.NewLabels(k8s.ExtractNamespacedName(&es))})
	esNsn := k8s.ExtractNamespacedName(&es)

	var nameCollisions map[string]string
	var fileSettingsVersion int64
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
			fileSettingsVersion, err = fs.persistedVersion()
			return err
		}
		if err := fs.SetPKIRoleMappings(desired); err != nil {
			return err
		}
		if err := fs.Save(ctx, c, &es, WithAdditiveMetadata()); err != nil {
			return err
		}
		fileSettingsVersion = fs.Version()
		return nil
	}); err != nil {
		return PKIEvaluations{}, err
	}

	immediate, conflictFree, err := evaluationUpdates(requests, pkiRealmConfigured, conflictMessages, nameCollisions)
	if err != nil {
		return PKIEvaluations{}, err
	}
	if err := patchEvaluationAnnotations(ctx, c, immediate); err != nil {
		return PKIEvaluations{}, err
	}
	return PKIEvaluations{updates: conflictFree, fileSettingsVersion: fileSettingsVersion}, nil
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

// evaluationUpdates returns the PKIEvaluationAnnotation updates of the request Secrets whose annotation does not match
// their evaluation: first the removals, when the PKI realm is not configured, and the conflicts, then the
// conflict-free evaluations. conflictMessages is keyed by public key fingerprint, nameCollisions by role mapping name.
func evaluationUpdates(
	requests []rolemapping.Request,
	pkiRealmConfigured bool,
	conflictMessages map[string]string,
	nameCollisions map[string]string,
) ([]evaluationUpdate, []evaluationUpdate, error) {
	var immediate, conflictFree []evaluationUpdate
	for _, req := range requests {
		var desired, conflict string
		if pkiRealmConfigured {
			conflict = conflictMessages[req.PublicKeyFingerprint]
			if conflict == "" {
				conflict = nameCollisions[pkiRoleMappingName(req)]
			}
			value, err := json.Marshal(rolemapping.Evaluation{
				PublicKeyFingerprint: req.PublicKeyFingerprint,
				Roles:                req.Roles,
				Conflict:             conflict,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("building PKI evaluation annotation for Secret %s/%s: %w", req.SecretNamespace, req.SecretName, err)
			}
			desired = string(value)
		}
		if req.EvaluationAnnotation == desired {
			continue // annotation already reflects the desired state
		}
		update := evaluationUpdate{namespace: req.SecretNamespace, name: req.SecretName, value: desired}
		if desired == "" || conflict != "" {
			immediate = append(immediate, update)
		} else {
			conflictFree = append(conflictFree, update)
		}
	}
	return immediate, conflictFree, nil
}

// patchEvaluationAnnotations applies the PKIEvaluationAnnotation updates to the request Secrets. Patch errors are
// aggregated so the caller can retry.
func patchEvaluationAnnotations(ctx context.Context, c k8s.Client, updates []evaluationUpdate) error {
	var errs []error
	for _, update := range updates {
		// annotationVal is nil (JSON null) when the value is empty, which removes the annotation.
		var annotationVal any
		if update.value != "" {
			annotationVal = update.value
		}
		patch, err := json.Marshal(map[string]any{
			"metadata": map[string]any{
				"annotations": map[string]any{rolemapping.PKIEvaluationAnnotation: annotationVal},
			},
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("building PKI evaluation annotation patch for Secret %s/%s: %w", update.namespace, update.name, err))
			continue
		}
		secret := &corev1.Secret{}
		secret.Namespace = update.namespace
		secret.Name = update.name
		if err := c.Patch(ctx, secret, client.RawPatch(types.MergePatchType, patch)); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("patching PKI evaluation annotation on Secret %s/%s: %w", update.namespace, update.name, err))
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
			commonv1.ECKPKIRoleMappingManagedByKey:   commonv1.ECKPKIRoleMappingManagedByECK,
			commonv1.ECKPKIRoleMappingResourceUIDKey: request.ResourceUID,
			roleMappingAssociationKey:                request.AssociationName,
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
