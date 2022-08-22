// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ptr "k8s.io/utils/pointer"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoscalingv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/dev"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	logconf "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

type EsClientProvider func(ctx context.Context, c k8s.Client, dialer net.Dialer, es esv1.Elasticsearch) (esclient.Client, error)

const (
	// ControllerName is the name of the autoscaling controller based on the dedicated Elasticsearch autoscaling resource. It supersedes the legacy
	// controller which is reading the autoscaling specification in an annotation on the Elasticsearch resource.
	ControllerName = "elasticsearch-autoscaler"

	enterpriseFeaturesDisabledMsg = "Autoscaling is an enterprise feature. Enterprise features are disabled"
)

// licenseCheckRequeue is the default duration used to retry a licence check if the cluster is supposed to be managed by
// the autoscaling controller and if the licence is not valid.
var licenseCheckRequeue = reconcile.Result{
	Requeue:      true,
	RequeueAfter: 60 * time.Second,
}

// baseReconcileAutoscaling is the base struct for both the legacy and the CRD based reconcilers.
type baseReconcileAutoscaling struct {
	k8s.Client
	operator.Parameters
	esClientProvider EsClientProvider
	recorder         record.EventRecorder
	licenseChecker   license.Checker

	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r baseReconcileAutoscaling) withRecorder(recorder record.EventRecorder) baseReconcileAutoscaling {
	r.recorder = recorder
	return r
}

// ReconcileElasticsearchAutoscaler reconciles autoscaling policies and Elasticsearch resources specifications based on
// Elasticsearch autoscaling API response.
type ReconcileElasticsearchAutoscaler struct {
	baseReconcileAutoscaling
	Watches watches.DynamicWatches
}

// NewReconcilers returns both a legacy (ES annotation based) and new (CRD based) reconcile.Reconciler
func NewReconcilers(mgr manager.Manager, params operator.Parameters) (*ReconcileElasticsearchAutoscalingAnnotation, *ReconcileElasticsearchAutoscaler) {
	c := mgr.GetClient()
	reconcileAutoscaling := baseReconcileAutoscaling{
		Client:           c,
		Parameters:       params,
		esClientProvider: newElasticsearchClient,
		recorder:         mgr.GetEventRecorderFor(ControllerName),
		licenseChecker:   license.NewLicenseChecker(c, params.OperatorNamespace),
	}
	return &ReconcileElasticsearchAutoscalingAnnotation{reconcileAutoscaling.withRecorder(mgr.GetEventRecorderFor(LegacyControllerName))},
		&ReconcileElasticsearchAutoscaler{
			baseReconcileAutoscaling: reconcileAutoscaling.withRecorder(mgr.GetEventRecorderFor(ControllerName)),
			Watches:                  watches.NewDynamicWatches(),
		}
}

func dyamicWatchName(request reconcile.Request) string {
	return fmt.Sprintf("%s-%s-referenced-es-watch", request.Namespace, request.Name)
}

