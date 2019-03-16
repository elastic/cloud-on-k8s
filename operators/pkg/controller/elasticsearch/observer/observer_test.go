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
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/stretchr/testify/require"
)

func fakeEsClient200() client.Client {
	return client.NewMockClient(version.MustParse("6.7.0"), func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(fixtures.ClusterStateSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
}

func createAndRunTestObserver() *Observer {
	fake := fakeEsClient200()
	obs := NewObserver(cluster("cluster"), &fake, Settings{
		ObservationInterval: 1 * time.Microsecond,
		RequestTimeout:      1 * time.Second,
	})
	obs.Start()
	return obs
}

func TestObserver_retrieveState(t *testing.T) {
	fake := fakeEsClient200()
	observer := Observer{
		esClient: &fake,
	}
	require.Equal(t, observer.LastObservationTime(), time.Time{})
	observer.retrieveState(context.Background())
	require.NotEqual(t, observer.LastObservationTime(), time.Time{})
}

func TestNewObserver(t *testing.T) {
	observer := createAndRunTestObserver()
	initialObservationTime := observer.LastObservationTime()
	// check observer is running by looking at its last observation time
	test.RetryUntilSuccess(t, func() error {
		if observer.LastObservationTime() == initialObservationTime {
			return errors.New("Observer does not seem to perform any request")
		}
		return nil
	})
	observer.Stop()
}

func TestObserver_Stop(t *testing.T) {
	observer := createAndRunTestObserver()
	// force at least one observation for time comparison
	observer.retrieveState(context.Background())
	observer.Stop()
	// should be safe to call multiple times
	observer.Stop()
	// should stop running at some point
	test.RetryUntilSuccess(t, func() error {
		observationTime := observer.LastObservationTime()
		// optimistically check nothing new happened after 50ms
		time.Sleep(50 * time.Millisecond)
		if observationTime != observer.LastObservationTime() {
			return errors.New("Observer does not seem to be stopped yet")
		}
		return nil
	})
}
