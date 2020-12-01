// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"context"
	"crypto/sha256"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

func NewParams(
	context context.Context,
	client k8s.Client,
	eventRecorder record.EventRecorder,
	watches watches.DynamicWatches,
	agent agentv1alpha1.Agent,
) Params {
	return Params{
		Context:       context,
		Logger:        log.FromContext(context),
		Client:        client,
		EventRecorder: eventRecorder,
		Watches:       watches,
		Agent:         agent,
	}
}

type Params struct {
	Context context.Context
	Logger  logr.Logger

	Client        k8s.Client
	EventRecorder record.EventRecorder
	Watches       watches.DynamicWatches

	Agent agentv1alpha1.Agent
}

func (dp Params) K8sClient() k8s.Client {
	return dp.Client
}

func (dp Params) Recorder() record.EventRecorder {
	return dp.EventRecorder
}

func (dp Params) DynamicWatches() watches.DynamicWatches {
	return dp.Watches
}

func (p *Params) GetPodTemplate() v1.PodTemplateSpec {
	if p.Agent.Spec.DaemonSet != nil {
		return p.Agent.Spec.DaemonSet.PodTemplate
	}

	return p.Agent.Spec.Deployment.PodTemplate
}

func internalReconcile(params Params) *reconciler.Results {
	results := reconciler.NewResult(params.Context)

	agentVersion, err := version.Parse(params.Agent.Spec.Version)
	if err != nil {
		return results.WithError(err)
	}
	if !association.AllowVersion(*agentVersion, &params.Agent, params.Logger, params.EventRecorder) {
		return results // will eventually retry
	}

	configHash := sha256.New224()
	if err := reconcileConfig(params, configHash); err != nil {
		return results.WithError(err)
	}

	// we need to deref the secret here (if any) to include it in the configHash otherwise Agent will not be rolled on content changes
	if err := commonassociation.WriteAssocsToConfigHash(params.Client, params.Agent.GetAssociations(), configHash); err != nil {
		return results.WithError(err)
	}

	// todo agent
	//keystoreResources, err := keystore.NewResources(
	//	params,
	//	&params.Beat,
	//	namer,
	//	NewLabels(params.Beat),
	//	initContainerParameters(params.Beat.Spec.Type),
	//)
	//if err != nil {
	//	return results.WithError(err)
	//}

	podTemplate := buildPodTemplate(params /*keystoreResources,*/, configHash)
	return results.WithResults(reconcilePodVehicle(params, podTemplate))
}

// secure settings
// configref
// status
// validate
