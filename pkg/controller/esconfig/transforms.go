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

type transformFn func(url string, body string) (string, error)

// TODO refactor out the key parsing
func removeNameWrapper(url string, body string) (string, error) {
	var wrapper map[string]interface{}
	err := json.Unmarshal([]byte(body), &wrapper)
	if err != nil {
		return "", errors.WithStack(err)
	}

	// get the object name
	s := strings.Split(url, "/")
	key := s[len(s)-1]
	val, ok := wrapper[key]
	if !ok {
		return "", errors.New(fmt.Sprintf("body does not contain key %s", key))
	}
	// TODO leave this as a byte array?
	rejson, err := json.Marshal(val)
	return string(rejson), nil
}

func removeArrayWrapper(url string, body string) (string, error) {
	var wrapper map[string][]map[string]interface{}
	err := json.Unmarshal([]byte(body), &wrapper)
	if err != nil {
		return "", errors.WithStack(err)
	}

	typeSingular := parseType(url)
	if typeSingular == "" {
		return "", errors.New(fmt.Sprintf("cannot parse type from url: %s", url))
	}
	plural := pluralizeResourceType(typeSingular)
	list, ok := wrapper[plural]
	if !ok {
		return "", errors.New(fmt.Sprintf("body does not contain key %s", plural))
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
					rejson, err := json.Marshal(val)
					if err != nil {
						return "", errors.WithStack(err)
					}
					return string(rejson), nil
				}
			}
		}
	}
	return "", errors.New(fmt.Sprintf("could not find object %s", name))

}

// returns the singular type
func parseType(url string) string {
	s := strings.Split(url, "/")
	if len(s) > 0 {
		return strings.TrimPrefix(s[1], "_")
	}
	return ""
}

// TODO can all APIs be pluralized like this?
func pluralizeResourceType(resourceType string) string {
	return fmt.Sprintf("%ss", resourceType)
}
