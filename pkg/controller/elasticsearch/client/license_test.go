// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	fixtures "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetLicense(t *testing.T) {
	tests := []struct {
		expectedPath string
		version      version.Version
	}{
		{
			expectedPath: "/_xpack/license",
			version:      version.MustParse("6.8.0"),
		},
		{
			expectedPath: "/_license",
			version:      version.MustParse("7.0.0"),
		},
	}

	for _, tt := range tests {
		testClient := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(fixtures.LicenseGetSample)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
		got, err := testClient.GetLicense(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "893361dc-9749-4997-93cb-802e3d7fa4xx", got.UID)
		assert.Equal(t, "platinum", got.Type)
		assert.EqualValues(t, time.Unix(0, 1548115200000*int64(time.Millisecond)).UTC(), *got.IssueDate)
		assert.Equal(t, int64(1548115200000), got.IssueDateInMillis)
		assert.EqualValues(t, time.Unix(0, 1561247999999*int64(time.Millisecond)).UTC(), *got.ExpiryDate)
		assert.Equal(t, int64(1561247999999), got.ExpiryDateInMillis)
		assert.Equal(t, 100, got.MaxNodes)
		assert.Equal(t, "issuer", got.Issuer)
		assert.Equal(t, int64(1548115200000), got.StartDateInMillis)
	}
}

func TestClient_UpdateLicense(t *testing.T) {
	tests := []struct {
		expectedPath string
		version      version.Version
	}{
		{
			expectedPath: "/_xpack/license",
			version:      version.MustParse("6.8.0"),
		},
		{
			expectedPath: "/_license",
			version:      version.MustParse("7.0.0"),
		},
	}
	for _, tt := range tests {
		testClient := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(fixtures.LicenseUpdateResponseSample)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
		in := LicenseUpdateRequest{
			Licenses: []License{
				{
					UID:                "893361dc-9749-4997-93cb-802e3d7fa4xx",
					Type:               "basic",
					IssueDateInMillis:  0,
					ExpiryDateInMillis: 0,
					MaxNodes:           1,
					IssuedTo:           "unit-test",
					Issuer:             "test-issuer",
					Signature:          "xx",
				},
			},
		}
		got, err := testClient.UpdateLicense(context.Background(), in)
		assert.NoError(t, err)
		assert.Equal(t, true, got.Acknowledged)
		assert.Equal(t, "valid", got.LicenseStatus)
	}

}

func TestClient_StartBasic(t *testing.T) {
	tests := []struct {
		expectedPath string
		version      version.Version
	}{
		{
			expectedPath: "/_xpack/license/start_basic",
			version:      version.MustParse("6.8.0"),
		},
		{
			expectedPath: "/_license/start_basic",
			version:      version.MustParse("7.0.0"),
		},
	}

	for _, tt := range tests {
		testClient := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(`{"acknowledged":true,"basic_was_started":true}`)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
		got, err := testClient.StartBasic(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, true, got.Acknowledged)
		assert.Equal(t, true, got.BasicWasStarted)
		assert.Empty(t, got.ErrorMessage)
	}
}
