// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	ClusterStateSample = `{}`
)

func fakeEsClient200() client.Client {
	return client.NewMockClient(version.MustParse("7.0.0"),
		func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(ClusterStateSample)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
}

func TestUpdateRemoteCluster(t *testing.T) {
	type args struct {
		initialObjects []runtime.Object
		esClient       esclient.Client
		es             v1alpha1.Elasticsearch
		reconcileState *reconcile.State
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Remove remote cluster",
			args: args{
				esClient:       fakeEsClient200(),
				initialObjects: []runtime.Object{},
				reconcileState: &reconcile.State{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := k8s.WrapClient(fake.NewFakeClient(tt.args.initialObjects...))
			if err := UpdateRemoteCluster(fakeClient, tt.args.esClient, tt.args.es, tt.args.reconcileState); (err != nil) != tt.wantErr {
				t.Errorf("UpdateRemoteCluster() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
