// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package telemetry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/about"
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	esav1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	mapsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/v2/pkg/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
)

const (
	resourceCount            = "resource_count"
	podCount                 = "pod_count"
	helmManagedResourceCount = "helm_resource_count"
	timestampFieldName       = "timestamp"
)

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
	ctx = ulog.InitInContext(ctx, "telemetry")
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
		scpStats,
		logstashStats,
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
	ctx = tracing.NewContextTransaction(ctx, r.tracer, tracing.PeriodicTxType, "telemetry-reporter", nil)
	defer tracing.EndContextTransaction(ctx)

	log := ulog.FromContext(ctx)

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

	log := ulog.FromContext(ctx)

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
		HelmManagedResourceCount    int32                    `json:"helm_resource_count"`
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

			if isManagedByHelm(es.Labels) {
				stats.HelmManagedResourceCount++
			}
			if es.IsAutoscalingAnnotationSet() {
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

	var esaList esav1alpha1.ElasticsearchAutoscalerList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(context.Background(), &esaList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}
		stats.AutoscaledResourceCount += int32(len(esaList.Items))
	}

	return "elasticsearches", stats, nil
}

func isManagedByHelm(labels map[string]string) bool {
	if val, ok := labels["helm.sh/chart"]; ok {
		return strings.HasPrefix(val, "eck-elasticsearch-") || strings.HasPrefix(val, "eck-kibana-")
	}

	return false
}

func kbStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	stats := map[string]int32{resourceCount: 0, podCount: 0, helmManagedResourceCount: 0}

	var kbList kbv1.KibanaList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(context.Background(), &kbList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, kb := range kbList.Items {
			stats[resourceCount]++
			stats[podCount] += kb.Status.AvailableNodes

			if isManagedByHelm(kb.Labels) {
				stats[helmManagedResourceCount]++
			}
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

func logstashStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	const (
		pipelineCount               = "pipeline_count"
		pipelineRefCount            = "pipeline_ref_count"
		serviceCount                = "service_count"
		stackMonitoringLogsCount    = "stack_monitoring_logs_count"
		stackMonitoringMetricsCount = "stack_monitoring_metrics_count"
	)
	stats := map[string]int32{resourceCount: 0, podCount: 0, stackMonitoringLogsCount: 0,
		stackMonitoringMetricsCount: 0, serviceCount: 0, pipelineCount: 0, pipelineRefCount: 0}

	var logstashList logstashv1alpha1.LogstashList
	for _, ns := range managedNamespaces {
		if err := k8sClient.List(context.Background(), &logstashList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, ls := range logstashList.Items {
			ls := ls
			stats[resourceCount]++
			stats[serviceCount] += int32(len(ls.Spec.Services))
			stats[podCount] += ls.Status.AvailableNodes
			stats[pipelineCount] += int32(len(ls.Spec.Pipelines))
			if ls.Spec.PipelinesRef != nil {
				stats[pipelineRefCount]++
			}
			if monitoring.IsLogsDefined(&ls) {
				stats[stackMonitoringLogsCount]++
			}
			if monitoring.IsMetricsDefined(&ls) {
				stats[stackMonitoringMetricsCount]++
			}
		}
	}
	return "logstashes", stats, nil
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

// stackConfigPolicyStats models StackConfigPolicy resources usage statistics.
type stackConfigPolicyStats struct {
	ResourceCount            int `json:"resource_count"`
	ConfiguredResourcesCount int `json:"configured_resources_count"`
	Settings                 struct {
		ClusterSettingsCount           int `json:"cluster_settings_count"`
		SnapshotRepositoriesCount      int `json:"snapshot_repositories_count"`
		SnapshotLifecyclePoliciesCount int `json:"snapshot_lifecycle_policies_count"`
		RoleMappingsCount              int `json:"role_mappings_count"`
		IndexLifecyclePoliciesCount    int `json:"index_lifecycle_policies_count"`
		IngestPipelinesCount           int `json:"ingest_pipelines_count"`
		ComponentTemplatesCount        int `json:"component_templates_count"`
		ComposableIndexTemplatesCount  int `json:"composable_index_templates_count"`
	} `json:"settings"`
}

func scpStats(k8sClient k8s.Client, managedNamespaces []string) (string, interface{}, error) {
	stats := stackConfigPolicyStats{}
	for _, ns := range managedNamespaces {
		var scpList policyv1alpha1.StackConfigPolicyList
		if err := k8sClient.List(context.Background(), &scpList, client.InNamespace(ns)); err != nil {
			return "", nil, err
		}

		for _, scp := range scpList.Items {
			stats.ResourceCount++
			stats.ConfiguredResourcesCount += scp.Status.Resources
			if scp.Spec.Elasticsearch.ClusterSettings != nil {
				stats.Settings.ClusterSettingsCount += len(scp.Spec.Elasticsearch.ClusterSettings.Data)
			}
			if scp.Spec.Elasticsearch.SnapshotRepositories != nil {
				stats.Settings.SnapshotRepositoriesCount += len(scp.Spec.Elasticsearch.SnapshotRepositories.Data)
			}
			if scp.Spec.Elasticsearch.SnapshotLifecyclePolicies != nil {
				stats.Settings.SnapshotLifecyclePoliciesCount += len(scp.Spec.Elasticsearch.SnapshotLifecyclePolicies.Data)
			}
			if scp.Spec.Elasticsearch.SecurityRoleMappings != nil {
				stats.Settings.RoleMappingsCount += len(scp.Spec.Elasticsearch.SecurityRoleMappings.Data)
			}
			if scp.Spec.Elasticsearch.IndexLifecyclePolicies != nil {
				stats.Settings.IndexLifecyclePoliciesCount += len(scp.Spec.Elasticsearch.IndexLifecyclePolicies.Data)
			}
			if scp.Spec.Elasticsearch.IngestPipelines != nil {
				stats.Settings.IngestPipelinesCount += len(scp.Spec.Elasticsearch.IngestPipelines.Data)
			}
			if scp.Spec.Elasticsearch.IndexTemplates.ComponentTemplates != nil {
				stats.Settings.ComponentTemplatesCount += len(scp.Spec.Elasticsearch.IndexTemplates.ComponentTemplates.Data)
			}
			if scp.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates != nil {
				stats.Settings.ComposableIndexTemplatesCount += len(scp.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates.Data)
			}
		}
	}
	return "stackconfigpolicies", stats, nil
}
