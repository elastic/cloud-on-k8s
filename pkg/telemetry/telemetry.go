// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/ghodss/yaml"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	mapsv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/license"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
)

const (
	resourceCount = "resource_count"
	podCount      = "pod_count"

	timestampFieldName = "timestamp"
)

var log = ulog.Log.WithName("usage")

type ECKTelemetry struct {
	ECK ECK `json:"eck"`
}

type ECK struct {
	about.OperatorInfo
	Stats   map[string]interface{} `json:"stats"`
	License map[string]string      `json:"license"`
}

type getStatsFn func(k8s.Client, []string) (string, interface{}, error)

func NewReporter(
	info about.OperatorInfo,
	client client.Client,
	operatorNamespace string,
	managedNamespaces []string,
	telemetryInterval time.Duration,
	tracer *apm.Tracer,
) Reporter {
	if len(managedNamespaces) == 0 {
		// treat no managed namespaces as managing all namespaces, ie. set empty string for namespace filtering
		managedNamespaces = append(managedNamespaces, "")
	}

	return Reporter{
		operatorInfo:      info,
		client:            client,
		operatorNamespace: operatorNamespace,
		managedNamespaces: managedNamespaces,
		telemetryInterval: telemetryInterval,
		tracer:            tracer,
	}
}

type Reporter struct {
	operatorInfo      about.OperatorInfo
	client            k8s.Client
	operatorNamespace string
	managedNamespaces []string
	telemetryInterval time.Duration
	tracer            *apm.Tracer
}

func (r *Reporter) Start(ctx context.Context) {
	ticker := time.NewTicker(r.telemetryInterval)
	for range ticker.C {
		r.report(ctx)
	}
}

func marshalTelemetry(ctx context.Context, info about.OperatorInfo, stats map[string]interface{}, license map[string]string) ([]byte, error) {
	span, _ := apm.StartSpan(ctx, "marshal_telemetry", tracing.SpanTypeApp)
	defer span.End()

	return yaml.Marshal(ECKTelemetry{
		ECK: ECK{
			OperatorInfo: info,
			Stats:        stats,
			License:      license,
		},
	})
}

func (r *Reporter) getResourceStats(ctx context.Context) (map[string]interface{}, error) {
	span, _ := apm.StartSpan(ctx, "get_resource_stats", tracing.SpanTypeApp)
	defer span.End()

	stats := map[string]interface{}{}
	for _, f := range []getStatsFn{
		esStats,
		kbStats,
		apmStats,
		beatStats,
		entStats,
		agentStats,
		mapsStats,
	} {
		key, statsPart, err := f(r.client, r.managedNamespaces)
		if err != nil {
			return nil, err
		}
		stats[key] = statsPart
	}

	return stats, nil
}

func (r *Reporter) report(ctx context.Context) {
	ctx = tracing.NewContextTransaction(ctx, r.tracer, "telemetry-reporter", "report", nil)
	defer tracing.EndContextTransaction(ctx)

	stats, err := r.getResourceStats(ctx)
	if err != nil {
		log.Error(err, "failed to get resource stats")
		return
	}

	licenseInfo, err := r.getLicenseInfo(ctx)
	if err != nil {
		log.Error(err, "failed to get operator license secret")
		// it's ok to go on
	}

	telemetryBytes, err := marshalTelemetry(ctx, r.operatorInfo, stats, licenseInfo)
	if err != nil {
		log.Error(err, "failed to marshal telemetry data")
		return
	}

	for _, ns := range r.managedNamespaces {
		var kibanaList kbv1.KibanaList
		if err := r.client.List(ctx, &kibanaList, client.InNamespace(ns)); err != nil {
			log.Error(err, "failed to list Kibanas")
			continue
		}
		for _, kb := range kibanaList.Items {
			r.reconcileKibanaSecret(ctx, kb, telemetryBytes)
		}
	}
}

func (r *Reporter) reconcileKibanaSecret(ctx context.Context, kb kbv1.Kibana, telemetryBytes []byte) {
	span, ctx := apm.StartSpan(ctx, "reconcile_kibana_secret", tracing.SpanTypeApp)
	defer span.End()

	var secret corev1.Secret
	nsName := types.NamespacedName{Namespace: kb.Namespace, Name: kibana.SecretName(kb)}
	if err := r.client.Get(ctx, nsName, &secret); err != nil {
		log.Error(err, "failed to get Kibana secret")
		return
	}

	if secret.Data == nil {
		// should not happen, but just to be safe
		secret.Data = make(map[string][]byte)
	}

	secret.Data[kibana.TelemetryFilename] = telemetryBytes

	if _, err := reconciler.ReconcileSecret(ctx, r.client, secret, nil); err != nil {
		log.Error(err, "failed to reconcile Kibana secret")
		return
	}
}

