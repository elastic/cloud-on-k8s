// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	v1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	configMapName = "elastic-operator"
	keyName       = "eck.yaml"
)

func main() {
	conf, err := extractConfig(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting config: %v", err)
		os.Exit(1)
	}

	fmt.Fprint(os.Stdout, conf)
}

func extractConfig(stream io.Reader) (string, error) {
	if err := apiextv1beta1.AddToScheme(scheme.Scheme); err != nil {
		return "", fmt.Errorf("failed to register api-extensions: %w", err)
	}

	decoder := scheme.Codecs.UniversalDeserializer()
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(stream))

	for {
		yamlBytes, err := yamlReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", fmt.Errorf("failed to find ConfigMap named %s", configMapName)
			}

			return "", fmt.Errorf("failed to read YAML: %w", err)
		}

		runtimeObj, _, err := decoder.Decode(yamlBytes, nil, nil)
		if err != nil {
			return "", fmt.Errorf("failed to decode YAML: %w", err)
		}

		if cm, ok := runtimeObj.(*v1.ConfigMap); ok && cm.Name == configMapName {
			return cm.Data[keyName], nil
		}
	}
}
