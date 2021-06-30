// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"context"
	"crypto/sha256"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

const (
	FleetServerPort int32 = 8220
)

type Params struct {
	Context context.Context

	Client        k8s.Client
	EventRecorder record.EventRecorder
	Watches       watches.DynamicWatches

	Agent agentv1alpha1.Agent
}

func (p Params) K8sClient() k8s.Client {
	return p.Client
}

func (p Params) Recorder() record.EventRecorder {
	return p.EventRecorder
}

func (p Params) DynamicWatches() watches.DynamicWatches {
	return p.Watches
}

func (p *Params) GetPodTemplate() corev1.PodTemplateSpec {
	if p.Agent.Spec.DaemonSet != nil {
		return p.Agent.Spec.DaemonSet.PodTemplate
	}

	return p.Agent.Spec.Deployment.PodTemplate
}

func (p *Params) Logger() logr.Logger {
	return log.FromContext(p.Context)
}

func internalReconcile(params Params) *reconciler.Results {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	agentVersion, err := version.Parse(params.Agent.Spec.Version)
	if err != nil {
		return results.WithError(err)
	}
	if !association.AllowVersion(agentVersion, &params.Agent, params.Logger(), params.EventRecorder) {
		return results // will eventually retry
	}

	configHash := sha256.New224()
	if res := reconcileConfig(params, configHash); res.HasError() {
		return results.WithResults(res)
	}

	// we need to deref the secret here (if any) to include it in the configHash otherwise Agent will not be rolled on content changes
	if err := commonassociation.WriteAssocsToConfigHash(params.Client, params.Agent.GetAssociations(), configHash); err != nil {
		return results.WithError(err)
	}

	err = ReconcileService(params)
	if err != nil {
		return results.WithError(err)
	}

	podTemplate := buildPodTemplate(params /*keystoreResources,*/, configHash)
	return results.WithResults(reconcilePodVehicle(params, podTemplate))
}

func ReconcileService(params Params) error {
	svc := NewService(params.Agent)

	// setup Service only when Fleet Server is enabled
	if !params.Agent.Spec.EnableFleetServer {
		// clean up if it was previously set up
		if err := params.Client.Get(params.Context, k8s.ExtractNamespacedName(svc), svc); err == nil {
			err := params.Client.Delete(params.Context, svc)
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}

		return nil
	}

	_, err := common.ReconcileService(params.Context, params.Client, svc, &params.Agent)
	return err
}

func NewService(agent agentv1alpha1.Agent) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: agent.Spec.HTTP.Service.ObjectMeta,
		Spec:       agent.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = agent.Namespace
	svc.ObjectMeta.Name = HttpServiceName(agent.Name)

	labels := NewLabels(agent)
	ports := []corev1.ServicePort{
		{
			Name:     agent.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     FleetServerPort,
		},
	}
	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}
