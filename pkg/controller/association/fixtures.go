// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type testAPMServer struct {
	elasticsearchRef, kibanaRef          commonv1.ObjectSelector
	esConfAnnotations, kbConfAnnotations bool
	esAssociation, kbAssociation         *commonv1.AssociationConf
}

func (t testAPMServer) build() *apmv1.ApmServer {
	apmServer := &apmv1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "apm-server-test",
			Namespace:   "apm-ns",
			Annotations: make(map[string]string),
		},
		Spec: apmv1.ApmServerSpec{
			Image:            "test-image",
			Count:            1,
			ElasticsearchRef: t.elasticsearchRef,
			KibanaRef:        t.kibanaRef,
		},
	}

	if t.esAssociation != nil {
		apmv1.NewApmEsAssociation(apmServer).SetAssociationConf(t.esAssociation)
	}

	if t.kbAssociation != nil {
		apmv1.NewApmKibanaAssociation(apmServer).SetAssociationConf(t.kbAssociation)
	}

	if t.esConfAnnotations {
		apmServer.ObjectMeta.Annotations["association.k8s.elastic.co/es-conf"] = `{"authSecretName":"auth-secret", "authSecretKey":"apm-user", "caSecretName": "ca-secret", "url":"https://es.svc:9300"}`
	}

	if t.kbConfAnnotations {
		apmServer.ObjectMeta.Annotations["association.k8s.elastic.co/kb-conf"] = `{"authSecretName":"auth-secret", "authSecretKey":"apm-kb-user", "caSecretName": "ca-secret", "url":"https://kb.svc:5601"}`
	}
	return apmServer
}

func (t testAPMServer) withElasticsearchRef() testAPMServer {
	t.elasticsearchRef = commonv1.ObjectSelector{Name: "es"}
	return t
}

func (t testAPMServer) withKibanaRef() testAPMServer {
	t.kibanaRef = commonv1.ObjectSelector{Name: "kb"}
	return t
}

func (t testAPMServer) withElasticsearchAssoc() testAPMServer {
	t.esAssociation = &commonv1.AssociationConf{
		AuthSecretName: "auth-secret",
		AuthSecretKey:  "apm-user",
		CACertProvided: true,
		CASecretName:   "ca-secret",
		URL:            "https://es.svc:9300",
	}
	return t
}

func (t testAPMServer) withKibanaAssoc() testAPMServer {
	t.kbAssociation = &commonv1.AssociationConf{
		AuthSecretName: "auth-secret",
		AuthSecretKey:  "apm-kb-user",
		CACertProvided: true,
		CASecretName:   "ca-secret",
		URL:            "https://kb.svc:5601",
	}
	return t
}

func (t testAPMServer) withEsConfAnnotations() testAPMServer {
	t.esConfAnnotations = true
	return t
}

func (t testAPMServer) withKbConfAnnotations() testAPMServer {
	t.kbConfAnnotations = true
	return t
}

func newTestAPMServer() testAPMServer {
	return testAPMServer{}
}
