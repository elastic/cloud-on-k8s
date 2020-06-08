// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import "testing"

func TestNamer_ConfigSecretName(t *testing.T) {
	for _, tt := range []struct {
		name         string
		resourceName string
		typ          string
		want         string
	}{
		{
			name:         "filebeat type",
			resourceName: "my",
			typ:          "filebeat",
			want:         "my-beat-filebeat-config",
		},
		{
			name:         "metricbeat type",
			resourceName: "sample-metricbeat",
			typ:          "metricbeat",
			want:         "sample-metricbeat-beat-metricbeat-config",
		},
		{
			name:         "other type",
			resourceName: "x",
			typ:          "mybeat",
			want:         "x-beat-mybeat-config",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := ConfigSecretName(tt.typ, tt.resourceName)
			if got != tt.want {
				t.Errorf("config secret name is %s while %s was expected", got, tt.want)
			}
		})
	}
}

func TestNamer_Name(t *testing.T) {
	for _, tt := range []struct {
		name         string
		resourceName string
		typ          string
		want         string
	}{
		{
			name:         "filebeat type",
			resourceName: "my",
			typ:          "filebeat",
			want:         "my-beat-filebeat",
		},
		{
			name:         "metricbeat type",
			resourceName: "sample-metricbeat",
			typ:          "metricbeat",
			want:         "sample-metricbeat-beat-metricbeat",
		},
		{
			name:         "other type",
			resourceName: "x",
			typ:          "mybeat",
			want:         "x-beat-mybeat",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Name(tt.resourceName, tt.typ)
			if got != tt.want {
				t.Errorf("config secret name is %s while %s was expected", got, tt.want)
			}
		})
	}
}
