// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	corev1 "k8s.io/api/core/v1"
)

// fileLogStyleEnvVar returns the environment variable to configure the Logstash container to write logs to disk
func fileLogStyleEnvVar() corev1.EnvVar {
	return corev1.EnvVar{Name: "LOG_STYLE", Value: "file"}
}
