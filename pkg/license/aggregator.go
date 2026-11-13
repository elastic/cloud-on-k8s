// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	essettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/logstash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/pointer"
)

// aggregator aggregates the total of resources of all Elastic managed components
type aggregator struct {
	client k8s.Client
}

type aggregate func(ctx context.Context) (managedMemory, error)

// aggregateMemory aggregates the total memory of all Elastic managed components
func (a aggregator) aggregateMemory(ctx context.Context) (memoryUsage, error) {
	usage := newMemoryUsage()

	for _, f := range []aggregate{
		a.aggregateElasticsearchMemory,
		a.aggregateKibanaMemory,
		a.aggregateApmServerMemory,
		a.aggregateEnterpriseSearchMemory,
		a.aggregateLogstashMemory,
		a.aggregateFleetServerMemory,
	} {
		memory, err := f(ctx)
		if err != nil {
			return memoryUsage{}, err
		}
		usage.add(memory)
	}

	return usage, nil
}

func (a aggregator) aggregateElasticsearchMemory(ctx context.Context) (managedMemory, error) {
	var esList esv1.ElasticsearchList
	err := a.client.List(ctx, &esList)
	if err != nil {
		return managedMemory{}, errors.Wrap(err, "failed to aggregate Elasticsearch memory")
	}

	var total resource.Quantity
	for _, es := range esList.Items {
		for _, nodeSet := range es.Spec.NodeSets {
			mem, err := containerMemLimits(
				containersWithResourceOverrides(
					esv1.ElasticsearchContainerName,
					nodeSet.PodTemplate,
					nodeSet.Resources.ContainerResources(),
				),
				esv1.ElasticsearchContainerName,
				essettings.EnvEsJavaOpts,
				memFromJavaOpts,
				nodespec.DefaultMemoryLimits,
			)
			if err != nil {
				return managedMemory{}, errors.Wrap(err, "failed to aggregate Elasticsearch memory")
			}

			total.Add(multiply(mem, nodeSet.Count))
			ulog.FromContext(ctx).V(1).Info("Collecting", "namespace", es.Namespace, "es_name", es.Name,
				"memory", mem.String(), "count", nodeSet.Count)
		}
	}

	return managedMemory{total, elasticsearchKey}, nil
}

func (a aggregator) aggregateEnterpriseSearchMemory(ctx context.Context) (managedMemory, error) {
	var entList entv1.EnterpriseSearchList
	err := a.client.List(ctx, &entList)
	if err != nil {
		return managedMemory{}, errors.Wrap(err, "failed to aggregate Enterprise Search memory")
	}

	var total resource.Quantity
	for _, ent := range entList.Items {
		mem, err := containerMemLimits(
			containersWithResourceOverrides(
				entv1.EnterpriseSearchContainerName,
				ent.Spec.PodTemplate,
				ent.Spec.Resources,
			),
			entv1.EnterpriseSearchContainerName,
			enterprisesearch.EnvJavaOpts,
			memFromJavaOpts,
			enterprisesearch.DefaultMemoryLimits,
		)
		if err != nil {
			return managedMemory{}, errors.Wrap(err, "failed to aggregate Enterprise Search memory")
		}

		total.Add(multiply(mem, ent.Spec.Count))
		ulog.FromContext(ctx).V(1).Info("Collecting", "namespace", ent.Namespace, "ent_name", ent.Name,
			"memory", mem.String(), "count", ent.Spec.Count)
	}

	return managedMemory{total, entSearchKey}, nil
}

