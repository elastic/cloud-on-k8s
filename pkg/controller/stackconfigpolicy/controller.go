// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	commonlabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	controllerName = "stackconfigpolicy-controller"
)

var (
	// defaultRequeue is the default requeue interval for this controller. It is longer than the default interval used elsewhere to account
	// for secret propagation times and the time it takes for Elasticsearch to observe the updates.
	defaultRequeue = 30 * time.Second
)

// Add creates a new StackConfigPolicy Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r)
}

// newReconciler returns a new reconcile.Reconciler of StackConfigPolicy.
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileStackConfigPolicy {
	k8sClient := mgr.GetClient()
	return &ReconcileStackConfigPolicy{
		Client:           k8sClient,
		esClientProvider: commonesclient.NewClient,
		recorder:         mgr.GetEventRecorderFor(controllerName),
		licenseChecker:   license.NewLicenseChecker(k8sClient, params.OperatorNamespace),
		params:           params,
		dynamicWatches:   watches.NewDynamicWatches(),
	}
}

func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileStackConfigPolicy) error {
	// watch for changes to StackConfigPolicy
	if err := c.Watch(source.Kind(mgr.GetCache(), &policyv1alpha1.StackConfigPolicy{}, &handler.TypedEnqueueRequestForObject[*policyv1alpha1.StackConfigPolicy]{})); err != nil {
		return err
	}

	// watch for changes to Elasticsearch and reconcile all StackConfigPolicy
	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &esv1.Elasticsearch{}, reconcileRequestForAllPolicies(r.Client))); err != nil {
		return err
	}

	// watch for changes to Kibana and reconcile all StackConfigPolicy
	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &kibanav1.Kibana{}, reconcileRequestForAllPolicies(r.Client))); err != nil {
		return err
	}

	// watch Secrets soft owned by StackConfigPolicy
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, reconcileRequestForSoftOwnerPolicy())); err != nil {
		return err
	}

	// watch dynamically refrenced secrets
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
}

func reconcileRequestForSoftOwnerPolicy() handler.TypedEventHandler[*corev1.Secret, reconcile.Request] {
	return handler.TypedEnqueueRequestsFromMapFunc[*corev1.Secret](func(ctx context.Context, secret *corev1.Secret) []reconcile.Request {
		softOwner, referenced := reconciler.SoftOwnerRefFromLabels(secret.GetLabels())
		if !referenced || softOwner.Kind != policyv1alpha1.Kind {
			return nil
		}
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{Namespace: softOwner.Namespace, Name: softOwner.Name}},
		}
	})
}

// requestsAllStackConfigPolicies returns the requests to reconcile all StackConfigPolicy resources.
func reconcileRequestForAllPolicies(clnt k8s.Client) handler.TypedEventHandler[client.Object, reconcile.Request] {
	return handler.TypedEnqueueRequestsFromMapFunc[client.Object](func(ctx context.Context, es client.Object) []reconcile.Request {
		var stackConfigList policyv1alpha1.StackConfigPolicyList
		err := clnt.List(context.Background(), &stackConfigList)
		if err != nil {
			ulog.Log.Error(err, "Fail to list StackConfigurationList while watching Elasticsearch")
			return nil
		}
		requests := make([]reconcile.Request, 0)
		for _, stackConfig := range stackConfigList.Items {
			stackConfig := stackConfig
			requests = append(requests, reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&stackConfig)})
		}
		return requests
	})
}

var _ reconcile.Reconciler = &ReconcileStackConfigPolicy{}

// ReconcileStackConfigPolicy reconciles a StackConfigPolicy object
type ReconcileStackConfigPolicy struct {
	k8s.Client
	esClientProvider commonesclient.Provider
	recorder         record.EventRecorder
	licenseChecker   license.Checker
	params           operator.Parameters
	dynamicWatches   watches.DynamicWatches
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for a StackConfigPolicy object and makes changes based on the state read and what is
// in the StackConfigPolicy.Spec.
func (r *ReconcileStackConfigPolicy) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.params.Tracer, controllerName, "policy_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	// retrieve the StackConfigPolicy resource
	var policy policyv1alpha1.StackConfigPolicy
	err := r.Client.Get(ctx, request.NamespacedName, &policy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(ctx,
				types.NamespacedName{
					Namespace: request.Namespace,
					Name:      request.Name,
				})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// skip unmanaged resources
	if common.IsUnmanaged(ctx, &policy) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	// the StackConfigPolicy will be deleted nothing to do other than remove the watches
	if policy.IsMarkedForDeletion() {
		return reconcile.Result{}, r.onDelete(ctx, k8s.ExtractNamespacedName(&policy))
	}

	// main reconciliation logic
	results, status := r.doReconcile(ctx, policy)

	// update status
	if err := r.updateStatus(ctx, policy, status); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithRequeue().Aggregate()
		}
		results.WithError(err)
	}

	return results.Aggregate()
}

// esMap is a type alias for a Map of Elasticsearch indexed by NamespaceName useful to manipulate the Elasticsearch
// clusters configured by a StackConfigPolicy.
type esMap map[types.NamespacedName]esv1.Elasticsearch

