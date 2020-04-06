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
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
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
	AssociatedObjTemplate func() commonv1.Associated
	// AssociationName is the name of the association (eg. "kb-es").
	AssociationName string
	// AssociatedShortName is the short name of the associated resource type (eg. "kb").
	AssociatedShortName string
	// AssociationLabels are labels set on resources created for association purpose.
	AssociationLabels func(associated types.NamespacedName) map[string]string
	// UserSecretSuffix is used as a suffix in the name of the secret holding user data in the associated namespace.
	UserSecretSuffix string
	// ESUserRole is the role to use for the Elasticsearch user created by the association.
	ESUserRole string
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

	associated := r.AssociatedObjTemplate()
	if err := FetchWithAssociation(ctx, r.Client, request, associated); err != nil {
		if apierrors.IsNotFound(err) {
			// object resource has been deleted, remove artifacts related to the association.
			return reconcile.Result{}, r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(associated) {
		r.log(associated).Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	if !associated.GetDeletionTimestamp().IsZero() {
		// Object is being deleted, short-circuit reconciliation
		return reconcile.Result{}, nil
	}

	if compatible, err := r.isCompatible(ctx, associated); err != nil || !compatible {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if err := annotation.UpdateControllerVersion(ctx, r.Client, associated, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	results := reconciler.NewResult(ctx)
	newStatus, err := r.doReconcile(ctx, associated)
	if err != nil {
		results.WithError(err)
	}

	// we want to attempt a status update even in the presence of errors
	if err := r.updateStatus(ctx, associated, newStatus); err != nil {
		return defaultRequeue, tracing.CaptureError(ctx, err)
	}
	return results.
		WithResult(RequeueRbacCheck(r.accessReviewer)).
		WithResult(resultFromStatus(newStatus)).
		Aggregate()
}

func (r *Reconciler) doReconcile(ctx context.Context, associated commonv1.Associated) (commonv1.AssociationStatus, error) {
	assocKey := k8s.ExtractNamespacedName(associated)
	assocLabels := r.AssociationLabels(assocKey)

	// garbage collect leftover resources that are not required anymore
	if err := deleteOrphanedResources(ctx, r, associated, assocLabels); err != nil {
		r.log(associated).Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}

	esRef := associated.ElasticsearchRef()
	if !esRef.IsDefined() {
		// clean up watchers and remove artifacts related to the association
		if err := r.onDelete(assocKey); err != nil {
			return commonv1.AssociationFailed, err
		}
		// remove the configuration in the annotation, other leftover resources are already garbage-collected
		return commonv1.AssociationUnknown, RemoveAssociationConf(r.Client, associated)
	}
	if esRef.Namespace == "" {
		// no namespace provided: default to the associated resource namespace
		esRef.Namespace = associated.GetNamespace()
	}

	// retrieve the Elasticsearch resource
	es, associationStatus, err := r.getElasticsearch(ctx, associated, esRef)
	if associationStatus != "" || err != nil {
		return associationStatus, err
	}

	// check if reference to Elasticsearch is allowed to be established
	if allowed, err := CheckAndUnbind(r.accessReviewer, associated, &es, r, r.recorder); err != nil || !allowed {
		return commonv1.AssociationPending, err
	}

	// watch resources related to the referenced ES
	if err := r.setDynamicWatches(associated, esRef.NamespacedName()); err != nil {
		return commonv1.AssociationFailed, err
	}

	if err := ReconcileEsUser(
		ctx,
		r.Client,
		associated,
		assocLabels,
		r.ESUserRole,
		r.UserSecretSuffix,
		es,
	); err != nil {
		return commonv1.AssociationPending, err
	}

	caSecret, err := r.ReconcileCASecret(associated, esRef.NamespacedName())
	if err != nil {
		return commonv1.AssociationPending, err // maybe not created yet
	}

	// construct the expected association configuration
	authSecretRef := UserSecretKeySelector(associated, r.UserSecretSuffix)
	expectedAssocConf := &commonv1.AssociationConf{
		AuthSecretName: authSecretRef.Name,
		AuthSecretKey:  authSecretRef.Key,
		CACertProvided: caSecret.CACertProvided,
		CASecretName:   caSecret.Name,
		URL:            services.ExternalServiceURL(es),
	}

	// update the association configuration if necessary
	return r.updateAssocConf(ctx, expectedAssocConf, associated)
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
	associated commonv1.Associated,
	elasticsearchRef commonv1.ObjectSelector,
) (esv1.Elasticsearch, commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "get_elasticsearch", tracing.SpanTypeApp)
	defer span.End()

	var es esv1.Elasticsearch
	err := r.Get(elasticsearchRef.NamespacedName(), &es)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, associated, events.EventAssociationError,
			"Failed to find referenced backend %s: %v", elasticsearchRef.NamespacedName(), err)
		if apierrors.IsNotFound(err) {
			// ES is not found, remove any existing backend configuration and retry in a bit.
			if err := RemoveAssociationConf(r.Client, associated); err != nil && !apierrors.IsConflict(err) {
				r.log(associated).Error(err, "Failed to remove Elasticsearch output from EnterpriseSearch object")
				return esv1.Elasticsearch{}, commonv1.AssociationPending, err
			}
			return esv1.Elasticsearch{}, commonv1.AssociationPending, nil
		}
		return esv1.Elasticsearch{}, commonv1.AssociationFailed, err
	}
	return es, "", nil
}

// Unbind removes the association resources.
func (r *Reconciler) Unbind(associated commonv1.Associated) error {
	// Ensure that user in Elasticsearch is deleted to prevent illegitimate access
	if err := k8s.DeleteSecretMatching(r.Client, r.userLabelSelector(k8s.ExtractNamespacedName(associated))); err != nil {
		return err
	}
	// Also remove the association configuration
	return RemoveAssociationConf(r.Client, associated)
}

// updateAssocConf updates associated with the expected association conf.
func (r *Reconciler) updateAssocConf(ctx context.Context, expectedAssocConf *commonv1.AssociationConf, associated commonv1.Associated) (commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "update_assoc_conf", tracing.SpanTypeApp)
	defer span.End()

	if !reflect.DeepEqual(expectedAssocConf, associated.AssociationConf()) {
		r.log(associated).Info("Updating spec with Elasticsearch association configuration")
		if err := UpdateAssociationConf(r.Client, associated, expectedAssocConf); err != nil {
			if apierrors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			r.log(associated).Error(err, "Failed to update EnterpriseSearch association configuration")
			return commonv1.AssociationPending, err
		}
		associated.SetAssociationConf(expectedAssocConf)
	}
	return commonv1.AssociationEstablished, nil
}

// updateStatus updates the associated resource status.
func (r *Reconciler) updateStatus(ctx context.Context, associated commonv1.Associated, newStatus commonv1.AssociationStatus) error {
	span, _ := apm.StartSpan(ctx, "update_association_status", tracing.SpanTypeApp)
	defer span.End()

	oldStatus := associated.AssociationStatus()
	if !reflect.DeepEqual(oldStatus, newStatus) {
		associated.SetAssociationStatus(newStatus)
		if err := r.Status().Update(associated); err != nil {
			return err
		}
		r.recorder.AnnotatedEventf(
			associated,
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
	r.removeWatches(associated)
	// delete user Secret in the Elasticsearch namespace
	return k8s.DeleteSecretMatching(r.Client, r.userLabelSelector(associated))
}
