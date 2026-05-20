// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"fmt"
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

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/pointer"
)

func reconcilePodVehicle(params Params, podTemplate corev1.PodTemplateSpec) (*reconciler.Results, agentv1alpha1.AgentStatus) {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	spec := params.Agent.Spec
	name := Name(params.Agent.Name)
	rp := ReconciliationParams{
		ctx:         params.Context,
		meta:        params.Meta,
		client:      params.Client,
		agent:       params.Agent,
		podTemplate: podTemplate,
	}

	var toDelete []client.Object
	var expectedVehicle client.Object
	var reconciliationFunc func(params ReconciliationParams, object client.Object) (int32, int32, error)
	switch {
	case spec.DaemonSet != nil:
		ds, err := buildExpectedDaemonSet(rp)
		if err != nil {
			return results.WithError(err), params.Status
		}
		expectedVehicle = &ds
		reconciliationFunc = reconcileDaemonSet
		toDelete = append(toDelete,
			&v1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: params.Agent.Namespace,
				},
			},
			&v1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: params.Agent.Namespace,
				},
			},
		)
	case spec.Deployment != nil:
		d, err := buildExpectedDeployment(rp)
		if err != nil {
			return results.WithError(err), params.Status
		}
		expectedVehicle = &d
		reconciliationFunc = reconcileDeployment
		toDelete = append(toDelete,
			&v1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: params.Agent.Namespace,
				},
			},
			&v1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: params.Agent.Namespace,
				},
			},
		)
	case spec.StatefulSet != nil:
		sts, err := buildExpectedStatefulSet(rp)
		if err != nil {
			return results.WithError(err), params.Status
		}
		expectedVehicle = &sts
		reconciliationFunc = reconcileStatefulSet
		toDelete = append(toDelete,
			&v1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: params.Agent.Namespace,
				},
			},
			&v1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: params.Agent.Namespace,
				},
			},
		)
	}

	if common.IsOrchestrationPaused(&params.Agent) {
		err := common.SetPausedConditionAndEmitEvent(params.Context, params.Client, params.EventRecorder,
			&params.Agent, expectedVehicle)
		params.Status.Conditions = params.Status.Conditions.MergeWith(params.Agent.Status.Conditions...)
		if !resourceIsSteady(params.Agent) {
			return results.WithError(err).WithRequeue(reconciler.DefaultRequeue), params.Status
		}
		return results.WithError(err), params.Status
	}
	common.MaybeResetPausedCondition(params.Recorder(), &params.Agent)
	params.Status.Conditions = params.Status.Conditions.MergeWith(params.Agent.Status.Conditions...)

	ready, desired, err := reconciliationFunc(rp, expectedVehicle)
	if err != nil {
		return results.WithError(err), params.Status
	}

	for _, obj := range toDelete {
		// clean up the other ones
		if err := params.Client.Get(params.Context, types.NamespacedName{
			Namespace: params.Agent.Namespace,
			Name:      name,
		}, obj); err == nil {
			results.WithError(params.Client.Delete(params.Context, obj))
		} else if !apierrors.IsNotFound(err) {
			results.WithError(err)
		}
	}

	var status agentv1alpha1.AgentStatus
	if status, err = calculateStatus(&params, ready, desired); err != nil {
		err = errors.Wrap(err, "while calculating status")
	}

	return results.WithError(err), status
}

func buildExpectedDeployment(rp ReconciliationParams) (v1.Deployment, error) {
	d := deployment.New(deployment.Params{
		Name:                 Name(rp.agent.Name),
		Namespace:            rp.agent.Namespace,
		Selector:             rp.agent.GetIdentityLabels(),
		Metadata:             rp.meta,
		PodTemplateSpec:      rp.podTemplate,
		Replicas:             pointer.Int32OrDefault(rp.agent.Spec.Deployment.Replicas, int32(1)),
		RevisionHistoryLimit: rp.agent.Spec.RevisionHistoryLimit,
		Strategy:             rp.agent.Spec.Deployment.Strategy,
	})
	if err := controllerutil.SetControllerReference(&rp.agent, &d, scheme.Scheme); err != nil {
		return v1.Deployment{}, err
	}
	return deployment.WithTemplateHash(d), nil
}

func reconcileDeployment(rp ReconciliationParams, obj client.Object) (int32, int32, error) {
	expected, ok := obj.(*v1.Deployment)
	if !ok {
		return 0, 0, fmt.Errorf("%T is not a Deployment", obj)
	}
	reconciled, err := deployment.Reconcile(rp.ctx, rp.client, *expected, &rp.agent)
	if err != nil {
		return 0, 0, err
	}
	return reconciled.Status.ReadyReplicas, reconciled.Status.Replicas, nil
}

