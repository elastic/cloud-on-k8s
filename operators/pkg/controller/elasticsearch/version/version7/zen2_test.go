// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fakeEsClient(method string) esclient.Client {
	return esclient.NewMockClient(version.MustParse("7.0.0"), func(req *http.Request) *http.Response {
		var statusCode int
		var respBody io.ReadCloser

		if strings.Contains(req.URL.RequestURI(), "/_cluster/voting_config_exclusions") &&
			req.Method == method {
			respBody = ioutil.NopCloser(bytes.NewBufferString("OK"))
			statusCode = 200

		} else {
			respBody = ioutil.NopCloser(bytes.NewBufferString("KO"))
			statusCode = 400
		}

		return &http.Response{
			StatusCode: statusCode,
			Body:       respBody,
			Header:     make(http.Header),
			Request:    req,
		}
	})
}

func TestUpdateZen2Settings(t *testing.T) {
	type args struct {
		esClient           esclient.Client
		minVersion         version.Version
		changes            mutation.Changes
		performableChanges mutation.PerformableChanges
	}
	masterPodFixture := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: label.NodeTypesMasterLabelName.AsMap(true),
		},
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Mixed clusters with pre-7.x.x nodes, don't use zen2 API",
			args: args{
				esClient:   fakeEsClient("No request expected"),
				minVersion: version.MustParse("6.8.0"),
				changes: mutation.Changes{
					ToCreate: nil,
					ToKeep:   nil,
					ToDelete: []pod.PodWithConfig{
						{
							Pod: masterPodFixture,
						},
					},
				},
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: nil,
						ToKeep:   nil,
						ToDelete: []pod.PodWithConfig{
							{Pod: masterPodFixture},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "No changes: delete voting exclusions",
			args: args{
				esClient:           fakeEsClient(http.MethodDelete),
				minVersion:         version.MustParse("7.0.0"),
				changes:            mutation.Changes{},
				performableChanges: mutation.PerformableChanges{},
			},
			wantErr: false,
		},
		{
			name: "Delete master: set voting exclusion",
			args: args{
				esClient:   fakeEsClient(http.MethodPost),
				minVersion: version.MustParse("7.0.0"),
				changes: mutation.Changes{
					ToCreate: nil,
					ToKeep:   nil,
					ToDelete: []pod.PodWithConfig{
						{
							Pod: masterPodFixture,
						},
					},
				},
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: nil,
						ToKeep:   nil,
						ToDelete: []pod.PodWithConfig{
							{Pod: masterPodFixture},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := UpdateZen2Settings(tt.args.esClient, tt.args.minVersion, tt.args.changes, tt.args.performableChanges); (err != nil) != tt.wantErr {
				t.Errorf("UpdateZen2Settings() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
