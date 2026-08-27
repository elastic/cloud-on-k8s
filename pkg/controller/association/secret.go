// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"hash"
	"maps"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	commonhash "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	esClient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// authUsernameUnmanagedSecretKey is the name of the key for the username when using a secret to reference an unmanaged resource
	authUsernameUnmanagedSecretKey = "username"
	// authPasswordUnmanagedSecretKey is the name of the key for the password when using a secret to reference an unmanaged resource
	authPasswordUnmanagedSecretKey = "password"
	// authAPIKeyUnmanagedSecretKey is the name of the key for the API key when using a secret to reference an unmanaged resource
	authAPIKeyUnmanagedSecretKey = "api-key"
)

const (
	AuthTypeUnmanagedBasic = iota
	AuthTypeUnmanagedAPIKey
)

// AdditionalSecretLabelName marks secrets created as cross-namespace copies by the additional-secrets
// mechanism. deleteStaleAdditionalSecrets uses this label (combined with association labels) to find
// and GC copies that are no longer needed.
const AdditionalSecretLabelName = "association.k8s.elastic.co/additional-secret"

// ExpectedConfigFromUnmanagedAssociation returns the association configuration to associate the external unmanaged resource referenced
// in the given association.
func (r *Reconciler) ExpectedConfigFromUnmanagedAssociation(association commonv1.Association) (commonv1.AssociationConf, error) {
	assocRef := association.AssociationRef()
	info, err := GetUnmanagedAssociationConnectionInfoFromSecret(r.Client, association)
	if err != nil {
		return commonv1.AssociationConf{}, err
	}

	ver, isServerless, err := r.ReferencedResourceVersion(r.Client, association)
	if err != nil {
		return commonv1.AssociationConf{}, err
	}

	// set url, version
	expectedAssocConf := commonv1.AssociationConf{
		Version:    ver,
		Serverless: isServerless,
		URL:        info.URL,
		// points the auth secret to the custom secret
		AuthSecretName: assocRef.GetSecretName(),
		CACertProvided: info.CaCert != "",
	}

	if info.APIKey != "" {
		expectedAssocConf.IsAPIKey = true
		expectedAssocConf.AuthSecretKey = authAPIKeyUnmanagedSecretKey
	} else {
		expectedAssocConf.IsAPIKey = false
		expectedAssocConf.AuthSecretKey = authPasswordUnmanagedSecretKey
	}

	// points the ca secret to the custom secret if needed
	if expectedAssocConf.CACertProvided {
		expectedAssocConf.CASecretName = assocRef.GetSecretName()
	}

	return expectedAssocConf, err
}

// UnmanagedAssociationConnectionInfo holds connection information stored in a custom Secret to reach over HTTP an Elastic resource not managed by ECK
// referenced in an Association. The resource can thus be external to the local Kubernetes cluster.
type UnmanagedAssociationConnectionInfo struct {
	URL      string
	Username string
	Password string
	APIKey   string
	CaCert   string
}

type UnmanagedAssociation interface {
	AssociationRef() commonv1.AssociationRef
	SupportsAuthAPIKey() bool
}

// GetUnmanagedAssociationConnectionInfoFromSecret returns the UnmanagedAssociationConnectionInfo corresponding to the Secret referenced in the ObjectSelector o.
func GetUnmanagedAssociationConnectionInfoFromSecret(c k8s.Client, association UnmanagedAssociation) (*UnmanagedAssociationConnectionInfo, error) {
	var secretRef corev1.Secret
	assocRef := association.AssociationRef()
	secretRefKey := assocRef.NamespacedName()
	if err := c.Get(context.Background(), secretRefKey, &secretRef); err != nil {
		return nil, err
	}

	ref := UnmanagedAssociationConnectionInfo{}
	caCert, ok := secretRef.Data[certificates.CAFileName]
	if ok {
		ref.CaCert = string(caCert)
	}

	url, ok := secretRef.Data["url"]
	if !ok {
		return nil, fmt.Errorf("url secret key doesn't exist in secret %s", assocRef.GetSecretName())
	}
	ref.URL = string(url)

	if association.SupportsAuthAPIKey() {
		if apiKey, ok := secretRef.Data[authAPIKeyUnmanagedSecretKey]; ok {
			ref.APIKey = string(apiKey)
			return &ref, nil
		}
	}

	username, ok := secretRef.Data[authUsernameUnmanagedSecretKey]
	if !ok {
		return nil, fmt.Errorf("username secret key doesn't exist in secret %s", assocRef.GetSecretName())
	}
	ref.Username = string(username)

	password, ok := secretRef.Data[authPasswordUnmanagedSecretKey]
	if !ok {
		return nil, fmt.Errorf("password secret key doesn't exist in secret %s", assocRef.GetSecretName())
	}
	ref.Password = string(password)

	return &ref, nil
}

// Version performs an HTTP GET request to the unmanaged Elastic resource at the given path and returns a string extracted
// from the returned result using the given json path and validates it is a valid semver version.
func (r UnmanagedAssociationConnectionInfo) Version(path string, versionPattern VersionPattern) (string, bool, error) {
	if err := r.Request(path, versionPattern); err != nil {
		return "", false, err
	}
	ver, err := versionPattern.GetVersion()
	if err != nil {
		return "", false, err
	}
	return ver, versionPattern.IsServerless(), nil
}

type VersionPattern interface {
	IsServerless() bool
	GetVersion() (string, error)
}

