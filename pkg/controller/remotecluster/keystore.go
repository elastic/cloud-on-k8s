// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remotecluster

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/util/sets"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	aliasesAnnotationName = "elasticsearch.k8s.elastic.co/remote-cluster-api-keys"

	remoteClusterAPIKeysType = "remote-cluster-api-keys"
)

var (
	credentialsSecretSettingsRegEx = regexp.MustCompile(`^cluster\.remote\.([\w-]+)\.credentials$`)
)

type APIKeyStore struct {
	// aliases maps cluster aliased with the expected key ID
	aliases map[string]AliasValue
	// keys maps the ID of an API Key (not its name), to the encoded cross-cluster API key.
	keys map[string]string
	// resourceVersion is the ResourceVersion as observed when the Secret has been loaded.
	resourceVersion string
	// uid is the UID of the Secret as observed when the Secret has been loaded.
	uid types.UID
}

type AliasValue struct {
	// Namespace of the remote cluster.
	Namespace string `json:"namespace"`
	// Name of the remote cluster.
	Name string `json:"name"`
	// ID is the key ID.
	ID string `json:"id"`
}

func (aks *APIKeyStore) KeyIDFor(alias string) string {
	if aks == nil {
		return ""
	}
	return aks.aliases[alias].ID
}

func LoadAPIKeyStore(ctx context.Context, c k8s.Client, owner *esv1.Elasticsearch) (*APIKeyStore, error) {
	secretName := types.NamespacedName{
		Name:      esv1.RemoteAPIKeysSecretName(owner.Name),
		Namespace: owner.Namespace,
	}
	// Attempt to read the Secret
	keyStoreSecret := &corev1.Secret{}
	if err := c.Get(ctx, secretName, keyStoreSecret); err != nil {
		if errors.IsNotFound(err) {
			ulog.FromContext(ctx).V(1).Info("No APIKeyStore Secret found",
				"namespace", owner.Namespace,
				"es_name", owner.Name,
			)
			// Return an empty store
			return &APIKeyStore{}, nil
		}
	}

	// Read the key aliased
	aliases := make(map[string]AliasValue)
	if aliasesAnnotation, ok := keyStoreSecret.Annotations[aliasesAnnotationName]; ok {
		if err := json.Unmarshal([]byte(aliasesAnnotation), &aliases); err != nil {
			return nil, err
		}
	}

	// Read the current encoded cross-cluster API keys.
	keys := make(map[string]string)
	for settingName, encodedAPIKey := range keyStoreSecret.Data {
		strings := credentialsSecretSettingsRegEx.FindStringSubmatch(settingName)
		if len(strings) != 2 {
			ulog.FromContext(ctx).V(1).Info(
				fmt.Sprintf("Unknown remote cluster credential setting: %s", settingName),
				"namespace", owner.Namespace,
				"es_name", owner.Name,
			)
			continue
		}
		keys[strings[1]] = string(encodedAPIKey)
	}
	return &APIKeyStore{
		aliases:         aliases,
		keys:            keys,
		resourceVersion: keyStoreSecret.ResourceVersion,
		uid:             keyStoreSecret.UID,
	}, nil
}

func (aks *APIKeyStore) Update(remoteClusterName, remoteClusterNamespace, alias, keyID, encodedKeyValue string) *APIKeyStore {
	if aks.aliases == nil {
		aks.aliases = make(map[string]AliasValue)
	}
	aks.aliases[alias] = AliasValue{
		Namespace: remoteClusterNamespace,
		Name:      remoteClusterName,
		ID:        keyID,
	}
	if aks.keys == nil {
		aks.keys = make(map[string]string)
	}
	aks.keys[alias] = encodedKeyValue
	return aks
}

func (aks *APIKeyStore) Aliases() []string {
	if aks == nil {
		return nil
	}
	aliases := make([]string, len(aks.aliases))
	i := 0
	for alias := range aks.aliases {
		aliases[i] = alias
		i++
	}
	return aliases
}

func (aks *APIKeyStore) Delete(alias string) *APIKeyStore {
	delete(aks.aliases, alias)
	delete(aks.keys, alias)
	return aks
}

const (
	credentialsKeyFormat = "cluster.remote.%s.credentials"
)

func (aks *APIKeyStore) Save(ctx context.Context, c k8s.Client, owner *esv1.Elasticsearch) error {
	secretName := types.NamespacedName{
		Name:      esv1.RemoteAPIKeysSecretName(owner.Name),
		Namespace: owner.Namespace,
	}
	if aks.IsEmpty() {
		// Check if the Secret still exist.
		if err := c.Get(ctx, secretName, &corev1.Secret{}); err != nil {
			if errors.IsNotFound(err) {
				// Secret does not exist.
				return nil
			}
			return err
		}
		// Delete the Secret used to load the current state.
		deleteOptions := make([]client.DeleteOption, 0, 2)
		if aks.uid != "" {
			deleteOptions = append(deleteOptions, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &aks.uid}})
		}
		if aks.resourceVersion != "" {
			deleteOptions = append(deleteOptions, &client.DeleteOptions{Preconditions: &metav1.Preconditions{ResourceVersion: &aks.resourceVersion}})
		}

		if err := c.Delete(ctx,
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName.Name, Namespace: secretName.Namespace}},
			deleteOptions...,
		); err != nil {
			return err
		}
		return nil
	}

	aliases, err := json.Marshal(aks.aliases)
	if err != nil {
		return err
	}
	data := make(map[string][]byte, len(aks.keys))
	for k, v := range aks.keys {
		data[fmt.Sprintf(credentialsKeyFormat, k)] = []byte(v)
	}
	expectedLabels := labels.AddCredentialsLabel(label.NewLabels(k8s.ExtractNamespacedName(owner)))
	expectedLabels[commonv1.TypeLabelName] = remoteClusterAPIKeysType
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName.Name,
			Namespace: secretName.Namespace,
			Annotations: map[string]string{
				aliasesAnnotationName: string(aliases),
			},
			Labels: expectedLabels,
		},
		Data: data,
	}
	if _, err := reconciler.ReconcileSecret(ctx, c, expected, owner); err != nil {
		return err
	}
	return nil
}

func (aks *APIKeyStore) IsEmpty() bool {
	if aks == nil {
		return true
	}
	return len(aks.aliases) == 0
}

func (aks *APIKeyStore) ForCluster(namespace string, name string) sets.Set[string] {
	aliases := sets.New[string]()
	if aks == nil {
		return aliases
	}
	for alias, c := range aks.aliases {
		if c.Name == name && c.Namespace == namespace {
			aliases.Insert(alias)
		}
	}
	return aliases
}
