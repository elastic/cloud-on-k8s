package support

const (
	EnvEsJavaOpts                             = "ES_JAVA_OPTS"
	EnvNodeMaster                             = "node.master"
	EnvNodeData                               = "node.data"
	EnvNodeIngest                             = "node.ingest"
	EnvNodeML                                 = "node.ml"
	EnvXPackSecurityEnabled                   = "xpack.security.enabled"
	EnvXPackLicenseSelfGeneratedType          = "xpack.license.self_generated.type"
	EnvXPackSecurityAuthcReservedRealmEnabled = "xpack.security.authc.reserved_realm.enabled"
	EnvProbeUsername                          = "PROBE_USERNAME"
	EnvPathData                               = "path.data"
	EnvPathLogs                               = "path.logs"
)

// comparableEnvVars is the list of environment variable names
// whose value should be compared between a running pod and a pod spec
// when performing changes against a cluster topology
var comparableEnvVars = []string{
	EnvEsJavaOpts,
	EnvNodeMaster,
	EnvNodeData,
	EnvNodeIngest,
	EnvNodeML,
	EnvXPackSecurityEnabled,
	EnvXPackLicenseSelfGeneratedType,
	EnvXPackSecurityAuthcReservedRealmEnabled,
	EnvProbeUsername,
	EnvPathData,
	EnvPathLogs,
}
