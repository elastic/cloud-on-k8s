package observer

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/stretchr/testify/require"
)

func fakeEsClient(statusCodes []int) client.Client {
	i := 0
	return client.NewMockClient(func(req *http.Request) *http.Response {
		defer func() { i++ }()
		return &http.Response{
			StatusCode: statusCodes[i],
			Body:       ioutil.NopCloser(bytes.NewBufferString(fixtures.ClusterStateSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
}

func TestRetrieveState(t *testing.T) {
	fakeClient := fakeEsClient([]int{200, 500})
	state := RetrieveState(&fakeClient, 10*time.Second)
	require.NotNil(t, state.ClusterState)
	require.Nil(t, state.ClusterHealth)
	// and the other way around
	fakeClient = fakeEsClient([]int{500, 200})
	state = RetrieveState(&fakeClient, 10*time.Second)
	require.Nil(t, state.ClusterState)
	require.NotNil(t, state.ClusterHealth)
}
