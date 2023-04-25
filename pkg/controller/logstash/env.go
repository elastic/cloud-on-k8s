// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"errors"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
)

func buildEnv(params Params, esAssociations []commonv1.Association) ([]corev1.EnvVar, error) {
	var envs []corev1.EnvVar //nolint:prealloc
	for _, assoc := range esAssociations {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return nil, err
		}

		credentials, err := association.ElasticsearchAuthSettings(params.Context, params.Client, assoc)
		if err != nil {
			return nil, err
		}

		clusterName, err := getClusterName(assoc)
		if err != nil {
			return nil, err
		}

		normalizedClusterName := normalize(clusterName)

		envs = append(envs, createEnvVar(normalizedClusterName+"_ES_HOSTS", assocConf.GetURL()))
		envs = append(envs, createEnvVar(normalizedClusterName+"_ES_USER", credentials.Username))
		envs = append(envs, corev1.EnvVar{
			Name: normalizedClusterName + "_ES_PASSWORD",
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
			envs = append(envs, createEnvVar(normalizedClusterName+"_ES_SSL_CERTIFICATE_AUTHORITY", caPath))
		}
	}

	return envs, nil
}

func getClusterName(assoc commonv1.Association) (string, error) {
	lses, ok := assoc.(*v1alpha1.LogstashESAssociation)
	if !ok {
		return "", errors.New("cannot cast association to LogstashESAssociation")
	}
	return lses.ClusterName, nil
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
