// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type transformFn func(url string, body []byte) ([]byte, error)

// TODO refactor out the key parsing
func removeNameWrapper(url string, body []byte) ([]byte, error) {
	var transformedBody []byte
	var wrapper map[string]interface{}
	err := json.Unmarshal(body, &wrapper)
	if err != nil {
		return transformedBody, errors.WithStack(err)
	}

	// get the object name
	s := strings.Split(url, "/")
	key := s[len(s)-1]
	val, ok := wrapper[key]
	if !ok {
		return transformedBody, errors.New(fmt.Sprintf("body does not contain key %s", key))
	}
	transformedBody, err = json.Marshal(val)
	return transformedBody, err
}

func removeArrayWrapper(url string, body []byte) ([]byte, error) {
	var transformedBody []byte
	var wrapper map[string][]map[string]interface{}
	err := json.Unmarshal(body, &wrapper)
	if err != nil {
		return transformedBody, errors.WithStack(err)
	}

	typeSingular := parseType(url)
	if typeSingular == "" {
		return transformedBody, errors.New(fmt.Sprintf("cannot parse type from url: %s", url))
	}
	plural := pluralizeResourceType(typeSingular)
	list, ok := wrapper[plural]
	if !ok {
		return transformedBody, errors.New(fmt.Sprintf("body does not contain key %s", plural))
	}
	// get the object name
	s := strings.Split(url, "/")
	name := s[len(s)-1]

	/*
		here is an example doc that we need to parse. there's not a lot here, just the error checking is verbose and the dynamic
		key names (for instance "component_templates" and "component_template" in the example) makes it hard to use structs rather than maps
		{
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
		  }
	*/
	for _, item := range list {
		if itemName, ok := item["name"]; ok {
			if itemName == name {
				if val, ok2 := item[typeSingular]; ok2 {
					transformedBody, err := json.Marshal(val)
					if err != nil {
						return transformedBody, errors.WithStack(err)
					}
					return transformedBody, nil
				}
			}
		}
	}
	return transformedBody, errors.New(fmt.Sprintf("could not find object %s", name))

}

// returns the singular type
func parseType(url string) string {
	s := strings.Split(url, "/")
	if len(s) > 1 {
		return strings.TrimPrefix(s[1], "_")
	}
	return ""
}

// TODO can all APIs be pluralized like this?
func pluralizeResourceType(resourceType string) string {
	return fmt.Sprintf("%ss", resourceType)
}

// removeResourceWrapper is useful when the object is wrapped both in an object with the name and the resource type
// for instance, an SLM policy at /_slm/policy/nightly-snapshots would return
/*
	{
		"nightly-snapshots": {
			"version": 1,
			"modified_date_millis": 1603409192704,
			"policy": {
			"name": "<nightly-snap-{now/d}>",
			"schedule": "0 30 1 * * ?",
			"repository": "my_repository",
			"config": {
				"indices": [
				"*"
				]
			},
			"retention": {
				"expire_after": "30d",
				"min_count": 5,
				"max_count": 50
			}
			},
			"next_execution_millis": 1603416600000,
			"stats": {
			"policy": "nightly-snapshots",
			"snapshots_taken": 0,
			"snapshots_failed": 0,
			"snapshots_deleted": 0,
			"snapshot_deletion_failures": 0
			}
		}
	}
*/
// when what we want is the contents of the `policy` object

func removeResourceWrapper(url string, body []byte) ([]byte, error) {
	transformedBody, err := removeNameWrapper(url, body)
	if err != nil {
		return transformedBody, err
	}
	var wrapper map[string]interface{}
	err = json.Unmarshal(transformedBody, &wrapper)
	if err != nil {
		return transformedBody, errors.WithStack(err)
	}
	// get subresource name
	s := strings.Split(url, "/")
	var key string
	if len(s) > 1 {
		key = s[2]
	} else {
		return transformedBody, errors.New(fmt.Sprintf("cannot parse resource name from url: %s", url))
	}
	val, ok := wrapper[key]
	if !ok {
		var keys []string
		for key := range wrapper {
			keys = append(keys, key)
		}
		return transformedBody, errors.New(fmt.Sprintf("body does not contain key: %s, keys: %s", key, keys))
	}
	transformedBody, err = json.Marshal(val)
	return transformedBody, err
}

func noopTransform(url string, body []byte) ([]byte, error) {
	return body, nil
}

// TODO is it safe to assume all APIs under a given prefix use the same convention?
// TODO how do we handle indexes that are not prefixed with underscores? need to be smarter
var transformMap = map[string]transformFn{
	"component_template": removeArrayWrapper,
	"snapshot":           removeNameWrapper,
	"slm":                removeResourceWrapper,
	"ilm":                removeNameWrapper,
	"data_stream":        noopTransform,
	"index_template":     removeArrayWrapper,
	"ingest":             removeNameWrapper,
}

func applyTransforms(url string, body []byte) ([]byte, error) {
	resourceType := parseType(url)
	if fn, ok := transformMap[resourceType]; ok {
		return fn(url, body)
	}
	// no transforms defined for this type
	// TODO should we pick a default or not?
	return body, nil
}
