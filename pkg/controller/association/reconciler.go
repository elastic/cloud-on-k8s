// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"

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
	// AssociatedObjTemplate builds an empty typed associated object (eg. &Kibana{}).
	AssociationObjTemplate func() commonv1.Association
	// ElasticsearchRef is a function which returns the maybe transitive Elasticsearch reference (eg. APMServer -> Kibana -> Elasticsearch).
	// In the case of a transitive reference this is used to create the Elasticsearch user.
	ElasticsearchRef func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error)
	// AssociatedNamer is used to build the name of the Secret which contains the CA of the target.
	AssociatedNamer name.Namer
	// ExternalServiceURL is used to build the external service url as it will be set in the resource configuration.
	ExternalServiceURL func(c k8s.Client, association commonv1.Association) (string, error)
	// AssociationName is the name of the association (eg. "kb-es").
	AssociationName string
	// AssociatedShortName is the short name of the associated resource type (eg. "kb").
	AssociatedShortName string
	// AssociationLabels are labels set on resources created for association purpose.
	AssociationLabels func(associated types.NamespacedName) map[string]string
	// UserSecretSuffix is used as a suffix in the name of the secret holding user data in the associated namespace.
	UserSecretSuffix string
	// CASecretLabelName is the name of the label added on the Secret that contains the CA of the remote service.
	CASecretLabelName string
	// ESUserRole is the role to use for the Elasticsearch user created by the association.
	ESUserRole func(commonv1.Associated) (string, error)
	// SetDynamicWatches allows to set some specific watches.
	SetDynamicWatches func(association commonv1.Association, watches watches.DynamicWatches) error
	// ClearDynamicWatches is called when the controller needs to clear the specific watches set for this association.
	ClearDynamicWatches func(associated types.NamespacedName, watches watches.DynamicWatches)
}

