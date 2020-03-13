// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/cloud-on-k8s/pkg/utils/test"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func fakeEsClient200(user client.BasicAuth) client.Client {
	return client.NewMockClientWithUser(version.MustParse("6.8.0"),
		user,
		func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(fixtures.SampleShards)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
}

func createAndRunTestObserver(onObs OnObservation) *Observer {
	fakeEsClient := fakeEsClient200(client.BasicAuth{})
	obs := NewObserver(cluster("cluster"), fakeEsClient, Settings{
		ObservationInterval: 1 * time.Microsecond,
		RequestTimeout:      1 * time.Second,
	}, onObs)
	obs.Start()
	return obs
}

func TestObserver_retrieveState(t *testing.T) {
	counter := int32(0)
	onObservation := func(cluster types.NamespacedName, previousState State, newState State) {
		atomic.AddInt32(&counter, 1)
	}
	fakeEsClient := fakeEsClient200(client.BasicAuth{})
	observer := Observer{
		esClient:      fakeEsClient,
		onObservation: onObservation,
	}
	observer.retrieveState(context.Background())
	require.Equal(t, int32(1), atomic.LoadInt32(&counter))
	observer.retrieveState(context.Background())
	require.Equal(t, int32(2), atomic.LoadInt32(&counter))
}

func TestObserver_retrieveState_nilFunction(t *testing.T) {
	var nilFunc OnObservation
	fakeEsClient := fakeEsClient200(client.BasicAuth{})
	observer := Observer{
		esClient:      fakeEsClient,
		onObservation: nilFunc,
	}
	// should not panic
	observer.retrieveState(context.Background())
}

func TestNewObserver(t *testing.T) {
	events := make(chan types.NamespacedName)
	onObservation := func(cluster types.NamespacedName, previousState State, newState State) {
		events <- cluster
	}
	observer := createAndRunTestObserver(onObservation)
	defer observer.Stop()
	// let it observe at least 3 times
	require.Equal(t, types.NamespacedName{Namespace: "ns", Name: "cluster"}, <-events)
	require.Equal(t, types.NamespacedName{Namespace: "ns", Name: "cluster"}, <-events)
	require.Equal(t, types.NamespacedName{Namespace: "ns", Name: "cluster"}, <-events)
}

func TestObserver_Stop(t *testing.T) {
	counter := int32(0)
	onObservation := func(cluster types.NamespacedName, previousState State, newState State) {
		atomic.AddInt32(&counter, 1)
	}
	observer := createAndRunTestObserver(onObservation)
	// force at least one observation
	observer.retrieveState(context.Background())
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