func (a aggregator) aggregateKibanaMemory(ctx context.Context) (managedMemory, error) {
	var kbList kbv1.KibanaList
	err := a.client.List(ctx, &kbList)
	if err != nil {
		return managedMemory{}, errors.Wrap(err, "failed to aggregate Kibana memory")
	}

	var total resource.Quantity
	for _, kb := range kbList.Items {
		// a Kibana renders one Deployment per active pool (a single pool, or the UI and the background tasks pools),
		// each with its own pod template and resources shorthand
		for _, role := range kb.ActiveRoles() {
			podTemplate, err := kibana.PodTemplateSpecForRole(kb, role)
			if err != nil {
				return managedMemory{}, errors.Wrap(err, "failed to aggregate Kibana memory")
			}
			mem, err := containerMemLimits(
				containersWithResourceOverrides(kbv1.KibanaContainerName, podTemplate, kb.GetResourcesForRole(role)),
				kbv1.KibanaContainerName,
				kibana.EnvNodeOptions, memFromNodeOptions,
				kibana.DefaultMemoryLimits,
			)
			if err != nil {
				return managedMemory{}, errors.Wrap(err, "failed to aggregate Kibana memory")
			}

			count, err := a.kibanaPoolReplicas(ctx, kb, role)
			if err != nil {
				return managedMemory{}, errors.Wrap(err, "failed to aggregate Kibana memory")
			}

			total.Add(multiply(mem, count))
			ulog.FromContext(ctx).V(1).Info("Collecting", "namespace", kb.Namespace, "kibana_name", kb.Name,
				"role", role.LabelValue, "memory", mem.String(), "count", count)
		}
	}

	return managedMemory{total, kibanaKey}, nil
}

// kibanaPoolReplicas returns the number of replicas of the given Kibana pool.
// The spec is authoritative for the single pool, the UI pool and a background tasks pool with a count as the
// controller owns their replicas. A background tasks pool without a count is scaled by an HPA, its replicas are read
// from the live Deployment.
func (a aggregator) kibanaPoolReplicas(ctx context.Context, kb kbv1.Kibana, role kblabel.Role) (int32, error) {
	if !role.IsBackgroundTasks() {
		return kb.Spec.Count, nil
	}

	if kb.Spec.BackgroundTasks.Count != nil {
		return *kb.Spec.BackgroundTasks.Count, nil
	}

	var dp appsv1.Deployment
	nsn := types.NamespacedName{Namespace: kb.Namespace, Name: kbv1.BackgroundTasksDeployment(kb.Name)}
	if err := a.client.Get(ctx, nsn, &dp); client.IgnoreNotFound(err) != nil {
		return 0, err
	}
	// a missing Deployment or nil replicas both resolve to the Kubernetes default of 1 replica
	return pointer.Int32OrDefault(dp.Spec.Replicas, 1), nil
}

func (a aggregator) aggregateLogstashMemory(ctx context.Context) (managedMemory, error) {
	var lsList lsv1alpha1.LogstashList
	err := a.client.List(ctx, &lsList)
	if err != nil {
		return managedMemory{}, errors.Wrap(err, "failed to aggregate Logstash memory")
	}

	var total resource.Quantity
	for _, ls := range lsList.Items {
		mem, err := containerMemLimits(
			containersWithResourceOverrides(
				lsv1alpha1.LogstashContainerName,
				ls.Spec.PodTemplate,
				ls.Spec.Resources,
			),
			lsv1alpha1.LogstashContainerName,
			logstash.EnvJavaOpts, memFromJavaOpts,
			logstash.DefaultMemoryLimit,
		)
		if err != nil {
			return managedMemory{}, errors.Wrap(err, "failed to aggregate Logstash memory")
		}

		total.Add(multiply(mem, ls.Spec.Count))
		ulog.FromContext(ctx).V(1).Info("Collecting", "namespace", ls.Namespace, "logstash_name", ls.Name,
			"memory", mem.String(), "count", ls.Spec.Count)
	}

	return managedMemory{total, logstashKey}, nil
}

