// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"context"
	"io/ioutil"
	"net/http"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

// kind of a pain since the json objects have the name as the top level key
type repoResponse struct {
	RepoType string       `json:"type"`
	Settings repoSettings `json:"settings"`
}
type repoSettings struct {
}

func CheckSnapshotRepo(client client.Client) error {
	// https://www.elastic.co/guide/en/elasticsearch/reference/current/snapshots-register-repository.html
	// can also use /_all
	url := "/_snapshot/my_backup"
	_, err := GetBody(url, client)
	return err
}

// GetBody returns the body of a given URL. It is up to the caller to validate the response is useful
func GetBody(url string, esClient client.Client) ([]byte, error) {
	var resBytes []byte
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return resBytes, err
	}

	res, err := esClient.Request(context.Background(), req)
	if err != nil {
		return resBytes, err
	}
	defer res.Body.Close()
	resBytes, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return resBytes, err
	}
	return resBytes, nil
}
