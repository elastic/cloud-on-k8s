// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
// +build beat e2e

package beat

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/helper"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EnvPodIP = "POD_IP"
)

func TestFilebeatNoAutodiscoverRecipe(t *testing.T) {
	name := "fb-no-autodiscover"
	pod, loggedString := loggingTestPod(name)
	customize := func(builder beat.Builder) beat.Builder {
		return builder.
			WithRoles(beat.PSPClusterRoleName).
			WithESValidations(
				beat.HasMessageContaining(loggedString),
			)
	}

	runBeatRecipe(t, "filebeat_no_autodiscover.yaml", customize, pod)
}

func TestFilebeatAutodiscoverRecipe(t *testing.T) {
	name := "fb-autodiscover"
	pod, loggedString := loggingTestPod(name)
	customize := func(builder beat.Builder) beat.Builder {
		return builder.
			WithRoles(beat.PSPClusterRoleName).
			WithESValidations(
				beat.HasEventFromPod(pod.Name),
				beat.HasMessageContaining(loggedString),
			)
	}

	runBeatRecipe(t, "filebeat_autodiscover.yaml", customize, pod)
}

func TestFilebeatAutodiscoverByMetadataRecipe(t *testing.T) {
	name := "fb-autodiscover-meta"
	podBad, badLog := loggingTestPod(name + "-bad")
	podLabel, goodLog := loggingTestPod(name + "-label")
	podLabel.Labels["log-label"] = "true"

	customize := func(builder beat.Builder) beat.Builder {
		return builder.
			WithRoles(beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
			WithESValidations(
				beat.HasEventFromPod(podLabel.Name),
				beat.HasMessageContaining(goodLog),
				beat.NoMessageContaining(badLog),
			)
	}

	runBeatRecipe(t, "filebeat_autodiscover_by_metadata.yaml", customize, podLabel, podBad)
}

func TestMetricbeatHostsRecipe(t *testing.T) {
	customize := func(builder beat.Builder) beat.Builder {
		return builder.
			WithRoles(beat.PSPClusterRoleName).
			WithESValidations(
				beat.HasEvent("event.dataset:system.cpu"),
				beat.HasEvent("event.dataset:system.load"),
				beat.HasEvent("event.dataset:system.memory"),
				beat.HasEvent("event.dataset:system.network"),
				beat.HasEvent("event.dataset:system.process"),
				beat.HasEvent("event.dataset:system.process.summary"),
				beat.HasEvent("event.dataset:system.fsstat"),
			)
	}

	runBeatRecipe(t, "metricbeat_hosts.yaml", customize)
}

func TestMetricbeatStackMonitoringRecipe(t *testing.T) {
	name := "fb-autodiscover"
	pod, loggedString := loggingTestPod(name)
	customize := func(builder beat.Builder) beat.Builder {
		// update ref to monitored cluster credentials
		if strings.HasPrefix(builder.Beat.ObjectMeta.Name, "metricbeat") {
			currSecretName := builder.Beat.Spec.Deployment.PodTemplate.Spec.Containers[0].Env[1].ValueFrom.SecretKeyRef.Name
			newSecretName := strings.Replace(currSecretName, "elasticsearch", fmt.Sprintf("elasticsearch-%s", builder.Suffix), 1)
			builder.Beat.Spec.Deployment.PodTemplate.Spec.Containers[0].Env[1].ValueFrom.SecretKeyRef.Name = newSecretName

			// We are using the pod's IP address exposed through the downward API to detect if the test is running in an IPv6 environment.
			if net.ToIPFamily(os.Getenv(EnvPodIP)) == corev1.IPv6Protocol {
				// In an IPv6 environment we need to patch the configuration to add some brackets around the data.host variable.
				json, err := builder.Beat.Spec.Config.MarshalJSON()
				if err != nil {
					require.NoError(t, err, "Failed to extract configuration")
				}
				config := strings.ReplaceAll(string(json), "${data.host}", "[${data.host}]")
				if err := builder.Beat.Spec.Config.UnmarshalJSON([]byte(config)); err != nil {
					require.NoError(t, err, "Failed to convert back to json configuration")
				}
			}
		}

		metricbeatValidations := []beat.ValidationFunc{
			beat.HasMonitoringEvent("metricset.name:cluster_stats"),
			beat.HasMonitoringEvent("metricset.name:enrich"),
			beat.HasMonitoringEvent("metricset.name:index"),
			beat.HasMonitoringEvent("metricset.name:index_summary"),
			beat.HasMonitoringEvent("metricset.name:node_stats"),
			beat.HasMonitoringEvent("metricset.name:stats"),
			beat.HasMonitoringEvent("metricset.name:shard"),
			beat.HasMonitoringEvent("kibana_stats.kibana.status:green"),
		}

		if version.MustParse(builder.Beat.Spec.Version).LT(version.MinFor(8, 0, 0)) {
			// before 8.0.0, `metricset.name` was not indexed
			metricbeatValidations = []beat.ValidationFunc{
				beat.HasMonitoringEvent("type:cluster_stats"),
				beat.HasMonitoringEvent("type:enrich_coordinator_stats"),
				beat.HasMonitoringEvent("type:index_stats"),
				beat.HasMonitoringEvent("type:index_recovery"),
				beat.HasMonitoringEvent("type:indices_stats"),
				beat.HasMonitoringEvent("node_stats.node_master:true"),
				beat.HasMonitoringEvent("kibana_stats.kibana.status:green"),
			}
		}

		return builder.
			WithRoles(beat.PSPClusterRoleName).
			WithESValidations(append(
				metricbeatValidations,
				// filebeat validations
				beat.HasEventFromPod(pod.Name),
				beat.HasMessageContaining(loggedString),
			)...)
	}

	runBeatRecipe(t, "stack_monitoring.yaml", customize, pod)
}

func TestHeartbeatEsKbHealthRecipe(t *testing.T) {
	customize := func(builder beat.Builder) beat.Builder {
		cfg := settings.MustCanonicalConfig(builder.Beat.Spec.Config.Data)
		yamlBytes, err := cfg.Render()
		require.NoError(t, err)

		spec := builder.Beat.Spec
		newEsHost := fmt.Sprintf("%s.%s.svc", esv1.HTTPService(spec.ElasticsearchRef.Name), builder.Beat.Namespace)
		newKbHost := fmt.Sprintf("%s.%s.svc", kbv1.HTTPService(spec.KibanaRef.Name), builder.Beat.Namespace)

		yaml := string(yamlBytes)
		yaml = strings.ReplaceAll(yaml, "elasticsearch-es-http.default.svc", newEsHost)
		yaml = strings.ReplaceAll(yaml, "kibana-kb-http.default.svc", newKbHost)

		builder.Beat.Spec.Config = &commonv1.Config{}
		err = settings.MustParseConfig([]byte(yaml)).Unpack(&builder.Beat.Spec.Config.Data)
		require.NoError(t, err)

		return builder.
			WithRoles(beat.PSPClusterRoleName).
			WithESValidations(
				beat.HasEvent("monitor.status:up"),
			)
	}

	runBeatRecipe(t, "heartbeat_es_kb_health.yaml", customize)
}

func TestAuditbeatHostsRecipe(t *testing.T) {

	if test.Ctx().Provider == "kind" || test.Ctx().HasTag(test.ArchARMTag) {
		// Skipping test because recipe relies on syscall audit rules unavailable on arm64
		// Also: kind doesn't support configuring required settings
		// see https://github.com/elastic/cloud-on-k8s/issues/3328 for more context
		t.SkipNow()
	}

	customize := func(builder beat.Builder) beat.Builder {
		return builder.
			WithRoles(beat.AuditbeatPSPClusterRoleName).
			WithESValidations(
				beat.HasEvent("event.dataset:file"),
				beat.HasEvent("event.module:file_integrity"),
			)
	}

	runBeatRecipe(t, "auditbeat_hosts.yaml", customize)
}

func TestPacketbeatDnsHttpRecipe(t *testing.T) {
	customize := func(builder beat.Builder) beat.Builder {
		if !(test.Ctx().Provider == "kind" && test.Ctx().KubernetesMajorMinor() == "1.12") {
			// there are some issues with kind 1.12 and tracking http traffic
			builder = builder.WithESValidations(beat.HasEvent("event.dataset:http"))
		}

		return builder.
			WithRoles(beat.PacketbeatPSPClusterRoleName).
			WithESValidations(
				beat.HasEvent("event.dataset:flow"),
				beat.HasEvent("event.dataset:dns"),
			)
	}

	runBeatRecipe(t, "packetbeat_dns_http.yaml", customize)
}

func TestJournalbeatHostsRecipe(t *testing.T) {
	customize := func(builder beat.Builder) beat.Builder {
		return builder.
			WithRoles(beat.JournalbeatPSPClusterRoleName)
	}

	runBeatRecipe(t, "journalbeat_hosts.yaml", customize)
}

func runBeatRecipe(
	t *testing.T,
	fileName string,
	customize func(builder beat.Builder) beat.Builder,
	additionalObjects ...client.Object,
) {
	filePath := path.Join("../../../config/recipes/beats", fileName)
	namespace := test.Ctx().ManagedNamespace(0)
	suffix := rand.String(4)

	transformationsWrapped := func(builder test.Builder) test.Builder {
		beatBuilder, ok := builder.(beat.Builder)
		if !ok {
			return builder
		}

		if isStackIncompatible(beatBuilder.Beat) {
			t.SkipNow()
		}

		// OpenShift requires different securityContext than provided in the recipe.
		// Skipping it altogether to reduce maintenance burden.
		if strings.HasPrefix(test.Ctx().Provider, "ocp") {
			t.SkipNow()
		}

		beatBuilder.Suffix = suffix

		if customize != nil {
			beatBuilder = customize(beatBuilder)
		}

		return beatBuilder.
			WithESValidations(beat.HasEventFromBeat(beatcommon.Type(beatBuilder.Beat.Spec.Type)))
	}

	helper.RunFile(t, filePath, namespace, suffix, additionalObjects, transformationsWrapped)
}

// isStackIncompatible returns true iff Beat version is higher than tested Stack version
func isStackIncompatible(beat beatv1beta1.Beat) bool {
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	beatVersion := version.MustParse(beat.Spec.Version)
	return beatVersion.GT(stackVersion)
}

func loggingTestPod(name string) (*corev1.Pod, string) {
	podBuilder := beat.NewPodBuilder(name)
	return &podBuilder.Pod, podBuilder.Logged
}
