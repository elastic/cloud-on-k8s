// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"hash"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/jsonpath"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	commonhash "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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

// ExpectedConfigFromUnmanagedAssociation returns the association configuration to associate the external unmanaged resource referenced
// in the given association.
func (r *Reconciler) ExpectedConfigFromUnmanagedAssociation(association commonv1.Association) (commonv1.AssociationConf, error) {
	assocRef := association.AssociationRef()
	info, err := GetUnmanagedAssociationConnectionInfoFromSecret(r.Client, association)
	if err != nil {
		return commonv1.AssociationConf{}, err
	}

	var ver string
	ver, err = r.ReferencedResourceVersion(r.Client, association)
	if err != nil {
		return commonv1.AssociationConf{}, err
	}

	// set url, version
	expectedAssocConf := commonv1.AssociationConf{
		Version: ver,
		URL:     info.URL,
		// points the auth secret to the custom secret
		AuthSecretName: assocRef.SecretName,
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
		expectedAssocConf.CASecretName = assocRef.SecretName
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
	AssociationRef() commonv1.ObjectSelector
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
		return nil, fmt.Errorf("url secret key doesn't exist in secret %s", assocRef.SecretName)
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
		return nil, fmt.Errorf("username secret key doesn't exist in secret %s", assocRef.SecretName)
	}
	ref.Username = string(username)

	password, ok := secretRef.Data[authPasswordUnmanagedSecretKey]
	if !ok {
		return nil, fmt.Errorf("password secret key doesn't exist in secret %s", assocRef.SecretName)
	}
	ref.Password = string(password)

	return &ref, nil
}

// Version performs an HTTP GET request to the unmanaged Elastic resource at the given path and returns a string extracted
// from the returned result using the given json path and validates it is a valid semver version.
func (r UnmanagedAssociationConnectionInfo) Version(path string, jsonPath string) (string, error) {
	ver, err := r.Request(path, jsonPath)
	if err != nil {
		return "", err
	}

	// validate the version
	if _, err := version.Parse(ver); err != nil {
		return "", err
	}

	return ver, nil
}

// Request performs an HTTP GET request to the unmanaged Elastic resource at the given path and returns a string extracted
// from the returned result using the given json path.
func (r UnmanagedAssociationConnectionInfo) Request(path string, jsonPath string) (string, error) {
	url := r.URL + path
	req, err := http.NewRequest("GET", url, nil) //nolint:noctx
	if err != nil {
		return "", err
	}

	if r.APIKey != "" {
		req.Header.Set("Authorization", "ApiKey "+r.APIKey)
	} else {
		req.SetBasicAuth(r.Username, r.Password)
	}

	httpClient := &http.Client{
		Timeout: client.DefaultESClientTimeout,
	}
	// configure CA if it exists
	if r.CaCert != "" {
		caCerts, err := certificates.ParsePEMCerts([]byte(r.CaCert))
		if err != nil {
			return "", err
		}
		certPool := x509.NewCertPool()
		for _, c := range caCerts {
			certPool.AddCert(c)
		}
		httpClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: certPool}} //nolint:gosec
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("error requesting %q, statusCode = %d", url, resp.StatusCode)
	}

	var obj interface{}
	if err = json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return "", err
	}

	// extract the version using the json path
	j := jsonpath.New(jsonPath)
	if err := j.Parse(jsonPath); err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	if err := j.Execute(buf, obj); err != nil {
		return "", err
	}
	return buf.String(), nil
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

// copySecret will copy the source secret to the target namespace adding labels from the associated object to ensure garbage collection happens.
func copySecret(ctx context.Context, client k8s.Client, secHash hash.Hash, targetNamespace string, source types.NamespacedName) error {
	var original corev1.Secret
	if err := client.Get(ctx, source, &original); err != nil {
		return err
	}
	// update the hash if there are additional secrets event if
	// they are in the same namespace to ensure that the pods are
	// rotated when the original CA secret is updated.
	commonhash.WriteHashObject(secHash, original.Data)
	if targetNamespace == original.Namespace {
		return nil
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        original.Name,
			Namespace:   targetNamespace,
			Labels:      original.Labels,
			Annotations: original.Annotations,
		},
		Data: original.Data,
		Type: original.Type,
	}

	_, err := reconciler.ReconcileSecret(ctx, client, expected, nil)
	return err
}
