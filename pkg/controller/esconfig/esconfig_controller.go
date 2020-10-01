// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/nsf/jsondiff"
	"github.com/pkg/errors"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	escv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/esconfig/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "esconfig-controller"
)

// Add creates a new ESConfig Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, reconciler, params)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileElasticsearchConfig {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileElasticsearchConfig{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
	}
}

func addWatches(c controller.Controller, r *ReconcileElasticsearchConfig) error {
	err := c.Watch(&source.Kind{Type: &escv1alpha1.ElasticsearchConfig{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileElasticsearchConfig{}

// ReconcileElasticsearchConfig reconciles an ApmServer object
type ReconcileElasticsearchConfig struct {
	k8s.Client
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *ReconcileElasticsearchConfig) K8sClient() k8s.Client {
	return r.Client
}

func (r *ReconcileElasticsearchConfig) DynamicWatches() watches.DynamicWatches {
	return r.dynamicWatches
}

func (r *ReconcileElasticsearchConfig) Recorder() record.EventRecorder {
	return r.recorder
}

var _ driver.Interface = &ReconcileElasticsearchConfig{}

// Reconcile reads that state of the cluster for an ES Config object and makes changes based on the state read
// and what is in the spec.
func (r *ReconcileElasticsearchConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := common.NewReconciliationContext(crlog.Log.WithName(controllerName), request.NamespacedName, "elasticsearch")
	return tracing.TraceReconciliation(ctx, request, "elasticsearch", r.doReconcile)
}

func (r *ReconcileElasticsearchConfig) doReconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {

	logger := tracing.LoggerFromContext(ctx)
	defer common.LogReconciliationRun(logger, request, &r.iteration)()

	var esc escv1alpha1.ElasticsearchConfig
	err := association.FetchWithAssociations(ctx, r.Client, request, &esc)
	if err != nil {
		// ES config has since been deleted
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// TODO should this requeue? other controllers dont, im guessing just expecting to requeue based on the error
		return reconcile.Result{}, err
	}

	if common.IsUnmanaged(&esc) {
		logger.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", esc.Namespace, "esc_name", esc.Name)
		return reconcile.Result{}, nil
	}

	compatible, err := r.isCompatible(ctx, &esc)
	if err != nil || !compatible {
		return reconcile.Result{}, err
	}

	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, &esc); err != nil {
		return reconcile.Result{}, err
	}

	// TODO is there better way to get the ES version? we need it for the version to create the client
	var es esv1.Elasticsearch
	ns := esc.Namespace
	if esc.Spec.ElasticsearchRef.Namespace != "" {
		ns = esc.Spec.ElasticsearchRef.Namespace
	}
	esNsn := types.NamespacedName{
		Name:      esc.Spec.ElasticsearchRef.Name,
		Namespace: ns,
	}
	// TODO switch this to use the regular crclient so we can pass contexts?
	err = r.Client.Get(esNsn, &es)
	if err != nil {
		logger.Error(err, "Associated object doesn't exist yet")
		k8s.EmitErrorEvent(r.recorder, err, &esc, events.EventAssociationError, err.Error())
		return reconcile.Result{Requeue: true}, err
	}

	escl, err := NewESClient(ctx, r.Parameters.Dialer, r.Client, es)
	if err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	for _, op := range esc.Spec.Operations {
		err = ReconcileOperation(ctx, escl, op)
		if err != nil {
			return reconcile.Result{Requeue: true}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileElasticsearchConfig) isCompatible(ctx context.Context, esc *escv1alpha1.ElasticsearchConfig) (bool, error) {
	// we give this an empty selector as there's no indicator that it was reconciled by a previous version. we do want to set the annotation though for troubleshooting purposes
	selector := map[string]string{}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, esc, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, esc, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func (r *ReconcileElasticsearchConfig) validate(ctx context.Context, esc *escv1alpha1.ElasticsearchConfig) error {
	return tracing.DoInSpan(ctx, "validate", func(ctx context.Context) error {
		if err := esc.ValidateCreate(); err != nil {
			tracing.LoggerFromContext(ctx).Error(err, "Validation failed")
			k8s.EmitErrorEvent(r.recorder, err, esc, events.EventReasonValidation, err.Error())
			return err
		}
		return nil
	})
}

// ReconcileOperation reconciles an individual esconfig operation
func ReconcileOperation(ctx context.Context, client esclient.Client, operation escv1alpha1.ElasticsearchConfigOperation) error {
	// this is already checked for errors at the beginning of the loop
	// TODO there is no need for this to be a url, we only use it as a string
	opURL, _ := url.Parse(operation.URL)
	needsUpdate, err := updateRequired(ctx, client, opURL, []byte(operation.Body))
	if err != nil {
		return err
	}
	if !needsUpdate {
		return nil
	}
	logger := tracing.LoggerFromContext(ctx)
	logger.V(1).Info("Content is different, need to send PUT", "url", opURL, "body", operation.Body)
	put, err := http.NewRequest(http.MethodPut, opURL.String(), ioutil.NopCloser(bytes.NewBufferString(operation.Body)))
	if err != nil {
		return errors.WithStack(err)
	}
	// TODO emit errors as an event?
	resp, err := client.Request(ctx, put)
	errors.WithStack(err)

	// only bother parsing the response if debug logging is enabled
	/*
		Does provide useful debugging info so would likely be useful as an event
		2020-09-21T18:57:35.940-0500	DEBUG	esconfig-controller	Response from PUT	{"service.version": "1.3.0-SNAPSHOT+c9f2cc7d", "url": "/_snapshot/my_repository", "status_code": 500, "body": "{\"error\":{\"root_cause\":[{\"type\":\"repository_exception\",\"reason\":\"[my_repository] location [my_backup_location] doesn't match any of the locations specified by path.repo because this setting is empty\"}],\"type\":\"repository_exception\",\"reason\":\"[my_repository] failed to create repository\",\"caused_by\":{\"type\":\"repository_exception\",\"reason\":\"[my_repository] location [my_backup_location] doesn't match any of the locations specified by path.repo because this setting is empty\"}},\"status\":500}"}
	*/
	if logger.V(1).Enabled() {
		defer resp.Body.Close()
		respBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			// todo wrap in span
			return err
		}
		logger.V(1).Info("Response from PUT", "url", opURL, "status_code", resp.StatusCode, "body", string(respBytes))
		return nil
	}

	return err
}

func updateRequired(ctx context.Context, client esclient.Client, opURL *url.URL, body []byte) (bool, error) {
	get, err := http.NewRequest(http.MethodGet, opURL.String(), nil)
	if err != nil {
		return false, errors.WithStack(err)
	}
	// The Elasticsearch endpoint will be added automatically to the request URL which should therefore just be the path
	// with a leading /
	logger := tracing.LoggerFromContext(ctx)
	logger.V(1).Info("Requesting url", "url", opURL)
	ctx, cancel := context.WithTimeout(ctx, esclient.DefaultReqTimeout)
	defer cancel()
	// we handle errors by checking the status code
	getResp, _ := client.Request(ctx, get)

	// nothing exists at this url yet, time to create it
	if getResp.StatusCode == http.StatusNotFound {
		logger.V(1).Info("resource does not exist yet", "url", opURL)
		return true, nil
	}

	// TODO checking for a 200 might be wrong, really any status code not 200 might mean we need to update. but a 200 indicates we can and should compare the bodies
	// TODO should bodies always be required to be specified? I think probably? I don't know of any ES API that you can send an empty PUT. maybe we require at a minimum "{}"?
	if getResp.StatusCode != http.StatusOK {
		err = errors.New("status unacceptable")
		// TODO consider logging body of error here since it can have useful information
		logger.Error(err, "error getting current setting", "status_code", getResp.StatusCode, "url", opURL)
		return false, err
	}

	defer getResp.Body.Close()
	respBytes, err := ioutil.ReadAll(getResp.Body)
	if err != nil {
		return false, errors.WithStack(err)
	}

	// TODO would emitting the text difference be helpful?
	// Compare requires some "options" or will panic
	opts := jsondiff.DefaultConsoleOptions()
	diff, _ := jsondiff.Compare(respBytes, body, &opts)
	switch diff {
	case jsondiff.SupersetMatch, jsondiff.FullMatch:
		logger.V(1).Info("Content returned is a match, no action required", "url", opURL, "actual", string(respBytes), "expected", string(body))
		return false, nil
	case jsondiff.BothArgsAreInvalidJson:
		// should never happen since we parsed the expected body before
		err := errors.New("neither the expected nor actual body can be parsed successfully")
		logger.Error(err, "url", opURL, "actual", string(respBytes), "expected", string(body))
		return false, err
	case jsondiff.FirstArgIsInvalidJson:
		err := errors.New("the response body from the server cannot be parsed successfully")
		logger.Error(err, "url", opURL, "actual", string(respBytes), "expected", string(body))
		return false, err
	case jsondiff.SecondArgIsInvalidJson:
		// should never happen since we parsed the expected body before
		err := errors.New("the expected body in this operation cannot be parsed successfully")
		logger.Error(err, "url", opURL, "actual", string(respBytes), "expected", string(body))
		return false, err
	default:
		logger.V(1).Info("Content returned is a not a superset match, reconciliation required", "url", opURL, "actual", string(respBytes), "expected", string(body))
		return true, nil
	}
}