// kbMap is a type alias for a Map of Kibana indexed by NamespacedName useful to manipulate the Kibana
// instances configured by a StackConfigPolicy.
type kbMap map[types.NamespacedName]kibanav1.Kibana

func (r *ReconcileStackConfigPolicy) doReconcile(ctx context.Context, policy policyv1alpha1.StackConfigPolicy) (*reconciler.Results, policyv1alpha1.StackConfigPolicyStatus) {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Reconcile StackConfigPolicy")

	results := reconciler.NewResult(ctx)
	status := policyv1alpha1.NewStatus(policy)
	defer status.Update()

	// Enterprise license check
	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return results.WithError(err), status
	}
	if !enabled {
		msg := "StackConfigPolicy is an enterprise feature. Enterprise features are disabled"
		log.Info(msg)
		r.recorder.Eventf(&policy, corev1.EventTypeWarning, events.EventReconciliationError, msg)
		// we don't have a good way of watching for the license level to change so just requeue with a reasonably long delay
		return results.WithRequeue(5 * time.Minute), status
	}
	// run validation in case the webhook is disabled
	if err := r.validate(ctx, &policy); err != nil {
		status.Phase = policyv1alpha1.InvalidPhase
		r.recorder.Eventf(&policy, corev1.EventTypeWarning, events.EventReasonValidation, err.Error())
		return results.WithError(err), status
	}

	// check for weight conflicts with other policies
	if err := r.checkWeightConflicts(ctx, &policy); err != nil {
		status.Phase = policyv1alpha1.ConflictPhase
		r.recorder.Eventf(&policy, corev1.EventTypeWarning, events.EventReasonValidation, err.Error())
		return results.WithError(err), status
	}

	// reconcile elasticsearch resources
	results, status = r.reconcileElasticsearchResources(ctx, policy, status)

	// reconcile kibana resources
	kibanaResults, status := r.reconcileKibanaResources(ctx, policy, status)

	// Combine results from kibana reconciliation with results from Elasticsearch reconciliation
	results.WithResults(kibanaResults)

	// requeue if not ready
	if status.Phase != policyv1alpha1.ReadyPhase {
		results.WithRequeue(defaultRequeue)
	}

	return results, status
}

// findPoliciesForElasticsearch finds all StackConfigPolicies that target a given Elasticsearch cluster
func (r *ReconcileStackConfigPolicy) findPoliciesForElasticsearch(ctx context.Context, es esv1.Elasticsearch) ([]policyv1alpha1.StackConfigPolicy, error) {
	var allPolicies policyv1alpha1.StackConfigPolicyList
	err := r.Client.List(ctx, &allPolicies)
	if err != nil {
		return nil, err
	}

	var matchingPolicies []policyv1alpha1.StackConfigPolicy
	for _, policy := range allPolicies.Items {
		// Check if policy's resource selector matches this Elasticsearch
		selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.ResourceSelector)
		if err != nil {
			continue // Skip malformed selectors
		}

		// Check namespace restrictions
		if policy.Namespace != r.params.OperatorNamespace && policy.Namespace != es.Namespace {
			continue
		}

		if selector.Matches(labels.Set(es.Labels)) {
			matchingPolicies = append(matchingPolicies, policy)
		}
	}

	return matchingPolicies, nil
}

// findPoliciesForKibana finds all StackConfigPolicies that target a given Kibana instance
func (r *ReconcileStackConfigPolicy) findPoliciesForKibana(ctx context.Context, kibana kibanav1.Kibana) ([]policyv1alpha1.StackConfigPolicy, error) {
	var allPolicies policyv1alpha1.StackConfigPolicyList
	err := r.Client.List(ctx, &allPolicies)
	if err != nil {
		return nil, err
	}

	var matchingPolicies []policyv1alpha1.StackConfigPolicy
	for _, policy := range allPolicies.Items {
		// Check if policy's resource selector matches this Kibana
		selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.ResourceSelector)
		if err != nil {
			continue // Skip malformed selectors
		}

		// Check namespace restrictions
		if policy.Namespace != r.params.OperatorNamespace && policy.Namespace != kibana.Namespace {
			continue
		}

		if selector.Matches(labels.Set(kibana.Labels)) {
			matchingPolicies = append(matchingPolicies, policy)
		}
	}

	return matchingPolicies, nil
}