func (r *Reporter) getLicenseInfo(ctx context.Context) (map[string]string, error) {
	span, _ := apm.StartSpan(ctx, "get_license_info", tracing.SpanTypeApp)
	defer span.End()

	nsn := types.NamespacedName{
		Namespace: r.operatorNamespace,
		Name:      license.LicensingCfgMapName,
	}

	var licenseConfigMap corev1.ConfigMap
	if err := r.client.Get(ctx, nsn, &licenseConfigMap); err != nil {
		return nil, err
	}

	// remove timestamp field as it doesn't carry any significant information
	delete(licenseConfigMap.Data, timestampFieldName)

	return licenseConfigMap.Data, nil
}

type downwardNodeLabelsStats struct {
	// ResourceCount is the number of resources which are relying on the node labels downward API.
	ResourceCount int32 `json:"resource_count"`
	// DistinctNodeLabelsCount is the number of distinct labels used.
	DistinctNodeLabelsCount int32 `json:"distinct_node_labels_count"`
}

func esStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	stats := struct {
		ResourceCount               int32                    `json:"resource_count"`
		PodCount                    int32                    `json:"pod_count"`
		AutoscaledResourceCount     int32                    `json:"autoscaled_resource_count"`
		StackMonitoringLogsCount    int32                    `json:"stack_monitoring_logs_count"`
		StackMonitoringMetricsCount int32                    `json:"stack_monitoring_metrics_count"`
		DownwardNodeLabels          *downwardNodeLabelsStats `json:"downward_node_labels,omitempty"`
	}{}
	distinctNodeLabels := set.Make()
	var resourcesWithDownwardLabels int32
	var esList esv1.ElasticsearchList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(context.Background(), &esList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, es := range esList.Items {
			es := es
			stats.ResourceCount++
			stats.PodCount += es.Status.AvailableNodes
			if es.IsAutoscalingDefined() {
				stats.AutoscaledResourceCount++
			}
			if es.HasDownwardNodeLabels() {
				resourcesWithDownwardLabels++
				distinctNodeLabels.MergeWith(set.Make(es.DownwardNodeLabels()...))
			}
			if monitoring.IsLogsDefined(&es) {
				stats.StackMonitoringLogsCount++
			}
			if monitoring.IsMetricsDefined(&es) {
				stats.StackMonitoringMetricsCount++
			}
		}
	}
	if resourcesWithDownwardLabels > 0 {
		stats.DownwardNodeLabels = &downwardNodeLabelsStats{
			ResourceCount:           resourcesWithDownwardLabels,
			DistinctNodeLabelsCount: int32(distinctNodeLabels.Count()),
		}
	}
	return "elasticsearches", stats, nil
}

func kbStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	stats := map[string]int32{resourceCount: 0, podCount: 0}

	var kbList kbv1.KibanaList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(context.Background(), &kbList, client.InNamespace(ns)); err != nil {
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
		if err := k8sClient.List(context.Background(), &apmList, client.InNamespace(ns)); err != nil {
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
		if err := k8sClient.List(context.Background(), &beatList, client.InNamespace(ns)); err != nil {
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

	var entList entv1.EnterpriseSearchList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(context.Background(), &entList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, ent := range entList.Items {
			stats[resourceCount]++
			stats[podCount] += ent.Status.AvailableNodes
		}
	}
	return "enterprisesearches", stats, nil
}

func agentStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	multipleRefsKey := "multiple_refs"
	fleetModeKey := "fleet_mode"
	fleetServerKey := "fleet_server"
	stats := map[string]int32{resourceCount: 0, podCount: 0, multipleRefsKey: 0}

	var agentList agentv1alpha1.AgentList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(context.Background(), &agentList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, agent := range agentList.Items {
			stats[resourceCount]++
			stats[podCount] += agent.Status.AvailableNodes
			if len(agent.Spec.ElasticsearchRefs) > 1 {
				stats[multipleRefsKey]++
			}
			if agent.Spec.FleetModeEnabled() {
				stats[fleetModeKey]++
			}
			if agent.Spec.FleetServerEnabled {
				stats[fleetServerKey]++
			}
		}
	}
	return "agents", stats, nil
}

func mapsStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	stats := map[string]int32{resourceCount: 0, podCount: 0}

	var mapsList mapsv1alpha1.ElasticMapsServerList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(context.Background(), &mapsList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, maps := range mapsList.Items {
			stats[resourceCount]++
			stats[podCount] += maps.Status.AvailableNodes
		}
	}
	return "maps", stats, nil
}
