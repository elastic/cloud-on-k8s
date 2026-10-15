// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"reflect"
	"strings"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/v3/pkg/about"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/rolemapping"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

var defaultRequeue = reconcile.Result{RequeueAfter: reconciler.DefaultRequeue}

// AdditionalSecretNamer is a Namer for transitive Elasticsearch secrets (CA copies and client
// certificate copies) placed in fleet-managed agent namespaces. Secrets use DNS subdomain naming
// (253 chars max) and can therefore be longer than label-constrained resource names.
var AdditionalSecretNamer = name.NewSecretNamer()

// AdditionalSecret describes one secret that should be copied into the associated resource's namespace.
// The reconciler copies every descriptor returned by the AdditionalSecrets callback and GCs any stale
// copies (those that are no longer returned) automatically via deleteStaleAdditionalSecrets.
type AdditionalSecret struct {
	// Source is the namespace/name of the secret to copy from (in the referenced resource's namespace).
	Source types.NamespacedName
	// Keys is an optional list of data keys to include in the copy; all keys are copied when empty.
	Keys []string
	// TargetName is the name to give the copy in the associated resource's namespace.
	// Defaults to Source.Name when empty.
	TargetName string
}

// AssociationInfo contains information specific to a particular associated resource (eg. Kibana, APMServer, etc.).
type AssociationInfo struct { //nolint:revive
	// AssociationType identifies the type of the resource for association (eg. kibana for APM to Kibana association,
	// elasticsearch for Beat to Elasticsearch association)
	AssociationType commonv1.AssociationType
	// AssociatedObjTemplate builds an empty typed associated object (eg. &Kibana{} for a Kibana to Elasticsearch association).
	AssociatedObjTemplate func() commonv1.Associated
	// AssociatedObjListTemplate builds an empty typed list of associated objects (e.g. &KibanaList{}).
	// Used by WatchNamespaceFlips to re-enqueue all associated objects when a namespace state change to match.
	AssociatedObjListTemplate func() client.ObjectList
	// ReferencedObjTemplate builds an empty referenced object (e.g. Elasticsearch{} for a Kibana to Elasticsearch association).
	ReferencedObjTemplate func() client.Object
	// ReferencedResourceNamer is used to build the name of the Secret which contains the CA of the referenced resource
	// (the Elasticsearch Namer for a Kibana to Elasticsearch association).
	ReferencedResourceNamer name.Namer
	// ExternalServiceURL is used to build the external service url as it will be set in the resource configuration.
	ExternalServiceURL func(c k8s.Client, association commonv1.Association) (string, error)
	// AssociationName is the name of the association (eg. "kb-es").
	AssociationName string
	// AssociatedShortName is the short name of the associated resource type (eg. "kb").
	AssociatedShortName string

	// AdditionalSecrets are additional secrets to copy from an association's namespace to the associated resource namespace.
	// Currently this is only used for copying the CA from an Elasticsearch association to the same namespace as
	// an Agent referencing a Fleet Server. See AdditionalSecret.Keys for per-secret key filtering.
	AdditionalSecrets func(context.Context, k8s.Client, commonv1.Association) ([]AdditionalSecret, error)
	// Labels are labels set on all resources created for association purpose. Note that some resources will be also
	// labelled with AssociationResourceNameLabelName and AssociationResourceNamespaceLabelName in addition to any
	// labels provided here.
	Labels func(associated types.NamespacedName) map[string]string
	// AssociationConfAnnotationNameBase is prefix of the name of the annotation used to define the config for the
	// associated resource. The annotation is used by the association controller to store the configuration and by
	// the controller which is managing the associated resource to build the appropriate configuration. The annotation
	// base is used to recognize annotations eligible for removal when association is removed.
	AssociationConfAnnotationNameBase string
	// ReferencedResourceVersion returns the currently running version of the referenced resource.
	// It may return an empty string if the version is unknown. A boolean is also returned, set to true if the referenced
	// resource is a serverless project running in https://cloud.elastic.co/
	ReferencedResourceVersion func(c k8s.Client, association commonv1.Association) (string, bool, error)
	// AssociationResourceNameLabelName is a label used on resources needed for an association. It identifies the name
	// of the associated resource (eg. user secret allowing to connect Beat to Kibana will have this label pointing to the
	// Beat resource).
	AssociationResourceNameLabelName string
	// AssociationResourceNamespaceLabelName is a label used on resources needed for an association. It identifies the
	// namespace of the associated resource (eg. user secret allowing to connect Beat to Kibana will have this label
	// pointing to the Beat resource).
	AssociationResourceNamespaceLabelName string

	// ReconcileTransitiveESSecrets is an optional callback invoked during association reconciliation to manage
	// secrets related to a transitive Elasticsearch reference (e.g. Agent -> Fleet Server -> Elasticsearch).
	// It receives the association metadata for label propagation and returns a TransitiveESRef (stored in the
	// association conf annotation) along with reconciler results for requeue scheduling.
	ReconcileTransitiveESSecrets func(context.Context, k8s.Client, commonv1.Association, metadata.Metadata) (*commonv1.TransitiveESRef, *reconciler.Results)

	// ElasticsearchRef resolves the (possibly transitive) Elasticsearch reference for this association.
	// When set, RBAC is checked against the resolved ES before any side-effecting operations.
	// Must be set whenever ElasticsearchUserCreation is non-nil. May also be set without
	// ElasticsearchUserCreation to apply the transitive ES RBAC check for associations that
	// copy ES-sourced secrets without creating a dedicated ES user (e.g. Agent -> Fleet Server -> Elasticsearch).
	ElasticsearchRef func(ctx context.Context, c k8s.Client, assoc commonv1.Association) (bool, commonv1.AssociationRef, error)

	// ElasticsearchUserCreation specifies settings to create an Elasticsearch user as part of the association.
	// May be nil if no user creation is required.
	ElasticsearchUserCreation *ElasticsearchUserCreation
}