func (r *ReconcileStackConfigPolicy) reconcileElasticsearchResources(ctx context.Context, policy policyv1alpha1.StackConfigPolicy, status policyv1alpha1.StackConfigPolicyStatus) (*reconciler.Results, policyv1alpha1.StackConfigPolicyStatus) {
	defer tracing.Span(&ctx)()
	log := ulog.FromContext(ctx)
	log.V(1).Info("Reconcile Elasticsearch resources")

	results := reconciler.NewResult(ctx)

	// prepare the selector to find Elastic resources to configure
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels:      policy.Spec.ResourceSelector.MatchLabels,
		MatchExpressions: policy.Spec.ResourceSelector.MatchExpressions,
	})
	if err != nil {
		return results.WithError(err), status
	}
	listOpts := client.ListOptions{LabelSelector: selector}

	// restrict the search to the policy namespace if it is different from the operator namespace
	if policy.Namespace != r.params.OperatorNamespace {
		listOpts.Namespace = policy.Namespace
	}

	// find the list of Elasticsearch to configure
	var esList esv1.ElasticsearchList
	if err := r.Client.List(ctx, &esList, &listOpts); err != nil {
		return results.WithError(err), status
	}

	configuredResources := esMap{}
	for _, es := range esList.Items {
		log.V(1).Info("Reconcile StackConfigPolicy", "es_namespace", es.Namespace, "es_name", es.Name)
		es := es

		// keep the list of ES to be configured
		esNsn := k8s.ExtractNamespacedName(&es)
		configuredResources[esNsn] = es

		// version gate for the ES file-based settings feature
		v, err := version.Parse(es.Spec.Version)
		if err != nil {
			return results.WithError(err), status
		}
		if v.LT(filesettings.FileBasedSettingsMinPreVersion) {
			err = fmt.Errorf("invalid version to configure resource Elasticsearch %s/%s: actual %s, expected >= %s", es.Namespace, es.Name, v, filesettings.FileBasedSettingsMinVersion)
			r.recorder.Eventf(&policy, corev1.EventTypeWarning, events.EventReasonUnexpected, err.Error())
			results.WithError(err)
			err = status.AddPolicyErrorFor(esNsn, policyv1alpha1.ErrorPhase, err.Error(), policyv1alpha1.ElasticsearchResourceType)
			if err != nil {
				return results.WithError(err), status
			}
			continue
		}

		// the file Settings Secret must exist, if not it will be created empty by the ES controller
		var actualSettingsSecret corev1.Secret
		err = r.Client.Get(ctx, types.NamespacedName{Namespace: es.Namespace, Name: esv1.FileSettingsSecretName(es.Name)}, &actualSettingsSecret)
		if err != nil && apierrors.IsNotFound(err) {
			// requeue if the Secret has not been created yet
			return results.WithRequeue(defaultRequeue), status
		}
		if err != nil {
			return results.WithError(err), status
		}

		// Find all policies that target this Elasticsearch cluster
		allPolicies, err := r.findPoliciesForElasticsearch(ctx, es)
		if err != nil {
			return results.WithError(err), status
		}

		// extract the metadata that should be propagated to children
		meta := metadata.Propagate(&es, metadata.Metadata{Labels: eslabel.NewLabels(k8s.ExtractNamespacedName(&es))})

		// create the expected Settings Secret from all applicable policies
		var expectedSecret corev1.Secret
		var expectedVersion int64
		switch len(allPolicies) {
		case 0:
			// No policies target this resource - skip (shouldn't happen in practice)
			continue
		case 1:
			// Single policy - use the original approach for backward compatibility
			expectedSecret, expectedVersion, err = filesettings.NewSettingsSecretWithVersion(esNsn, &actualSettingsSecret, &allPolicies[0], meta)
		default:
			// Multiple policies - use the multi-policy approach
			expectedSecret, expectedVersion, err = filesettings.NewSettingsSecretWithVersionFromPolicies(esNsn, &actualSettingsSecret, allPolicies, meta)
		}
		if err != nil {
			return results.WithError(err), status
		}

		if err := filesettings.ReconcileSecret(ctx, r.Client, expectedSecret, &es); err != nil {
			return results.WithError(err), status
		}

		// Handle secret mounts and config from all policies
		if err := r.reconcileSecretMountsFromPolicies(ctx, es, allPolicies, meta); err != nil {
			if apierrors.IsNotFound(err) {
				err = status.AddPolicyErrorFor(esNsn, policyv1alpha1.ErrorPhase, err.Error(), policyv1alpha1.ElasticsearchResourceType)
				if err != nil {
					return results.WithError(err), status
				}
				results.WithRequeue(defaultRequeue)
			}
			continue
		}

		// create expected elasticsearch config secret from all policies
		expectedConfigSecret, err := r.newElasticsearchConfigSecretFromPolicies(allPolicies, es)
		if err != nil {
			return results.WithError(err), status
		}

		if err = filesettings.ReconcileSecret(ctx, r.Client, expectedConfigSecret, &es); err != nil {
			return results.WithError(err), status
		}

		// Check if required Elasticsearch config and secret mounts are applied from all policies.
		configAndSecretMountsApplied, err := r.elasticsearchConfigAndSecretMountsAppliedFromPolicies(ctx, allPolicies, es)
		if err != nil {
			return results.WithError(err), status
		}

		// get /_cluster/state to get the Settings currently configured in ES
		currentSettings, err := r.getClusterStateFileSettings(ctx, es)
		if err != nil {
			err = status.AddPolicyErrorFor(esNsn, policyv1alpha1.UnknownPhase, err.Error(), policyv1alpha1.ElasticsearchResourceType)
			if err != nil {
				return results.WithError(err), status
			}
			// requeue if ES not reachable
			results.WithRequeue(defaultRequeue)
		}

		// update the ES resource status for this ES
		err = status.UpdateResourceStatusPhase(esNsn, newElasticsearchResourceStatus(currentSettings, expectedVersion), configAndSecretMountsApplied, policyv1alpha1.ElasticsearchResourceType)
		if err != nil {
			return results.WithError(err), status
		}
	}

	// Add dynamic watches on the additional secret mounts
	// This will also remove dynamic watches for secrets that no longer are refrenced in the stackconfigpolicy
	if err = r.addDynamicWatchesOnAdditionalSecretMounts(policy); err != nil {
		return results.WithError(err), status
	}

	// reset/delete Settings secrets for resources no longer selected by this policy
	results.WithError(handleOrphanSoftOwnedSecrets(ctx, r.Client, k8s.ExtractNamespacedName(&policy), configuredResources, nil, policyv1alpha1.ElasticsearchResourceType))

	return results, status
}