func (a aggregator) aggregateApmServerMemory(ctx context.Context) (managedMemory, error) {
	var asList apmv1.ApmServerList
	err := a.client.List(ctx, &asList)
	if err != nil {
		return managedMemory{}, errors.Wrap(err, "failed to aggregate APM Server memory")
	}

	var total resource.Quantity
	for _, as := range asList.Items {
		mem, err := containerMemLimits(
			containersWithResourceOverrides(
				apmv1.ApmServerContainerName,
				as.Spec.PodTemplate,
				as.Spec.Resources,
			),
			apmv1.ApmServerContainerName,
			"", nil, // no fallback with limits defined in an env var
			apmserver.DefaultMemoryLimits,
		)
		if err != nil {
			return managedMemory{}, errors.Wrap(err, "failed to aggregate APM Server memory")
		}

		total.Add(multiply(mem, as.Spec.Count))
		ulog.FromContext(ctx).V(1).Info("Collecting", "namespace", as.Namespace, "as_name", as.Name,
			"memory", mem.String(), "count", as.Spec.Count)
	}

	return managedMemory{total, apmKey}, nil
}

// aggregateFleetServerMemory aggregates the memory of the Elastic Agents running Fleet Server.
// Regular Elastic Agents are not billable, but count towards the license when used to deploy Fleet Server.
func (a aggregator) aggregateFleetServerMemory(ctx context.Context) (managedMemory, error) {
	var agentList agentv1alpha1.AgentList
	err := a.client.List(ctx, &agentList)
	if err != nil {
		return managedMemory{}, errors.Wrap(err, "failed to aggregate Fleet Server memory")
	}

	var total resource.Quantity
	for _, agt := range agentList.Items {
		if !agt.Spec.FleetServerEnabled {
			continue
		}

		mem, err := containerMemLimits(
			containersWithResourceOverrides(
				agentv1alpha1.AgentContainerName,
				agentPodTemplate(agt),
				agt.Spec.Resources,
			),
			agentv1alpha1.AgentContainerName,
			"", nil, // no fallback with limits defined in an env var
			agent.DefaultFleetMemoryLimits,
		)
		if err != nil {
			return managedMemory{}, errors.Wrap(err, "failed to aggregate Fleet Server memory")
		}

		count, err := a.fleetServerReplicas(ctx, agt)
		if err != nil {
			return managedMemory{}, errors.Wrap(err, "failed to aggregate Fleet Server memory")
		}

		total.Add(multiply(mem, count))
		ulog.FromContext(ctx).V(1).Info("Collecting", "namespace", agt.Namespace, "agent_name", agt.Name,
			"memory", mem.String(), "count", count)
	}

	return managedMemory{total, fleetServerKey}, nil
}

// agentPodTemplate returns the pod template of the configured Agent deployment mode.
func agentPodTemplate(agt agentv1alpha1.Agent) corev1.PodTemplateSpec {
	switch {
	case agt.Spec.DaemonSet != nil:
		return agt.Spec.DaemonSet.PodTemplate
	case agt.Spec.Deployment != nil:
		return agt.Spec.Deployment.PodTemplate
	case agt.Spec.StatefulSet != nil:
		return agt.Spec.StatefulSet.PodTemplate
	}
	return corev1.PodTemplateSpec{}
}

// fleetServerReplicas returns the number of replicas of the given Fleet Server Agent.
// The spec is authoritative for a Deployment or a StatefulSet as the controller always sets their replicas and the
// Agent resource cannot be scaled by an HPA. The number of DaemonSet pods depends on the number of Kubernetes nodes
// and is read from the live object.
func (a aggregator) fleetServerReplicas(ctx context.Context, agt agentv1alpha1.Agent) (int32, error) {
	switch {
	case agt.Spec.DaemonSet != nil:
		var ds appsv1.DaemonSet
		nsn := types.NamespacedName{Namespace: agt.Namespace, Name: agent.Name(agt.Name)}
		if err := a.client.Get(ctx, nsn, &ds); err != nil {
			// the number of nodes the DaemonSet will cover is unknown until the DaemonSet exists
			return 0, client.IgnoreNotFound(err)
		}
		return ds.Status.DesiredNumberScheduled, nil
	case agt.Spec.Deployment != nil:
		return pointer.Int32OrDefault(agt.Spec.Deployment.Replicas, 1), nil
	case agt.Spec.StatefulSet != nil:
		return pointer.Int32OrDefault(agt.Spec.StatefulSet.Replicas, 1), nil
	}
	return 0, nil // no deployment mode configured, nothing is deployed
}