type ElasticsearchUserCreation struct {
	// UserSecretSuffix is used as a suffix in the name of the secret holding user data in the associated namespace.
	UserSecretSuffix string
	// ESUserRole is the role to use for the Elasticsearch user created by the association.
	ESUserRole func(commonv1.Associated) (string, error)
	// SupportsAuthentication reports whether the associated resource supports the given authentication mode.
	// Nil means every mode is supported.
	SupportsAuthentication func(obj commonv1.Associated, mode commonv1.AuthenticationMode) bool
}

// AssociationResourceLabels returns all labels required by a resource to allow identifying both its Associated resource
// (ie. Kibana for Kibana-ES association) and its Association resource (ie. ES for APM-ES association).
func (a AssociationInfo) AssociationResourceLabels(
	associated types.NamespacedName,
	association types.NamespacedName,
) client.MatchingLabels {
	return maps.Merge(
		map[string]string{
			a.AssociationResourceNameLabelName:      association.Name,
			a.AssociationResourceNamespaceLabelName: association.Namespace,
		},
		a.Labels(associated),
	)
}

// userLabelSelector returns labels selecting the ES user secret, including association labels and user type label.
func (a AssociationInfo) userLabelSelector(
	associated types.NamespacedName,
	association types.NamespacedName,
) client.MatchingLabels {
	return maps.Merge(
		map[string]string{commonv1.TypeLabelName: user.AssociatedUserType},
		a.AssociationResourceLabels(associated, association),
	)
}

// serviceAccountTokenLabelSelector returns labels selecting ES-side service-account-token secrets for this association.
func (a AssociationInfo) serviceAccountTokenLabelSelector(
	associated types.NamespacedName,
	association types.NamespacedName,
) client.MatchingLabels {
	return maps.Merge(
		map[string]string{commonv1.TypeLabelName: user.ServiceAccountTokenType},
		a.AssociationResourceLabels(associated, association),
	)
}

// pkiRoleMappingRequestLabelSelector returns labels selecting Elasticsearch-side PKI role mapping
// request Secrets for this association.
func (a AssociationInfo) pkiRoleMappingRequestLabelSelector(
	associated types.NamespacedName,
	association types.NamespacedName,
) client.MatchingLabels {
	return maps.Merge(
		map[string]string{commonv1.TypeLabelName: rolemapping.PKIRoleMappingRequestType},
		a.AssociationResourceLabels(associated, association),
	)
}

