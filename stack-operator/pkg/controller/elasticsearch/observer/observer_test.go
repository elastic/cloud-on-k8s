package observer

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/test"
	"github.com/stretchr/testify/require"
)

func fakeEsClient200() client.Client {
	return client.NewMockClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(fixtures.ClusterStateSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
}

func createTestObserver() *Observer {
	fake := fakeEsClient200()
	return NewObserver(clusterName("cluster"), &fake, Settings{
		ObservationInterval: 1 * time.Microsecond,
		RequestTimeout:      1 * time.Second,
	})
}

func TestObserver_retrieveState(t *testing.T) {
	fake := fakeEsClient200()
	observer := Observer{
		esClient: &fake,
	}
	require.Equal(t, observer.LastObservationTime(), time.Time{})
	observer.retrieveState()
	require.NotEqual(t, observer.LastObservationTime(), time.Time{})
}

func TestNewObserver(t *testing.T) {
	observer := createTestObserver()
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
	observer := createTestObserver()
	// force at least one observation for time comparison
	observer.retrieveState()
	observer.Stop()
	// should be safe to call multiple times
	observer.Stop()
	// should stop running at some point
	time.Sleep(1 * time.Millisecond)
	observationTime := observer.LastObservationTime()
	// optimistically check nothing new happened after 10ms
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, observationTime, observer.LastObservationTime())
}
