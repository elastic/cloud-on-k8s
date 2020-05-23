// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import "testing"

func TestNamer_ConfigSecretName(t *testing.T) {
	namer := Namer{}
	for _, typeName := range []struct {
		name string
		typ  string
		want string
	}{
		{
			name: "my",
			typ:  "filebeat",
			want: "my-beat-filebeat-config",
		},
		{
			name: "sample-metricbeat",
			typ:  "metricbeat",
			want: "sample-metricbeat-beat-metricbeat-config",
		},
	} {
		got := namer.ConfigSecretName(typeName.typ, typeName.name)
		if got != typeName.want {
			t.Errorf("config secret name is %s while %s was expected", got, typeName.want)
		}
	}
}

func TestNamer_Name(t *testing.T) {
	namer := Namer{}
	for _, typeName := range []struct {
		name string
		typ  string
		want string
	}{
		{
			name: "my",
			typ:  "filebeat",
			want: "my-beat-filebeat",
		},
		{
			name: "sample-metricbeat",
			typ:  "metricbeat",
			want: "sample-metricbeat-beat-metricbeat",
		},
	} {
		got := namer.Name(typeName.typ, typeName.name)
		if got != typeName.want {
			t.Errorf("config secret name is %s while %s was expected", got, typeName.want)
		}
	}
}
