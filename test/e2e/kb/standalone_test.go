// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// +build kb e2e

package kb

import (
	"context"
	"fmt"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestKibanaStandalone tests running Kibana without an automatic association to Elasticsearch.
func TestKibanaStandalone(t *testing.T) {
	fileRealmSecretName := "kibana-user"
	kbUser := "kb-user"
	kbPassword := "mypassword"
	kbPasswordHash := "$2a$10$qrurU7ju08g0eCXgh5qZmOWfKLhWMs/ca3uXz1l6.eFf09UH6YXFy" // nolint

	fileRealmSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fileRealmSecretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		StringData: map[string]string{
			filerealm.UsersFile:      kbUser + ":" + kbPasswordHash,
			filerealm.UsersRolesFile: controller.KibanaSystemUserBuiltinRole + ":" + kbUser,
		},
	}

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{}.WithStep(test.Step{
			Name: "Create file realm secret",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(fileRealmSecret)
			}),
		})
	})

	after := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{}.WithStep(
			test.Step{
				Name: "Delete file realm secret",
				Test: test.Eventually(func() error {
					err := k.Client.Delete(context.Background(), &fileRealmSecret)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					return nil
				}),
			})
	})

	// set up a 1-node Kibana deployment manually connected to Elasticsearch
	name := "test-kb-standalone"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	esBuilder.Elasticsearch.Spec.Auth = esv1.Auth{
		FileRealm: []esv1.FileRealmSource{
			{SecretRef: commonv1.SecretRef{SecretName: fileRealmSecretName}}},
	}

	kbBuilder := kibana.NewBuilder(name).
		WithNodeCount(1).
		WithConfig(map[string]interface{}{
			"elasticsearch.hosts": []string{
				fmt.Sprintf("https://%s-es-http:9200", esBuilder.Name()),
			},
			"elasticsearch.username":             kbUser,
			"elasticsearch.password":             kbPassword,
			"elasticsearch.ssl.verificationMode": "none",
		}).
		// this is necessary for the Kibana e2e test steps to be able to run successfully
		// it does not actually set up the association
		WithExternalElasticsearchRef(commonv1.ObjectSelector{
			Namespace: esBuilder.Namespace(),
			Name:      esBuilder.Name(),
		}).
		WithRestrictedSecurityContext()

	test.BeforeAfterSequence(before, after, esBuilder, kbBuilder).RunSequential(t)
}
