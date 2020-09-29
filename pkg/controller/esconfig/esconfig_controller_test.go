// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO actually add tests
func Test_updateRequired(t *testing.T) {
	tests := []struct {
		name    string
		want    bool
		fn      esclient.RoundTripFunc
		url     string
		body    string
		wantErr bool
	}{
		{
			name: "happy path",
			url:  "/test",
			// TODO make this a no updates required test
			want:    true,
			wantErr: false,
			fn: func(req *http.Request) *http.Response {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Request:    req,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := version.From(7, 9, 1)
			client := esclient.NewMockClient(v, tt.fn)
			ctx := common.NewMockContext()
			testURL, _ := url.Parse(tt.url)
			actual, err := updateRequired(ctx, client, testURL, []byte(tt.body))

			if tt.wantErr {
				require.Error(t, err)
			} else {

				require.NoError(t, err)
			}

			assert.Equal(t, tt.want, actual)
		})
	}
}
