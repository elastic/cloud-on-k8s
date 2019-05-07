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

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/test"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func fakeEsClient200(user client.UserAuth) client.Client {
	return client.NewMockClientWithUser(version.MustParse("6.7.0"),
		user,
		func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(fixtures.ClusterStateSample)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
}

func createAndRunTestObserver(onObs OnObservation) *Observer {
	fakeK8sClient := k8s.WrapClient(fake.NewFakeClient())
	fakeEsClient := fakeEsClient200(client.UserAuth{})
	obs := NewObserver(fakeK8sClient, nil, nil, cluster("cluster"), fakeEsClient, Settings{
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
	fakeK8sClient := k8s.WrapClient(fake.NewFakeClient())
	fakeEsClient := fakeEsClient200(client.UserAuth{})
	observer := Observer{
		k8sClient:     fakeK8sClient,
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
	fakeK8sClient := k8s.WrapClient(fake.NewFakeClient())
	fakeEsClient := fakeEsClient200(client.UserAuth{})
	observer := Observer{
		k8sClient:     fakeK8sClient,
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
