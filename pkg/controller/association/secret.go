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

// authPasswordRefSecretKey is the name of the key for the password when using a custom secret
const authPasswordRefSecretKey = "password"

func GetAuthFromSecretOr(client k8s.Client, assocRef commonv1.ObjectSelector, other func() (string, string, error)) (string, string, error) {
	if assocRef.IsObjectTypeSecret() {
		ref, err := GetRefObjectFromSecret(client, assocRef)
		if err != nil {
			return "", "", err
		}
		return ref.Username, ref.Password, nil
	}
	return other()
}

// RefObject holds connection information stored in a custom Secret to reach over HTTP a referenced Elastic resource external to the
// local Kubernetes cluster.
type RefObject struct {
	URL      string
	Username string
	Password string
	CaCert   string
}

// GetRefObjectFromSecret returns the RefObject corresponding to the Secret referenced in the ObjectSelector o
func GetRefObjectFromSecret(c k8s.Client, o commonv1.ObjectSelector) (*RefObject, error) {
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
	password, ok := secretRef.Data[authPasswordRefSecretKey]
	if !ok {
		return nil, fmt.Errorf("password secret key doesn't exist in secret %s", o.Name)
	}

	ref := RefObject{URL: string(url), Username: string(username), Password: string(password)}
	caCert, ok := secretRef.Data[certificates.CAFileName]
	if ok {
		ref.CaCert = string(caCert)
	}

	return &ref, nil
}

func (r RefObject) Request(path string, jsonPath string) (string, error) {
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
	err = json.NewDecoder(resp.Body).Decode(&obj)
	if err != nil {
		return "", err
	}

	// extract the version using the json path
	j := jsonpath.New(jsonPath)
	if err := j.Parse(jsonPath); err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	err = j.Execute(buf, obj)
	if err != nil {
		return "", err
	}
	ver := buf.String()

	// valid the version
	_, err = version.Parse(ver)
	if err != nil {
		return "", err
	}

	return ver, nil
}

// filterSecretRef returns those associations that reference a Kubernetes secret.
func filterSecretRef(associations []commonv1.Association) []commonv1.Association {
	var r []commonv1.Association
	for _, a := range associations {
		if a.AssociationRef().IsObjectTypeSecret() {
			r = append(r, a)
		}
	}
	return r
}

// filterElasticRef returns those associations that reference an Elastic resource.
func filterElasticRef(associations []commonv1.Association) []commonv1.Association {
	var r []commonv1.Association
	for _, a := range associations {
		if !a.AssociationRef().IsObjectTypeSecret() {
			r = append(r, a)
		}
	}
	return r
}
