// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	docker_client "github.com/docker/docker/client"
)

type redhatRepsonsesFunc func() *httptest.Server

func getImagesResponse(tag string, status string) []byte {
	response := `{ "data": [
		{
			"_id": "61afdc84702c563e8996407e",
			"architecture": "amd64",
			"certifications": [],
			"cpe_ids_rh_base_images": [],
			"freshness_grades": [],
			"object_type": "containerImage",
			"parsed_data": {},
			"repositories": [
				{
					"_links": {
						"repository": {
							"href": "/v1/repositories/registry/registry.rhc4tp.engineering.redhat.com/repository/ospid-664938b1-f0c8-4989-99de-be0992395aa0/eck-operator"
						}
					},
					"push_date": "2021-12-07T22:39:22.039000+00:00",
					"registry": "registry.rhc4tp.engineering.redhat.com",
					"repository": "ospid-664938b1-f0c8-4989-99de-be0992395aa0/eck-operator",
					"tags": [
						{
							"_links": {
								"tag_history": {
									"href": "/v1/tag-history/registry/registry.rhc4tp.engineering.redhat.com/repository/ospid-664938b1-f0c8-4989-99de-be0992395aa0/eck-operator/tag/%[1]s"
								}
							},
							"added_date": "2021-12-07T22:39:22.039000+00:00",
							"name": "%[1]s"
						},
						{
							"_links": {
								"tag_history": {
									"href": "/v1/tag-history/registry/registry.rhc4tp.engineering.redhat.com/repository/ospid-664938b1-f0c8-4989-99de-be0992395aa0/eck-operator/tag/%[1]s"
								}
							},
							"added_date": "2021-12-07T22:39:23.189000+00:00",
							"name": "%[1]s"
						}
					]
				},
				{
					"_links": {
						"repository": {
							"href": "/v1/repositories/registry/registry.connect.redhat.com/repository/elastic/eck-operator"
						}
					},
					"push_date": "2021-12-08T02:47:04+00:00",
					"registry": "registry.connect.redhat.com",
					"repository": "elastic/eck-operator",
					"tags": [
						{
							"_links": {
								"tag_history": {
									"href": "/v1/tag-history/registry/registry.connect.redhat.com/repository/elastic/eck-operator/tag/latest"
								}
							},
							"added_date": "2021-12-07T22:47:05.481000+00:00",
							"name": "latest"
						},
						{
							"_links": {
								"tag_history": {
									"href": "/v1/tag-history/registry/registry.connect.redhat.com/repository/elastic/eck-operator/tag/%s"
								}
							},
							"added_date": "2021-12-07T22:47:07.192000+00:00",
							"name": "%[1]s"
						}
					]
				}
			],
			"scan_status": "%[2]s"
		}
	]}`
	return []byte(fmt.Sprintf(response, tag, status))
}

type testDockerclient struct {
	docker_client.CommonAPIClient
	imageListCalled, loginCalled, imageTagCalled, imagePushCalled bool
}

func (d *testDockerclient) ImageList(_ context.Context, _ types.ImageListOptions) ([]types.ImageSummary, error) {
	d.imageListCalled = true
	return []types.ImageSummary{
		{
			ID: "fake",
		},
	}, nil
}

func (d *testDockerclient) RegistryLogin(_ context.Context, _ types.AuthConfig) (registry.AuthenticateOKBody, error) {
	d.loginCalled = true
	return registry.AuthenticateOKBody{}, nil
}

func (d *testDockerclient) ImageTag(_ context.Context, _ string, _ string) error {
	d.imageTagCalled = true
	return nil
}

func (d *testDockerclient) ImagePush(_ context.Context, _ string, _ types.ImagePushOptions) (io.ReadCloser, error) {
	d.imagePushCalled = true
	return nil, nil
}

func TestPublishImage(t *testing.T) {
	tests := []struct {
		name                        string
		config                      PublishConfig
		generateRedhatResponsesFunc redhatRepsonsesFunc
		wantErr                     bool
		verify                      func(*testing.T, PublishConfig)
	}{
		{
			"Image exists in project, and force not set, does not attempt image push",
			PublishConfig{
				DockerClient:             &testDockerclient{},
				ProjectID:                "012345",
				Tag:                      "1.9.0",
				RedhatConnectRegistryKey: "fake",
				RedhatCatalogAPIKey:      "fake",
			},
			func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/images") {
						w.WriteHeader(200)
						w.Write(getImagesResponse("1.9.0", string(scanStatusPassed)))
						return
					}
					w.WriteHeader(404)
				}))
			},
			false,
			func(t *testing.T, c PublishConfig) {
				docker, ok := c.DockerClient.(*testDockerclient)
				if !ok {
					t.Errorf("failed to convert dockerclient into test client: %t", c.DockerClient)
					return
				}
				if docker.imagePushCalled {
					t.Error("docker image push should not have been called")
				}
			},
		},
		{
			"Image does not exist in project; attempts image push; image eventually passes scan",
			PublishConfig{
				DockerClient:             &testDockerclient{},
				ProjectID:                "012345",
				Tag:                      "1.9.0",
				RedhatConnectRegistryKey: "fake",
				RedhatCatalogAPIKey:      "fake",
				ImageScanTimeout:         4 * time.Second,
			},
			func() *httptest.Server {
				count := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/images") {
						defer func() { count++ }()
						w.WriteHeader(200)
						switch count {
						case 0:
							w.Write([]byte(`{"data": []}`))
							return
						case 1:
							w.Write(getImagesResponse("1.9.0", string(scanStatusInProgress)))
							return
						default:
							w.Write(getImagesResponse("1.9.0", string(scanStatusPassed)))
							return
						}
					}
					if strings.HasSuffix(r.URL.Path, "/requests/tags") {
						w.WriteHeader(200)
						return
					}
					w.WriteHeader(404)
				}))
				return srv
			},
			false,
			func(t *testing.T, c Config) {
				docker, ok := c.DockerClient.(*testDockerclient)
				if !ok {
					t.Errorf("failed to convert dockerclient into test client: %t", c.DockerClient)
					return
				}
				if !docker.imagePushCalled {
					t.Error("docker image push should have been called")
				}
				if !docker.imageListCalled {
					t.Error("docker image list should have been called")
				}
				if !docker.imageTagCalled {
					t.Error("docker image tag should have been called")
				}
				if !docker.loginCalled {
					t.Error("docker login should have been called")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.generateRedhatResponsesFunc()
			defer srv.Close()
			catalogAPIURL = srv.URL
			if err := PublishImage(tt.config); (err != nil) != tt.wantErr {
				t.Errorf("PublishImage() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.verify(t, tt.config)
		})
	}
}
