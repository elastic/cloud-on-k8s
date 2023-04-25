// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_getEnvVars(t *testing.T) {
	fakeLogstashUserSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "logstash-sample-default-elasticsearch-sample-logstash-user", Namespace: "default"},
		Data:       map[string][]byte{"default-logstash-sample-default-elasticsearch-sample-logstash-user": []byte("1234567890")},
	}

	fakeExternalEsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "external-cloud-es-ref", Namespace: "default"},
		Data: map[string][]byte{
			"url":      []byte("https://some.gcp.cloud.es.io"),
			"username": []byte("fake_user"),
			"password": []byte("fake_password"),
		},
	}

	params := Params{
		Logstash: logstashv1alpha1.Logstash{
			Spec: logstashv1alpha1.LogstashSpec{
				ElasticsearchRefs: []logstashv1alpha1.ElasticsearchCluster{
					{
						ObjectSelector: commonv1.ObjectSelector{Name: "elasticsearch-sample", Namespace: "default"},
						ClusterName:    "production",
					},
				},
			},
		},
		Client:  k8s.NewFakeClient(&fakeLogstashUserSecret, &fakeExternalEsSecret),
		Context: context.Background(),
	}

	for _, tt := range []struct {
		name          string
		params        Params
		setAssocConfs func(assocs []commonv1.Association)
		wantEnvs      []corev1.EnvVar
	}{
		{
			name: "no es ref",
			params: Params{
				Logstash: logstashv1alpha1.Logstash{
					Spec: logstashv1alpha1.LogstashSpec{},
				},
				Client:  k8s.NewFakeClient(),
				Context: context.Background(),
			},
			setAssocConfs: func(assocs []commonv1.Association) {},
			wantEnvs:      []corev1.EnvVar(nil),
		},
		{
			name:   "es ref",
			params: params,
			setAssocConfs: func(assocs []commonv1.Association) {
				assocs[0].SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "logstash-sample-default-elasticsearch-sample-logstash-user",
					AuthSecretKey:  "default-logstash-sample-default-elasticsearch-sample-logstash-user",
					CACertProvided: true,
					CASecretName:   "logstash-sample-logstash-es-default-elasticsearch-sample-ca",
					URL:            "https://elasticsearch-sample-es-http.default.svc:9200",
					Version:        "8.7.0",
				})
				assocs[0].SetNamespace("default")
			},
			wantEnvs: []corev1.EnvVar{
				{Name: "PRODUCTION_ES_HOSTS", Value: "https://elasticsearch-sample-es-http.default.svc:9200"},
				{Name: "PRODUCTION_ES_USER", Value: "default-logstash-sample-default-elasticsearch-sample-logstash-user"},
				{Name: "PRODUCTION_ES_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "logstash-sample-default-elasticsearch-sample-logstash-user",
							},
							Key: "default-logstash-sample-default-elasticsearch-sample-logstash-user",
						},
					},
				},
				{Name: "PRODUCTION_ES_SSL_CERTIFICATE_AUTHORITY", Value: "/mnt/elastic-internal/elasticsearch-association/default/elasticsearch-sample/certs/ca.crt"},
			},
		},
		{
			name:   "es ref without tls",
			params: params,
			setAssocConfs: func(assocs []commonv1.Association) {
				assocs[0].SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "logstash-sample-default-elasticsearch-sample-logstash-user",
					AuthSecretKey:  "default-logstash-sample-default-elasticsearch-sample-logstash-user",
					CACertProvided: false,
					URL:            "http://elasticsearch-sample-es-http.default.svc:9200",
					Version:        "8.7.0",
				})
				assocs[0].SetNamespace("default")
			},
			wantEnvs: []corev1.EnvVar{
				{Name: "PRODUCTION_ES_HOSTS", Value: "http://elasticsearch-sample-es-http.default.svc:9200"},
				{Name: "PRODUCTION_ES_USER", Value: "default-logstash-sample-default-elasticsearch-sample-logstash-user"},
				{Name: "PRODUCTION_ES_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "logstash-sample-default-elasticsearch-sample-logstash-user",
							},
							Key: "default-logstash-sample-default-elasticsearch-sample-logstash-user",
						},
					},
				},
			},
		},
		{
			name:   "es ref with secretName",
			params: params,
			setAssocConfs: func(assocs []commonv1.Association) {
				assocs[0].SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "external-cloud-es-ref",
					AuthSecretKey:  "password",
					CACertProvided: false,
					CASecretName:   "",
					URL:            "https://some.gcp.cloud.es.io",
					Version:        "8.7.0",
				})
				assocs[0].SetNamespace("default")
			},
			wantEnvs: []corev1.EnvVar{
				{Name: "PRODUCTION_ES_HOSTS", Value: "https://some.gcp.cloud.es.io"},
				{Name: "PRODUCTION_ES_USER", Value: "fake_user"},
				{Name: "PRODUCTION_ES_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "external-cloud-es-ref",
							},
							Key: "password",
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assocs := tt.params.Logstash.GetAssociations()
			tt.setAssocConfs(assocs)
			envs, err := buildEnv(params, assocs)
			require.NoError(t, err)
			require.Equal(t, tt.wantEnvs, envs)
		})
	}
}
