// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package observer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/test"
)

func fakeEsClient200(user client.BasicAuth) client.Client {
	return client.NewMockClientWithUser(version.MustParse("6.8.0"),
		user,
		func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(fixtures.SampleShards)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
}

func createAndRunTestObserver(onObs OnObservation) *Observer {
	fakeEsClient := fakeEsClient200(client.BasicAuth{})
	obs := NewObserver(cluster("cluster"), fakeEsClient, Settings{ObservationInterval: 1 * time.Microsecond}, onObs)
	obs.Start()
	return obs
}

func TestObserver_observe(t *testing.T) {
	counter := int32(0)
	onObservation := func(cluster types.NamespacedName, previousHealth, newHealth esv1.ElasticsearchHealth) {
		atomic.AddInt32(&counter, 1)
	}
	fakeEsClient := fakeEsClient200(client.BasicAuth{})
	observer := Observer{
		esClient:      fakeEsClient,
		onObservation: onObservation,
	}
	observer.observe(context.Background())
	require.Equal(t, int32(1), atomic.LoadInt32(&counter))
	observer.observe(context.Background())
	require.Equal(t, int32(2), atomic.LoadInt32(&counter))
}

func TestObserver_observe_nilFunction(t *testing.T) {
	var nilFunc OnObservation
	fakeEsClient := fakeEsClient200(client.BasicAuth{})
	observer := Observer{
		esClient:      fakeEsClient,
		onObservation: nilFunc,
	}
	// should not panic
	observer.observe(context.Background())
}

func TestNewObserver(t *testing.T) {
	events := make(chan types.NamespacedName)
	onObservation := func(cluster types.NamespacedName, previousHealth, newHealth esv1.ElasticsearchHealth) {
		events <- cluster
	}
	doneCh := make(chan struct{})
	go func() {
		// let it observe at least 3 times
		require.Equal(t, types.NamespacedName{Namespace: "ns", Name: "cluster"}, <-events)
		require.Equal(t, types.NamespacedName{Namespace: "ns", Name: "cluster"}, <-events)
		require.Equal(t, types.NamespacedName{Namespace: "ns", Name: "cluster"}, <-events)
		close(doneCh)
	}()
	observer := createAndRunTestObserver(onObservation)
	defer observer.Stop()
	<-doneCh
}

func TestObserver_Stop(t *testing.T) {
	counter := int32(0)
	onObservation := func(cluster types.NamespacedName, previousHealth, newHealth esv1.ElasticsearchHealth) {
		atomic.AddInt32(&counter, 1)
	}
	observer := createAndRunTestObserver(onObservation)
	// force at least one observation
	observer.observe(context.Background())
	// stop the observer
	observer.Stop()
	// should be safe to call multiple times
	observer.Stop()
	// should stop running at some point
	lastCounter := int32(0)
	test.RetryUntilSuccess(t, func() error {
		currentCounter := atomic.LoadInt32(&counter)
		if lastCounter != currentCounter {
			lastCounter = currentCounter
			return errors.New("observer does not seem stopped yet")
		}
		return nil
	})
}

func fakeEsClient(healthRespErr bool) client.Client {
	return client.NewMockClient(version.MustParse("6.8.0"), func(req *http.Request) *http.Response {
		statusCode := 200
		var respBody io.ReadCloser

		if strings.Contains(req.URL.RequestURI(), "health") {
			respBody = io.NopCloser(bytes.NewBufferString(fixtures.HealthSample))
			if healthRespErr {
				statusCode = 500
			}
		}

		return &http.Response{
			StatusCode: statusCode,
			Body:       respBody,
			Header:     make(http.Header),
			Request:    req,
		}
	})
}

func TestRetrieveHealth(t *testing.T) {
	tests := []struct {
		name          string
		healthRespErr bool
		expected      esv1.ElasticsearchHealth
	}{
		{
			name:          "health ok",
			healthRespErr: false,
			expected:      esv1.ElasticsearchGreenHealth,
		},
		{
			name:          "unknown health",
			healthRespErr: true,
			expected:      esv1.ElasticsearchUnknownHealth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := types.NamespacedName{Namespace: "ns1", Name: "es1"}
			esClient := fakeEsClient(tt.healthRespErr)
			health := retrieveHealth(context.Background(), cluster, esClient)
			require.Equal(t, tt.expected, health)
		})
	}
}

func Test_nonNegativeTimeout(t *testing.T) {
	type args struct {
		observationInterval time.Duration
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "positive observation interval == timeout",
			args: args{
				observationInterval: 1 * time.Second,
			},
			want: 1 * time.Second,
		},
		{
			name: "0 observation interval == default",
			args: args{
				observationInterval: 0,
			},
			want: defaultObservationTimeout,
		},
		{
			name: "negative observation interval == default",
			args: args{
				observationInterval: -1 * time.Second,
			},
			want: defaultObservationTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nonNegativeTimeout(tt.args.observationInterval); got != tt.want {
				t.Errorf("nonNegativeTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}