// containerMemLimits reads the container memory limits from the resource specification with fallback
// on the environment variable and on the default limits.
// The containers are expected to carry the resources shorthand already, see containersWithResourceOverrides.
func containerMemLimits(
	containers []corev1.Container,
	containerName string,
	envVarName string,
	envLookup func(envVar string) (resource.Quantity, error),
	defaultLimit resource.Quantity,
) (resource.Quantity, error) {
	var mem resource.Quantity
	for _, container := range containers {
		//nolint:nestif
		if container.Name == containerName {
			mem = *container.Resources.Limits.Memory()

			// if memory is defined at the container level, maybe fallback to the environment variable
			if envLookup != nil && mem.IsZero() {
				for _, envVar := range container.Env {
					if envVar.Name == envVarName {
						var err error
						mem, err = envLookup(envVar.Value)
						if err != nil {
							return resource.Quantity{}, err
						}
					}
				}
			}
		}
	}

	// if still no memory found, fallback to the default limits
	if mem.IsZero() {
		mem = defaultLimit
	}

	return mem, nil
}

// containersWithResourceOverrides returns the containers of the pod template with the resources shorthand applied
// to the main container, mirroring how the controllers build the pod template.
// The operator default resources are deliberately not applied, so that the memory is read in the documented order:
// the resources shorthand, the pod template memory limit, the application environment variable, the default limit.
func containersWithResourceOverrides(containerName string, podTemplateSpec corev1.PodTemplateSpec, overrides commonv1.Resources) []corev1.Container {
	b := defaults.NewPodTemplateBuilder(podTemplateSpec, containerName).
		WithResourcesAndOverrides(corev1.ResourceRequirements{}, overrides)

	return b.PodTemplate.Spec.Containers
}

// maxHeapSizeRe is the pattern to extract the max Java heap size (-Xmx<size>[g|G|m|M|k|K] in binary units)
var maxHeapSizeRe = regexp.MustCompile(`-Xmx([0-9]+)([gGmMkK]?)(?:\s.*|$)`)

// memFromJavaOpts extracts the maximum Java heap size from a Java options string, multiplies the value by 2
// (giving twice the JVM memory to the container is a common thing people do)
// and converts it to a resource.Quantity
// If no value is found the function returns the 0 value.
func memFromJavaOpts(javaOpts string) (resource.Quantity, error) {
	match := maxHeapSizeRe.FindStringSubmatch(javaOpts)
	if match == nil {
		// Xmx is not set, return a 0 quantity
		return resource.Quantity{}, nil
	}
	if len(match) != 3 {
		return resource.Quantity{}, errors.Errorf("cannot extract max jvm heap size from %s", javaOpts)
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return resource.Quantity{}, err
	}
	suffix := match[2]
	if suffix != "" {
		// capitalize the suffix and add `i` to have a surjection of [g|G|m|M|k|K] in [Gi|Mi|Ki]
		suffix = strings.ToUpper(match[2]) + "i"
	}
	// multiply by 2 and convert it to a quantity using the suffix
	return resource.ParseQuantity(fmt.Sprintf("%d%s", value*2, suffix))
}

// nodeHeapSizeRe is the pattern to extract the max heap size of the node memory (--max-old-space-size=<mb_size>)
var nodeHeapSizeRe = regexp.MustCompile("--max-old-space-size=([0-9]*)")

// memFromNodeOptions extracts the Node heap size from a Node options string and converts it to a resource.Quantity
func memFromNodeOptions(nodeOpts string) (resource.Quantity, error) {
	match := nodeHeapSizeRe.FindStringSubmatch(nodeOpts)
	if len(match) != 2 {
		return resource.Quantity{}, errors.Errorf("cannot extract max node heap size from %s", nodeOpts)
	}

	return resource.ParseQuantity(match[1] + "M")
}

// multiply multiplies a resource.Quantity by a value
func multiply(q resource.Quantity, v int32) resource.Quantity {
	var result resource.Quantity
	result.Set(q.Value() * int64(v))
	return result
}
