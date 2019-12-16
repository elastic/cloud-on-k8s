// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package resource

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	asv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	essettings "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	kbconfig "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/config"
	kbpod "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/pod"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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
	err := a.client.List(&esList)
	if err != nil {
		return resource.Quantity{}, err
	}

	var total resource.Quantity
	for _, es := range esList.Items {
		for _, nodeSet := range es.Spec.NodeSets {
			var mem resource.Quantity

			// read the container memory limits
			for _, container := range nodeSet.PodTemplate.Spec.Containers {
				if container.Name == esv1.ElasticsearchContainerName {
					mem = *container.Resources.Limits.Memory()

					// if not, fallback to twice the max JVM heap size
					if mem.IsZero() {
						for _, envVar := range container.Env {
							if envVar.Name == essettings.EnvEsJavaOpts {
								mem, err = memFromJavaOpts(envVar.Value)
								if err != nil {
									return resource.Quantity{}, err
								}
							}
						}
					}
				}
			}

			// if not, fallback to the default limits
			if mem.IsZero() {
				mem = nodespec.DefaultMemoryLimits
			}

			total.Add(multiply(mem, nodeSet.Count))
			log.V(1).Info("Collecting", "es_name", es.Name, "memory", mem.String(), "count", nodeSet.Count)
		}
	}

	return total, nil
}

func (a Aggregator) aggregateKibanaMemory() (resource.Quantity, error) {
	var kbList kbv1.KibanaList
	err := a.client.List(&kbList)
	if err != nil {
		return resource.Quantity{}, err
	}

	var total resource.Quantity
	for _, kb := range kbList.Items {
		var mem resource.Quantity

		// read the container memory limits
		for _, container := range kb.Spec.PodTemplate.Spec.Containers {
			if container.Name == kbv1.KibanaContainerName {
				mem = *container.Resources.Limits.Memory()

				// if not, fallback to the max JVM heap size
				if mem.IsZero() {
					for _, envVar := range container.Env {
						if envVar.Name == kbconfig.EnvNodeOpts {
							mem, err = memFromNodeOptions(envVar.Value)
							if err != nil {
								return resource.Quantity{}, err
							}
						}
					}
				}
			}
		}

		// if not, fallback to the default limits
		if mem.IsZero() {
			mem = kbpod.DefaultMemoryLimits
		}

		total.Add(multiply(mem, kb.Spec.Count))
		log.V(1).Info("Collecting", "kibana_name", kb.Name, "memory", mem.String(), "count", kb.Spec.Count)
	}

	return total, nil
}

func (a Aggregator) aggregateApmServerMemory() (resource.Quantity, error) {
	var asList asv1.ApmServerList
	err := a.client.List(&asList)
	if err != nil {
		return resource.Quantity{}, err
	}

	var total resource.Quantity
	for _, as := range asList.Items {
		var mem resource.Quantity

		// read the container memory limits
		for _, container := range as.Spec.PodTemplate.Spec.Containers {
			if container.Name == asv1.ApmServerContainerName {
				mem = *container.Resources.Limits.Memory()
			}
		}

		// if not, fallback to the default limits
		if mem.IsZero() {
			mem = apmserver.DefaultMemoryLimits
		}

		total.Add(multiply(mem, as.Spec.Count))
		log.V(1).Info("Collecting", "as_name", as.Name, "memory", mem.String(), "count", as.Spec.Count)
	}

	return total, nil
}

// maxHeapSizePattern is the pattern to extract the max Java heap size (-Xmx<size>[g|G|m|M|k|K])
const maxHeapSizePattern = "-Xmx([0-9]*)([gGmMkK]*)"

var maxHeapSizeRe = regexp.MustCompile(maxHeapSizePattern)

// memFromJavaOpts extracts the maximum Java heap size from a Java options string, multiplies the value by 2
// and converts it to a resource.Quantity
func memFromJavaOpts(javaOpts string) (resource.Quantity, error) {
	match := maxHeapSizeRe.FindStringSubmatch(javaOpts)
	if len(match) != 3 {
		return resource.Quantity{}, fmt.Errorf("cannot extract max jvm heap size from %s", javaOpts)
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return resource.Quantity{}, err
	}
	// capitalize the suffix unless it's a K to have a surjection of [g|G|m|M|k|K] in [G|M|k]
	suffix := match[2]
	switch suffix {
	case "K", "k":
		suffix = "k"
	default:
		suffix = strings.ToUpper(suffix)
	}
	// multiply by 2 and convert it in a quantity using the suffix
	return resource.ParseQuantity(fmt.Sprintf("%d%s", value*2, suffix))
}

// nodeHeapSizePattern is the pattern to extract the max heap size of the node memory (--max-old-space-size=<mb_size>)
const nodeHeapSizePattern = "--max-old-space-size=([0-9]*)"

var nodeHeapSizeRe = regexp.MustCompile(nodeHeapSizePattern)

// memFromNodeOptions extracts the Node heap size from a Node options string and converts it to a resource.Quantity
func memFromNodeOptions(nodeOpts string) (resource.Quantity, error) {
	match := nodeHeapSizeRe.FindStringSubmatch(nodeOpts)
	if len(match) != 2 {
		return resource.Quantity{}, fmt.Errorf("cannot extract max node heap size from %s", nodeOpts)
	}

	return resource.ParseQuantity(match[1] + "M")
}

// multiply multiplies a resource.Quantity by a value
func multiply(q resource.Quantity, v int32) resource.Quantity {
	var result resource.Quantity
	result.Set(q.Value() * int64(v))
	return result
}
