// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func reconcileRefSecret(c k8s.Client, assocRef commonv1.ObjectSelector) (*commonv1.AssociationConf, error) {
	ref, err := GetRefObjectFromSecret(c, assocRef)
	if err != nil {
		return nil, err
	}

	// request / to extract version and check the HTTP connexion
	clusterInfo, err := ref.requestRoot()
	if err != nil {
		return nil, err
	}

	// set url, version
	assocConf := commonv1.AssociationConf{
		Version:        clusterInfo.Version.Number,
		URL:            ref.URL,
		CACertProvided: ref.CaCert == "",
	}
	// points the ca secret to the ref secret if needed
	if assocConf.CACertProvided {
		assocConf.CASecretName = assocRef.Name
	}

	return &assocConf, nil
}

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

// RefObject holds data stored in a custom Secret to reach over HTTP a referenced Elastic resource external to the
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
	password, ok := secretRef.Data["password"]
	if !ok {
		return nil, fmt.Errorf("password secret key doesn't exist in secret %s", o.Name)
	}

	ref := RefObject{URL: string(url), Username: string(username), Password: string(password)}
	caCert, ok := secretRef.Data["ca.cert"]
	if ok {
		ref.CaCert = string(caCert)
	}

	return &ref, nil
}

func (r RefObject) requestRoot() (*esclient.Info, error) {
	req, err := http.NewRequest("GET", r.URL, nil) //nolint:noctx
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(r.Username, r.Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("error requesting /, statusCode = %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var clusterInfo esclient.Info // TODO: handle Elastic resource other than ES
	if err := json.NewDecoder(resp.Body).Decode(&clusterInfo); err != nil {
		return nil, errors.Wrap(err, "Error request /")
	}

	return &clusterInfo, nil
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