// Request performs an HTTP GET request to the unmanaged Elastic resource at the given path and returns a string extracted
// from the returned result using the given json path.
func (r UnmanagedAssociationConnectionInfo) Request(path string, out any) error {
	url := r.URL + path
	req, err := http.NewRequest("GET", url, nil) //nolint:noctx
	if err != nil {
		return err
	}

	if r.APIKey != "" {
		req.Header.Set("Authorization", "ApiKey "+r.APIKey)
	} else {
		req.SetBasicAuth(r.Username, r.Password)
	}

	httpClient := &http.Client{
		Timeout: esClient.DefaultESClientTimeout,
	}
	// configure CA if it exists
	if r.CaCert != "" {
		caCerts, err := certificates.ParsePEMCerts([]byte(r.CaCert))
		if err != nil {
			return err
		}
		certPool := x509.NewCertPool()
		for _, c := range caCerts {
			certPool.AddCert(c)
		}
		httpClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: certPool}}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("error requesting %q, statusCode = %d", url, resp.StatusCode)
	}

	if err = json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}

// filterUnmanagedElasticRef returns those associations that reference using a Kubernetes secret an Elastic resource not managed by ECK.
func filterUnmanagedElasticRef(associations []commonv1.Association) []commonv1.Association {
	var r []commonv1.Association
	for _, a := range associations {
		if a.AssociationRef().IsExternal() {
			r = append(r, a)
		}
	}
	return r
}

// filterManagedElasticRef returns those associations that reference an Elastic resource managed by ECK.
func filterManagedElasticRef(associations []commonv1.Association) []commonv1.Association {
	var r []commonv1.Association
	for _, a := range associations {
		if !a.AssociationRef().IsExternal() {
			r = append(r, a)
		}
	}
	return r
}

// copySecret copies the source secret into targetNamespace under destName, hashing the
// (filtered) source data into secHash. Keys restricts which data keys are included; all keys
// are copied when empty. extraLabels are merged onto the source labels. owner is set as the
// controller owner reference so Kubernetes GC removes the copy on deletion.
// Returns true when a secret reconciliation is issued (source and target in different namespaces),
// false when both share a namespace (hash-only path, no Secret is written).
func copySecret(
	ctx context.Context,
	c k8s.Client,
	secHash hash.Hash,
	targetNamespace string,
	source types.NamespacedName,
	keys []string,
	targetName string,
	owner client.Object,
	extraLabels map[string]string,
) (bool, error) {
	var original corev1.Secret
	if err := c.Get(ctx, source, &original); err != nil {
		return false, err
	}

	data := original.Data
	if len(keys) > 0 {
		data = make(map[string][]byte, len(keys))
		for _, k := range keys {
			if v, ok := original.Data[k]; ok {
				data[k] = v
			} else {
				ulog.FromContext(ctx).V(1).Info("requested key not found in source secret", "key", k, "secret_name", source.Name, "secret_namespace", source.Namespace)
			}
		}
	}

	// Hash only the data that will be copied. Hashing original.Data when keys is set
	// would cause AdditionalSecretsHash to change on credential rotations even though
	// the CA cert (the only field that actually reaches pods) has not changed.
	commonhash.WriteHashObject(secHash, data)
	if targetNamespace == original.Namespace {
		return false, nil
	}

	merged := make(map[string]string, len(original.Labels)+len(extraLabels)+1)
	maps.Copy(merged, original.Labels)
	maps.Copy(merged, extraLabels)
	merged[AdditionalSecretLabelName] = "true"

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        targetName,
			Namespace:   targetNamespace,
			Labels:      merged,
			Annotations: original.Annotations,
		},
		Data: data,
		Type: original.Type,
	}

	// kubectl.kubernetes.io/last-applied-configuration embeds the full original Secret
	// manifest as base64, which would re-expose any credential fields that were filtered
	// out of Data. Always drop it unconditionally.
	if _, err := reconciler.ReconcileSecret(ctx, c, expected, owner,
		reconciler.WithAnnotationsToRemove(corev1.LastAppliedConfigAnnotation)); err != nil {
		return false, err
	}
	return true, nil
}

// deleteStaleAdditionalSecrets lists secrets in targetNamespace that carry AdditionalSecretLabelName
// and the given assocLabels, then deletes those whose name is not in copiedNames.
// copiedNames holds the names of copies that must be preserved; any copy absent from the set is
// deleted. Pass nil to delete all matching copies (used by Unbind).
func deleteStaleAdditionalSecrets(ctx context.Context, c client.Client, targetNamespace string, assocLabels map[string]string, copiedNames sets.Set[string]) error {
	selector := make(client.MatchingLabels, len(assocLabels)+1)
	maps.Copy(selector, assocLabels)
	selector[AdditionalSecretLabelName] = "true"

	var secrets corev1.SecretList
	if err := c.List(ctx, &secrets, client.InNamespace(targetNamespace), selector); err != nil {
		return err
	}
	for i := range secrets.Items {
		if exists := copiedNames.Has(secrets.Items[i].Name); exists {
			continue
		}
		if err := k8s.DeleteSecretIfExists(ctx, c, k8s.ExtractNamespacedName(&secrets.Items[i])); err != nil {
			return err
		}
		ulog.FromContext(ctx).V(1).Info("Deleted stale additional secret copy", "secret_namespace", secrets.Items[i].Namespace, "secret_name", secrets.Items[i].Name)
	}
	return nil
}