func (r *ReconcileElasticsearchAutoscaler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, ControllerName, "es_autoscaler_name", request)
	defer common.LogReconciliationRun(logconf.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	log := logconf.FromContext(ctx)
	// Fetch the ElasticsearchAutoscaler instance
	var esa autoscalingv1alpha1.ElasticsearchAutoscaler
	if err := r.Get(ctx, request.NamespacedName, &esa); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("ElasticsearchAutoscaler %s/%s not found")
			r.Watches.ReferencedResources.RemoveHandlerForKey(dyamicWatchName(request))
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Ensure we watch the associated Elasticsearch
	esNamespacedName := types.NamespacedName{Name: esa.Spec.ElasticsearchRef.Name, Namespace: request.Namespace}
	r.Watches.ReferencedResources.AddHandler(watches.NamedWatch{
		Name:    dyamicWatchName(request),
		Watched: []types.NamespacedName{esNamespacedName},
		Watcher: request.NamespacedName,
	})

	if common.IsUnmanaged(ctx, &esa) {
		msg := "Object is currently not managed by this controller. Skipping reconciliation"
		log.Info(msg, "namespace", request.Namespace, "es_autoscaler_name", request.Name)
		return r.reportAsInactive(ctx, log, esa, msg)
	}

	// Fetch the Elasticsearch resource
	var es esv1.Elasticsearch
	if err := r.Get(ctx, esNamespacedName, &es); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("Elasticsearch resource %s/%s not found", esNamespacedName.Namespace, esNamespacedName.Name)
			log.Info(msg, "namespace", request.Namespace, "es_autoscaler_name", request.Name, "es_name", esNamespacedName.Name, "error", err.Error())
			return r.reportAsInactive(ctx, log, esa, msg)
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if es.IsAutoscalingAnnotationSet() {
		err := fmt.Errorf("deprecated autoscaling annotation %s found on Elasticsearch %s/%s", esv1.ElasticsearchAutoscalingSpecAnnotationName, esNamespacedName.Namespace, esNamespacedName.Namespace)
		log.Error(err, "Cannot use the Elasticsearch Autoscaler resource and the autoscaling annotation", "namespace", request.Namespace, "es_autoscaler_name", request.Name, "es_name", esNamespacedName.Name, "error", err.Error())
		return r.reportAsInactive(ctx, log, esa, err.Error())
	}

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !enabled {
		log.Info(enterpriseFeaturesDisabledMsg)
		r.recorder.Eventf(&es, corev1.EventTypeWarning, license.EventInvalidLicense, enterpriseFeaturesDisabledMsg)
		_, err := r.reportAsInactive(ctx, log, esa, enterpriseFeaturesDisabledMsg)
		// We still schedule a reconciliation in case a valid license is applied later
		return licenseCheckRequeue, err
	}

	// Get autoscaling policies and the associated node sets.
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return reconcile.Result{}, err
	}
	autoscaledNodeSets, nodeSetErr := es.GetAutoscaledNodeSets(v, esa.Spec.AutoscalingPolicySpecs)
	if nodeSetErr != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, nodeSetErr)
	}
	log.V(1).Info(
		"Autoscaling policies and node sets",
		"policies", autoscaledNodeSets.Names(),
		"namespace", request.Namespace,
		"es_autoscaler_name", request.Name,
	)

	// Import existing resources in the current Status if the cluster is managed by some autoscaling policies but
	// the status annotation does not exist.
	if err := status.ImportExistingResources(
		log,
		r.Client,
		esa.Spec.AutoscalingPolicySpecs,
		es,
		autoscaledNodeSets,
		&esa.Status,
	); err != nil {
		_, _ = r.reportAsUnhealthy(ctx, log, esa, fmt.Sprintf("error while importing resources from the status subresource: %s", err.Error()))
		// Status is updated on a best effort basis, we don't really care if the error above is not reported.
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	statusBuilder := newStatusBuilder(log, esa.Spec.AutoscalingPolicySpecs)
	results := &reconciler.Results{}

	// Call the main function
	reconciledEs, reconcileInternalErr := r.reconcileInternal(ctx, es, esa.Status, statusBuilder, autoscaledNodeSets, &esa)
	if reconcileInternalErr != nil {
		// we do not return immediately as not all errors prevent to compute a reconciled Elasticsearch resource.
		results.WithError(reconcileInternalErr)
	}

	// Update the new status
	newStatus := statusBuilder.Build()
	esa.Status.ObservedGeneration = ptr.Int64(esa.Generation)
	esa.Status.Conditions = esa.Status.Conditions.MergeWith(newStatus.Conditions...)
	esa.Status.AutoscalingPolicyStatuses = newStatus.AutoscalingPolicyStatuses
	updateStatus, err := r.updateStatus(ctx, log, esa)
	if err != nil {
		return reconcile.Result{}, err
	}
	results.WithResult(updateStatus)

	if reconciledEs == nil {
		// no reconciled Elasticsearch has been returned, likely to be the case if a fatal error prevented
		// to calculate the new compute and storage resources.
		return results.Aggregate()
	}

	// Update the Elasticsearch resource
	if err := r.Client.Update(ctx, reconciledEs); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		return results.WithError(err).Aggregate()
	}
	return results.WithResult(defaultResult(&esa)).Aggregate()
}

// reportAsUnhealthy reports the autoscaler as inactive in the status.
func (r *ReconcileElasticsearchAutoscaler) reportAsUnhealthy(
	ctx context.Context,
	log logr.Logger,
	esa autoscalingv1alpha1.ElasticsearchAutoscaler,
	message string,
) (reconcile.Result, error) {
	now := metav1.Now()
	newStatus := esa.Status.DeepCopy()
	newStatus.ObservedGeneration = ptr.Int64Ptr(esa.Generation)
	newStatus.Conditions = newStatus.Conditions.MergeWith(
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerActive,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: now,
			Message:            "Autoscaler is unhealthy",
		},
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerHealthy,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Message:            message,
		},
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerOnline,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Message:            "Autoscaler is unhealthy",
		},
	)
	// Insert a new limited status if there is none.
	if newStatus.Conditions.Index(v1alpha1.ElasticsearchAutoscalerLimited) < 0 {
		newStatus.Conditions = newStatus.Conditions.MergeWith(v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerLimited,
			Status:             corev1.ConditionUnknown,
			LastTransitionTime: now,
		})
	}
	esa.Status = *newStatus
	return r.updateStatus(ctx, log, esa)
}

