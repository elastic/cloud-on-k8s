// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/configs"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// Params are a set of parameters used during internal reconciliation of Logstash.
type Params struct {
	Context context.Context

	Client        k8s.Client
	EventRecorder record.EventRecorder
	Watches       watches.DynamicWatches

	Logstash logstashv1alpha1.Logstash
	Status   logstashv1alpha1.LogstashStatus

	OperatorParams    operator.Parameters
	KeystoreResources *keystore.Resources
	APIServerConfig   configs.APIServer // resolved API server config

	// Expectations control some expectations set on resources in the cache, in order to
	// avoid doing certain operations if the cache hasn't seen an up-to-date resource yet.
	Expectations *expectations.Expectations
}

// K8sClient returns the Kubernetes client.
func (p Params) K8sClient() k8s.Client {
	return p.Client
}

// Recorder returns the Kubernetes event recorder.
func (p Params) Recorder() record.EventRecorder {
	return p.EventRecorder
}

// DynamicWatches returns the set of stateful dynamic watches used during reconciliation.
func (p Params) DynamicWatches() watches.DynamicWatches {
	return p.Watches
}

// GetPodTemplate returns the configured pod template for the associated Elastic Logstash.
func (p *Params) GetPodTemplate() corev1.PodTemplateSpec {
	return p.Logstash.Spec.PodTemplate
}

// Logger returns the configured logger for use during reconciliation.
func (p *Params) Logger() logr.Logger {
	return log.FromContext(p.Context)
}

func newStatus(logstash logstashv1alpha1.Logstash) logstashv1alpha1.LogstashStatus {
	status := logstash.Status
	status.ObservedGeneration = logstash.Generation
	return status
}

func internalReconcile(params Params) (*reconciler.Results, logstashv1alpha1.LogstashStatus) {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	// ensure that the label used by expectations is set as it is not in place if the logstash
	// resource was created with ECK < 2.12
	if err := ensureSTSNameLabelIsSetOnPods(params); err != nil {
		return results.WithError(err), params.Status
	}

	_, apiSvc, err := reconcileServices(params)
	if err != nil {
		return results.WithError(err), params.Status
	}

	apiSvcTLS := params.Logstash.APIServerTLSOptions()

	_, results = certificates.Reconciler{
		K8sClient:             params.Client,
		DynamicWatches:        params.Watches,
		Owner:                 &params.Logstash,
		TLSOptions:            apiSvcTLS,
		Namer:                 logstashv1alpha1.Namer,
		Labels:                labels.NewLabels(params.Logstash),
		Services:              []corev1.Service{apiSvc},
		GlobalCA:              params.OperatorParams.GlobalCA,
		CACertRotation:        params.OperatorParams.CACertRotation,
		CertRotation:          params.OperatorParams.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(params.Context)
	if results.HasError() {
		_, err := results.Aggregate()
		k8s.MaybeEmitErrorEvent(params.Recorder(), err, &params.Logstash, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return results, params.Status
	}

	configHash := fnv.New32a()

	if _, params.APIServerConfig, err = reconcileConfig(params, apiSvcTLS.Enabled(), configHash); err != nil {
		return results.WithError(err), params.Status
	}

	// reconcile beats config secrets if Stack Monitoring is defined
	if err := stackmon.ReconcileConfigSecrets(params.Context, params.Client, params.Logstash, params.APIServerConfig); err != nil {
		return results.WithError(err), params.Status
	}

	// We intentionally DO NOT pass the configHash here. We don't want to consider the pipeline definitions in the
	// hash of the config to ensure that a pipeline change does not automatically trigger a restart
	// of the pod, but allows Logstash's automatic reload of pipelines to take place
	if err := reconcilePipeline(params); err != nil {
		return results.WithError(err), params.Status
	}

	params.Logstash.Spec.VolumeClaimTemplates = volume.AppendDefaultPVCs(params.Logstash.Spec.VolumeClaimTemplates,
		params.Logstash.Spec.PodTemplate.Spec)

	if keystoreResources, err := reconcileKeystore(params, configHash); err != nil {
		return results.WithError(err), params.Status
	} else if keystoreResources != nil {
		params.KeystoreResources = keystoreResources
	}

	podTemplate, err := buildPodTemplate(params, configHash)
	if err != nil {
		return results.WithError(err), params.Status
	}
	return reconcileStatefulSet(params, podTemplate)
}

// expectationsSatisfied checks that resources in our local cache match what we expect.
// If not, it's safer to not move on with StatefulSets and Pods reconciliation.
func (p *Params) expectationsSatisfied(ctx context.Context) (bool, string, error) {
	log := ulog.FromContext(ctx)
	// make sure the cache is up-to-date
	expectationsOK, reason, err := p.Expectations.Satisfied()
	if err != nil {
		return false, "Cache is not up to date", err
	}
	if !expectationsOK {
		log.V(1).Info("Cache expectations are not satisfied yet, re-queueing", "namespace", p.Logstash.Namespace, "ls_name", p.Logstash.Name, "reason", reason)
		return false, reason, nil
	}
	actualStatefulSet, err := retrieveActualStatefulSet(p.Client, p.Logstash)
	notFound := apierrors.IsNotFound(err)

	if err != nil && !notFound {
		return false, "Cannot retrieve actual stateful sets", err
	}

	if !notFound {
		// make sure StatefulSet statuses have been reconciled by the StatefulSet controller
		pendingStatefulSetReconciliation := isPendingReconciliation(actualStatefulSet)
		if pendingStatefulSetReconciliation {
			log.V(1).Info("StatefulSets observedGeneration is not reconciled yet, re-queueing", "namespace", p.Logstash.Namespace, "ls_name", p.Logstash.Name)
			return false, fmt.Sprintf("observedGeneration is not reconciled yet for StatefulSet %s", actualStatefulSet.Name), nil
		}
	}
	return podReconciliationDone(ctx, p.Client, actualStatefulSet)
}

func podReconciliationDone(ctx context.Context, c k8s.Client, sset appsv1.StatefulSet) (bool, string, error) {
	return statefulset.PodReconciliationDone(ctx, c, sset, labels.StatefulSetNameLabelName)
}

func isPendingReconciliation(sset appsv1.StatefulSet) bool {
	return sset.Generation != sset.Status.ObservedGeneration
}

func ensureSTSNameLabelIsSetOnPods(params Params) error {
	sts, err := retrieveActualStatefulSet(params.Client, params.Logstash)
	if apierrors.IsNotFound(err) {
		// maybe the sts doesn't exist yet or was deleted, let it be (re)created
		return nil
	}
	if err != nil {
		return err
	}
	if val, ok := sts.Spec.Template.Labels[labels.StatefulSetNameLabelName]; ok && (val == logstashv1alpha1.Name(params.Logstash.Name)) {
		// label is already in place, great
		return nil
	}
	// add the missing label and update the sts resource
	if sts.Spec.Template.Labels == nil {
		sts.Spec.Template.Labels = map[string]string{}
	}
	sts.Spec.Template.Labels[labels.StatefulSetNameLabelName] = logstashv1alpha1.Name(params.Logstash.Name)
	return params.Client.Update(params.Context, &sts)
}
