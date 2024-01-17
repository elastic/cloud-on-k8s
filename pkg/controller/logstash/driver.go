// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"hash/fnv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/configs"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
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
		Labels:                NewLabels(params.Logstash),
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
