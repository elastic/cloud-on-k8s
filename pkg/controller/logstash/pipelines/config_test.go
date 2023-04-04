// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pipelines

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelinesConfig_Render(t *testing.T) {
	config := MustFromSpec(
		[]map[string]interface{}{
			{
				"pipeline.id":   "demo",
				"config.string": "input { exec { command => \"uptime\" interval => 5 } } output { stdout{} }",
			},
			{
				"pipeline.id":                 "standard",
				"pipeline.workers":            1,
				"queue.type":                  "persisted",
				"queue.drain":                 true,
				"dead_letter_queue.max_bytes": "1024mb",
				"path.config":                 "/tmp/logstash/*.config",
			},
		},
	)
	output, err := config.Render()
	require.NoError(t, err)
	expected := []byte(`- config.string: input { exec { command => "uptime" interval => 5 } } output { stdout{} }
  pipeline.id: demo
- dead_letter_queue.max_bytes: 1024mb
  path.config: /tmp/logstash/*.config
  pipeline.id: standard
  pipeline.workers: 1
  queue.drain: true
  queue.type: persisted
`)
	require.Equal(t, string(expected), string(output))
}

func TestParsePipelinesConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *Config
		wantErr bool
	}{
		{
			name:    "no input",
			input:   "",
			want:    EmptyConfig(),
			wantErr: false,
		},
		{
			name:  "simple input",
			input: "- pipeline.id: demo\n  config.string: input { exec { command => \"${ENV}\" interval => 5 } }",
			want: MustFromSpec(
				[]map[string]interface{}{
					{
						"pipeline.id":   "demo",
						"config.string": "input { exec { command => \"${ENV}\" interval => 5 } }",
					},
				},
			),
			wantErr: false,
		},
		{
			name:  "number input",
			input: "- pipeline.id: main\n  pipeline.workers: 4",
			want: MustFromSpec(
				[]map[string]interface{}{
					{
						"pipeline.id":      "main",
						"pipeline.workers": 4,
					},
				},
			),
			wantErr: false,
		},
		{
			name:  "boolean input",
			input: "- pipeline.id: main\n  queue.drain: false",
			want: MustFromSpec(
				[]map[string]interface{}{
					{
						"pipeline.id": "main",
						"queue.drain": false,
					},
				},
			),
			wantErr: false,
		},
		{
			name:  "trim whitespaces between key and value",
			input: "- pipeline.id :  demo \n  path.config :  /tmp/logstash/*.config ",
			want: MustFromSpec(
				[]map[string]interface{}{
					{
						"pipeline.id": "demo",
						"path.config": "/tmp/logstash/*.config",
					},
				},
			),
			wantErr: false,
		},
		{
			name:    "tabs are invalid in YAML",
			input:   "\ta: b   \n    c:d     ",
			wantErr: true,
		},
		{
			name:  "trim newlines",
			input: "- pipeline.id: demo \n\n- pipeline.id: demo2 \n",
			want: MustFromSpec(
				[]map[string]interface{}{
					{"pipeline.id": "demo"},
					{"pipeline.id": "demo2"},
				},
			),
			wantErr: false,
		},
		{
			name:  "ignore comments",
			input: "- pipeline.id: demo \n#this is a comment\n  pipeline.workers: \"1\"\n",
			want: MustFromSpec(
				[]map[string]interface{}{
					{
						"pipeline.id":      "demo",
						"pipeline.workers": "1",
					},
				},
			),
			wantErr: false,
		},
		{
			name:  "support quotes",
			input: `- "pipeline.id": "quote"`,
			want: MustFromSpec(
				[]map[string]interface{}{
					{"pipeline.id": "quote"},
				},
			),
			wantErr: false,
		},
		{
			name:  "support special characters",
			input: `- config.string: "${node.ip}%.:=+è! /"`,
			want: MustFromSpec(
				[]map[string]interface{}{
					{"config.string": `${node.ip}%.:=+è! /`},
				},
			),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got == tt.want {
				return
			}

			if diff, _ := got.Diff(tt.want); diff {
				gotRendered, err := got.Render()
				require.NoError(t, err)
				wantRendered, err := tt.want.Render()
				require.NoError(t, err)
				t.Errorf("Parse(), want: %s, got: %s", wantRendered, gotRendered)
			}
		})
	}
}

func TestPipelinesConfig_Diff(t *testing.T) {
	tests := []struct {
		name     string
		c        *Config
		c2       *Config
		wantDiff bool
	}{
		{
			name:     "nil diff",
			c:        nil,
			c2:       nil,
			wantDiff: false,
		},
		{
			name: "lhs nil",
			c:    nil,
			c2: MustFromSpec(
				[]map[string]interface{}{
					{"a": "a"},
					{"b": "b"},
				},
			),
			wantDiff: true,
		},
		{
			name: "rhs nil",
			c: MustFromSpec(
				[]map[string]interface{}{
					{"a": "a"},
				},
			),
			c2:       nil,
			wantDiff: true,
		},
		{
			name: "same multi key value",
			c: MustFromSpec(
				[]map[string]interface{}{
					{"a": "a", "b": "b", "c": 1, "d": true},
				},
			),
			c2: MustFromSpec(
				[]map[string]interface{}{
					{"c": 1, "b": "b", "a": "a", "d": true},
				},
			),
			wantDiff: false,
		},
		{
			name: "different value",
			c: MustFromSpec(
				[]map[string]interface{}{
					{"a": "a"},
				},
			),
			c2: MustFromSpec(
				[]map[string]interface{}{
					{"a": "b"},
				},
			),
			wantDiff: true,
		},
		{
			name: "array size different",
			c: MustFromSpec(
				[]map[string]interface{}{
					{"a": "a"},
				},
			),
			c2: MustFromSpec(
				[]map[string]interface{}{
					{"a": "a"},
					{"a": "a"},
				},
			),
			wantDiff: true,
		},
		{
			name: "respects list order",
			c: MustFromSpec(
				[]map[string]interface{}{
					{"a": "a"},
					{"b": "b"},
				},
			),
			c2: MustFromSpec(
				[]map[string]interface{}{
					{"b": "b"},
					{"a": "a"},
				},
			),
			wantDiff: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := tt.c.Diff(tt.c2)
			if (err != nil) != tt.wantDiff {
				t.Errorf("Diff() got unexpected differences. wantDiff: %t, err: %v", tt.wantDiff, err)
				return
			}

			require.Equal(t, tt.wantDiff, diff)
		})
	}
}
