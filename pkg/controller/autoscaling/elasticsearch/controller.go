// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

type EsClientProvider func(ctx context.Context, c k8s.Client, dialer net.Dialer, es esv1.Elasticsearch) (esclient.Client, error)

const (
	controllerName = "elasticsearch-autoscaling"

	enterpriseFeaturesDisabledMsg = "Autoscaling is an enterprise feature. Enterprise features are disabled"
)

// licenseCheckRequeue is the default duration used to retry a licence check if the cluster is supposed to be managed by
// the autoscaling controller and if the licence is not valid.
var licenseCheckRequeue = reconcile.Result{
	Requeue:      true,
	RequeueAfter: 60 * time.Second,
}

// ReconcileElasticsearch reconciles autoscaling policies and Elasticsearch resources specifications based on
// Elasticsearch autoscaling API response.
type ReconcileElasticsearch struct {
	k8s.Client
	operator.Parameters
	esClientProvider EsClientProvider
	recorder         record.EventRecorder
	licenseChecker   license.Checker

	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileElasticsearch {
	c := mgr.GetClient()
	return &ReconcileElasticsearch{
		Client:           c,
		Parameters:       params,
		esClientProvider: newElasticsearchClient,
		recorder:         mgr.GetEventRecorderFor(controllerName),
		licenseChecker:   license.NewLicenseChecker(c, params.OperatorNamespace),
	}
}

// Reconcile updates the ResourceRequirements and PersistentVolumeClaim fields for each elasticsearch container in a
// NodeSet managed by an autoscaling policy. ResourceRequirements are updated according to the response of the Elasticsearch
// _autoscaling/capacity API and given the constraints provided by the user in the autoscaling specification.
func (r *ReconcileElasticsearch) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, controllerName, "es_name", request)
	defer common.LogReconciliationRunNoSideEffects(logconf.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	// Fetch the Elasticsearch instance
	var es esv1.Elasticsearch
	requeue, err := r.fetchElasticsearch(ctx, request, &es)
	if err != nil || requeue {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if !es.IsAutoscalingDefined() {
		return reconcile.Result{}, nil
	}

	log := logconf.FromContext(ctx)

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled()
	if err != nil {
		return reconcile.Result{}, err
	}
	if !enabled {
		log.Info(enterpriseFeaturesDisabledMsg)
		r.recorder.Eventf(&es, corev1.EventTypeWarning, license.EventInvalidLicense, enterpriseFeaturesDisabledMsg)
		// We still schedule a reconciliation in case a valid license is applied later
		return licenseCheckRequeue, nil
	}

	if common.IsUnmanaged(&es) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", es.Namespace, "es_name", es.Name)
		return reconcile.Result{}, nil
	}

	// Get resource policies from the Elasticsearch spec
	autoscalingSpecification, err := es.GetAutoscalingSpecification()
	if err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Validate Elasticsearch and Autoscaling spec
	if err := validation.ValidateElasticsearch(es, r.ExposedNodeLabels); err != nil {
		log.Error(
			err,
			"Elasticsearch manifest validation failed",
			"namespace", es.Namespace,
			"es_name", es.Name,
		)
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Build status from annotation or existing resources
	autoscalingStatus, err := status.From(es)
	if err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if len(autoscalingSpecification.AutoscalingPolicySpecs) == 0 && len(autoscalingStatus.AutoscalingPolicyStatuses) == 0 {
		// This cluster is not managed by the autoscaler
		return reconcile.Result{}, nil
	}

	// Get autoscaling policies and the associated node sets.
	autoscaledNodeSets, nodeSetErr := autoscalingSpecification.GetAutoscaledNodeSets()
	if nodeSetErr != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, nodeSetErr)
	}
	log.V(1).Info("Autoscaling policies and node sets", "policies", autoscaledNodeSets.Names())

	// Import existing resources in the current Status if the cluster is managed by some autoscaling policies but
	// the status annotation does not exist.
	if err := autoscalingStatus.ImportExistingResources(log, r.Client, autoscalingSpecification, autoscaledNodeSets); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Call the main function
	current, err := r.reconcileInternal(ctx, autoscalingStatus, autoscaledNodeSets, autoscalingSpecification, es)
	if err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}
	results := &reconciler.Results{}
	return results.WithResult(defaultResult(autoscalingSpecification)).WithResult(current).Aggregate()
}

func defaultResult(autoscalingSpecification esv1.AutoscalingSpec) reconcile.Result {
	return reconcile.Result{
		Requeue:      true,
		RequeueAfter: autoscalingSpecification.GetPollingPeriodOrDefault(),
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
	if err := c.Get(context.Background(), key, &controllerUserSecret); err != nil {
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
	if err := c.Get(context.Background(), key, &caSecret); err != nil {
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
		esclient.Timeout(es),
	), nil
}
