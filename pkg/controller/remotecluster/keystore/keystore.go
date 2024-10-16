// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/go-logr/logr"

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

	RemoteClusterAPIKeysType = "remote-cluster-api-keys"
)

var (
	credentialsSecretSettingsRegEx = regexp.MustCompile(`^cluster\.remote\.([\w-]+)\.credentials$`)
)

type APIKeyStore struct {
	log logr.Logger
	// aliases maps cluster aliased with the expected key ID
	aliases map[string]AliasValue
	// encodedKeys maps the remote cluster alias, as define in the client cluster, to the encoded cross-cluster API key.
	encodedKeys map[string]string
	// resourceVersion is the ResourceVersion as observed when the Secret has been loaded.
	resourceVersion string
	// uid is the UID of the Secret as observed when the Secret has been loaded.
	uid types.UID
	// pendingChanges are the pending changes, they are used to record changes until they are observed in the underlying Secret.
	pendingChanges *pendingChanges
}

type AliasValue struct {
	// Namespace of the remote cluster.
	Namespace string `json:"namespace"`
	// Name of the remote cluster.
	Name string `json:"name"`
	// ID is the key ID.
	ID string `json:"id"`
}

func (aks *APIKeyStore) GetAliases() map[string]AliasValue {
	if aks == nil {
		return nil
	}
	return aks.aliases
}

func (aks *APIKeyStore) KeyIDFor(alias string) string {
	if aks == nil {
		return ""
	}
	return aks.aliases[alias].ID
}

func loadAPIKeyStore(ctx context.Context, log logr.Logger, c k8s.Client, owner *esv1.Elasticsearch, pendingChanges *pendingChanges) (*APIKeyStore, error) {
	secretName := types.NamespacedName{
		Name:      esv1.RemoteAPIKeysSecretName(owner.Name),
		Namespace: owner.Namespace,
	}
	// Attempt to read the Secret
	keyStoreSecret := &corev1.Secret{}
	if err := c.Get(ctx, secretName, keyStoreSecret); err != nil {
		if errors.IsNotFound(err) {
			ulog.FromContext(ctx).V(1).Info("No APIKeyStore Secret found")
			// Return an empty store
			emptyKeystore := &APIKeyStore{log: log, pendingChanges: pendingChanges}
			return emptyKeystore.withPendingChanges(), nil
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
	encodedKeys := make(map[string]string)
	for settingName, encodedAPIKey := range keyStoreSecret.Data {
		strings := credentialsSecretSettingsRegEx.FindStringSubmatch(settingName)
		if len(strings) != 2 {
			ulog.FromContext(ctx).V(1).Info(
				fmt.Sprintf("Unknown remote cluster credential setting: %s", settingName),
			)
			continue
		}
		encodedKeys[strings[1]] = string(encodedAPIKey)
	}
	apiKeyStore := &APIKeyStore{
		log:             log,
		aliases:         aliases,
		encodedKeys:     encodedKeys,
		resourceVersion: keyStoreSecret.ResourceVersion,
		uid:             keyStoreSecret.UID,
		pendingChanges:  pendingChanges,
	}
	return apiKeyStore.withPendingChanges(), nil
}

// withPendingChanges checks if the pending changes are reflected in the Secret. If it is the case these changes are removed from the expected changes.
// If not there are "virtually" added to the current keystore.
func (aks *APIKeyStore) withPendingChanges() *APIKeyStore {
	pendingChanges := aks.pendingChanges.Get()
	var pendingAdds, pendingDeletions int
	for _, pendingChange := range pendingChanges {
		if pendingChange.key.IsEmpty() {
			if aks.KeyIDFor(pendingChange.alias) == "" {
				aks.log.Info(fmt.Sprintf("Change for alias %s observed, key has been deleted in API keystore", pendingChange.alias))
				aks.pendingChanges.ForgetDeleteAlias(pendingChange.alias)
				continue
			}
			// We are still expecting this deletion
			pendingDeletions++
			aks.Delete(pendingChange.alias)
			continue
		}
		// Check if the key is available in the underlying Secret
		if keyIDInSecret := aks.KeyIDFor(pendingChange.alias); keyIDInSecret == pendingChange.key.keyID {
			aks.log.Info(fmt.Sprintf("Change for alias %s observed, key %s saved in API keystore", pendingChange.alias, keyIDInSecret))
			// Forget this change
			aks.pendingChanges.ForgetAddKey(pendingChange.alias)
			continue
		}
		// Change is not reflected in the Secret yet.
		pendingAdds++
		aks.update(pendingChange.remoteClusterName, pendingChange.remoteClusterNamespace, pendingChange.alias, pendingChange.key.keyID, pendingChange.key.encodedValue)
	}

	if pendingAdds > 0 || pendingDeletions > 0 {
		aks.log.Info("Pending changes in API keystore", "add", pendingAdds, "deletion", pendingDeletions)
	}
	return aks
}

func (aks *APIKeyStore) Update(remoteClusterName, remoteClusterNamespace, alias, keyID, encodedKeyValue string) *APIKeyStore {
	// Save the change in memory
	aks.pendingChanges.AddKey(remoteClusterName, remoteClusterNamespace, alias, keyID, encodedKeyValue)
	// Load the change in this instance of the store
	aks.update(remoteClusterName, remoteClusterNamespace, alias, keyID, encodedKeyValue)
	return aks
}

func (aks *APIKeyStore) update(remoteClusterName, remoteClusterNamespace, alias, keyID, encodedKeyValue string) {
	if aks.aliases == nil {
		aks.aliases = make(map[string]AliasValue)
	}
	aks.aliases[alias] = AliasValue{
		Namespace: remoteClusterNamespace,
		Name:      remoteClusterName,
		ID:        keyID,
	}
	if aks.encodedKeys == nil {
		aks.encodedKeys = make(map[string]string)
	}
	aks.encodedKeys[alias] = encodedKeyValue
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
	// Save the change in memory
	aks.pendingChanges.DeleteAlias(alias)
	// Load the change in this instance of the store
	aks.delete(alias)
	return aks
}

func (aks *APIKeyStore) delete(alias string) *APIKeyStore {
	delete(aks.aliases, alias)
	delete(aks.encodedKeys, alias)
	return aks
}

const (
	credentialsKeyFormat = "cluster.remote.%s.credentials"
)

// Save sync the in memory content of the API keystore into the Secret.
func (aks *APIKeyStore) Save(ctx context.Context, c k8s.Client, owner *esv1.Elasticsearch) error {
	secretName := types.NamespacedName{
		Name:      esv1.RemoteAPIKeysSecretName(owner.Name),
		Namespace: owner.Namespace,
	}
	if aks.IsEmpty() {
		return aks.deleteSecret(ctx, c, secretName)
	}

	aliases, err := json.Marshal(aks.aliases)
	if err != nil {
		return err
	}
	data := make(map[string][]byte, len(aks.encodedKeys))
	for k, v := range aks.encodedKeys {
		data[fmt.Sprintf(credentialsKeyFormat, k)] = []byte(v)
	}
	expectedLabels := labels.AddCredentialsLabel(label.NewLabels(k8s.ExtractNamespacedName(owner)))
	expectedLabels[commonv1.TypeLabelName] = RemoteClusterAPIKeysType
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

func (aks *APIKeyStore) deleteSecret(ctx context.Context, c k8s.Client, secretName types.NamespacedName) error {
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
		if errors.IsNotFound(err) {
			return nil
		}
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

// ForCluster returns
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