// userLabelSelector returns labels selecting the ES user secret, including association labels and user type label.
func (a AssociationInfo) userLabelSelector(associated types.NamespacedName) client.MatchingLabels {
	return maps.Merge(
		map[string]string{common.TypeLabelName: user.AssociatedUserType},
		a.AssociationLabels(associated),
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
func (r *Reconciler) log(associated commonv1.Associated) logr.Logger {
	return r.logger.WithValues(
		"namespace", associated.GetNamespace(),
		fmt.Sprintf("%s_name", r.AssociatedShortName), associated.GetName(),
	)
}

func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(r.logger, request, fmt.Sprintf("%s_name", r.AssociatedShortName), &r.iteration)()
	tx, ctx := tracing.NewTransaction(r.Tracer, request.NamespacedName, r.AssociationName)
	defer tracing.EndTransaction(tx)

	association := r.AssociationObjTemplate()
	if err := FetchWithAssociations(ctx, r.Client, request, association.Associated()); err != nil {
		if apierrors.IsNotFound(err) {
			// object resource has been deleted, remove artifacts related to the association.
			return reconcile.Result{}, r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(association) {
		r.log(association).Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	if !association.GetDeletionTimestamp().IsZero() {
		// Object is being deleted, short-circuit reconciliation
		return reconcile.Result{}, nil
	}

	if compatible, err := r.isCompatible(ctx, association.Associated()); err != nil || !compatible {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if err := annotation.UpdateControllerVersion(ctx, r.Client, association, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	results := reconciler.NewResult(ctx)
	newStatus, err := r.doReconcile(ctx, association)
	if err != nil {
		results.WithError(err)
	}

	// we want to attempt a status update even in the presence of errors
	if err := r.updateStatus(ctx, association, newStatus); err != nil {
		return defaultRequeue, tracing.CaptureError(ctx, err)
	}
	return results.
		WithResult(RequeueRbacCheck(r.accessReviewer)).
		WithResult(resultFromStatus(newStatus)).
		Aggregate()
}

func (r *Reconciler) doReconcile(ctx context.Context, association commonv1.Association) (commonv1.AssociationStatus, error) {
	assocKey := k8s.ExtractNamespacedName(association)
	assocLabels := r.AssociationLabels(assocKey)

	// retrieve the Elasticsearch resource, since it can be a transitive reference we need to use the provided ElasticsearchRef function
	associatedResourceFound, esRef, err := r.ElasticsearchRef(r.Client, association)
	if err != nil {
		return commonv1.AssociationFailed, err
	}

	// garbage collect leftover resources that are not required anymore
	if err := deleteOrphanedResources(ctx, r, esRef, association, assocLabels); err != nil {
		r.log(association).Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}

	associationRef := association.AssociationRef()
	if !associationRef.IsDefined() {
		// clean up watchers and remove artifacts related to the association
		if err := r.onDelete(assocKey); err != nil {
			return commonv1.AssociationFailed, err
		}
		// remove the configuration in the annotation, other leftover resources are already garbage-collected
		return commonv1.AssociationUnknown, RemoveAssociationConf(r.Client, association.Associated(), association.AssociationConfAnnotationName())
	}

	// set additional watches, in the case of a transitive Elasticsearch reference we must watch the intermediate resource
	if r.SetDynamicWatches != nil {
		if err := r.SetDynamicWatches(association, r.watches); err != nil {
			return commonv1.AssociationFailed, err
		}
	}

	// the associated resource does not exist yet, set status to Pending
	if !associatedResourceFound {
		return commonv1.AssociationPending, RemoveAssociationConf(r.Client, association.Associated(), association.AssociationConfAnnotationName())
	}

	es, associationStatus, err := r.getElasticsearch(ctx, association, esRef)
	if associationStatus != "" || err != nil {
		return associationStatus, err
	}

	// from this point we have checked that all the associated resources are set and have been found.

	// check if reference to Elasticsearch is allowed to be established
	if allowed, err := CheckAndUnbind(r.accessReviewer, association, &es, r, r.recorder); err != nil || !allowed {
		return commonv1.AssociationPending, err
	}

	// watch resources related to the referenced ES and the target service
	if err := r.setUserAndCaWatches(
		association,
		associationRef.NamespacedName(),
		esRef.NamespacedName(),
		r.AssociationInfo.AssociatedNamer,
	); err != nil {
		return commonv1.AssociationFailed, err
	}

	userRole, err := r.ESUserRole(association.Associated())
	if err != nil {
		return commonv1.AssociationFailed, err
	}

	if err := ReconcileEsUser(
		ctx,
		r.Client,
		association,
		assocLabels,
		userRole,
		r.UserSecretSuffix,
		es,
	); err != nil {
		return commonv1.AssociationPending, err
	}

	caSecret, err := r.ReconcileCASecret(
		association,
		r.AssociationInfo.AssociatedNamer,
		associationRef.NamespacedName(),
		r.CASecretLabelName)
	if err != nil {
		return commonv1.AssociationPending, err // maybe not created yet
	}

	url, err := r.AssociationInfo.ExternalServiceURL(r.Client, association)
	if err != nil {
		return commonv1.AssociationPending, err // maybe not created yet
	}

	// construct the expected association configuration
	authSecretRef := UserSecretKeySelector(association, r.UserSecretSuffix)
	expectedAssocConf := &commonv1.AssociationConf{
		AuthSecretName: authSecretRef.Name,
		AuthSecretKey:  authSecretRef.Key,
		CACertProvided: caSecret.CACertProvided,
		CASecretName:   caSecret.Name,
		URL:            url,
	}

	// update the association configuration if necessary
	return r.updateAssocConf(ctx, expectedAssocConf, association)
}

// isCompatible returns true if the given resource can be reconciled by the current controller.
func (r *Reconciler) isCompatible(ctx context.Context, associated commonv1.Associated) (bool, error) {
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, associated, r.AssociationLabels(k8s.ExtractNamespacedName(associated)), r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, associated, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
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
	err := r.Get(elasticsearchRef.NamespacedName(), &es)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, association, events.EventAssociationError,
			"Failed to find referenced backend %s: %v", elasticsearchRef.NamespacedName(), err)
		if apierrors.IsNotFound(err) {
			// ES is not found, remove any existing backend configuration and retry in a bit.
			if err := RemoveAssociationConf(r.Client, association.Associated(), association.AssociationConfAnnotationName()); err != nil && !apierrors.IsConflict(err) {
				r.log(association).Error(err, "Failed to remove Elasticsearch output from EnterpriseSearch object")
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
	if err := k8s.DeleteSecretMatching(r.Client, r.userLabelSelector(k8s.ExtractNamespacedName(association))); err != nil {
		return err
	}
	// Also remove the association configuration
	return RemoveAssociationConf(r.Client, association.Associated(), association.AssociationConfAnnotationName())
}

// updateAssocConf updates associated with the expected association conf.
func (r *Reconciler) updateAssocConf(
	ctx context.Context,
	expectedAssocConf *commonv1.AssociationConf,
	association commonv1.Association,
) (commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "update_assoc_conf", tracing.SpanTypeApp)
	defer span.End()

	if !reflect.DeepEqual(expectedAssocConf, association.AssociationConf()) {
		r.log(association).Info("Updating spec with Elasticsearch association configuration")
		if err := UpdateAssociationConf(r.Client, association.Associated(), expectedAssocConf, association.AssociationConfAnnotationName()); err != nil {
			if apierrors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			r.log(association).Error(err, "Failed to update EnterpriseSearch association configuration")
			return commonv1.AssociationPending, err
		}
		association.SetAssociationConf(expectedAssocConf)
	}
	return commonv1.AssociationEstablished, nil
}

// updateStatus updates the associated resource status.
func (r *Reconciler) updateStatus(ctx context.Context, association commonv1.Association, newStatus commonv1.AssociationStatus) error {
	span, _ := apm.StartSpan(ctx, "update_association_status", tracing.SpanTypeApp)
	defer span.End()

	oldStatus := association.AssociationStatus()
	if !reflect.DeepEqual(oldStatus, newStatus) {
		association.SetAssociationStatus(newStatus)
		if err := r.Status().Update(association.Associated()); err != nil {
			return err
		}
		r.recorder.AnnotatedEventf(
			association.Associated(),
			annotation.ForAssociationStatusChange(oldStatus, newStatus),
			corev1.EventTypeNormal,
			events.EventAssociationStatusChange,
			"Association status changed from [%s] to [%s]", oldStatus, newStatus)
	}
	return nil
}

func resultFromStatus(status commonv1.AssociationStatus) reconcile.Result {
	switch status {
	case commonv1.AssociationPending:
		return defaultRequeue // retry
	default:
		return reconcile.Result{} // we are done or there is not much we can do
	}
}

func (r *Reconciler) onDelete(associated types.NamespacedName) error {
	// remove dynamic watches
	if r.SetDynamicWatches != nil {
		r.ClearDynamicWatches(associated, r.watches)
	}
	r.removeWatches(associated)
	// delete user Secret in the Elasticsearch namespace
	return k8s.DeleteSecretMatching(r.Client, r.userLabelSelector(associated))
}