func (r *ReconcileStackConfigPolicy) reconcileKibanaResources(ctx context.Context, policy policyv1alpha1.StackConfigPolicy, status policyv1alpha1.StackConfigPolicyStatus) (*reconciler.Results, policyv1alpha1.StackConfigPolicyStatus) {
	defer tracing.Span(&ctx)()
	log := ulog.FromContext(ctx)
	log.V(1).Info("Reconcile Kibana Resources")

	results := reconciler.NewResult(ctx)

	// prepare the selector to find Kibana resources to configure
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels:      policy.Spec.ResourceSelector.MatchLabels,
		MatchExpressions: policy.Spec.ResourceSelector.MatchExpressions,
	})
	if err != nil {
		return results.WithError(err), status
	}
	listOpts := client.ListOptions{LabelSelector: selector}

	// restrict the search to the policy namespace if it is different from the operator namespace
	if policy.Namespace != r.params.OperatorNamespace {
		listOpts.Namespace = policy.Namespace
	}

	// find the list of Kibana to configure
	var kibanaList kibanav1.KibanaList
	if err := r.Client.List(ctx, &kibanaList, &listOpts); err != nil {
		return results.WithError(err), status
	}

	configuredResources := kbMap{}
	for _, kibana := range kibanaList.Items {
		log.V(1).Info("Reconcile StackConfigPolicy", "kibana_namespace", kibana.Namespace, "kibana_name", kibana.Name)
		kibana := kibana

		// keep the list of Kibana to be configured
		kibanaNsn := k8s.ExtractNamespacedName(&kibana)

		// Find all policies that target this Kibana instance
		allPolicies, err := r.findPoliciesForKibana(ctx, kibana)
		if err != nil {
			return results.WithError(err), status
		}

		// Check if any policy has Kibana config
		hasKibanaConfig := false
		for _, p := range allPolicies {
			if p.Spec.Kibana.Config != nil {
				hasKibanaConfig = true
				break
			}
		}

		// Create the Secret that holds the Kibana configuration from all policies.
		if hasKibanaConfig {
			// Only add to configured resources if at least one policy has Kibana config set.
			configuredResources[kibanaNsn] = kibana

			var expectedConfigSecret corev1.Secret

			switch len(allPolicies) {
			case 0:
				// No policies target this resource - skip (shouldn't happen in practice)
				continue
			case 1:
				// Single policy - use the original approach for backward compatibility
				expectedConfigSecret, err = newKibanaConfigSecret(allPolicies[0], kibana)
			default:
				// Multiple policies - use the multi-policy approach
				expectedConfigSecret, err = r.newKibanaConfigSecretFromPolicies(allPolicies, kibana)
			}

			if err != nil {
				return results.WithError(err), status
			}

			if err = filesettings.ReconcileSecret(ctx, r.Client, expectedConfigSecret, &kibana); err != nil {
				return results.WithError(err), status
			}
		}

		// Check if required Kibana configs from all policies are applied.
		var configApplied bool
		if len(allPolicies) > 1 {
			configApplied, err = r.kibanaConfigAppliedFromPolicies(allPolicies, kibana)
		} else if len(allPolicies) == 1 {
			configApplied, err = kibanaConfigApplied(r.Client, allPolicies[0], kibana)
		} else {
			configApplied = true // No policies, so nothing to apply
		}
		if err != nil {
			return results.WithError(err), status
		}

		// update the Kibana resource status for this Kibana
		err = status.UpdateResourceStatusPhase(kibanaNsn, policyv1alpha1.ResourcePolicyStatus{}, configApplied, policyv1alpha1.KibanaResourceType)
		if err != nil {
			return results.WithError(err), status
		}
	}

	// delete Settings secrets for resources no longer selected by this policy
	results.WithError(deleteOrphanSoftOwnedSecrets(ctx, r.Client, k8s.ExtractNamespacedName(&policy), nil, configuredResources, policyv1alpha1.KibanaResourceType))

	return results, status
}

