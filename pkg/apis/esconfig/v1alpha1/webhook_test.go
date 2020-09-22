// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"testing"
)

func Test_validateOperations(t *testing.T) {
	tests := []struct {
		name     string
		esc      *ElasticsearchConfig
		wantErrs bool
	}{
		{
			name: "invalid url",
			esc: &ElasticsearchConfig{
				Spec: ElasticsearchConfigSpec{
					Operations: []ElasticsearchConfigOperation{
						{
							// all parse url really checks is if there are control characters
							URL: "\t",
						},
					},
				},
			},
			wantErrs: true,
		},
		{
			name: "invalid json",
			esc: &ElasticsearchConfig{
				Spec: ElasticsearchConfigSpec{
					Operations: []ElasticsearchConfigOperation{
						{

							Body: "{",
						},
					},
				},
			},
			wantErrs: true,
		},
		{
			name: "happy path",
			esc: &ElasticsearchConfig{
				Spec: ElasticsearchConfigSpec{
					Operations: []ElasticsearchConfigOperation{
						{
							URL: "_snapshot/my_gcs_repository",
							Body: `{
								"type": "gcs",
								"settings": {
								  "bucket": "testbucket",
								  "client": "default"
								}
							  }`,
						},
					},
				},
			},
			wantErrs: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := validateOperations(tt.esc)
			actualErrors := len(actual) > 0

			if tt.wantErrs != actualErrors {
				t.Errorf("failed validateOperations(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.wantErrs, tt.esc.Spec.Operations)
			}
		})
	}
}
