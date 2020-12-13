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
	// AssociationType identifies the type of the resource for association (eg. kibana for APM to Kibana association,
	// elasticsearch for Beat to Elasticsearch association)
	AssociationType commonv1.AssociationType
	// AssociatedObjTemplate builds an empty typed associated object (eg. &Kibana{} for Kibana to Elasticsearch association).
	AssociatedObjTemplate func() commonv1.Associated
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
	// AssociationLabels are labels set on resources created for association purpose. Note that resources will be also
	// labelled with AssociationResourceNameLabelName and AssociationResourceNamespaceLabelName in addition to any
	// labels provided here.
	AssociatedLabels func(associated types.NamespacedName) map[string]string
	// AssociationConfAnnotationNameBase is the name of the annotation used to define the config for the associated resource.
	// It is used by the association controller to store the configuration and by the controller which is
	// managing the associated resource to build the appropriate configuration.
	AssociationConfAnnotationNameBase string
	// UserSecretSuffix is used as a suffix in the name of the secret holding user data in the associated namespace.
	UserSecretSuffix string
	// CASecretLabelName is the name of the label added on the Secret that contains the CA of the remote service.
	//CASecretLabelName string
	// ESUserRole is the role to use for the Elasticsearch user created by the association.
	ESUserRole func(commonv1.Associated) (string, error)
	// SetDynamicWatches allows to set some specific watches.
	SetDynamicWatches func(association commonv1.Association, watches watches.DynamicWatches) error
	// ClearDynamicWatches is called when the controller needs to clear the specific watches set for this association.
	ClearDynamicWatches func(associated types.NamespacedName, watches watches.DynamicWatches)
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
}

func (a AssociationInfo) AssociationLabels(
	associated types.NamespacedName,
	association types.NamespacedName,
) client.MatchingLabels {
	return maps.Merge(
		map[string]string{
			a.AssociationResourceNameLabelName:      association.Name,
			a.AssociationResourceNamespaceLabelName: association.Namespace,
		},
		a.AssociatedLabels(associated),
	)
}

