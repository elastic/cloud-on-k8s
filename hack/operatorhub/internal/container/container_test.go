// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type syncTestManager struct {
	expectedSyncCalls int
	actualSyncCalls   int
	getImagesReponse  []Image
	server            *httptest.Server
}

func getImagesResponse(t *testing.T, imgs []Image) []byte {
	t.Helper()
	b, err := json.Marshal(GetImagesResponse{Images: imgs})
	if err != nil {
		t.Fatalf("while marshaling images: %s", err)
		return nil
	}
	return b
}

func (s *syncTestManager) createHTTPHandler(getImagesResponse []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/containers/v1/projects/certification/id/fake/images" {
			w.WriteHeader(http.StatusOK)
			w.Write(getImagesResponse)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/containers/v1/projects/certification/id/fake/requests/images" {
			s.actualSyncCalls++
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf(`{"error": "invalid path: '%s'}`, r.URL.Path)))
	})
}

func Test_syncImagesTaggedAsLatest(t *testing.T) {
	tests := []struct {
		name    string
		config  CommonConfig
		newTag  Tag
		mgr     *syncTestManager
		wantErr bool
	}{
		{
			name: "Publishing 2.7.0 with existing 2.6.0 and 2.7.0 tagged as latest calls sync for 2.6.0",
			config: CommonConfig{
				ProjectID:           "fake",
				RedhatCatalogAPIKey: "fake",
			},
			newTag: Tag{Name: "2.7.0"},
			mgr: &syncTestManager{
				expectedSyncCalls: 1,
				getImagesReponse: []Image{
					{
						ID: "01234",
						Repositories: []Repository{
							{
								Repository: "redhat-isv-containers/fake",
								Tags: []Tag{
									{
										Name: "latest",
									},
									{
										Name: "2.6.0",
									},
								},
							},
						},
					},
					{
						ID: "54321",
						Repositories: []Repository{
							{
								Repository: "redhat-isv-containers/fake",
								Tags: []Tag{
									{
										Name: "latest",
									},
									{
										Name: "2.7.0",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Publishing 2.7.0 with only 2.7.0 tagged as latest does not call sync operation",
			config: CommonConfig{
				ProjectID:           "fake",
				RedhatCatalogAPIKey: "fake",
			},
			newTag: Tag{Name: "2.7.0"},
			mgr: &syncTestManager{
				expectedSyncCalls: 0,
				getImagesReponse: []Image{
					{
						ID: "54321",
						Repositories: []Repository{
							{
								Repository: "redhat-isv-containers/fake",
								Tags: []Tag{
									{
										Name: "latest",
									},
									{
										Name: "2.7.0",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mgr.server = httptest.NewServer(tt.mgr.createHTTPHandler(getImagesResponse(t, tt.mgr.getImagesReponse)))
			defer tt.mgr.server.Close()
			catalogAPIURL = fmt.Sprintf("%s/%s", tt.mgr.server.URL, "api/containers/v1")
			if err := syncImagesTaggedAsLatest(tt.config, tt.newTag); (err != nil) != tt.wantErr {
				t.Errorf("syncImagesTaggedAsLatest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.mgr.expectedSyncCalls != tt.mgr.actualSyncCalls {
				t.Errorf("syncImagesTaggedAsLatest() actual = %d, expected %d", tt.mgr.actualSyncCalls, tt.mgr.expectedSyncCalls)
			}
		})
	}
}