func buildExpectedStatefulSet(rp ReconciliationParams) (v1.StatefulSet, error) {
	s := statefulset.New(statefulset.Params{
		Name:                 Name(rp.agent.Name),
		Namespace:            rp.agent.Namespace,
		ServiceName:          rp.agent.Spec.StatefulSet.ServiceName,
		Selector:             rp.agent.GetIdentityLabels(),
		Metadata:             rp.meta,
		PodTemplateSpec:      rp.podTemplate,
		VolumeClaimTemplates: rp.agent.Spec.StatefulSet.VolumeClaimTemplates,
		Replicas:             pointer.Int32OrDefault(rp.agent.Spec.StatefulSet.Replicas, int32(1)),
		PodManagementPolicy:  rp.agent.Spec.StatefulSet.PodManagementPolicy,
		RevisionHistoryLimit: rp.agent.Spec.RevisionHistoryLimit,
	})
	if err := controllerutil.SetControllerReference(&rp.agent, &s, scheme.Scheme); err != nil {
		return v1.StatefulSet{}, err
	}
	return statefulset.WithTemplateHash(s), nil
}

func reconcileStatefulSet(rp ReconciliationParams, obj client.Object) (int32, int32, error) {
	expected, ok := obj.(*v1.StatefulSet)
	if !ok {
		return 0, 0, fmt.Errorf("%T is not a StatefulSet", obj)
	}
	reconciled, err := statefulset.Reconcile(rp.ctx, rp.client, *expected, &rp.agent)
	if err != nil {
		return 0, 0, err
	}
	return reconciled.Status.ReadyReplicas, reconciled.Status.Replicas, nil
}

func buildExpectedDaemonSet(rp ReconciliationParams) (v1.DaemonSet, error) {
	ds := daemonset.New(daemonset.Params{
		PodTemplate:          rp.podTemplate,
		Name:                 Name(rp.agent.Name),
		Owner:                &rp.agent,
		Metadata:             rp.meta,
		Selectors:            rp.agent.GetIdentityLabels(),
		RevisionHistoryLimit: rp.agent.Spec.RevisionHistoryLimit,
		Strategy:             rp.agent.Spec.DaemonSet.UpdateStrategy,
	})
	if err := controllerutil.SetControllerReference(&rp.agent, &ds, scheme.Scheme); err != nil {
		return v1.DaemonSet{}, err
	}
	return daemonset.WithTemplateHash(ds), nil
}

func reconcileDaemonSet(rp ReconciliationParams, obj client.Object) (int32, int32, error) {
	expected, ok := obj.(*v1.DaemonSet)
	if !ok {
		return 0, 0, fmt.Errorf("%T is not a DaemonSet", obj)
	}
	reconciled, err := daemonset.Reconcile(rp.ctx, rp.client, *expected, &rp.agent)
	if err != nil {
		return 0, 0, err
	}
	return reconciled.Status.NumberReady, reconciled.Status.DesiredNumberScheduled, nil
}

// resourceIsSteady returns whether the underlying Agent resource is in its ready state.
func resourceIsSteady(agent agentv1alpha1.Agent) bool {
	return agent.Status.ObservedGeneration == agent.Generation &&
		agent.Status.ExpectedNodes == agent.Status.AvailableNodes
}

// ReconciliationParams are the parameters used during an Elastic Agent's reconciliation.
type ReconciliationParams struct {
	ctx         context.Context
	meta        metadata.Metadata
	client      k8s.Client
	agent       agentv1alpha1.Agent
	podTemplate corev1.PodTemplateSpec
}

// calculateStatus will calculate a new status from the state of the pods within the k8s cluster
// and will return any error encountered.
func calculateStatus(params *Params, ready, desired int32) (agentv1alpha1.AgentStatus, error) {
	agent := params.Agent
	status := params.Status

	pods, err := k8s.PodsMatchingLabels(params.Client, agent.Namespace, map[string]string{NameLabelName: agent.Name})
	if err != nil {
		return status, err
	}

	status.Version = common.LowestVersionFromPods(params.Context, status.Version, pods, VersionLabelName)
	status.AvailableNodes = ready
	status.ExpectedNodes = desired
	health, err := CalculateHealth(agent.GetAssociations(), ready, desired)
	if err != nil {
		return status, err
	}
	status.Health = health
	return status, nil
}

// updateStatus will update the Elastic Agent's status within the k8s cluster, using the given Elastic Agent and status.
func updateStatus(ctx context.Context, agent agentv1alpha1.Agent, client client.Client, status agentv1alpha1.AgentStatus) error {
	if reflect.DeepEqual(agent.Status, status) {
		return nil
	}
	agent.Status = status
	return common.UpdateStatus(ctx, client, &agent)
}
