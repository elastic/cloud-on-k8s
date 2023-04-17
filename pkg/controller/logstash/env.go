// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
)

func buildEnv(esAssociations []commonv1.Association) ([]corev1.EnvVar, error) {
	var envs []corev1.EnvVar //nolint:prealloc
	for _, assoc := range esAssociations {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return nil, err
		}

		esRefName := normalize(getEsRefNamespacedName(assoc))
		envs = append(envs, createEnvVar(esRefName+"_ES_HOSTS", assocConf.GetURL()))
		envs = append(envs, createEnvVar(esRefName+"_ES_USERNAME", assocConf.AuthSecretKey))
		envs = append(envs, corev1.EnvVar{
			Name: esRefName + "_ES_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: assocConf.AuthSecretName,
					},
					Key: assocConf.AuthSecretKey,
				},
			},
		})

		if assocConf.GetCACertProvided() {
			caPath := filepath.Join(certificatesDir(assoc), certificates.CAFileName)
			envs = append(envs, createEnvVar(esRefName+"_ES_SSL_CERTIFICATE_AUTHORITY", caPath))
		}
	}

	return envs, nil
}

func getEsRefNamespacedName(assoc commonv1.Association) string {
	ref := assoc.AssociationRef()
	return fmt.Sprintf("%s_%s", ref.Namespace, ref.Name)
}

func normalize(nn string) string {
	return strings.ToUpper(strings.ReplaceAll(nn, "-", "_"))
}

func createEnvVar(key string, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  key,
		Value: value,
	}
}
