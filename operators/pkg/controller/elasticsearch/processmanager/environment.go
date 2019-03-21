package processmanager

import corev1 "k8s.io/api/core/v1"

const (
	CommandPath = "/usr/share/elasticsearch/bin/process-manager"

	EnvProcessName = "PROC_NAME"
	EnvProcessCmd  = "PROC_CMD"
)

func NewEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: EnvProcessName, Value: "es"},
		{Name: EnvProcessCmd, Value: "/usr/local/bin/docker-entrypoint.sh"},
	}
}
