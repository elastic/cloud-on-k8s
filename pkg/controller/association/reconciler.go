// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"fmt"
	"hash"
	"hash/fnv"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/v2/pkg/about"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

var (
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// AssociationInfo contains information specific to a particular associated resource (eg. Kibana, APMServer, etc.).
type AssociationInfo struct { //nolint:revive
	// AssociationType identifies the type of the resource for association (eg. kibana for APM to Kibana association,
	// elasticsearch for Beat to Elasticsearch association)
	AssociationType commonv1.AssociationType
	// AssociatedObjTemplate builds an empty typed associated object (eg. &Kibana{} for a Kibana to Elasticsearch association).
	AssociatedObjTemplate func() commonv1.Associated
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
	// an Agent referencing a Fleet Server.
	AdditionalSecrets func(context.Context, k8s.Client, commonv1.Association) ([]types.NamespacedName, error)
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
	// It may return an empty string if the version is unknown.
	ReferencedResourceVersion func(c k8s.Client, association commonv1.Association) (string, error)
	// AssociationResourceNameLabelName is a label used on resources needed for an association. It identifies the name
	// of the associated resource (eg. user secret allowing to connect Beat to Kibana will have this label pointing to the
	// Beat resource).
	AssociationResourceNameLabelName string
	// AssociationResourceNamespaceLabelName is a label used on resources needed for an association. It identifies the
	// namespace of the associated resource (eg. user secret allowing to connect Beat to Kibana will have this label
	// pointing to the Beat resource).
	AssociationResourceNamespaceLabelName string

	// ElasticsearchUserCreation specifies settings to create an Elasticsearch user as part of the association.
	// May be nil if no user creation is required.
	ElasticsearchUserCreation *ElasticsearchUserCreation
}

type ElasticsearchUserCreation struct {
	// ElasticsearchRef is a function which returns the maybe transitive Elasticsearch reference (eg. APMServer -> Kibana -> Elasticsearch).
	// In the case of a transitive reference this is used to create the Elasticsearch user.
	ElasticsearchRef func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error)
	// UserSecretSuffix is used as a suffix in the name of the secret holding user data in the associated namespace.
	UserSecretSuffix string
	// ESUserRole is the role to use for the Elasticsearch user created by the association.
	ESUserRole func(commonv1.Associated) (string, error)
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

// Reconciler reconciles a generic association for a specific AssociationInfo.
type Reconciler struct {
	AssociationInfo

	k8s.Client
	accessReviewer rbac.AccessReviewer
	recorder       record.EventRecorder
	watches        watches.DynamicWatches
	operator.Parameters
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

	// reconcile watches for all associations of this type
	if err := r.reconcileWatches(ctx, associatedKey, associations); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	results := reconciler.NewResult(ctx)
	newStatusMap := commonv1.AssociationStatusMap{}
	for _, association := range associations {
		newStatus, err := r.reconcileAssociation(ctx, association)
		if err != nil {
			results.WithError(err)
		}

		newStatusMap[association.AssociationRef().NamespacedName().String()] = newStatus
	}

	// we want to attempt a status update even in the presence of errors
	if err := r.updateStatus(ctx, associated, newStatusMap); err != nil && apierrors.IsConflict(err) {
		log.V(1).Info(
			"Conflict while updating status",
			"namespace", associatedKey.Namespace,
			"name", associatedKey.Name)
		return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
	} else if err != nil {
		return defaultRequeue, tracing.CaptureError(ctx, errors.Wrapf(err, "while updating status"))
	}
	return results.
		WithResult(RequeueRbacCheck(r.accessReviewer)).
		WithResult(resultFromStatuses(newStatusMap)).
		Aggregate()
}

func (r *Reconciler) reconcileAssociation(ctx context.Context, association commonv1.Association) (commonv1.AssociationStatus, error) {
	assocRef := association.AssociationRef()
	log := ulog.FromContext(ctx)

	// the referenced object can be an Elastic resource or a custom Secret
	referencedObj := r.ReferencedObjTemplate()
	if assocRef.IsExternal() {
		referencedObj = &corev1.Secret{}
	}

	// check if the referenced object exists
	exists, err := k8s.ObjectExists(r.Client, assocRef.NamespacedName(), referencedObj)
	if err != nil {
		return commonv1.AssociationFailed, err
	}
	if !exists {
		// the associated resource does not exist (yet), set status to Pending and remove the existing association conf
		return commonv1.AssociationPending, RemoveAssociationConf(ctx, r.Client, association)
	}

	if assocRef.IsExternal() {
		log.V(1).Info("Association with an unmanaged resource", "name", association.Associated().GetName(), "ref_name", assocRef.Name)
		// external reference, update association conf to associate the unmanaged resource
		expectedAssocConf, err := r.ExpectedConfigFromUnmanagedAssociation(association)
		if err != nil {
			r.recorder.Eventf(association.Associated(), corev1.EventTypeWarning, events.EventAssociationError, "Failed to reconcile external resource %q: %v", assocRef.NameOrSecretName(), err.Error())
			return commonv1.AssociationFailed, err
		}
		return r.updateAssocConf(ctx, &expectedAssocConf, association)
	}

	caSecret, err := r.ReconcileCASecret(
		ctx,
		association,
		r.AssociationInfo.ReferencedResourceNamer,
		assocRef.NamespacedName(),
	)
	if err != nil {
		return commonv1.AssociationPending, err // maybe not created yet
	}

	var secretsHash hash.Hash32
	if r.AdditionalSecrets != nil {
		secretsHash = fnv.New32a()
		additionalSecrets, err := r.AdditionalSecrets(ctx, r.Client, association)
		if err != nil {
			return commonv1.AssociationPending, err // maybe not created yet
		}
		for _, sec := range additionalSecrets {
			if err := copySecret(ctx, r.Client, secretsHash, association.GetNamespace(), sec); err != nil {
				return commonv1.AssociationPending, err
			}
		}
	}

	url, err := r.AssociationInfo.ExternalServiceURL(r.Client, association)
	if err != nil {
		// the Service may not have been created by the resource controller yet
		if apierrors.IsNotFound(err) {
			log.Info("Associated resource Service is not available yet", "error", err, "name", association.Associated().GetName(), "ref_name", assocRef.Name)
			return commonv1.AssociationPending, nil
		}
		return commonv1.AssociationPending, err
	}

	// propagate the currently running version of the referenced resource (example: Elasticsearch version).
	// The Kibana controller (for example) can then delay a Kibana version upgrade if Elasticsearch is not upgraded yet.
	ver, err := r.ReferencedResourceVersion(r.Client, association)
	if err != nil {
		return commonv1.AssociationPending, err
	}

	// construct the expected association configuration
	expectedAssocConf := &commonv1.AssociationConf{
		CACertProvided: caSecret.CACertProvided,
		CASecretName:   caSecret.Name,
		URL:            url,
		Version:        ver,
	}

	if secretsHash != nil {
		expectedAssocConf.AdditionalSecretsHash = fmt.Sprint(secretsHash.Sum32())
	}

	if r.ElasticsearchUserCreation == nil {
		// no user creation required, update the association conf as such
		expectedAssocConf.AuthSecretName = commonv1.NoAuthRequiredValue
		return r.updateAssocConf(ctx, expectedAssocConf, association)
	}

	// since Elasticsearch can be a transitive reference we need to use the provided ElasticsearchRef function
	found, esAssocRef, err := r.ElasticsearchUserCreation.ElasticsearchRef(r.Client, association)
	if err != nil {
		return commonv1.AssociationFailed, err
	}
	// the Elasticsearch ref does not exist yet, set status to Pending
	if !found {
		return commonv1.AssociationPending, RemoveAssociationConf(ctx, r.Client, association)
	}

	if esAssocRef.IsExternal() {
		log.V(1).Info("Association with a transitive unmanaged Elasticsearch, skip user creation",
			"name", association.Associated().GetName(), "ref_name", assocRef.Name, "es_ref_name", esAssocRef.Name)
		// this a transitive unmanaged Elasticsearch, no user creation, update the association conf as such
		expectedAssocConf.AuthSecretName = esAssocRef.SecretName
		expectedAssocConf.AuthSecretKey = authPasswordUnmanagedSecretKey
		return r.updateAssocConf(ctx, expectedAssocConf, association)
	}

	// retrieve the Elasticsearch resource
	es, associationStatus, err := r.getElasticsearch(ctx, association, esAssocRef)
	if associationStatus != "" || err != nil {
		return associationStatus, err
	}

	// check if reference to Elasticsearch is allowed to be established
	if allowed, err := CheckAndUnbind(ctx, r.accessReviewer, association, &es, r, r.recorder); err != nil || !allowed {
		return commonv1.AssociationPending, err
	}

	serviceAccount, err := association.ElasticServiceAccount()
	if err != nil {
		return commonv1.AssociationPending, err
	}
	// Detect if we should use a service account.
	var esHints hints.OrchestrationsHints
	if len(serviceAccount) > 0 {
		// We must first ensure that the relevant orchestration hint is set on the Elasticsearch cluster.
		esHints, err = hints.NewFrom(es)
		if err != nil {
			return commonv1.AssociationPending, err
		}
		if !esHints.ServiceAccounts.IsSet() {
			log.Info("Waiting for Elasticsearch to report if service accounts are fully rolled out")
			return commonv1.AssociationPending, nil
		}
	}

	// If it is the case create the related Secrets and update the association configuration on the associated resource.
	assocLabels := r.AssociationResourceLabels(k8s.ExtractNamespacedName(association.Associated()), assocRef.NamespacedName())
	if len(serviceAccount) > 0 && esHints.ServiceAccounts.IsTrue() {
		applicationSecretName := secretKey(association, r.ElasticsearchUserCreation.UserSecretSuffix)
		log.V(1).Info("Ensure service account exists", "sa", serviceAccount)
		err := ReconcileServiceAccounts(
			ctx,
			r.Client,
			es,
			assocLabels,
			applicationSecretName,
			UserKey(association, es.Namespace, r.ElasticsearchUserCreation.UserSecretSuffix),
			serviceAccount,
			association.GetName(),
			association.GetUID(),
		)
		if err != nil {
			return commonv1.AssociationFailed, err
		}
		expectedAssocConf.AuthSecretName = applicationSecretName.Name
		expectedAssocConf.AuthSecretKey = "token"
		expectedAssocConf.IsServiceAccount = true
		// update the association configuration if necessary
		return r.updateAssocConf(ctx, expectedAssocConf, association)
	}

	userRole, err := r.ElasticsearchUserCreation.ESUserRole(association.Associated())
	if err != nil {
		return commonv1.AssociationFailed, err
	}

	if err := reconcileEsUserSecret(
		ctx,
		r.Client,
		association,
		assocLabels,
		userRole,
		r.ElasticsearchUserCreation.UserSecretSuffix,
		es,
	); err != nil {
		return commonv1.AssociationPending, err
	}

	authSecretRef := UserSecretKeySelector(association, r.ElasticsearchUserCreation.UserSecretSuffix)
	expectedAssocConf.AuthSecretName = authSecretRef.Name
	expectedAssocConf.AuthSecretKey = authSecretRef.Key

	// update the association configuration if necessary
	return r.updateAssocConf(ctx, expectedAssocConf, association)
}

// getElasticsearch attempts to retrieve the referenced Elasticsearch resource. If not found, it removes
// any existing association configuration on associated, and returns AssociationPending.
func (r *Reconciler) getElasticsearch(
	ctx context.Context,
	association commonv1.Association,
	elasticsearchRef commonv1.ObjectSelector,
) (esv1.Elasticsearch, commonv1.AssociationStatus, error) {
	span, ctx := apm.StartSpan(ctx, "get_elasticsearch", tracing.SpanTypeApp)
	defer span.End()

	var es esv1.Elasticsearch
	err := r.Get(ctx, elasticsearchRef.NamespacedName(), &es)
	if err != nil {
		k8s.MaybeEmitErrorEvent(r.recorder, err, association, events.EventAssociationError,
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

// Unbind removes the association resources.
func (r *Reconciler) Unbind(ctx context.Context, association commonv1.Association) error {
	// Ensure that user in Elasticsearch is deleted to prevent illegitimate access
	if err := k8s.DeleteSecretMatching(
		ctx,
		r.Client,
		r.userLabelSelector(
			k8s.ExtractNamespacedName(association),
			association.AssociationRef().NamespacedName(),
		)); err != nil {
		return err
	}
	// Also remove the association configuration
	return RemoveAssociationConf(ctx, r.Client, association)
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
		annotations, err := annotation.ForAssociationStatusChange(oldStatus, newStatus)
		if err != nil {
			return err
		}
		r.recorder.AnnotatedEventf(
			associated,
			annotations,
			corev1.EventTypeNormal,
			events.EventAssociationStatusChange,
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

func (r *Reconciler) onDelete(ctx context.Context, associated types.NamespacedName) {
	// remove watches
	r.removeWatches(associated)

	// delete user Secret in the Elasticsearch namespace
	if err := deleteOrphanedResources(ctx, r.Client, r.AssociationInfo, associated, nil); err != nil {
		ulog.FromContext(ctx).Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}
}

// NewTestAssociationReconciler creates a new AssociationReconciler given an AssociationInfo for testing.
func NewTestAssociationReconciler(assocInfo AssociationInfo, runtimeObjs ...client.Object) Reconciler {
	return Reconciler{
		AssociationInfo: assocInfo,
		Client:          k8s.NewFakeClient(runtimeObjs...),
		accessReviewer:  rbac.NewPermissiveAccessReviewer(),
		watches:         watches.NewDynamicWatches(),
		recorder:        record.NewFakeRecorder(10),
		Parameters: operator.Parameters{
			OperatorInfo: about.OperatorInfo{
				BuildInfo: about.BuildInfo{
					Version: "1.5.0",
				},
			},
		},
	}
}
