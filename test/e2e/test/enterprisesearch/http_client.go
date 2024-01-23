// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/types"

	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

const (
	ReqTimeout          = 1 * time.Minute
	AppSearchPrivateKey = "private-key"
)

type EnterpriseSearchClient struct { //nolint:revive
	httpClient *http.Client
	endpoint   string
	username   string
	password   string
}

func NewEnterpriseSearchClient(ent entv1.EnterpriseSearch, k *test.K8sClient) (EnterpriseSearchClient, error) {
	var caCerts []*x509.Certificate
	scheme := "http"
	if ent.Spec.HTTP.TLS.Enabled() {
		scheme = "https"
		crts, err := k.GetHTTPCerts(entv1.Namer, ent.Namespace, ent.Name)
		if err != nil {
			return EnterpriseSearchClient{}, err
		}
		caCerts = crts
	}
	esNamespace := ent.Spec.ElasticsearchRef.Namespace
	if len(esNamespace) == 0 {
		esNamespace = ent.Namespace
	}
	password, err := k.GetElasticPassword(types.NamespacedName{Namespace: esNamespace, Name: ent.Spec.ElasticsearchRef.Name})
	if err != nil {
		return EnterpriseSearchClient{}, err
	}
	endpoint := fmt.Sprintf("%s://%s.%s.svc:%d", scheme, ent.Name+"-ent-http", ent.Namespace, enterprisesearch.HTTPPort)
	return EnterpriseSearchClient{
		httpClient: test.NewHTTPClient(caCerts),
		endpoint:   endpoint,
		username:   "elastic",
		password:   password,
	}, nil
}

func (e EnterpriseSearchClient) doRequest(request *http.Request) ([]byte, error) {
	if len(request.Header.Get("Authorization")) == 0 {
		// no api key set, use basic auth
		request.SetBasicAuth(e.username, e.password)
	}

	request.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), ReqTimeout)
	defer cancel()

	resp, err := e.httpClient.Do(request.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("http response status code is %d)", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (e EnterpriseSearchClient) HealthCheck() error {
	// check the endpoint responds to requests
	url := e.endpoint + "/api/ent/v1/internal/health"
	req, err := http.NewRequest(http.MethodGet, url, http.NoBody) //nolint:noctx
	if err != nil {
		return err
	}
	_, err = e.doRequest(req)
	return err
}

func (e EnterpriseSearchClient) AppSearch() AppSearchClient {
	return AppSearchClient{EnterpriseSearchClient: e}
}

type AppSearchClient struct {
	EnterpriseSearchClient
	apiKey string
}

func (a AppSearchClient) WithAPIKey(key string) AppSearchClient {
	a.apiKey = key
	return a
}

func (a AppSearchClient) doRequest(request *http.Request) ([]byte, error) {
	if len(a.apiKey) > 0 {
		// set the api key header
		request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.apiKey))
	}
	return a.EnterpriseSearchClient.doRequest(request)
}

type CredentialsCollection struct {
	Results []struct {
		Name string `json:"name"`
		Key  string `json:"key"`
	} `json:"results"`
}

func (a AppSearchClient) GetAPIKey() (string, error) {
	url := a.endpoint + "/as/credentials/collection"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := a.doRequest(req)
	if err != nil {
		return "", err
	}

	var credentials CredentialsCollection
	if err := json.Unmarshal(resp, &credentials); err != nil {
		return "", err
	}
	for _, res := range credentials.Results {
		if res.Name == AppSearchPrivateKey {
			return res.Key, nil
		}
	}

	return "", fmt.Errorf("cannot find %s in results (len %d)", AppSearchPrivateKey, len(credentials.Results))
}

type Results struct {
	Results []map[string]interface{} `json:"results"`
}

func (a AppSearchClient) GetEngines() (Results, error) {
	url := a.endpoint + "/api/as/v1/engines"

	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx
	if err != nil {
		return Results{}, err
	}
	respBody, err := a.doRequest(req)
	if err != nil {
		return Results{}, err
	}
	var results Results
	return results, json.Unmarshal(respBody, &results)
}

func (a AppSearchClient) CreateEngine(name string) error {
	url := a.endpoint + "/api/as/v1/engines"
	body := []byte(fmt.Sprintf(`{"name": "%s"}`, name))

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	_, err = a.doRequest(req)
	return err
}

func (a AppSearchClient) GetDocuments(engineName string) (Results, error) {
	url := a.endpoint + fmt.Sprintf("/api/as/v1/engines/%s/documents/list", engineName)

	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx
	if err != nil {
		return Results{}, err
	}
	respBody, err := a.doRequest(req)
	if err != nil {
		return Results{}, err
	}
	var results Results
	return results, json.Unmarshal(respBody, &results)
}

func (a AppSearchClient) IndexDocument(engineName string, document string) error {
	url := a.endpoint + fmt.Sprintf("/api/as/v1/engines/%s/documents", engineName)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer([]byte(document))) //nolint:noctx
	if err != nil {
		return err
	}
	_, err = a.doRequest(req)
	return err
}

func (a AppSearchClient) SearchDocuments(engineName string, query string) (Results, error) {
	url := a.endpoint + fmt.Sprintf("/api/as/v1/engines/%s/search", engineName)
	body := fmt.Sprintf(`{"query": "%s"}`, query)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer([]byte(body))) //nolint:noctx
	if err != nil {
		return Results{}, err
	}

	respBody, err := a.doRequest(req)
	if err != nil {
		return Results{}, err
	}

	var results Results
	return results, json.Unmarshal(respBody, &results)
}
