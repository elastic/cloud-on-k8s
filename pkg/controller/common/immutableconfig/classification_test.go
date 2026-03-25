// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package immutableconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapClassifier_Classify(t *testing.T) {
	classifier := MapClassifier{
		"config.yml":  Immutable,
		"dynamic.yml": Dynamic,
	}

	tests := []struct {
		name     string
		filename string
		want     Classification
	}{
		{
			name:     "immutable file",
			filename: "config.yml",
			want:     Immutable,
		},
		{
			name:     "dynamic file",
			filename: "dynamic.yml",
			want:     Dynamic,
		},
		{
			name:     "unknown file",
			filename: "unknown.yml",
			want:     Unclassified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifier.Classify(tt.filename)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSplitByClassification(t *testing.T) {
	classifier := MapClassifier{
		"static.yml":  Immutable,
		"dynamic.yml": Dynamic,
	}

	tests := []struct {
		name          string
		data          map[string][]byte
		wantImmutable map[string][]byte
		wantDynamic   map[string][]byte
		wantErr       bool
	}{
		{
			name: "split correctly",
			data: map[string][]byte{
				"static.yml":  []byte("static content"),
				"dynamic.yml": []byte("dynamic content"),
			},
			wantImmutable: map[string][]byte{
				"static.yml": []byte("static content"),
			},
			wantDynamic: map[string][]byte{
				"dynamic.yml": []byte("dynamic content"),
			},
			wantErr: false,
		},
		{
			name: "all immutable",
			data: map[string][]byte{
				"static.yml": []byte("static content"),
			},
			wantImmutable: map[string][]byte{
				"static.yml": []byte("static content"),
			},
			wantDynamic: map[string][]byte{},
			wantErr:     false,
		},
		{
			name: "all dynamic",
			data: map[string][]byte{
				"dynamic.yml": []byte("dynamic content"),
			},
			wantImmutable: map[string][]byte{},
			wantDynamic: map[string][]byte{
				"dynamic.yml": []byte("dynamic content"),
			},
			wantErr: false,
		},
		{
			name:          "empty data",
			data:          map[string][]byte{},
			wantImmutable: map[string][]byte{},
			wantDynamic:   map[string][]byte{},
			wantErr:       false,
		},
		{
			name: "unclassified file",
			data: map[string][]byte{
				"static.yml":  []byte("static content"),
				"unknown.yml": []byte("unknown content"),
			},
			wantImmutable: nil,
			wantDynamic:   nil,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotImmutable, gotDynamic, err := SplitByClassification(tt.data, classifier)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unclassified")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantImmutable, gotImmutable)
			assert.Equal(t, tt.wantDynamic, gotDynamic)
		})
	}
}

func TestSplitStringByClassification(t *testing.T) {
	classifier := MapClassifier{
		"static.yml":  Immutable,
		"dynamic.yml": Dynamic,
	}

	data := map[string]string{
		"static.yml":  "static content",
		"dynamic.yml": "dynamic content",
	}

	gotImmutable, gotDynamic, err := SplitStringByClassification(data, classifier)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"static.yml": "static content"}, gotImmutable)
	assert.Equal(t, map[string]string{"dynamic.yml": "dynamic content"}, gotDynamic)
}

func TestMapClassifier_NamesWithClassification(t *testing.T) {
	classifier := MapClassifier{
		"config-volume":      Immutable,
		"jvm-options-volume": Immutable,
		"dynamic-volume":     Dynamic,
		"other-volume":       Dynamic,
	}

	t.Run("returns immutable names", func(t *testing.T) {
		names := classifier.NamesWithClassification(Immutable)
		assert.Len(t, names, 2)
		assert.Contains(t, names, "config-volume")
		assert.Contains(t, names, "jvm-options-volume")
	})

	t.Run("returns dynamic names", func(t *testing.T) {
		names := classifier.NamesWithClassification(Dynamic)
		assert.Len(t, names, 2)
		assert.Contains(t, names, "dynamic-volume")
		assert.Contains(t, names, "other-volume")
	})

	t.Run("returns empty for unclassified", func(t *testing.T) {
		names := classifier.NamesWithClassification(Unclassified)
		assert.Empty(t, names)
	})

	t.Run("returns empty for empty classifier", func(t *testing.T) {
		empty := MapClassifier{}
		names := empty.NamesWithClassification(Immutable)
		assert.Empty(t, names)
	})
}