// reportAsInactive reports the autoscaler as inactive in the status.
func (r *ReconcileElasticsearchAutoscaler) reportAsInactive(
	ctx context.Context,
	log logr.Logger,
	esa autoscalingv1alpha1.ElasticsearchAutoscaler,
	message string,
) (reconcile.Result, error) {
	now := metav1.Now()
	newStatus := esa.Status.DeepCopy()
	newStatus.ObservedGeneration = ptr.Int64Ptr(esa.Generation)
	newStatus.Conditions = newStatus.Conditions.MergeWith(
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerActive,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Message:            message,
		},
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerHealthy,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Message:            "Autoscaler is inactive",
		},
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerOnline,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Message:            "Autoscaler is inactive",
		},
	)
	// Insert a new limited status if there is none.
	if newStatus.Conditions.Index(v1alpha1.ElasticsearchAutoscalerLimited) < 0 {
		newStatus.Conditions = newStatus.Conditions.MergeWith(v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerLimited,
			Status:             corev1.ConditionUnknown,
			LastTransitionTime: now,
		})
	}
	esa.Status = *newStatus
	return r.updateStatus(ctx, log, esa)
}

func (r *ReconcileElasticsearchAutoscaler) updateStatus(
	ctx context.Context,
	log logr.Logger,
	esa autoscalingv1alpha1.ElasticsearchAutoscaler,
) (reconcile.Result, error) {
	results := &reconciler.Results{}
	if err := r.Client.Status().Update(ctx, &esa); err != nil {
		tracing.CaptureError(ctx, err)
		if apierrors.IsConflict(err) {
			log.V(1).Info(
				"Conflict while updating the status",
				"namespace", esa.Namespace,
				"es_autoscaler_name", esa.Name,
				"error", err.Error(),
			)
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		return results.WithError(err).Aggregate()
	}
	return results.Aggregate()
}

func defaultResult(autoscalingSpecification v1alpha1.AutoscalingSpec) reconcile.Result {
	requeueAfter := v1alpha1.DefaultPollingPeriod
	pollingPeriod := autoscalingSpecification.GetPollingPeriod()
	if pollingPeriod != nil {
		requeueAfter = pollingPeriod.Duration
	}
	return reconcile.Result{
		Requeue:      true,
		RequeueAfter: requeueAfter,
	}
}

func newElasticsearchClient(
	ctx context.Context,
	c k8s.Client,
	dialer net.Dialer,
	es esv1.Elasticsearch,
) (esclient.Client, error) {
	defer tracing.Span(&ctx)()
	url := services.ExternalServiceURL(es)
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}
	// Get user Secret
	var controllerUserSecret corev1.Secret
	key := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.InternalUsersSecret(es.Name),
	}
	if err := c.Get(ctx, key, &controllerUserSecret); err != nil {
		return nil, err
	}
	password, ok := controllerUserSecret.Data[user.ControllerUserName]
	if !ok {
		return nil, fmt.Errorf("controller user %s not found in Secret %s/%s", user.ControllerUserName, key.Namespace, key.Name)
	}

	// Get public certs
	var caSecret corev1.Secret
	key = types.NamespacedName{
		Namespace: es.Namespace,
		Name:      certificates.PublicCertsSecretName(esv1.ESNamer, es.Name),
	}
	if err := c.Get(ctx, key, &caSecret); err != nil {
		return nil, err
	}
	trustedCerts, ok := caSecret.Data[certificates.CertFileName]
	if !ok {
		return nil, fmt.Errorf("%s not found in Secret %s/%s", certificates.CertFileName, key.Namespace, key.Name)
	}
	caCerts, err := certificates.ParsePEMCerts(trustedCerts)
	if err != nil {
		return nil, err
	}
	return esclient.NewElasticsearchClient(
		dialer,
		k8s.ExtractNamespacedName(&es),
		url,
		esclient.BasicAuth{
			Name:     user.ControllerUserName,
			Password: string(password),
		},
		v,
		caCerts,
		esclient.Timeout(ctx, es),
		dev.Enabled,
	), nil
}
