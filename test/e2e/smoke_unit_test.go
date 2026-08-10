// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package e2e

import (
	"flag"
	"testing"
)

func TestIsSmokeSample(t *testing.T) {
	if flag.Lookup("testContextPath").Value.String() != "" {
		t.Skip("skipping unit test in e2e run")
	}

	tests := []struct {
		input string
		want  bool
	}{
		{"../../config/samples/apm/apm_es_kibana.yaml", true},
		{"../../config/samples/elasticsearch/elasticsearch.yaml", true},
		{"../../config/samples/logstash/logstash_pv.yaml", true},
		{"../../config/samples/logstash/logstash_stackmonitor.yaml", true},
		{"../../config/samples/logstash/logstash_es.yaml", false},
		{"../../config/samples/kibana/kibana_es.yaml", false},
		{"../../config/samples/agent/fleet_es_kibana.yaml", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isSmokeSample(tt.input); got != tt.want {
				t.Errorf("isSmokeSample(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
