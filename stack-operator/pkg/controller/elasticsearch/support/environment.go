package support

const (
	EnvClusterName = "cluster.name"

	EnvDiscoveryZenMinimumMasterNodes = "discovery.zen.minimum_master_nodes"
	EnvDiscoveryZenPingUnicastHosts   = "discovery.zen.ping.unicast.hosts"

	EnvEsJavaOpts         = "ES_JAVA_OPTS"
	EnvNetworkHost        = "network.host"
	EnvNetworkPublishHost = "network.publish_host"

	EnvNodeData   = "node.data"
	EnvNodeIngest = "node.ingest"
	EnvNodeMaster = "node.master"
	EnvNodeML     = "node.ml"

	EnvNodeName = "node.name"

	EnvPathData = "path.data"
	EnvPathLogs = "path.logs"

	EnvProbePasswordFile      = "PROBE_PASSWORD_FILE"
	EnvProbeUsername          = "PROBE_USERNAME"
	EnvReadinessProbeProtocol = "READINESS_PROBE_PROTOCOL"

	EnvTransportProfilesClientPort                                 = "transport.profiles.client.port"
	EnvTransportProfilesClientXPackSecuritySslClientAuthentication = "transport.profiles.client.xpack.security.ssl.client_authentication"
	EnvTransportProfilesClientXPackSecurityType                    = "transport.profiles.client.xpack.security.type"

	EnvXPackLicenseSelfGeneratedType                   = "xpack.license.self_generated.type"
	EnvXPackSecurityAuthcReservedRealmEnabled          = "xpack.security.authc.reserved_realm.enabled"
	EnvXPackSecurityEnabled                            = "xpack.security.enabled"
	EnvXPackSecurityHttpSslCertificate                 = "xpack.security.http.ssl.certificate"
	EnvXPackSecurityHttpSslCertificateAuthorities      = "xpack.security.http.ssl.certificate_authorities"
	EnvXPackSecurityHttpSslEnabled                     = "xpack.security.http.ssl.enabled"
	EnvXPackSecurityHttpSslKey                         = "xpack.security.http.ssl.key"
	EnvXPackSecurityTransportSslCertificate            = "xpack.security.transport.ssl.certificate"
	EnvXPackSecurityTransportSslCertificateAuthorities = "xpack.security.transport.ssl.certificate_authorities"
	EnvXPackSecurityTransportSslEnabled                = "xpack.security.transport.ssl.enabled"
	EnvXPackSecurityTransportSslKey                    = "xpack.security.transport.ssl.key"
	EnvXPackSecurityTransportSslTrustRestrictionsPath  = "xpack.security.transport.ssl.trust_restrictions.path"
	EnvXPackSecurityTransportSslVerificationMode       = "xpack.security.transport.ssl.verification_mode"
	EnvXPackSslCertificate                             = "xpack.ssl.certificate"
	EnvXPackSslCertificateAuthorities                  = "xpack.ssl.certificate_authorities"
	EnvXPackSslKey                                     = "xpack.ssl.key"
)

var (
	// ignoredVarsDuringComparison are environment variables that should be ignored when pods are compared to expected
	// pod specs
	ignoredVarsDuringComparison = []string{
		EnvNodeName,
		EnvDiscoveryZenMinimumMasterNodes,
		EnvNetworkPublishHost,
	}
)
