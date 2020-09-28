// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package telemetry

import (
	"fmt"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/ghodss/yaml"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	updateInterval = 10 * time.Second
	resourceCount  = "resource_count"
	podCount       = "pod_count"
)

var log = logf.Log.WithName("usage")

type getUsageFn func(k8s.Client, []string) (string, interface{}, error)

// ECK is a helper struct to marshal telemetry information.
type ECK struct {
	ECK   about.OperatorInfo     `json:"eck"`
	Usage map[string]interface{} `json:"eck_usage"`
}

func NewReporter(info about.OperatorInfo, client client.Client, managedNamespaces []string) Reporter {
	if len(managedNamespaces) == 0 {
		// treat no managed namespaces as managing all namespaces, ie. set empty string for namespace filtering
		managedNamespaces = append(managedNamespaces, "")
	}

	return Reporter{
		operatorInfo:      info,
		client:            k8s.WrapClient(client),
		managedNamespaces: managedNamespaces,
	}
}

type Reporter struct {
	operatorInfo      about.OperatorInfo
	client            k8s.Client
	managedNamespaces []string
}

func (r *Reporter) Start() {
	ticker := time.NewTicker(updateInterval)
	for range ticker.C {
		usage, err := r.getResourceUsage()
		if err != nil {
			log.Error(err, "failed to get resource usage")
			continue
		}

		telemetryBytes, err := yaml.Marshal(ECK{ECK: r.operatorInfo, Usage: usage})
		if err != nil {
			log.Error(err, "failed to marshal telemetry data")
		}

		for _, ns := range r.managedNamespaces {
			var kibanaList kbv1.KibanaList
			if err := r.client.List(&kibanaList, client.InNamespace(ns)); err != nil {
				log.Error(err, "failed to list kibanas")
				continue
			}
			for _, kb := range kibanaList.Items {
				var secret corev1.Secret
				nsName := types.NamespacedName{Namespace: kb.Namespace, Name: kibana.SecretName(kb)}
				if err := r.client.Get(nsName, &secret); err != nil {
					log.Error(err, "failed to get kibana secret")
					continue
				}

				secret.Data[kibana.TelemetryFilename] = telemetryBytes
				if err := r.client.Update(&secret); err != nil {
					log.Error(err, "failed to update kibana secret")
					continue
				}
			}
		}
	}
}

func (r *Reporter) getResourceUsage() (map[string]interface{}, error) {
	usage := map[string]interface{}{}
	for _, f := range []getUsageFn{
		esUsage,
		kbUsage,
		apmUsage,
		beatUsage,
		entUsage,
	} {
		key, usagePart, err := f(r.client, r.managedNamespaces)
		if err != nil {
			return nil, err
		}
		usage[key] = usagePart
	}

	return usage, nil
}

func esUsage(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	usage := map[string]int32{resourceCount: 0, podCount: 0}

	var esList esv1.ElasticsearchList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&esList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, es := range esList.Items {
			usage[resourceCount]++
			usage[podCount] += es.Status.AvailableNodes
		}
	}
	return "elasticsearches", usage, nil
}

func kbUsage(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	usage := map[string]int32{resourceCount: 0, podCount: 0}

	var kbList kbv1.KibanaList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&kbList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, kb := range kbList.Items {
			usage[resourceCount]++
			usage[podCount] += kb.Status.AvailableNodes
		}
	}
	return "kibanas", usage, nil
}

func apmUsage(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	usage := map[string]int32{resourceCount: 0, podCount: 0}

	var apmList apmv1.ApmServerList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&apmList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, apm := range apmList.Items {
			usage[resourceCount]++
			usage[podCount] += apm.Status.AvailableNodes
		}
	}
	return "apms", usage, nil
}

func beatUsage(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	typeToName := func(typ string) string { return fmt.Sprintf("%s_count", typ) }

	usage := map[string]int32{resourceCount: 0, podCount: 0}
	for typ := range beatv1beta1.KnownTypes {
		usage[typeToName(typ)] = 0
	}

	var beatList beatv1beta1.BeatList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&beatList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, beat := range beatList.Items {
			usage[resourceCount]++
			usage[typeToName(beat.Spec.Type)]++
			usage[podCount] += beat.Status.AvailableNodes
		}
	}

	return "beats", usage, nil
}

func entUsage(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	usage := map[string]int32{resourceCount: 0, podCount: 0}

	var entList entv1beta1.EnterpriseSearchList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&entList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, apm := range entList.Items {
			usage[resourceCount]++
			usage[podCount] += apm.Status.AvailableNodes
		}
	}
	return "enterprisesearches", usage, nil
}
