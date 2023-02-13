// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"testing"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckNameLength(t *testing.T) {
	testCases := []struct {
		name          string
		logstashName  string
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name:          "valid configuration",
			logstashName:  "test-logstash",
			wantErr:       false,
		},
		{
			name:          "long Logstash name",
			logstashName:  "extremely-long-winded-and-unnecessary-name-for-logstash",
			wantErr:       true,
			wantErrMsg:    "name exceeds maximum allowed length",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ls := Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.logstashName,
					Namespace: "test",
				},
				Spec: LogstashSpec{},
			}

			errList := checkNameLength(&ls)
			assert.Equal(t, tc.wantErr, len(errList) > 0)
		})
	}
}
