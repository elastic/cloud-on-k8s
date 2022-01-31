// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/jsonpath"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// authPasswordUnmanagedSecretKey is the name of the key for the password when using a secret to reference an unmanaged resource
const authPasswordUnmanagedSecretKey = "password"

func (r *Reconciler) ReconcileUnmanagedAssociation(ctx context.Context, association commonv1.Association) (commonv1.AssociationStatus, error) {
	assocRef := association.AssociationRef()
	info, err := GetUnmanagedAssociationConnexionInfoFromSecret(r.Client, assocRef)
	if err != nil {
		return commonv1.AssociationFailed, err
	}

	var ver string
	ver, err = r.ReferencedResourceVersion(r.Client, assocRef)
	if err != nil {
		return commonv1.AssociationFailed, err
	}

	// set url, version
	expectedAssocConf := commonv1.AssociationConf{
		Version: ver,
		URL:     info.URL,
		// points the auth secret to the custom secret
		AuthSecretName: assocRef.Name,
		AuthSecretKey:  authPasswordUnmanagedSecretKey,
		CACertProvided: info.CaCert != "",
	}
	// points the ca secret to the custom secret if needed
	if expectedAssocConf.CACertProvided {
		expectedAssocConf.CASecretName = assocRef.Name
	}

	return r.updateAssocConf(ctx, &expectedAssocConf, association)
}

func GetAuthFromUnmanagedSecretOr(client k8s.Client, unmanagedAssocRef commonv1.ObjectSelector, other func() (string, string, error)) (string, string, error) {
	if unmanagedAssocRef.IsObjectTypeSecret() {
		info, err := GetUnmanagedAssociationConnexionInfoFromSecret(client, unmanagedAssocRef)
		if err != nil {
			return "", "", err
		}
		return info.Username, info.Password, nil
	}
	return other()
}

// UnmanagedAssociationConnexionInfo holds connection information stored in a custom Secret to reach over HTTP an Elastic resource not managed by ECK
// referenced in an Association. The resource can thus be external to the local Kubernetes cluster.
type UnmanagedAssociationConnexionInfo struct {
	URL      string
	Username string
	Password string
	CaCert   string
}

// GetUnmanagedAssociationConnexionInfoFromSecret returns the UnmanagedAssociationConnexionInfo corresponding to the Secret referenced in the ObjectSelector o.
func GetUnmanagedAssociationConnexionInfoFromSecret(c k8s.Client, o commonv1.ObjectSelector) (*UnmanagedAssociationConnexionInfo, error) {
	var secretRef corev1.Secret
	secretRefKey := o.NamespacedName()
	if err := c.Get(context.Background(), secretRefKey, &secretRef); err != nil {
		return nil, err
	}
	url, ok := secretRef.Data["url"]
	if !ok {
		return nil, fmt.Errorf("url secret key doesn't exist in secret %s", o.Name)
	}
	username, ok := secretRef.Data["username"]
	if !ok {
		return nil, fmt.Errorf("username secret key doesn't exist in secret %s", o.Name)
	}
	password, ok := secretRef.Data[authPasswordUnmanagedSecretKey]
	if !ok {
		return nil, fmt.Errorf("password secret key doesn't exist in secret %s", o.Name)
	}

	ref := UnmanagedAssociationConnexionInfo{URL: string(url), Username: string(username), Password: string(password)}
	caCert, ok := secretRef.Data[certificates.CAFileName]
	if ok {
		ref.CaCert = string(caCert)
	}

	return &ref, nil
}

func (r UnmanagedAssociationConnexionInfo) Request(path string, jsonPath string) (string, error) {
	req, err := http.NewRequest("GET", r.URL+path, nil) //nolint:noctx
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(r.Username, r.Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("error requesting %s, statusCode = %d", path, resp.StatusCode)
	}

	defer resp.Body.Close()
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
	ver := buf.String()

	// validate the version
	if _, err := version.Parse(ver); err != nil {
		return "", err
	}

	return ver, nil
}

// filterUnmanagedElasticRef returns those associations that reference using a Kubernetes secret an Elastic resource not managed by ECK.
func filterUnmanagedElasticRef(associations []commonv1.Association) []commonv1.Association {
	var r []commonv1.Association
	for _, a := range associations {
		if a.AssociationRef().IsObjectTypeSecret() {
			r = append(r, a)
		}
	}
	return r
}

// filterManagedElasticRef returns those associations that reference an Elastic resource managed by ECK.
func filterManagedElasticRef(associations []commonv1.Association) []commonv1.Association {
	var r []commonv1.Association
	for _, a := range associations {
		if !a.AssociationRef().IsObjectTypeSecret() {
			r = append(r, a)
		}
	}
	return r
}
