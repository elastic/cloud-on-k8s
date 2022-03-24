// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

var (
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// AssociationInfo contains information specific to a particular associated resource (eg. Kibana, APMServer, etc.).
type AssociationInfo struct {
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
	ReferencedResourceVersion func(c k8s.Client, referencedRes types.NamespacedName) (string, error)
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
		map[string]string{common.TypeLabelName: user.AssociatedUserType},
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

	logger logr.Logger
}

// log with namespace and name fields set for the given association resource.
func (r *Reconciler) log(associatedNsName types.NamespacedName) logr.Logger {
	return r.logger.WithValues(
		"namespace", associatedNsName.Namespace,
		fmt.Sprintf("%s_name", r.AssociatedShortName), associatedNsName.Name,
	)
}

func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(r.logger, request, fmt.Sprintf("%s_name", r.AssociatedShortName), &r.iteration)()
	tx, ctx := tracing.NewTransaction(ctx, r.Tracer, request.NamespacedName, r.AssociationName)
	defer tracing.EndTransaction(tx)

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

	if common.IsUnmanaged(associated) {
		r.log(associatedKey).Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	if !associated.GetDeletionTimestamp().IsZero() {
		// Object is being deleted, short-circuit reconciliation
		return reconcile.Result{}, nil
	}

	if err := RemoveObsoleteAssociationConfs(r.Client, associated, r.AssociationConfAnnotationNameBase); err != nil {
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
		r.log(associatedKey).Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}

	// reconcile watches for all associations of this type
	if err := r.reconcileWatches(associatedKey, associations); err != nil {
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
	if err := r.updateStatus(ctx, associated, newStatusMap); err != nil {
		if apierrors.IsConflict(err) {
			log.V(1).Info("Conflict while updating status")
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		return defaultRequeue, tracing.CaptureError(ctx, err)
	}
	return results.
		WithResult(RequeueRbacCheck(r.accessReviewer)).
		WithResult(resultFromStatuses(newStatusMap)).
		Aggregate()
}

func (r *Reconciler) reconcileAssociation(ctx context.Context, association commonv1.Association) (commonv1.AssociationStatus, error) {
	exists, err := k8s.ObjectExists(r.Client, association.AssociationRef().NamespacedName(), r.ReferencedObjTemplate())
	if err != nil {
		return commonv1.AssociationFailed, err
	}
	if !exists {
		// the associated resource does not exist (yet), set status to Pending and remove the existing association conf
		return commonv1.AssociationPending, RemoveAssociationConf(r.Client, association)
	}

	associationRef := association.AssociationRef()
	assocLabels := r.AssociationResourceLabels(k8s.ExtractNamespacedName(association.Associated()), association.AssociationRef().NamespacedName())

	caSecret, err := r.ReconcileCASecret(
		association,
		r.AssociationInfo.ReferencedResourceNamer,
		associationRef.NamespacedName(),
	)
	if err != nil {
		return commonv1.AssociationPending, err // maybe not created yet
	}

	url, err := r.AssociationInfo.ExternalServiceURL(r.Client, association)
	if err != nil {
		return commonv1.AssociationPending, err // maybe not created yet
	}

	// Propagate the currently running version of the referenced resource (example: Elasticsearch version).
	// The Kibana controller (for example) can then delay a Kibana version upgrade if Elasticsearch is not upgraded yet.
	ver, err := r.ReferencedResourceVersion(r.Client, associationRef.NamespacedName())
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

	if r.ElasticsearchUserCreation == nil {
		// no user creation required, update the association conf as such
		expectedAssocConf.AuthSecretName = commonv1.NoAuthRequiredValue
		return r.updateAssocConf(ctx, expectedAssocConf, association)
	}

	// retrieve the Elasticsearch resource
	// since it can be a transitive reference we need to use the provided ElasticsearchRef function
	found, esRef, err := r.ElasticsearchUserCreation.ElasticsearchRef(r.Client, association)
	if err != nil {
		return commonv1.AssociationFailed, err
	}
	// the Elasticsearch resource does not exist yet, set status to Pending
	if !found {
		return commonv1.AssociationPending, RemoveAssociationConf(r.Client, association)
	}

	es, associationStatus, err := r.getElasticsearch(ctx, association, esRef)
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
	// Detect if we should use a service account. If it is the case create the related Secrets and update the association
	// configuration on the associated resource.
	if len(serviceAccount) > 0 {
		applicationSecretName := secretKey(association, r.ElasticsearchUserCreation.UserSecretSuffix)
		r.log(k8s.ExtractNamespacedName(association)).V(1).Info("Ensure service account exists", "sa", serviceAccount)
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
	span, _ := apm.StartSpan(ctx, "get_elasticsearch", tracing.SpanTypeApp)
	defer span.End()

	var es esv1.Elasticsearch
	err := r.Get(context.Background(), elasticsearchRef.NamespacedName(), &es)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, association, events.EventAssociationError,
			"Failed to find referenced backend %s: %v", elasticsearchRef.NamespacedName(), err)
		if apierrors.IsNotFound(err) {
			// ES is not found, remove any existing backend configuration and retry in a bit.
			if err := RemoveAssociationConf(r.Client, association); err != nil && !apierrors.IsConflict(err) {
				r.log(k8s.ExtractNamespacedName(association)).Error(err, "Failed to remove Elasticsearch association configuration")
				return esv1.Elasticsearch{}, commonv1.AssociationPending, err
			}
			return esv1.Elasticsearch{}, commonv1.AssociationPending, nil
		}
		return esv1.Elasticsearch{}, commonv1.AssociationFailed, err
	}
	return es, "", nil
}

// Unbind removes the association resources.
func (r *Reconciler) Unbind(association commonv1.Association) error {
	// Ensure that user in Elasticsearch is deleted to prevent illegitimate access
	if err := k8s.DeleteSecretMatching(
		r.Client,
		r.userLabelSelector(
			k8s.ExtractNamespacedName(association),
			association.AssociationRef().NamespacedName(),
		)); err != nil {
		return err
	}
	// Also remove the association configuration
	return RemoveAssociationConf(r.Client, association)
}

// updateAssocConf updates associated with the expected association conf.
func (r *Reconciler) updateAssocConf(
	ctx context.Context,
	expectedAssocConf *commonv1.AssociationConf,
	association commonv1.Association,
) (commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "update_assoc_conf", tracing.SpanTypeApp)
	defer span.End()

	assocConf, err := association.AssociationConf()
	if err != nil {
		return "", err
	}
	if !reflect.DeepEqual(expectedAssocConf, assocConf) {
		r.log(k8s.ExtractNamespacedName(association)).Info("Updating association configuration")
		if err := UpdateAssociationConf(r.Client, association, expectedAssocConf); err != nil {
			if apierrors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			r.log(k8s.ExtractNamespacedName(association)).Error(err, "Failed to update association configuration")
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
		r.log(associated).Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}
}

// NewTestAssociationReconciler creates a new AssociationReconciler given an AssociationInfo for testing.
func NewTestAssociationReconciler(assocInfo AssociationInfo, runtimeObjs ...runtime.Object) Reconciler {
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
		logger: log.WithName("test"),
	}
}