func newElasticsearchResourceStatus(currentSettings esclient.FileSettings, expectedVersion int64) policyv1alpha1.ResourcePolicyStatus {
	status := policyv1alpha1.ResourcePolicyStatus{
		CurrentVersion:  currentSettings.Version,
		ExpectedVersion: expectedVersion,
	}
	if currentSettings.Errors != nil {
		status.Error = policyv1alpha1.PolicyStatusError{
			Version: currentSettings.Errors.Version,
			Message: cleanStackTrace(currentSettings.Errors.Errors),
		}
	}
	return status
}

var (
	matchTabsAtSpaces         = regexp.MustCompile("[\t]+at\\s")
	matchTripleDotsNumberMore = regexp.MustCompile("... [0-9]+ more")
)

func cleanStackTrace(errors []string) string {
	for i, e := range errors {
		var msg []string
		for _, line := range strings.Split(e, "\n") {
			if matchTabsAtSpaces.MatchString(line) || matchTripleDotsNumberMore.MatchString(line) {
				continue
			}
			msg = append(msg, line)
		}
		errors[i] = strings.Trim(strings.Join(msg, "\n"), "\n")
	}
	return strings.Join(errors, ". ")
}

func (r *ReconcileStackConfigPolicy) validate(ctx context.Context, policy *policyv1alpha1.StackConfigPolicy) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := policy.ValidateCreate(); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, policy, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func (r *ReconcileStackConfigPolicy) updateStatus(ctx context.Context, scp policyv1alpha1.StackConfigPolicy, status policyv1alpha1.StackConfigPolicyStatus) error {
	span, _ := apm.StartSpan(ctx, "update_status", tracing.SpanTypeApp)
	defer span.End()

	if reflect.DeepEqual(status, scp.Status) {
		return nil // nothing to do
	}
	if status.IsDegraded(scp.Status) {
		r.recorder.Event(&scp, corev1.EventTypeWarning, events.EventReasonUnhealthy, "StackConfigPolicy health degraded")
	}
	ulog.FromContext(ctx).V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"status", status,
	)
	scp.Status = status
	return common.UpdateStatus(ctx, r.Client, &scp)
}

func (r *ReconcileStackConfigPolicy) onDelete(ctx context.Context, obj types.NamespacedName) error {
	defer tracing.Span(&ctx)()
	// Remove dynamic watches on secrets
	r.dynamicWatches.Secrets.RemoveHandlerForKey(additionalSecretMountsWatcherName(obj))
	// Send empty resource type so that we reset/delete secrets for configured elasticsearch and kibana clusters
	return handleOrphanSoftOwnedSecrets(ctx, r.Client, obj, nil, nil, "")
}

func handleOrphanSoftOwnedSecrets(
	ctx context.Context,
	c k8s.Client,
	softOwner types.NamespacedName,
	configuredESResources esMap,
	configuredKibanaResources kbMap,
	resourceType policyv1alpha1.ResourceType,
) error {
	err := resetOrphanSoftOwnedFileSettingSecrets(ctx, c, softOwner, configuredESResources, resourceType)
	if err != nil {
		return err
	}
	return deleteOrphanSoftOwnedSecrets(ctx, c, softOwner, configuredESResources, configuredKibanaResources, resourceType)
}

// resetOrphanSoftOwnedFileSettingSecrets resets secrets for the Elasticsearch clusters that are no longer configured
// by a given StackConfigPolicy.
// An optional list of Elasticsearch currently configured by the policy can be provided to filter secrets not to be modified. Without list,
// all secrets soft owned by the policy are reset.
func resetOrphanSoftOwnedFileSettingSecrets(
	ctx context.Context,
	c k8s.Client,
	softOwner types.NamespacedName,
	configuredESResources esMap,
	resourceType policyv1alpha1.ResourceType,
) error {
	log := ulog.FromContext(ctx)
	var secrets corev1.SecretList
	matchLabels := client.MatchingLabels{
		reconciler.SoftOwnerNamespaceLabel:              softOwner.Namespace,
		reconciler.SoftOwnerNameLabel:                   softOwner.Name,
		reconciler.SoftOwnerKindLabel:                   policyv1alpha1.Kind,
		commonlabels.StackConfigPolicyOnDeleteLabelName: commonlabels.OrphanSecretResetOnPolicyDelete,
	}

	if resourceType != "" {
		matchLabels[commonv1.TypeLabelName] = string(resourceType)
	}

	if err := c.List(ctx,
		&secrets,
		// search in all namespaces
		// restrict to secrets on which we set the soft owner labels
		matchLabels,
	); err != nil {
		return err
	}
	for i := range secrets.Items {
		s := secrets.Items[i]
		configuredApplicationType := s.Labels[commonv1.TypeLabelName]
		switch configuredApplicationType {
		case eslabel.Type:
			namespacedName := types.NamespacedName{
				Namespace: s.Namespace,
				Name:      s.Labels[eslabel.ClusterNameLabelName],
			}
			if _, exists := configuredESResources[namespacedName]; exists {
				continue
			}

			log.V(1).Info("Reconcile empty file settings Secret for Elasticsearch",
				"es_namespace", namespacedName.Namespace, "es_name", namespacedName.Name,
				"owner_namespace", softOwner.Namespace, "owner_name", softOwner.Name)

			var es esv1.Elasticsearch
			err := c.Get(ctx, namespacedName, &es)
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			if apierrors.IsNotFound(err) {
				// Elasticsearch has just been deleted
				return nil
			}

			if err := filesettings.ReconcileEmptyFileSettingsSecret(ctx, c, es, false); err != nil {
				return err
			}
		case kblabel.Type:
			// Currently we do not reset labels for kibana, so we shouldn't hit this.
			// Implement if needed in the future
			continue
		default:
			return fmt.Errorf("secret configured for unknown application type %s", configuredApplicationType)
		}
	}
	return nil
}

