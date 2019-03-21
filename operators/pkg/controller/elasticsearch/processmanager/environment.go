package processmanager

import corev1 "k8s.io/api/core/v1"

const (
	CommandPath = "/usr/share/elasticsearch/bin/process-manager"

	EnvProcName = "PM_PROC_NAME"
	EnvProcCmd  = "PM_PROC_CMD"
	EnvReaper   = "PM_REAPER"
	EnvTLS      = "PM_TLS"
	EnvCertPath = "PM_CERT_PATH"
	EnvKeyPath  = "PM_KEY_PATH"
)

func NewEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: EnvProcName, Value: "es"},
		{Name: EnvProcCmd, Value: "/usr/local/bin/docker-entrypoint.sh"},
	}
}
