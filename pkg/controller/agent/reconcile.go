// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"reflect"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/pkg/errors"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

func reconcilePodVehicle(params Params, podTemplate corev1.PodTemplateSpec, status agentv1alpha1.AgentStatus) (agentv1alpha1.AgentStatus, *reconciler.Results) {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	spec := params.Agent.Spec
	name := Name(params.Agent.Name)

	var toDelete client.Object
	var reconciliationFunc func(params ReconciliationParams) (int32, int32, error)
	switch {
	case spec.DaemonSet != nil:
		reconciliationFunc = reconcileDaemonSet
		toDelete = &v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: params.Agent.Namespace,
			},
		}
	case spec.Deployment != nil:
		reconciliationFunc = reconcileDeployment
		toDelete = &v1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: params.Agent.Namespace,
			},
		}
	}

	ready, desired, err := reconciliationFunc(ReconciliationParams{
		client:      params.Client,
		agent:       params.Agent,
		podTemplate: podTemplate,
	})

	if err != nil {
		return status, results.WithError(err)
	}

	// clean up the other one
	if err := params.Client.Get(context.Background(), types.NamespacedName{
		Namespace: params.Agent.Namespace,
		Name:      name,
	}, toDelete); err == nil {
		results.WithError(params.Client.Delete(context.Background(), toDelete))
	} else if !apierrors.IsNotFound(err) {
		results.WithError(err)
	}

	if err = calculateStatus(&params, ready, desired, &status); err != nil {
		err = errors.Wrap(err, "while calculating status")
	}

	return status, results.WithError(err)
}

func reconcileDeployment(rp ReconciliationParams) (int32, int32, error) {
	d := deployment.New(deployment.Params{
		Name:            Name(rp.agent.Name),
		Namespace:       rp.agent.Namespace,
		Selector:        NewLabels(rp.agent),
		Labels:          NewLabels(rp.agent),
		PodTemplateSpec: rp.podTemplate,
		Replicas:        pointer.Int32OrDefault(rp.agent.Spec.Deployment.Replicas, int32(1)),
		Strategy:        rp.agent.Spec.Deployment.Strategy,
	})
	if err := controllerutil.SetControllerReference(&rp.agent, &d, scheme.Scheme); err != nil {
		return 0, 0, err
	}

	reconciled, err := deployment.Reconcile(rp.client, d, &rp.agent)
	if err != nil {
		return 0, 0, err
	}

	return reconciled.Status.ReadyReplicas, reconciled.Status.Replicas, nil
}

func reconcileDaemonSet(rp ReconciliationParams) (int32, int32, error) {
	ds := daemonset.New(daemonset.Params{
		PodTemplate: rp.podTemplate,
		Name:        Name(rp.agent.Name),
		Owner:       &rp.agent,
		Labels:      NewLabels(rp.agent),
		Selectors:   NewLabels(rp.agent),
		Strategy:    rp.agent.Spec.DaemonSet.UpdateStrategy,
	})

	if err := controllerutil.SetControllerReference(&rp.agent, &ds, scheme.Scheme); err != nil {
		return 0, 0, err
	}

	reconciled, err := daemonset.Reconcile(rp.client, ds, &rp.agent)
	if err != nil {
		return 0, 0, err
	}

	return reconciled.Status.NumberReady, reconciled.Status.DesiredNumberScheduled, nil
}

// ReconciliationParams are the parameters used during an Elastic Agent's reconciliation.
type ReconciliationParams struct {
	client      k8s.Client
	agent       agentv1alpha1.Agent
	podTemplate corev1.PodTemplateSpec
}

// calculateStatus will calculate a new status from the state of the pods within the k8s cluster
// and will return any error encountered.
func calculateStatus(params *Params, ready, desired int32, status *agentv1alpha1.AgentStatus) error {
	agent := params.Agent

	pods, err := k8s.PodsMatchingLabels(params.Client, agent.Namespace, map[string]string{NameLabelName: agent.Name})
	if err != nil {
		return err
	}
	status.Version = common.LowestVersionFromPods(status.Version, pods, VersionLabelName)
	status.AvailableNodes = ready
	status.ExpectedNodes = desired
	health, err := CalculateHealth(agent.GetAssociations(), ready, desired)
	if err != nil {
		return err
	}
	status.Health = health
	return nil
}

// updateStatus will update the Elastic Agent's status within the k8s cluster, using the given Elastic Agent and status.
func updateStatus(agent agentv1alpha1.Agent, client client.Client, status agentv1alpha1.AgentStatus) error {
	if reflect.DeepEqual(agent.Status, status) {
		return nil
	}
	agent.Status = status
	return common.UpdateStatus(client, &agent)
}