// userLabelSelector returns labels selecting the ES user secret, including association labels and user type label.
func (a AssociationInfo) userLabelSelector(
	associated types.NamespacedName,
	association types.NamespacedName,
) client.MatchingLabels {
	return maps.Merge(
		map[string]string{common.TypeLabelName: user.AssociatedUserType},
		a.AssociationLabels(associated, association),
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
	if err := FetchWithAssociations(ctx, r.Client, request, associated); err != nil {
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

	associations := associated.GetAssociations()

	// garbage collect leftover resources that are not required anymore
	if err := deleteOrphanedResources(ctx, r.Client, r.AssociationInfo, associated); err != nil {
		r.log(associated).Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}

	if err := RemoveExcesiveAssociationConfs(r.Client, associated, associations, r.AssociationConfAnnotationNameBase); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	r.removeWatches(associatedKey, associations)

	results := reconciler.NewResult(ctx)
	newStatusGroup := commonv1.AssociationStatusGroup{}
	for _, association := range associations {
		if association.AssociatedType() != r.AssociationType {
			// some resources have more than one type of resource associations, making sure we are looking at the right
			// one for this controller
			continue
		}

		newStatus, err := r.doReconcile(ctx, association)
		if err != nil {
			results.WithError(err)
		}

		newStatusGroup[association.AssociationRef().NamespacedName().String()] = newStatus
	}

	// we want to attempt a status update even in the presence of errors
	if err := r.updateStatus(ctx, associated, newStatusGroup); err != nil {
		return defaultRequeue, tracing.CaptureError(ctx, err)
	}
	return results.
		WithResult(RequeueRbacCheck(r.accessReviewer)).
		WithResult(resultFromStatuses(newStatusGroup)).
		Aggregate()
}

func (r *Reconciler) doReconcile(ctx context.Context, association commonv1.Association) (commonv1.AssociationStatus, error) {
	// retrieve the Elasticsearch resource, since it can be a transitive reference we need to use the provided ElasticsearchRef function
	associatedResourceFound, esRef, err := r.ElasticsearchRef(r.Client, association)
	if err != nil {
		return commonv1.AssociationFailed, err
	}

	// set additional watches, in the case of a transitive Elasticsearch reference we must watch the intermediate resource
	if r.SetDynamicWatches != nil {
		if err := r.SetDynamicWatches(association, r.watches); err != nil {
			return commonv1.AssociationFailed, err
		}
	}

	// the associated resource does not exist yet, set status to Pending
	if !associatedResourceFound {
		return commonv1.AssociationPending, RemoveAssociationConf(
			r.Client,
			association.Associated(),
			association.AssociationConfAnnotationNameBase(),
			association.Id(),
		)
	}

	es, associationStatus, err := r.getElasticsearch(ctx, association, esRef)
	if associationStatus != "" || err != nil {
		return associationStatus, err
	}

	// from this point we have checked that all the associated resources are set and have been found.

	// check if reference to Elasticsearch is allowed to be established
	if allowed, err := CheckAndUnbind(ctx, r.accessReviewer, association, &es, r, r.recorder); err != nil || !allowed {
		return commonv1.AssociationPending, err
	}

	associationRef := association.AssociationRef()

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

	assocLabels := r.AssociationLabels(k8s.ExtractNamespacedName(association.Associated()), association.AssociationRef().NamespacedName())
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
	authSecretRef := UserSecretKeySelector(association, r.UserSecretSuffix)
	expectedAssocConf := &commonv1.AssociationConf{
		AuthSecretName: authSecretRef.Name,
		AuthSecretKey:  authSecretRef.Key,
		CACertProvided: caSecret.CACertProvided,
		CASecretName:   caSecret.Name,
		URL:            url,
		Version:        ver,
	}

	// update the association configuration if necessary
	return r.updateAssocConf(ctx, expectedAssocConf, association)
}

// isCompatible returns true if the given resource can be reconciled by the current controller.
func (r *Reconciler) isCompatible(ctx context.Context, associated commonv1.Associated) (bool, error) {
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, associated, r.AssociatedLabels(k8s.ExtractNamespacedName(associated)), r.OperatorInfo.BuildInfo.Version)
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
			if err := RemoveAssociationConf(
				r.Client,
				association.Associated(),
				association.AssociationConfAnnotationNameBase(),
				association.Id(),
			); err != nil && !apierrors.IsConflict(err) {
				r.log(association).Error(err, "Failed to remove Elasticsearch association configuration")
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
	return RemoveAssociationConf(
		r.Client,
		association.Associated(),
		association.AssociationConfAnnotationNameBase(),
		association.Id(),
	)
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
		r.log(association).Info("Updating association configuration")
		if err := UpdateAssociationConf(r.Client, association, expectedAssocConf); err != nil {
			if apierrors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			r.log(association).Error(err, "Failed to update association configuration")
			return commonv1.AssociationPending, err
		}
		association.SetAssociationConf(expectedAssocConf)
	}
	return commonv1.AssociationEstablished, nil
}

// updateStatus updates the associated resource status.
func (r *Reconciler) updateStatus(ctx context.Context, associated commonv1.Associated, newStatus commonv1.AssociationStatusGroup) error {
	span, _ := apm.StartSpan(ctx, "update_association_status", tracing.SpanTypeApp)
	defer span.End()

	oldStatus := associated.AssociationStatusGroup(r.AssociationType)
	if !reflect.DeepEqual(oldStatus, newStatus) {
		if err := associated.SetAssociationStatusGroup(r.AssociationType, newStatus); err != nil {
			return err
		}
		if err := r.Status().Update(associated); err != nil {
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

func resultFromStatuses(statusGroup commonv1.AssociationStatusGroup) reconcile.Result {
	for _, status := range statusGroup {
		switch status {
		case commonv1.AssociationPending:
			return defaultRequeue // retry
		}
	}

	return reconcile.Result{} // we are done or there is not much we can do
}

func (r *Reconciler) onDelete(associated types.NamespacedName) error {
	// remove dynamic watches
	if r.SetDynamicWatches != nil {
		r.ClearDynamicWatches(associated, r.watches)
	}
	// remove other watches
	r.removeWatches(associated, nil)
	// delete user Secret in the Elasticsearch namespace
	return k8s.DeleteSecretMatching(r.Client, r.userLabelSelector(associated))
}
