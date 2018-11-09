package elasticsearch

import (
	"fmt"
	"path"
	"strconv"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	corev1 "k8s.io/api/core/v1"
)

const (
	varEsJavaOpts                             = "ES_JAVA_OPTS"
	varNodeMaster                             = "node.master"
	varNodeData                               = "node.data"
	varNodeIngest                             = "node.ingest"
	varXPackSecurityEnabled                   = "xpack.security.enabled"
	varXPackLicenseSelfGeneratedType          = "xpack.license.self_generated.type"
	varXPackSecurityAuthcReservedRealmEnabled = "xpack.security.authc.reserved_realm.enabled"
	varProbeUsername                          = "PROBE_USERNAME"
	varProbePassword                          = "PROBE_PASSWORD"
	varPathData                               = "path.data"
	varPathLogs                               = "path.logs"
)

// comparableEnvVars is the list of environment variable names
// whose value should be compared between a running pod and a pod spec
// when performing changes against a cluster topology
var comparableEnvVars = []string{
	varEsJavaOpts,
	varNodeMaster,
	varNodeData,
	varNodeIngest,
	varXPackSecurityEnabled,
	varXPackLicenseSelfGeneratedType,
	varXPackSecurityAuthcReservedRealmEnabled,
	varProbeUsername,
	varPathData,
	varPathLogs,
}

// NewEnvironmentVars returns the environment vars to be associated to a pod
func NewEnvironmentVars(p NewPodSpecParams, dataVolume EmptyDirVolume, probeUser client.User, extraFilesSecretVolume SecretVolume) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "node.name", Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
		}},
		{Name: "discovery.zen.ping.unicast.hosts", Value: p.DiscoveryServiceName},
		{Name: "cluster.name", Value: p.ClusterName},
		{Name: "discovery.zen.minimum_master_nodes", Value: strconv.Itoa(p.DiscoveryZenMinimumMasterNodes)},
		{Name: "network.host", Value: "0.0.0.0"},
		{Name: "network.publish_host", Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
		}},

		{Name: varPathData, Value: dataVolume.DataPath()},
		{Name: varPathLogs, Value: dataVolume.LogsPath()},

		{
			Name:  "xpack.security.transport.ssl.trust_restrictions.path",
			Value: fmt.Sprintf("%s/trust.yml", extraFilesSecretVolume.VolumeMount().MountPath),
		},

		// TODO: the JVM options are hardcoded, but should be configurable
		{Name: varEsJavaOpts, Value: "-Xms1g -Xmx1g"},

		// TODO: dedicated node types support
		{Name: varNodeMaster, Value: fmt.Sprintf("%t", p.NodeTypes.Master)},
		{Name: varNodeData, Value: fmt.Sprintf("%t", p.NodeTypes.Data)},
		{Name: varNodeIngest, Value: fmt.Sprintf("%t", p.NodeTypes.Ingest)},

		{Name: varXPackSecurityEnabled, Value: "true"},
		{Name: varXPackLicenseSelfGeneratedType, Value: "trial"},
		{Name: varXPackSecurityAuthcReservedRealmEnabled, Value: "false"},
		{Name: "PROBE_USERNAME", Value: probeUser.Name},
		{Name: "PROBE_PASSWORD_FILE", Value: path.Join(probeUserSecretMountPath, probeUser.Name)},
		{Name: "transport.profiles.client.port", Value: strconv.Itoa(TransportClientPort)},
	}
}