// deleteOrphanSoftOwnedSecrets deletes secrets for the Elasticsearch/Kibana clusters that are no longer configured
// by a given StackConfigPolicy.
func deleteOrphanSoftOwnedSecrets(
	ctx context.Context,
	c k8s.Client,
	softOwner types.NamespacedName,
	configuredESResources esMap,
	configuredKibanaResources kbMap,
	resourceType policyv1alpha1.ResourceType,
) error {
	var secrets corev1.SecretList
	matchLabels := client.MatchingLabels{
		reconciler.SoftOwnerNamespaceLabel:              softOwner.Namespace,
		reconciler.SoftOwnerNameLabel:                   softOwner.Name,
		reconciler.SoftOwnerKindLabel:                   policyv1alpha1.Kind,
		commonlabels.StackConfigPolicyOnDeleteLabelName: commonlabels.OrphanSecretDeleteOnPolicyDelete,
	}

	if resourceType != "" {
		matchLabels[commonv1.TypeLabelName] = string(resourceType)
	}
	if err := c.List(ctx,
		&secrets,
		// search in all namespaces
		// restrict to secrets on which we set the soft owner labels
		matchLabels,
	); err != nil {
		return err
	}

	for i := range secrets.Items {
		secret := secrets.Items[i]
		configuredApplicationType := secret.Labels[commonv1.TypeLabelName]

		switch configuredApplicationType {
		case eslabel.Type:
			namespacedName := types.NamespacedName{
				Namespace: secret.Namespace,
				Name:      secret.Labels[eslabel.ClusterNameLabelName],
			}
			// check if they exist in the es map
			if _, exist := configuredESResources[namespacedName]; exist {
				continue
			}
		case kblabel.Type:
			namespacedName := types.NamespacedName{
				Namespace: secret.Namespace,
				Name:      secret.Labels[kblabel.KibanaNameLabelName],
			}
			// check if they exist in the kb map
			if _, exist := configuredKibanaResources[namespacedName]; exist {
				continue
			}
		default:
			return fmt.Errorf("secret configured for unknown application type %s", configuredApplicationType)
		}

		// given kibana/elasticsearch cluster is no longer managed by stack config policy, delete secret.
		err := c.Delete(ctx, &secret)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// getClusterStateFileSettings gets the file based settings currently configured in an Elasticsearch by calling the /_cluster/state API.
func (r *ReconcileStackConfigPolicy) getClusterStateFileSettings(ctx context.Context, es esv1.Elasticsearch) (esclient.FileSettings, error) {
	span, _ := apm.StartSpan(ctx, "get_cluster_state", tracing.SpanTypeApp)
	defer span.End()

	esClient, err := r.esClientProvider(ctx, r.Client, r.params.Dialer, es)
	if err != nil {
		return esclient.FileSettings{}, err
	}
	defer esClient.Close()

	clusterState, err := esClient.GetClusterState(ctx)
	if err != nil {
		return esclient.FileSettings{}, err
	}

	return clusterState.Metadata.ReservedState.FileSettings, nil
}

func (r *ReconcileStackConfigPolicy) addDynamicWatchesOnAdditionalSecretMounts(policy policyv1alpha1.StackConfigPolicy) error {
	// Add watches if there are additional secrets to be mounted
	watcher := types.NamespacedName{
		Name:      policy.Name,
		Namespace: policy.Namespace,
	}

	var secretSources []commonv1.NamespacedSecretSource //nolint:prealloc
	for _, secretMount := range policy.Spec.Elasticsearch.SecretMounts {
		secretSources = append(secretSources, commonv1.NamespacedSecretSource{
			SecretName: secretMount.SecretName,
			Namespace:  policy.Namespace,
		})
	}

	// Add dynamic watches on the secrets
	return watches.WatchUserProvidedNamespacedSecrets(
		watcher,
		r.dynamicWatches,
		additionalSecretMountsWatcherName(watcher),
		secretSources,
	)
}

func additionalSecretMountsWatcherName(watcher types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-additional-secret-mounts-watcher", watcher.Name, watcher.Namespace)
}

// checkWeightConflicts validates that no other StackConfigPolicy has the same weight
// and would create conflicting configuration for the same resources
func (r *ReconcileStackConfigPolicy) checkWeightConflicts(ctx context.Context, policy *policyv1alpha1.StackConfigPolicy) error {
	var allPolicies policyv1alpha1.StackConfigPolicyList
	if err := r.Client.List(ctx, &allPolicies); err != nil {
		return fmt.Errorf("failed to list StackConfigPolicies for weight conflict check: %w", err)
	}

	policySelector, err := metav1.LabelSelectorAsSelector(&policy.Spec.ResourceSelector)
	if err != nil {
		return fmt.Errorf("invalid resource selector: %w", err)
	}

	// Group policies by weight to detect conflicts more efficiently
	policiesByWeight := make(map[int32][]policyv1alpha1.StackConfigPolicy)
	for _, otherPolicy := range allPolicies.Items {
		// Skip self
		if otherPolicy.Namespace == policy.Namespace && otherPolicy.Name == policy.Name {
			continue
		}
		policiesByWeight[otherPolicy.Spec.Weight] = append(policiesByWeight[otherPolicy.Spec.Weight], otherPolicy)
	}

	// Check if any policies with the same weight could target overlapping resources and have conflicting settings
	conflictingPolicies := policiesByWeight[policy.Spec.Weight]
	if len(conflictingPolicies) == 0 {
		return nil // No conflicts
	}

	for _, otherPolicy := range conflictingPolicies {
		if r.policiesCouldOverlap(policy, &otherPolicy, policySelector) {
			// Check if the policies have conflicting settings
			if r.policiesHaveConflictingSettings(policy, &otherPolicy) {
				return fmt.Errorf("weight conflict detected: StackConfigPolicy %s/%s has the same weight (%d) and would overwrite conflicting settings. Policies with the same weight that target overlapping resources must configure different, non-conflicting settings",
					otherPolicy.Namespace, otherPolicy.Name, policy.Spec.Weight)
			}
		}
	}

	return nil
}

// policiesHaveConflictingSettings checks if two policies would configure conflicting settings
// that would overwrite each other. Returns true if both policies configure the same setting keys,
// or if both policies have completely empty configurations (to maintain existing behavior).
func (r *ReconcileStackConfigPolicy) policiesHaveConflictingSettings(policy1, policy2 *policyv1alpha1.StackConfigPolicy) bool {
	// Check if both policies are essentially empty (no meaningful configuration)
	if r.policyIsEmpty(policy1) && r.policyIsEmpty(policy2) {
		return true // Both empty policies would conflict in the same namespace/selectors
	}

	// Check Elasticsearch settings for conflicts
	if r.elasticsearchSettingsConflict(policy1, policy2) {
		return true
	}

	// Check Kibana settings for conflicts
	if r.kibanaSettingsConflict(policy1, policy2) {
		return true
	}

	return false
}

// policyIsEmpty checks if a policy has no meaningful configuration
func (r *ReconcileStackConfigPolicy) policyIsEmpty(policy *policyv1alpha1.StackConfigPolicy) bool {
	es := &policy.Spec.Elasticsearch
	kb := &policy.Spec.Kibana

	// Check if Elasticsearch settings are empty
	esEmpty := (es.ClusterSettings == nil || len(es.ClusterSettings.Data) == 0) &&
		(es.SnapshotRepositories == nil || len(es.SnapshotRepositories.Data) == 0) &&
		(es.SnapshotLifecyclePolicies == nil || len(es.SnapshotLifecyclePolicies.Data) == 0) &&
		(es.SecurityRoleMappings == nil || len(es.SecurityRoleMappings.Data) == 0) &&
		(es.IndexLifecyclePolicies == nil || len(es.IndexLifecyclePolicies.Data) == 0) &&
		(es.IngestPipelines == nil || len(es.IngestPipelines.Data) == 0) &&
		(es.IndexTemplates.ComponentTemplates == nil || len(es.IndexTemplates.ComponentTemplates.Data) == 0) &&
		(es.IndexTemplates.ComposableIndexTemplates == nil || len(es.IndexTemplates.ComposableIndexTemplates.Data) == 0) &&
		(es.Config == nil || len(es.Config.Data) == 0) &&
		len(es.SecretMounts) == 0

	// Check if Kibana settings are empty
	kbEmpty := (kb.Config == nil || len(kb.Config.Data) == 0)

	return esEmpty && kbEmpty
}

// elasticsearchSettingsConflict checks if two policies have conflicting Elasticsearch settings
func (r *ReconcileStackConfigPolicy) elasticsearchSettingsConflict(policy1, policy2 *policyv1alpha1.StackConfigPolicy) bool {
	es1 := &policy1.Spec.Elasticsearch
	es2 := &policy2.Spec.Elasticsearch

	// Check each type of setting for key conflicts
	if r.configsConflict(es1.ClusterSettings, es2.ClusterSettings) {
		return true
	}
	if r.configsConflict(es1.SnapshotRepositories, es2.SnapshotRepositories) {
		return true
	}
	if r.configsConflict(es1.SnapshotLifecyclePolicies, es2.SnapshotLifecyclePolicies) {
		return true
	}
	if r.configsConflict(es1.SecurityRoleMappings, es2.SecurityRoleMappings) {
		return true
	}
	if r.configsConflict(es1.IndexLifecyclePolicies, es2.IndexLifecyclePolicies) {
		return true
	}
	if r.configsConflict(es1.IngestPipelines, es2.IngestPipelines) {
		return true
	}
	if r.configsConflict(es1.IndexTemplates.ComponentTemplates, es2.IndexTemplates.ComponentTemplates) {
		return true
	}
	if r.configsConflict(es1.IndexTemplates.ComposableIndexTemplates, es2.IndexTemplates.ComposableIndexTemplates) {
		return true
	}
	if r.configsConflict(es1.Config, es2.Config) {
		return true
	}

	// Check secret mounts for path conflicts
	return r.secretMountsConflict(es1.SecretMounts, es2.SecretMounts)
}

// kibanaSettingsConflict checks if two policies have conflicting Kibana settings
func (r *ReconcileStackConfigPolicy) kibanaSettingsConflict(policy1, policy2 *policyv1alpha1.StackConfigPolicy) bool {
	kb1 := &policy1.Spec.Kibana
	kb2 := &policy2.Spec.Kibana

	return r.configsConflict(kb1.Config, kb2.Config)
}

// configsConflict checks if two Config objects have overlapping keys
func (r *ReconcileStackConfigPolicy) configsConflict(config1, config2 *commonv1.Config) bool {
	if config1 == nil || config2 == nil || config1.Data == nil || config2.Data == nil {
		return false
	}

	// Check if there are any common keys
	for key := range config1.Data {
		if _, exists := config2.Data[key]; exists {
			return true
		}
	}

	return false
}

// secretMountsConflict checks if two sets of secret mounts have overlapping mount paths
func (r *ReconcileStackConfigPolicy) secretMountsConflict(mounts1, mounts2 []policyv1alpha1.SecretMount) bool {
	if len(mounts1) == 0 || len(mounts2) == 0 {
		return false
	}

	paths1 := make(map[string]bool)
	for _, mount := range mounts1 {
		paths1[mount.MountPath] = true
	}

	for _, mount := range mounts2 {
		if paths1[mount.MountPath] {
			return true
		}
	}

	return false
}

// policiesCouldOverlap checks if two policies could potentially target the same resources
func (r *ReconcileStackConfigPolicy) policiesCouldOverlap(policy1, policy2 *policyv1alpha1.StackConfigPolicy, policy1Selector labels.Selector) bool {
	// Check namespace-based restrictions first
	if !r.namespacesCouldOverlap(policy1.Namespace, policy2.Namespace) {
		return false
	}

	// Parse policy2 selector
	policy2Selector, err := metav1.LabelSelectorAsSelector(&policy2.Spec.ResourceSelector)
	if err != nil {
		// If we can't parse the selector, assume they could overlap to be safe
		return true
	}

	// Check if selectors could match the same labels
	return r.selectorsCouldOverlap(policy1Selector, policy2Selector)
}

// namespacesCouldOverlap checks if two policies from different namespaces could target the same resources
// Based on the controller logic in reconcileElasticsearchResources and reconcileKibanaResources
func (r *ReconcileStackConfigPolicy) namespacesCouldOverlap(ns1, ns2 string) bool {
	// If both policies are in the same namespace, they can overlap
	if ns1 == ns2 {
		return true
	}

	// Check if either policy is in the operator namespace (can target resources in other namespaces)
	if ns1 == r.params.OperatorNamespace || ns2 == r.params.OperatorNamespace {
		return true
	}

	// Policies from different non-operator namespaces cannot overlap
	return false
}

// selectorsCouldOverlap checks if two label selectors could potentially match the same resources
func (r *ReconcileStackConfigPolicy) selectorsCouldOverlap(selector1, selector2 labels.Selector) bool {
	// If either selector matches everything, they overlap
	if selector1.Empty() || selector2.Empty() {
		return true
	}

	// Get requirements for both selectors
	reqs1, _ := selector1.Requirements()
	reqs2, _ := selector2.Requirements()

	// Create maps for easier lookup
	equalsReqs1 := make(map[string]map[string]bool)
	equalsReqs2 := make(map[string]map[string]bool)

	for _, req := range reqs1 {
		if req.Operator() == selection.Equals {
			if equalsReqs1[req.Key()] == nil {
				equalsReqs1[req.Key()] = make(map[string]bool)
			}
			for v := range req.Values() {
				equalsReqs1[req.Key()][v] = true
			}
		}
	}

	for _, req := range reqs2 {
		if req.Operator() == selection.Equals {
			if equalsReqs2[req.Key()] == nil {
				equalsReqs2[req.Key()] = make(map[string]bool)
			}
			for v := range req.Values() {
				equalsReqs2[req.Key()][v] = true
			}
		}
	}

	// Check for definitely disjoint selectors
	for key, values1 := range equalsReqs1 {
		if values2, exists := equalsReqs2[key]; exists {
			// Both selectors require the same key - check if value sets overlap
			hasOverlap := false
			for v := range values1 {
				if values2[v] {
					hasOverlap = true
					break
				}
			}
			if !hasOverlap {
				return false // Definitely no overlap for this key
			}
		}
	}

	// If we can't prove they're disjoint, assume they could overlap
	return true
}
