// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package discovery

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fakeEsClientZen2(method string) esclient.Client {
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

func TestZen2SetVotingExclusions(t *testing.T) {
	type args struct {
		esClient     esclient.Client
		deletingPods []corev1.Pod
	}
	masterPodFixture := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: label.NodeTypesMasterLabelName.AsMap(true),
		},
	}
	dataPodFixture := corev1.Pod{}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "No changes: delete voting exclusions",
			args: args{
				esClient: fakeEsClientZen2(http.MethodDelete),
			},
			wantErr: false,
		},
		{
			name: "Delete master: set voting exclusion",
			args: args{
				esClient:     fakeEsClientZen2(http.MethodPost),
				deletingPods: []corev1.Pod{masterPodFixture},
			},
			wantErr: false,
		},
		{
			name: "Delete non-master: delete voting exclusion",
			args: args{
				esClient:     fakeEsClientZen2(http.MethodDelete),
				deletingPods: []corev1.Pod{dataPodFixture},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Zen2SetVotingExclusions(
				tt.args.esClient, tt.args.deletingPods,
			); (err != nil) != tt.wantErr {
				t.Errorf("Zen2SetVotingExclusions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
