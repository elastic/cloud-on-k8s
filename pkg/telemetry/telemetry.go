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
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/ghodss/yaml"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	updateInterval = 1 * time.Hour
	resourceCount  = "resource_count"
	podCount       = "pod_count"
)

var log = logf.Log.WithName("usage")

type getStatsFn func(k8s.Client, []string) (string, interface{}, error)

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
		r.report()
	}
}

func marshalTelemetry(info about.OperatorInfo, stats map[string]interface{}) ([]byte, error) {
	return yaml.Marshal(struct {
		ECK struct {
			about.OperatorInfo
			Stats map[string]interface{} `json:"stats"`
		} `json:"eck"`
	}{
		ECK: struct {
			about.OperatorInfo
			Stats map[string]interface{} `json:"stats"`
		}{
			OperatorInfo: info,
			Stats:        stats,
		},
	})
}

func (r *Reporter) getResourceStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}
	for _, f := range []getStatsFn{
		esStats,
		kbStats,
		apmStats,
		beatStats,
		entStats,
	} {
		key, statsPart, err := f(r.client, r.managedNamespaces)
		if err != nil {
			return nil, err
		}
		stats[key] = statsPart
	}

	return stats, nil
}

func (r *Reporter) report() {
	stats, err := r.getResourceStats()
	if err != nil {
		log.Error(err, "failed to get resource stats")
		return
	}

	telemetryBytes, err := marshalTelemetry(r.operatorInfo, stats)
	if err != nil {
		log.Error(err, "failed to marshal telemetry data")
		return
	}

	for _, ns := range r.managedNamespaces {
		var kibanaList kbv1.KibanaList
		if err := r.client.List(&kibanaList, client.InNamespace(ns)); err != nil {
			log.Error(err, "failed to list Kibanas")
			continue
		}
		for _, kb := range kibanaList.Items {
			var secret corev1.Secret
			nsName := types.NamespacedName{Namespace: kb.Namespace, Name: kibana.SecretName(kb)}
			if err := r.client.Get(nsName, &secret); err != nil {
				log.Error(err, "failed to get Kibana secret")
				continue
			}

			if secret.Data == nil {
				// should not happen, but just to be safe
				secret.Data = make(map[string][]byte)
			}

			secret.Data[kibana.TelemetryFilename] = telemetryBytes

			if _, err := reconciler.ReconcileSecret(r.client, secret, nil); err != nil {
				log.Error(err, "failed to reconcile Kibana secret")
				continue
			}
		}
	}
}

func esStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	stats := map[string]int32{resourceCount: 0, podCount: 0}

	var esList esv1.ElasticsearchList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&esList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, es := range esList.Items {
			stats[resourceCount]++
			stats[podCount] += es.Status.AvailableNodes
		}
	}
	return "elasticsearches", stats, nil
}

func kbStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	stats := map[string]int32{resourceCount: 0, podCount: 0}

	var kbList kbv1.KibanaList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&kbList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, kb := range kbList.Items {
			stats[resourceCount]++
			stats[podCount] += kb.Status.AvailableNodes
		}
	}
	return "kibanas", stats, nil
}

func apmStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	stats := map[string]int32{resourceCount: 0, podCount: 0}

	var apmList apmv1.ApmServerList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&apmList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, apm := range apmList.Items {
			stats[resourceCount]++
			stats[podCount] += apm.Status.AvailableNodes
		}
	}
	return "apms", stats, nil
}

func beatStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	typeToName := func(typ string) string { return fmt.Sprintf("%s_count", typ) }

	stats := map[string]int32{resourceCount: 0, podCount: 0}
	for typ := range beatv1beta1.KnownTypes {
		stats[typeToName(typ)] = 0
	}

	var beatList beatv1beta1.BeatList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&beatList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, beat := range beatList.Items {
			stats[resourceCount]++
			stats[typeToName(beat.Spec.Type)]++
			stats[podCount] += beat.Status.AvailableNodes
		}
	}

	return "beats", stats, nil
}

func entStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	stats := map[string]int32{resourceCount: 0, podCount: 0}

	var entList entv1beta1.EnterpriseSearchList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(&entList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, apm := range entList.Items {
			stats[resourceCount]++
			stats[podCount] += apm.Status.AvailableNodes
		}
	}
	return "enterprisesearches", stats, nil
}
