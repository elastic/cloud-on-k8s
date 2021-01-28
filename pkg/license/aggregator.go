// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	essettings "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Aggregator aggregates the total of resources of all Elastic managed components
type Aggregator struct {
	client k8s.Client
}

type aggregate func() (resource.Quantity, error)

// AggregateMemory aggregates the total memory of all Elastic managed components
func (a Aggregator) AggregateMemory() (resource.Quantity, error) {
	var totalMemory resource.Quantity

	for _, f := range []aggregate{
		a.aggregateElasticsearchMemory,
		a.aggregateKibanaMemory,
		a.aggregateApmServerMemory,
	} {
		memory, err := f()
		if err != nil {
			return resource.Quantity{}, err
		}
		totalMemory.Add(memory)
	}

	return totalMemory, nil
}

func (a Aggregator) aggregateElasticsearchMemory() (resource.Quantity, error) {
	var esList esv1.ElasticsearchList
	err := a.client.List(context.Background(), &esList)
	if err != nil {
		return resource.Quantity{}, errors.Wrap(err, "failed to aggregate Elasticsearch memory")
	}

	var total resource.Quantity
	for _, es := range esList.Items {
		for _, nodeSet := range es.Spec.NodeSets {
			mem, err := containerMemLimits(
				nodeSet.PodTemplate.Spec.Containers,
				esv1.ElasticsearchContainerName,
				essettings.EnvEsJavaOpts, memFromJavaOpts,
				nodespec.DefaultMemoryLimits,
			)
			if err != nil {
				return resource.Quantity{}, errors.Wrap(err, "failed to aggregate Elasticsearch memory")
			}

			total.Add(multiply(mem, nodeSet.Count))
			log.V(1).Info("Collecting", "namespace", es.Namespace, "es_name", es.Name,
				"memory", mem.String(), "count", nodeSet.Count)
		}
	}

	return total, nil
}

func (a Aggregator) aggregateKibanaMemory() (resource.Quantity, error) {
	var kbList kbv1.KibanaList
	err := a.client.List(context.Background(), &kbList)
	if err != nil {
		return resource.Quantity{}, errors.Wrap(err, "failed to aggregate Kibana memory")
	}

	var total resource.Quantity
	for _, kb := range kbList.Items {
		mem, err := containerMemLimits(
			kb.Spec.PodTemplate.Spec.Containers,
			kbv1.KibanaContainerName,
			kibana.EnvNodeOpts, memFromNodeOptions,
			kibana.DefaultMemoryLimits,
		)
		if err != nil {
			return resource.Quantity{}, errors.Wrap(err, "failed to aggregate Kibana memory")
		}

		total.Add(multiply(mem, kb.Spec.Count))
		log.V(1).Info("Collecting", "namespace", kb.Namespace, "kibana_name", kb.Name,
			"memory", mem.String(), "count", kb.Spec.Count)
	}

	return total, nil
}

func (a Aggregator) aggregateApmServerMemory() (resource.Quantity, error) {
	var asList apmv1.ApmServerList
	err := a.client.List(context.Background(), &asList)
	if err != nil {
		return resource.Quantity{}, errors.Wrap(err, "failed to aggregate APM Server memory")
	}

	var total resource.Quantity
	for _, as := range asList.Items {
		mem, err := containerMemLimits(
			as.Spec.PodTemplate.Spec.Containers,
			apmv1.ApmServerContainerName,
			"", nil, // no fallback with limits defined in an env var
			apmserver.DefaultMemoryLimits,
		)
		if err != nil {
			return resource.Quantity{}, errors.Wrap(err, "failed to aggregate APM Server memory")
		}

		total.Add(multiply(mem, as.Spec.Count))
		log.V(1).Info("Collecting", "namespace", as.Namespace, "as_name", as.Name,
			"memory", mem.String(), "count", as.Spec.Count)
	}

	return total, nil
}

// containerMemLimits reads the container memory limits from the resource specification with fallback
// on the environment variable and on the default limits
func containerMemLimits(
	containers []corev1.Container,
	containerName string,
	envVarName string,
	envLookup func(envVar string) (resource.Quantity, error),
	defaultLimit resource.Quantity,
) (resource.Quantity, error) {
	var mem resource.Quantity
	for _, container := range containers {
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

// maxHeapSizeRe is the pattern to extract the max Java heap size (-Xmx<size>[g|G|m|M|k|K] in binary units)
var maxHeapSizeRe = regexp.MustCompile(`-Xmx([0-9]+)([gGmMkK]?)(?:\s.+|$)`)

// memFromJavaOpts extracts the maximum Java heap size from a Java options string, multiplies the value by 2
// (giving twice the JVM memory to the container is a common thing people do)
// and converts it to a resource.Quantity
func memFromJavaOpts(javaOpts string) (resource.Quantity, error) {
	match := maxHeapSizeRe.FindStringSubmatch(javaOpts)
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
