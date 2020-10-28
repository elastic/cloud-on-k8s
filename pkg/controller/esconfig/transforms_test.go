// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"testing"

	"github.com/nsf/jsondiff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_removeNameWrapper(t *testing.T) {
	body := `{
		"my_repository": {
		  "type": "fs",
		  "settings": {
			"location": "/tmp"
		  }
		}
	  }`

	want := `{
		"type": "fs",
		"settings": {
		  "location": "/tmp"
		}
	  }`
	url := "/_snapshot/my_repository"

	got, err := removeNameWrapper(url, body)
	require.NoError(t, err)
	opts := jsondiff.DefaultConsoleOptions()
	diff, s := jsondiff.Compare([]byte(want), []byte(got), &opts)
	assert.Equal(t, jsondiff.FullMatch, diff, "differences: %s", s)
}

func Test_removeArrayWrapper(t *testing.T) {
	body := `{
		"component_templates": [
		  {
			"name": "component_template1",
			"component_template": {
			  "template": {
				"mappings": {
				  "properties": {
					"@timestamp": {
					  "type": "date"
					}
				  }
				}
			  }
			}
		  }
		]
	  }`

	want := `{
		"template": {
		  "mappings": {
			"properties": {
			  "@timestamp": {
				"type": "date"
			  }
			}
		  }
		}
	  }`

	url := "/_component_template/component_template1"

	got, err := removeArrayWrapper(url, body)
	require.NoError(t, err)
	opts := jsondiff.DefaultConsoleOptions()
	diff, s := jsondiff.Compare([]byte(want), []byte(got), &opts)
	assert.Equal(t, jsondiff.FullMatch, diff, "differences: %s", s)
}