// Reconciler reconciles a generic association for a specific AssociationInfo.
type Reconciler struct {
	AssociationInfo

	k8s.Client
	accessReviewer rbac.AccessReviewer
	recorder       toolsevents.EventRecorder
	watches        watches.DynamicWatches
	operator.Parameters
	// referencedResourceKind is the CRD Kind of the referenced resource (e.g. "Elasticsearch"),
	// derived from the scheme at controller setup time via ReferencedObjTemplate.
	referencedResourceKind string
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	nameField := fmt.Sprintf("%s_name", r.AssociatedShortName)
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, r.AssociationName, nameField, request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)
	log := ulog.FromContext(ctx)

	associated := r.AssociatedObjTemplate()
	if err := r.Client.Get(ctx, request.NamespacedName, associated); err != nil {
		if apierrors.IsNotFound(err) {
			// object resource has been deleted, remove artifacts related to the association.
			r.onDelete(ctx, types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	associatedKey := k8s.ExtractNamespacedName(associated)

	if common.IsUnmanaged(ctx, associated) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	if !associated.GetDeletionTimestamp().IsZero() {
		// Object is being deleted, short-circuit reconciliation
		return reconcile.Result{}, nil
	}

	if err := RemoveObsoleteAssociationConfs(ctx, r.Client, associated, r.AssociationConfAnnotationNameBase); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// we are only interested in associations of the same target type here
	// (e.g. Kibana -> Enterprise Search, not Kibana -> Elasticsearch)
	associations := make([]commonv1.Association, 0)
	for _, association := range associated.GetAssociations() {
		if association.AssociationType() == r.AssociationType {
			associations = append(associations, association)
		}
	}

	// garbage collect leftover resources that are not required anymore
	if err := deleteOrphanedResources(ctx, r.Client, r.AssociationInfo, associatedKey, associations); err != nil {
		log.Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}

	results := reconciler.NewResult(ctx)
	// Watch setup can depend on transitive association configuration. Keep reconciling
	// if it fails so a denied reference can still be unbound on this pass.
	results.WithError(r.reconcileWatches(ctx, associatedKey, associations))
	newStatusMap := commonv1.AssociationStatusMap{}
	for _, association := range associations {
		newStatus, assocResults := r.reconcileAssociation(ctx, association)
		results.WithResults(assocResults)
		newStatusMap[association.AssociationRef().NamespacedName().String()] = newStatus
	}

	// we want to attempt a status update even in the presence of errors
	if err := r.updateStatus(ctx, associated, newStatusMap); err != nil && apierrors.IsConflict(err) {
		log.V(1).Info(
			"Conflict while updating status",
			"namespace", associatedKey.Namespace,
			"name", associatedKey.Name)
		return results.WithRequeue().Aggregate()
	} else if err != nil {
		return defaultRequeue, tracing.CaptureError(ctx, fmt.Errorf("while updating status: %w", err))
	}
	return results.
		WithResult(RequeueRbacCheck(r.accessReviewer)).
		WithResult(resultFromStatuses(newStatusMap)).
		Aggregate()
}

func (r *Reconciler) reconcileAssociation(ctx context.Context, association commonv1.Association) (commonv1.AssociationStatus, *reconciler.Results) {
	assocRef := association.AssociationRef()
	log := ulog.FromContext(ctx)
	results := reconciler.NewResult(ctx)

	// the referenced object can be an Elastic resource or a custom Secret
	referencedObj := r.ReferencedObjTemplate()
	if assocRef.IsExternal() {
		referencedObj = &corev1.Secret{}
	}

	// check if the referenced object exists
	exists, err := k8s.ObjectExists(r.Client, assocRef.NamespacedName(), referencedObj)
	if err != nil {
		return commonv1.AssociationFailed, results.WithError(err)
	}
	if !exists {
		// the associated resource does not exist (yet), set status to Pending and remove the existing association conf
		return commonv1.AssociationPending, results.WithError(RemoveAssociationConf(ctx, r.Client, association))
	}

	if assocRef.IsExternal() {
		log.V(1).Info("Association with an unmanaged resource", "name", association.Associated().GetName(), "ref_name", assocRef.NameOrSecretName())
		// external reference, update association conf to associate the unmanaged resource
		expectedAssocConf, err := r.ExpectedConfigFromUnmanagedAssociation(association)
		if err != nil {
			k8s.EmitEventf(r.recorder, association.Associated(), corev1.EventTypeWarning, events.EventAssociationError, events.EventActionAssociationReconciliation, "Failed to reconcile external resource %q: %v", assocRef.NameOrSecretName(), err.Error())
			return commonv1.AssociationFailed, results.WithError(err)
		}
		status, err := r.updateAssocConf(ctx, &expectedAssocConf, association)
		return status, results.WithError(err)
	}

	var directAssociationsUnbinder Unbinder
	enforcementAllAssoc := r.Parameters.RBACOnRefsMode.EnforcementAllAssociations()
	if enforcementAllAssoc {
		directAssociationsUnbinder = r
	}
	// Check RBAC for the directly-referenced object before calling resolveESAndCheckRBAC.
	// resolveESAndCheckRBAC can return early if the transitive ES is missing or in error.
	// That early return is unrelated to the direct association. Without this check first,
	// a denied direct reference may never be unbound.
	// Skip this check when the direct reference is an Elasticsearch. For a direct ES association,
	// the transitive target is the same object. resolveESAndCheckRBAC always checks it.
	// If both run, the function emits a duplicate warning.
	_, refObjIsES := referencedObj.(*esv1.Elasticsearch)
	if !refObjIsES {
		if allowed, err := CheckAndUnbind(ctx, r.accessReviewer, association, referencedObj,
			directAssociationsUnbinder, r.recorder); err != nil || (!allowed && enforcementAllAssoc) {
			return commonv1.AssociationPending, results.WithError(err)
		}
	}

	// Resolve the (possibly transitive) ES reference and check RBAC before any side-effecting operations.
	// esAssocRef is nil in two cases: ElasticsearchRef is not configured at all, or ElasticsearchRef returns
	// !found with ElasticsearchUserCreation==nil (e.g. Fleet Server in manual-setup mode with no ES ref).
	// The ElasticsearchUserCreation==nil guard below prevents any dereference in either path.
	esAssocRef, es, assocStatus, err := r.resolveESAndCheckRBAC(ctx, association)
	if assocStatus != "" || err != nil {
		return assocStatus, results.WithError(err)
	}

	// metadata to propagate to children
	assocMeta := metadata.Propagate(association, metadata.Metadata{Labels: r.AssociationResourceLabels(k8s.ExtractNamespacedName(association), association.AssociationRef().NamespacedName())})
	caSecret, err := r.ReconcileCASecret(
		ctx,
		association,
		r.AssociationInfo.ReferencedResourceNamer,
		assocRef.NamespacedName(),
		assocMeta,
	)
	if err != nil {
		return commonv1.AssociationPending, results.WithError(err) // maybe not created yet
	}

	secretsHash, err := r.reconcileAdditionalSecrets(ctx, association)
	if err != nil {
		return commonv1.AssociationPending, results.WithError(err)
	}

	url, err := r.AssociationInfo.ExternalServiceURL(r.Client, association)
	if err != nil {
		// the Service may not have been created by the resource controller yet
		if apierrors.IsNotFound(err) {
			log.Info("Associated resource Service is not available yet", "error", err, "name", association.Associated().GetName(), "ref_name", assocRef.NameOrSecretName())
			return commonv1.AssociationPending, results
		}
		return commonv1.AssociationPending, results.WithError(err)
	}

	// propagate the currently running version of the referenced resource (example: Elasticsearch version).
	// The Kibana controller (for example) can then delay a Kibana version upgrade if Elasticsearch is not upgraded yet.
	ver, isServerless, err := r.ReferencedResourceVersion(r.Client, association)
	if err != nil {
		return commonv1.AssociationPending, results.WithError(err)
	}

	// Reconcile or clean up client certificate depending on whether client authentication is required.
	hasClientAuthenticationRequired := annotation.HasClientAuthenticationRequired(referencedObj)
	var clientCertSecret *certificates.CertificatesSecret
	var clientCertSecretName string
	if hasClientAuthenticationRequired {
		var certResults *reconciler.Results
		clientCertSecret, certResults = r.reconcileClientCertificate(ctx, association, assocMeta)
		results.WithResults(certResults)
		if certResults.HasError() {
			return commonv1.AssociationPending, results
		}
		clientCertSecretName = clientCertSecret.Name
	} else {
		if err := DeleteClientCertSecret(ctx, r.Client, association, r.AssociationName); err != nil {
			return commonv1.AssociationPending, results.WithError(err)
		}
	}

	// construct the expected association configuration
	expectedAssocConf := &commonv1.AssociationConf{
		CACertProvided:       caSecret.CACertProvided,
		CASecretName:         caSecret.Name,
		URL:                  url,
		Version:              ver,
		Serverless:           isServerless,
		ClientCertSecretName: clientCertSecretName,
	}

	if r.ReconcileTransitiveESSecrets != nil {
		transitiveES, ret := r.ReconcileTransitiveESSecrets(ctx, r.Client, association, assocMeta)
		results.WithResults(ret)
		if ret.HasError() {
			return commonv1.AssociationPending, results
		}
		expectedAssocConf.TransitiveESRef = transitiveES
	}

	if secretsHash != nil {
		expectedAssocConf.AdditionalSecretsHash = fmt.Sprint(secretsHash.Sum32())
	}

	if r.ElasticsearchUserCreation == nil {
		// no user creation required, update the association conf as such
		expectedAssocConf.AuthSecretName = commonv1.NoAuthRequiredValue
		status, err := r.updateAssocConf(ctx, expectedAssocConf, association)
		return status, results.WithError(err)
	}

	if esAssocRef.IsExternal() {
		log.V(1).Info("Association with a transitive unmanaged Elasticsearch, skip user creation",
			"name", association.Associated().GetName(), "ref_name", assocRef.NameOrSecretName(), "es_ref_name", esAssocRef.NameOrSecretName())
		// this a transitive unmanaged Elasticsearch, no user creation, update the association conf as such
		expectedAssocConf.AuthSecretName = esAssocRef.GetSecretName()
		expectedAssocConf.AuthSecretKey = authPasswordUnmanagedSecretKey
		status, err := r.updateAssocConf(ctx, expectedAssocConf, association)
		return status, results.WithError(err)
	}

	authMode := assocRef.GetAuthenticationMode()
	ac, assocStatus, staleParams, err := r.reconcileAuth(ctx, association, es, assocMeta, authMode, hasClientAuthenticationRequired, clientCertSecret, ver)
	if assocStatus != "" || err != nil {
		return assocStatus, results.WithError(err)
	}
	expectedAssocConf.AuthSecretName = ac.secretName
	expectedAssocConf.AuthSecretKey = ac.secretKey
	expectedAssocConf.IsServiceAccount = ac.isServiceAccount

	// Delete stale auth artifacts only once the new conf is committed and the association is established.
	status, err := r.updateAssocConf(ctx, expectedAssocConf, association)
	if status == commonv1.AssociationEstablished && err == nil {
		if delErr := r.deleteStaleAuthRequestSecrets(ctx, association, staleParams); delErr != nil {
			return commonv1.AssociationPending, results.WithError(delErr)
		}
	}
	return status, results.WithError(err)
}

// authConf holds the authentication fields to be merged into AssociationConf after the auth mode is resolved.
type authConf struct {
	secretName       string
	secretKey        string
	isServiceAccount bool
}

// reconcileAuth runs the auth reconciler for the requested mode. It does not delete the Secrets of the other modes:
// it returns which ones are stale, so the caller can delete them once the new conf is committed. A non-empty status
// means the caller must stop and return it.
func (r *Reconciler) reconcileAuth(
	ctx context.Context,
	association commonv1.Association,
	es esv1.Elasticsearch,
	assocMeta metadata.Metadata,
	authMode commonv1.AuthenticationMode,
	hasClientAuthenticationRequired bool,
	clientCertSecret *certificates.CertificatesSecret,
	ver string,
) (authConf, commonv1.AssociationStatus, deleteStaleAuthRequestParams, error) {
	// r.ElasticsearchUserCreation is non-nil here: the nil guard in reconcileAssociation returns early
	// for association types that don't require user creation.
	userRole, err := r.ElasticsearchUserCreation.ESUserRole(association.Associated())
	if err != nil {
		return authConf{}, commonv1.AssociationFailed, deleteStaleAuthRequestParams{}, err
	}

	if r.ElasticsearchUserCreation.SupportsAuthentication != nil &&
		!r.ElasticsearchUserCreation.SupportsAuthentication(association.Associated(), authMode) {
		k8s.EmitEventf(r.recorder, association.Associated(), corev1.EventTypeWarning,
			events.EventAssociationError, events.EventActionAssociationConfiguration,
			"authentication mode %q is not supported for this resource type", authMode)
		return authConf{}, commonv1.AssociationPending, deleteStaleAuthRequestParams{}, nil
	}

	serviceAccount, err := association.ElasticServiceAccount()
	if err != nil {
		return authConf{}, commonv1.AssociationPending, deleteStaleAuthRequestParams{}, err
	}
	log := ulog.FromContext(ctx)
	var (
		ac                 authConf
		status             commonv1.AssociationStatus
		staleRequestParams deleteStaleAuthRequestParams
	)
	switch {
	case authMode == commonv1.AuthenticationModePKI:
		ac, status, err = r.reconcilePKIAuth(ctx, association, es, assocMeta, hasClientAuthenticationRequired, clientCertSecret, ver, userRole)
		staleRequestParams = deleteStaleAuthRequestParams{ServiceAccount: true, UserESSecrets: true, UserAppSecret: true}
	case len(serviceAccount) > 0:
		// We must first ensure that the relevant orchestration hint is set on the Elasticsearch cluster.
		esHints, hintsErr := hints.NewFrom(es)
		if hintsErr != nil {
			return authConf{}, commonv1.AssociationPending, deleteStaleAuthRequestParams{}, hintsErr
		}
		if !esHints.ServiceAccounts.IsSet() {
			log.Info("Waiting for Elasticsearch to report if service accounts are fully rolled out")
			return authConf{}, commonv1.AssociationPending, deleteStaleAuthRequestParams{}, nil
		}

		if esHints.ServiceAccounts.IsTrue() {
			ac, status, err = r.reconcileServiceAccountAuth(ctx, association, es, assocMeta, serviceAccount)
			// see deleteStaleAuthRequestParams.UserAppSecret
			staleRequestParams = deleteStaleAuthRequestParams{PKI: true, UserESSecrets: true}
		} else {
			ac, status, err = r.reconcilePlainCredentialsAuth(ctx, association, es, assocMeta, userRole)
			staleRequestParams = deleteStaleAuthRequestParams{PKI: true, ServiceAccount: true}
		}
	default:
		ac, status, err = r.reconcilePlainCredentialsAuth(ctx, association, es, assocMeta, userRole)
		staleRequestParams = deleteStaleAuthRequestParams{PKI: true, ServiceAccount: true}
	}

	return ac, status, staleRequestParams, err
}

// reconcilePKIAuth handles PKI mode: validates preconditions, writes the role mapping request, and
// returns the no-auth sentinel so the component skips credential-based authentication.
// Note: clientCertSecret must be non-nil when hasClientAuthenticationRequired is true.
func (r *Reconciler) reconcilePKIAuth(
	ctx context.Context,
	association commonv1.Association,
	es esv1.Elasticsearch,
	assocMeta metadata.Metadata,
	hasClientAuthenticationRequired bool,
	clientCertSecret *certificates.CertificatesSecret,
	ver string,
	userRole string,
) (authConf, commonv1.AssociationStatus, error) {
	// PKI authentication cannot work without mTLS being enabled on the Elasticsearch cluster.
	if !hasClientAuthenticationRequired {
		k8s.EmitEventf(r.recorder, association.Associated(), corev1.EventTypeWarning,
			events.EventAssociationError, events.EventActionAssociationConfiguration,
			"PKI authentication requires mTLS on the Elasticsearch cluster (spec.http.tls.client.authentication: true)")
		return authConf{}, commonv1.AssociationPending, nil
	}

	// The ECK-managed PKI realm is opt-in on the Elasticsearch side.
	if !es.Spec.Auth.PKI.Enabled {
		k8s.EmitEventf(r.recorder, association.Associated(), corev1.EventTypeWarning,
			events.EventAssociationError, events.EventActionAssociationConfiguration,
			"PKI authentication requires the PKI realm to be enabled on Elasticsearch %s/%s (spec.auth.pki.enabled: true)", es.Namespace, es.Name)
		return authConf{}, commonv1.AssociationPending, nil
	}

	// PKI authentication requires PKIAuthMinVersion. An unparseable version, such as the empty status version of a
	// new cluster, also leaves the association Pending.
	esVer, _ := version.Parse(ver)
	if !esVer.GTE(commonv1.PKIAuthMinVersion) {
		k8s.EmitEventf(r.recorder, association.Associated(), corev1.EventTypeWarning,
			events.EventAssociationError, events.EventActionAssociationConfiguration,
			"PKI authentication requires Elasticsearch %s or higher (current: %q)", version.WithoutPre(commonv1.PKIAuthMinVersion), ver)
		return authConf{}, commonv1.AssociationPending, nil
	}

	clientCert, err := certificates.GetPrimaryCertificate(clientCertSecret.CertPem())
	if err != nil {
		return authConf{}, commonv1.AssociationPending, fmt.Errorf("failed to parse client certificate: %w", err)
	}

	// The PKI realm takes the username from the Subject CN (default username_pattern), so authentication fails
	// without one, for example with SAN-only certificates. Only user-provided certificates can lack a CN: ECK always
	// sets it.
	if strings.TrimSpace(clientCert.Subject.CommonName) == "" {
		certSecretName := association.AssociationRef().GetClientCertificateSecretName()
		if certSecretName == "" {
			certSecretName = clientCertSecret.Name
		}
		k8s.EmitEventf(r.recorder, association.Associated(), corev1.EventTypeWarning,
			events.EventAssociationError, events.EventActionAssociationConfiguration,
			"PKI authentication requires the client certificate in Secret %s/%s to have a non-empty Subject Common Name, from which Elasticsearch derives the PKI username",
			association.GetNamespace(), certSecretName)
		return authConf{}, commonv1.AssociationPending, nil
	}

	evaluated, conflictMsg, err := reconcilePKIRoleMappingRequest(ctx, r.Client, association, assocMeta, r.AssociationName, clientCert, userRole, es)
	if err != nil {
		return authConf{}, commonv1.AssociationPending, err
	}
	// Until the ES driver has evaluated the current request, its role mapping may not be written, or the ECK-managed PKI
	// realm may not run on all the nodes yet. Stay Pending, keeping the committed conf and its credentials: the driver
	// annotates the request Secret, which triggers a reconciliation through the request Secret watch.
	if !evaluated {
		// The event is Normal: this is expected on every new request or client certificate rotation, and during the
		// rolling restart that loads the realm.
		k8s.EmitEventf(r.recorder, association.Associated(), corev1.EventTypeNormal,
			events.EventAssociationStatusChange, events.EventActionAssociationConfiguration,
			"Waiting for Elasticsearch %s/%s to evaluate the PKI role mapping request. It is evaluated once Elasticsearch is "+
				"reachable and the ECK-managed PKI realm runs on all its nodes, see the Elasticsearch status and events",
			es.Namespace, es.Name)
		return authConf{}, commonv1.AssociationPending, nil
	}
	if conflictMsg != "" {
		k8s.EmitEventf(r.recorder, association.Associated(), corev1.EventTypeWarning,
			events.EventAssociationError, events.EventActionAssociationConfiguration,
			"PKI role mapping conflict: %s", conflictMsg)
		return authConf{}, commonv1.AssociationPending, nil
	}

	return authConf{secretName: commonv1.NoAuthRequiredValue}, "", nil
}

// reconcileServiceAccountAuth creates or updates the Elasticsearch service-account token secret.
// The caller must confirm the SA orchestration hint is set and true before calling.
func (r *Reconciler) reconcileServiceAccountAuth(
	ctx context.Context,
	association commonv1.Association,
	es esv1.Elasticsearch,
	assocMeta metadata.Metadata,
	serviceAccount commonv1.ServiceAccountName,
) (authConf, commonv1.AssociationStatus, error) {
	log := ulog.FromContext(ctx)

	applicationSecretName := secretKey(association, r.ElasticsearchUserCreation.UserSecretSuffix)
	log.V(1).Info("Ensure service account exists", "sa", serviceAccount)
	if err := ReconcileServiceAccounts(
		ctx,
		r.Client,
		es,
		assocMeta,
		applicationSecretName,
		UserKey(association, es.Namespace, r.ElasticsearchUserCreation.UserSecretSuffix),
		serviceAccount,
		association.Associated(),
	); err != nil {
		return authConf{}, commonv1.AssociationFailed, err
	}

	return authConf{secretName: applicationSecretName.Name, secretKey: ServiceAccountTokenValueField, isServiceAccount: true}, "", nil
}

// reconcilePlainCredentialsAuth creates or updates the basic-auth user secret for the association.
func (r *Reconciler) reconcilePlainCredentialsAuth(
	ctx context.Context,
	association commonv1.Association,
	es esv1.Elasticsearch,
	assocMeta metadata.Metadata,
	userRole string,
) (authConf, commonv1.AssociationStatus, error) {
	if err := reconcileEsUserSecret(
		ctx,
		r.Client,
		association,
		assocMeta,
		userRole,
		r.ElasticsearchUserCreation.UserSecretSuffix,
		es,
		r.Parameters.PasswordGenerator,
	); err != nil {
		return authConf{}, commonv1.AssociationPending, err
	}
	authSecretRef := UserSecretKeySelector(association, r.ElasticsearchUserCreation.UserSecretSuffix)
	return authConf{secretName: authSecretRef.Name, secretKey: authSecretRef.Key}, "", nil
}

// getElasticsearch attempts to retrieve the referenced Elasticsearch resource. If not found, it removes
// any existing association configuration on associated, and returns AssociationPending.
func (r *Reconciler) getElasticsearch(
	ctx context.Context,
	association commonv1.Association,
	elasticsearchRef commonv1.AssociationRef,
) (esv1.Elasticsearch, commonv1.AssociationStatus, error) {
	span, ctx := apm.StartSpan(ctx, "get_elasticsearch", tracing.SpanTypeApp)
	defer span.End()

	var es esv1.Elasticsearch
	err := r.Get(ctx, elasticsearchRef.NamespacedName(), &es)
	if err != nil {
		k8s.MaybeEmitErrorEventf(r.recorder, err, association, events.EventAssociationError, events.EventActionElasticsearchRetrieval,
			"Failed to find referenced backend %s: %v", elasticsearchRef.NamespacedName(), err)
		if apierrors.IsNotFound(err) {
			// ES is not found, remove any existing backend configuration and retry in a bit.
			if err := RemoveAssociationConf(ctx, r.Client, association); err != nil && !apierrors.IsConflict(err) {
				ulog.FromContext(ctx).Error(err, "Failed to remove Elasticsearch association configuration")
				return esv1.Elasticsearch{}, commonv1.AssociationPending, err
			}
			return esv1.Elasticsearch{}, commonv1.AssociationPending, nil
		}
		return esv1.Elasticsearch{}, commonv1.AssociationFailed, err
	}
	return es, "", nil
}

type deleteStaleAuthRequestParams struct {
	// PKI deletes the PKI role mapping request Secret in the ES namespace.
	PKI bool
	// ServiceAccount deletes the ES-side service-account-token Secrets via label selector.
	ServiceAccount bool
	// UserESSecrets deletes ES-side user Secrets via label selector.
	UserESSecrets bool
	// UserAppSecret deletes the application-side auth Secret by name.
	// Must not be set in SA mode: the SA token is stored under the same name.
	UserAppSecret bool
}

// deleteStaleAuthRequestSecrets deletes the auth Secrets selected by p.
func (r *Reconciler) deleteStaleAuthRequestSecrets(ctx context.Context, association commonv1.Association, p deleteStaleAuthRequestParams) error {
	associated := k8s.ExtractNamespacedName(association)
	assocRef := association.AssociationRef().NamespacedName()
	var err error
	if p.PKI {
		err = errors.Join(err, k8s.DeleteSecretMatching(ctx, r.Client, r.pkiRoleMappingRequestLabelSelector(associated, assocRef)))
	}
	if p.ServiceAccount {
		err = errors.Join(err, k8s.DeleteSecretMatching(ctx, r.Client, r.serviceAccountTokenLabelSelector(associated, assocRef)))
	}
	if p.UserESSecrets {
		err = errors.Join(err, k8s.DeleteSecretMatching(ctx, r.Client, r.userLabelSelector(associated, assocRef)))
	}
	if p.UserAppSecret && r.ElasticsearchUserCreation != nil {
		err = errors.Join(err, k8s.DeleteSecretIfExists(ctx, r.Client, secretKey(association, r.ElasticsearchUserCreation.UserSecretSuffix)))
	}
	return err
}

// deleteAllAuthRequestSecrets deletes every auth-related secret for the association regardless of mode.
func (r *Reconciler) deleteAllAuthRequestSecrets(ctx context.Context, association commonv1.Association) error {
	return r.deleteStaleAuthRequestSecrets(ctx, association, deleteStaleAuthRequestParams{PKI: true, ServiceAccount: true, UserESSecrets: true, UserAppSecret: true})
}

// resolveESAndCheckRBAC finds the Elasticsearch reference for the association.
// The reference may be direct or transitive. The function checks RBAC against the resolved ES.
// It returns the resolved reference and ES object for the caller.
// A non-empty status or a non-nil error means the caller must return immediately.
func (r *Reconciler) resolveESAndCheckRBAC(ctx context.Context, association commonv1.Association) (commonv1.AssociationRef, esv1.Elasticsearch, commonv1.AssociationStatus, error) {
	if r.ElasticsearchRef == nil {
		return nil, esv1.Elasticsearch{}, "", nil
	}
	found, esAssocRef, err := r.ElasticsearchRef(ctx, r.Client, association)
	if err != nil {
		return nil, esv1.Elasticsearch{}, commonv1.AssociationFailed, err
	}
	if !found {
		if r.ElasticsearchUserCreation != nil {
			// The transitive ES is not yet established. Without it the function cannot create the ES user.
			return nil, esv1.Elasticsearch{}, commonv1.AssociationPending, RemoveAssociationConf(ctx, r.Client, association)
		}
		// No ES reference is set. No user creation is required.
		// Example: Fleet Server in manual-setup mode with no ES reference.
		// Skip the transitive RBAC check.
		return nil, esv1.Elasticsearch{}, "", nil
	}
	if esAssocRef.IsExternal() {
		return esAssocRef, esv1.Elasticsearch{}, "", nil
	}
	es, status, err := r.getElasticsearch(ctx, association, esAssocRef)
	if status != "" || err != nil {
		return nil, esv1.Elasticsearch{}, status, err
	}
	if allowed, err := CheckAndUnbind(ctx, r.accessReviewer, association, &es, r, r.recorder); err != nil || !allowed {
		return nil, esv1.Elasticsearch{}, commonv1.AssociationPending, err
	}
	return esAssocRef, es, "", nil
}

// deleteAllAssociationSecrets deletes every secret in the associated resource's namespace that
// carries the association's resource labels: the CA copy, additional-secret copies, and any
// transitive ES client cert secrets.
func (r *Reconciler) deleteAllAssociationSecrets(ctx context.Context, association commonv1.Association) error {
	assocLabels := r.AssociationResourceLabels(k8s.ExtractNamespacedName(association), association.AssociationRef().NamespacedName())
	var secrets corev1.SecretList
	if err := r.List(ctx, &secrets, client.InNamespace(association.GetNamespace()), assocLabels); err != nil {
		return err
	}
	for i := range secrets.Items {
		if err := k8s.DeleteSecretIfExists(ctx, r.Client, k8s.ExtractNamespacedName(&secrets.Items[i])); err != nil {
			return err
		}
	}
	return nil
}

// Unbind removes the association resources.
func (r *Reconciler) Unbind(ctx context.Context, association commonv1.Association) error {
	if err := r.deleteAllAuthRequestSecrets(ctx, association); err != nil {
		return err
	}

	// Delete all association-labeled secrets in the associated resource's namespace: CA copy,
	// additional-secret copies, and any transitive ES client cert secrets.
	if err := r.deleteAllAssociationSecrets(ctx, association); err != nil {
		return err
	}

	// Also remove the association configuration
	return RemoveAssociationConf(ctx, r.Client, association)
}

// reconcileAdditionalSecrets copies secrets required by the association into the associated
// resource's namespace, hashes their content, and GCs any stale copies from previous reconciles.
// Returns nil, nil when AdditionalSecrets is not configured for this association type.
// If AdditionalSecrets returns an error the stale GC is intentionally skipped: existing copies
// remain until the referenced resource is reachable again or Kubernetes GC removes them via the
// owner references set on each copy.
func (r *Reconciler) reconcileAdditionalSecrets(ctx context.Context, association commonv1.Association) (hash.Hash32, error) {
	if r.AdditionalSecrets == nil {
		return nil, nil
	}
	secretsHash := fnv.New32a()
	additionalSecrets, err := r.AdditionalSecrets(ctx, r.Client, association)
	if err != nil {
		return nil, err
	}
	assocLabels := r.AssociationResourceLabels(k8s.ExtractNamespacedName(association), association.AssociationRef().NamespacedName())
	copiedNames := sets.New[string]()
	for _, sec := range additionalSecrets {
		destName := sec.TargetName
		if destName == "" {
			destName = sec.Source.Name
		}
		copied, err := copySecret(ctx, r.Client, secretsHash, association.GetNamespace(), sec.Source, sec.Keys, destName, association.Associated(), assocLabels)
		if err != nil {
			return nil, err
		}
		if copied {
			copiedNames.Insert(destName)
		}
	}
	if err := deleteStaleAdditionalSecrets(ctx, r.Client, association.GetNamespace(), assocLabels, copiedNames); err != nil {
		return nil, err
	}
	return secretsHash, nil
}

// updateAssocConf updates associated with the expected association conf.
func (r *Reconciler) updateAssocConf(
	ctx context.Context,
	expectedAssocConf *commonv1.AssociationConf,
	association commonv1.Association,
) (commonv1.AssociationStatus, error) {
	span, ctx := apm.StartSpan(ctx, "update_assoc_conf", tracing.SpanTypeApp)
	defer span.End()
	log := ulog.FromContext(ctx)

	assocConf, err := association.AssociationConf()
	if err != nil {
		return "", err
	}
	if !reflect.DeepEqual(expectedAssocConf, assocConf) {
		log.Info("Updating association configuration")
		if err := UpdateAssociationConf(ctx, r.Client, association, expectedAssocConf); err != nil {
			if apierrors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			log.Error(err, "Failed to update association configuration")
			return commonv1.AssociationPending, err
		}
		association.SetAssociationConf(expectedAssocConf)
	}
	return commonv1.AssociationEstablished, nil
}

// updateStatus updates the associated resource status.
func (r *Reconciler) updateStatus(ctx context.Context, associated commonv1.Associated, newStatus commonv1.AssociationStatusMap) error {
	span, _ := apm.StartSpan(ctx, "update_association_status", tracing.SpanTypeApp)
	defer span.End()

	oldStatus := associated.AssociationStatusMap(r.AssociationType)

	// To correctly compare statuses without making the reconciler aware of singleton vs multiple associations status
	// differences we: set new status, get it from associated and only then compare with the oldStatus. Setting the
	// same status is harmless, setting a different status is fine as we have a copy of oldStatus above.
	if err := associated.SetAssociationStatusMap(r.AssociationType, newStatus); err != nil {
		return err
	}
	newStatus = associated.AssociationStatusMap(r.AssociationType)

	// shortcut if the two maps are nil or empty
	if len(oldStatus) == 0 && len(newStatus) == 0 {
		return nil
	}
	if !reflect.DeepEqual(oldStatus, newStatus) {
		if err := r.Status().Update(ctx, associated); err != nil {
			return err
		}
		k8s.EmitEventf(
			r.recorder,
			associated,
			corev1.EventTypeNormal,
			events.EventAssociationStatusChange,
			events.EventActionStatusUpdate,
			"Association status changed from [%s] to [%s]", oldStatus, newStatus)
	}
	return nil
}

func resultFromStatuses(statusMap commonv1.AssociationStatusMap) reconcile.Result {
	for _, status := range statusMap {
		if status == commonv1.AssociationPending {
			return defaultRequeue // retry
		}
	}

	return reconcile.Result{} // we are done or there is not much we can do
}

// OnNamespaceOutOfScope releases all controller-local state associated with the given associated
// resource when its namespace no longer matches the operator's namespace selector.
func (r *Reconciler) OnNamespaceOutOfScope(associated types.NamespacedName) {
	r.removeWatches(associated)
}

func (r *Reconciler) onDelete(ctx context.Context, associated types.NamespacedName) {
	r.OnNamespaceOutOfScope(associated)

	// delete user Secret in the Elasticsearch namespace
	if err := deleteOrphanedResources(ctx, r.Client, r.AssociationInfo, associated, nil); err != nil {
		ulog.FromContext(ctx).Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}
}

// NewTestAssociationReconciler creates a new AssociationReconciler given an AssociationInfo for testing.
func NewTestAssociationReconciler(assocInfo AssociationInfo, runtimeObjs ...client.Object) Reconciler {
	return NewTestAssociationReconcilerWithReviewer(assocInfo, rbac.NewPermissiveAccessReviewer(), operator.RBACOnRefsModeOff, runtimeObjs...)
}

func NewTestAssociationReconcilerWithReviewer(assocInfo AssociationInfo, reviewer rbac.AccessReviewer, rbacOnRefsMode operator.RBACOnRefsMode, runtimeObjs ...client.Object) Reconciler {
	return Reconciler{
		AssociationInfo: assocInfo,
		Client:          k8s.NewFakeClient(runtimeObjs...),
		accessReviewer:  reviewer,
		watches:         watches.NewDynamicWatches(),
		recorder:        toolsevents.NewFakeRecorder(10),
		RBACOnRefsMode:  rbacOnRefsMode,
		OperatorInfo: about.OperatorInfo{
			BuildInfo: about.BuildInfo{
				Version: "1.5.0",
			},
		},
	}
}
