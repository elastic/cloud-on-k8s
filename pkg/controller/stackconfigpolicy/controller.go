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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	commonesclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	commonlabels "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/filesettings"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	controllerName = "stackconfigpolicy-controller"
)

var (
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}
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

func reconcileRequestForSoftOwnerPolicy() handler.TypedEventHandler[*corev1.Secret] {
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
func reconcileRequestForAllPolicies(clnt k8s.Client) handler.TypedEventHandler[client.Object] {
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
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
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
		return results.WithResult(reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Minute}), status
	}
	// run validation in case the webhook is disabled
	if err := r.validate(ctx, &policy); err != nil {
		status.Phase = policyv1alpha1.InvalidPhase
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
		results.WithResult(defaultRequeue)
	}

	return results, status
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
			return results.WithResult(defaultRequeue), status
		}
		if err != nil {
			return results.WithError(err), status
		}

		// check that there is no other policy that already owns the Settings Secret
		currentOwner, ok := filesettings.CanBeOwnedBy(actualSettingsSecret, policy)
		if !ok {
			err = fmt.Errorf("conflict: resource Elasticsearch %s/%s already configured by StackConfigpolicy %s/%s", es.Namespace, es.Name, currentOwner.Namespace, currentOwner.Name)
			r.recorder.Eventf(&policy, corev1.EventTypeWarning, events.EventReasonUnexpected, err.Error())
			results.WithError(err)
			err = status.AddPolicyErrorFor(esNsn, policyv1alpha1.ConflictPhase, err.Error(), policyv1alpha1.ElasticsearchResourceType)
			if err != nil {
				return results.WithError(err), status
			}
			continue
		}

		// create the expected Settings Secret
		expectedSecret, expectedVersion, err := filesettings.NewSettingsSecretWithVersion(esNsn, &actualSettingsSecret, &policy)
		if err != nil {
			return results.WithError(err), status
		}

		if err := filesettings.ReconcileSecret(ctx, r.Client, expectedSecret, &es); err != nil {
			return results.WithError(err), status
		}

		// Copy all the Secrets that are present in spec.elasticsearch.secretMounts
		if err := reconcileSecretMounts(ctx, r.Client, es, &policy); err != nil {
			if apierrors.IsNotFound(err) {
				err = status.AddPolicyErrorFor(esNsn, policyv1alpha1.ErrorPhase, err.Error(), policyv1alpha1.ElasticsearchResourceType)
				if err != nil {
					return results.WithError(err), status
				}
				results.WithResult(defaultRequeue)
			}
			continue
		}

		// create expected elasticsearch config secret
		expectedConfigSecret, err := newElasticsearchConfigSecret(policy, es)
		if err != nil {
			return results.WithError(err), status
		}

		if err = filesettings.ReconcileSecret(ctx, r.Client, expectedConfigSecret, &es); err != nil {
			return results.WithError(err), status
		}

		// Check if required Elasticsearch config and secret mounts are applied.
		configAndSecretMountsApplied, err := elasticsearchConfigAndSecretMountsApplied(ctx, r.Client, policy, es)
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
			results.WithResult(defaultRequeue)
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

		// check that there is no other policy that already owns the kibana config secret
		currentOwner, ok, err := canBeOwned(ctx, r.Client, policy, kibana)
		if err != nil {
			return results.WithError(err), status
		}

		// record error if already owned by another stack config policy
		if !ok {
			err := fmt.Errorf("conflict: resource Kibana %s/%s already configured by StackConfigpolicy %s/%s", kibana.Namespace, kibana.Name, currentOwner.Namespace, currentOwner.Name)
			r.recorder.Eventf(&policy, corev1.EventTypeWarning, events.EventReasonUnexpected, err.Error())
			results.WithError(err)
			if err := status.AddPolicyErrorFor(kibanaNsn, policyv1alpha1.ConflictPhase, err.Error(), policyv1alpha1.KibanaResourceType); err != nil {
				return results.WithError(err), status
			}
			continue
		}

		// Create the Secret that holds the Kibana configuration.
		if policy.Spec.Kibana.Config != nil {
			// Only add to configured resources if Kibana config is set.
			// This will help clean up the config secret if config gets removed from the stack config policy.
			configuredResources[kibanaNsn] = kibana
			expectedConfigSecret, err := newKibanaConfigSecret(policy, kibana)
			if err != nil {
				return results.WithError(err), status
			}

			if err = filesettings.ReconcileSecret(ctx, r.Client, expectedConfigSecret, &kibana); err != nil {
				return results.WithError(err), status
			}
		}

		// Check if required Kibana configs are applied.
		configApplied, err := kibanaConfigApplied(r.Client, policy, kibana)
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
