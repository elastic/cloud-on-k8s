// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"time"

	pkgerrors "github.com/pkg/errors"
	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	ControllerName = "webhook-certificates-controller"
)

var _ reconcile.Reconciler = &ReconcileWebhookResources{}

// ReconcileWebhookResources reconciles the certificates used by the webhook server.
type ReconcileWebhookResources struct {
	// k8s.Client is used to watch resources
	k8s.Client
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64

	// webhook parameters
	webhookParams Params
	// resources are updated with a native Kubernetes client
	clientset kubernetes.Interface

	// APM tracer
	tracer *apm.Tracer
}

func (r *ReconcileWebhookResources) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.tracer, ControllerName, "validating_webhook_configuration", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	res := r.reconcileInternal(ctx)
	return res.Aggregate()
}

func (r *ReconcileWebhookResources) reconcileInternal(ctx context.Context) *reconciler.Results {
	res := &reconciler.Results{}
	wh, err := r.webhookParams.NewAdmissionControllerInterface(ctx, r.clientset)
	if err != nil {
		return res.WithError(err)
	}
	if err := r.webhookParams.ReconcileResources(ctx, r.clientset, wh); err != nil {
		return res.WithError(err)
	}

	// Get the latest content of the webhook CA
	webhookServerSecret, err := r.clientset.CoreV1().Secrets(r.webhookParams.Namespace).Get(ctx, r.webhookParams.SecretName, metav1.GetOptions{})
	if err != nil {
		return res.WithError(err)
	}
	serverCA := certificates.BuildCAFromSecret(ctx, *webhookServerSecret)
	if serverCA == nil {
		return res.WithError(
			pkgerrors.Errorf("cannot find CA in webhook secret %s/%s", r.webhookParams.Namespace, r.webhookParams.SecretName),
		)
	}

	res.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), serverCA.Cert.NotAfter, r.webhookParams.Rotation.RotateBefore),
	})
	return res
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, webhookParams Params, clientset kubernetes.Interface, tracer *apm.Tracer) *ReconcileWebhookResources {
	c := mgr.GetClient()
	return &ReconcileWebhookResources{
		Client:        c,
		webhookParams: webhookParams,
		clientset:     clientset,
		tracer:        tracer,
	}
}

// Add adds a new Controller to mgr with r as the reconcile.Reconciler
func Add(mgr manager.Manager, webhookParams Params, clientset kubernetes.Interface, webhook AdmissionControllerInterface, tracer *apm.Tracer) error {
	r := newReconciler(mgr, webhookParams, clientset, tracer)
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	secret := types.NamespacedName{
		Namespace: webhookParams.Namespace,
		Name:      webhookParams.SecretName,
	}

	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, &watches.NamedWatch[*corev1.Secret]{
		Name:    "webhook-server-cert",
		Watched: []types.NamespacedName{secret},
		Watcher: secret,
	})); err != nil {
		return err
	}

	webhookConfiguration := types.NamespacedName{
		Name: webhookParams.Name,
	}

	return c.Watch(source.Kind(mgr.GetCache(), webhook.getType(), &watches.NamedWatch[client.Object]{
		Name:    "validatingwebhookconfiguration",
		Watched: []types.NamespacedName{webhookConfiguration},
		Watcher: webhookConfiguration,
	}))
}
