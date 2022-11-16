// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

const (
	writeIndex        = "my-index"
	writeIndexUpdated = "my-index-updated"

	sampleUser                = "myuser"
	samplePassword            = "mypassword"
	samplePasswordHash        = "$2a$10$qrurU7ju08g0eCXgh5qZmOWfKLhWMs/ca3uXz1l6.eFf09UH6YXFy" // nolint
	samplePasswordUpdated     = "mypasswordupdated"
	samplePasswordUpdatedHash = "$2a$10$ckqhC0BB5OdXJhR1J7vbiu21e9BxJU1V6HHLOqbSo.TlZAocWWnie" // nolint
	samplePasswordCleartext   = "my-cleartext-password"
	sampleUsersFile           = sampleUser + ":" + samplePasswordHash
	sampleUsersFileUpdated    = sampleUser + ":" + samplePasswordUpdatedHash
	sampleUsersRolesFile      = "test_role:" + sampleUser
)

// sampleRolesFile returns a role spec allowing writes on the given index.
func sampleRolesFile(writeIndex string) string {
	return fmt.Sprintf(`
test_role:
  indices:
  - names: [ '%s' ]
    privileges: [ 'create_index', 'write' ]
`, writeIndex)
}

// TestESUserProvidedAuth tests that user-provided file realm and roles are correctly propagated to Elasticsearch.
// It basically does the following:
// - specify a custom file realm user and role
// - check the user can be used to ingest documents into an index allowed by the role
// - check the user cannot ingest documents into a non-allowed index
// - modify the user password and role
// - check the user can ingest documents into another index matching the updated role, using the updated password
// - remove the file realm and role secret refs
// - check the user cannot ingest documents anymore
func TestESUserProvidedAuth(t *testing.T) {
	k := test.NewK8sClientOrFatal()
	b := elasticsearch.NewBuilder("test-es-user-auth").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	// setup our own roles through a secret ref
	rolesSecretName := b.Elasticsearch.Name + "-sample-roles"
	rolesSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rolesSecretName,
			Namespace: b.Elasticsearch.Namespace,
		},
		StringData: map[string]string{
			user.RolesFile: sampleRolesFile(writeIndex),
		},
	}
	// setup our own file realm through a secret ref
	fileRealmSecretName := b.Elasticsearch.Name + "-sample-file-realm"
	fileRealmSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fileRealmSecretName,
			Namespace: b.Elasticsearch.Namespace,
		},
		StringData: map[string]string{
			filerealm.UsersFile:      sampleUsersFile,
			filerealm.UsersRolesFile: sampleUsersRolesFile,
		},
	}
	b.Elasticsearch.Spec.Auth = esv1.Auth{
		Roles:     []esv1.RoleSource{{SecretRef: v1.SecretRef{SecretName: rolesSecretName}}},
		FileRealm: []esv1.FileRealmSource{{SecretRef: v1.SecretRef{SecretName: fileRealmSecretName}}},
	}
	authSecrets := []corev1.Secret{rolesSecret, fileRealmSecret}

	test.StepList{}.
		// create secure settings secret
		WithStep(test.Step{
			Name: "Create file realm and role secrets",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(authSecrets...)
			}),
		}).
		// create the cluster
		WithSteps(b.InitTestSteps(k)).
		// wait until cluster is alive
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		// check roles and file realm have been propagated
		WithSteps(test.StepList{
			test.Step{
				Name: "ES API should be accessible using the custom user password and role",
				Test: func(t *testing.T) {
					esUser := esclient.BasicAuth{Name: sampleUser, Password: samplePassword}
					expectedStatusCode := 201
					err := postDocument(b.Elasticsearch, k, esUser, writeIndex, expectedStatusCode)
					require.NoError(t, err)
				},
			},
			test.Step{
				Name: "Writing a document on a different index should be unauthorized by the role",
				Test: func(t *testing.T) {
					esUser := esclient.BasicAuth{Name: sampleUser, Password: samplePassword}
					expectedStatusCode := 403
					index := "another-index"
					err := postDocument(b.Elasticsearch, k, esUser, index, expectedStatusCode)
					require.NoError(t, err)
				},
			},
			test.Step{
				Name: "Update password in the file realm secret",
				Test: test.Eventually(func() error {
					var existingSecret corev1.Secret
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&fileRealmSecret), &existingSecret); err != nil {
						return err
					}
					existingSecret.StringData = map[string]string{
						filerealm.UsersFile:      sampleUsersFileUpdated,
						filerealm.UsersRolesFile: sampleUsersRolesFile,
					}
					return k.Client.Update(context.Background(), &existingSecret)
				}),
			},
			test.Step{
				Name: "Update role in the roles secret",
				Test: test.Eventually(func() error {
					var existingSecret corev1.Secret
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&rolesSecret), &existingSecret); err != nil {
						return err
					}
					existingSecret.StringData = map[string]string{
						user.RolesFile: sampleRolesFile(writeIndexUpdated),
					}
					return k.Client.Update(context.Background(), &existingSecret)
				}),
			},
			test.Step{
				Name: "ES API should eventually be accessible using the updated password and the updated role",
				Test: test.Eventually(func() error {
					esUser := esclient.BasicAuth{Name: sampleUser, Password: samplePasswordUpdated}
					expectedStatusCode := 201
					return postDocument(b.Elasticsearch, k, esUser, writeIndexUpdated, expectedStatusCode)
				}),
			},
			test.Step{
				Name: "Update the secret to be a basic auth secret with a clear text password",
				Test: test.Eventually(func() error {
					var existingSecret corev1.Secret
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&fileRealmSecret), &existingSecret); err != nil {
						return err
					}
					existingSecret.Data = nil
					existingSecret.StringData = map[string]string{
						corev1.BasicAuthUsernameKey:  sampleUser,
						corev1.BasicAuthPasswordKey:  samplePasswordCleartext,
						user.BasicAuthSecretRolesKey: "test_role",
					}
					return k.Client.Update(context.Background(), &existingSecret)
				}),
			},
			test.Step{
				Name: "ES API should eventually be accessible using the updated password and the updated role",
				Test: test.Eventually(func() error {
					esUser := esclient.BasicAuth{Name: sampleUser, Password: samplePasswordCleartext}
					expectedStatusCode := 201
					return postDocument(b.Elasticsearch, k, esUser, writeIndexUpdated, expectedStatusCode)
				}),
			},
			test.Step{
				Name: "Remove secrets ref in the ES spec",
				Test: test.Eventually(func() error {
					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Elasticsearch), &es); err != nil {
						return err
					}
					es.Spec.Auth = esv1.Auth{}
					return k.Client.Update(context.Background(), &es)
				}),
			},
			test.Step{
				Name: "ES API should eventually not be accessible anymore since user has been removed",
				Test: test.Eventually(func() error {
					esUser := esclient.BasicAuth{Name: sampleUser, Password: samplePasswordUpdated}
					expectedStatusCode := 401
					return postDocument(b.Elasticsearch, k, esUser, writeIndexUpdated, expectedStatusCode)
				}),
			},
			test.Step{
				Name: "Delete auth secrets",
				Test: test.Eventually(func() error {
					for _, s := range authSecrets {
						err := k.Client.Delete(context.Background(), &s)
						if err != nil && !apierrors.IsNotFound(err) {
							return err
						}
					}
					return nil
				}),
			},
		}).
		WithSteps(b.DeletionTestSteps(k)).
		RunSequential(t)
}

func postDocument(es esv1.Elasticsearch, k *test.K8sClient, user esclient.BasicAuth, index string, expectedStatusCode int) error {
	esClient, err := elasticsearch.NewElasticsearchClientWithUser(es, k, user)
	if err != nil {
		return err
	}

	doc := bytes.NewBufferString(`{"foo": "bar"}`)
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("/%s/_doc", index), doc)
	if err != nil {
		return err
	}
	resp, err := esClient.Request(context.Background(), req)

	// The client double wraps unexpected status codes in an fmt.wrapError and esclient.APIError,
	// but still returns the correct resp. We want to ignore APIErrors here.
	var apiError *esclient.APIError
	if !errors.As(err, &apiError) {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatusCode {
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	return nil
}
