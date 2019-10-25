// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helper

import (
	"bufio"
	"fmt"
	"io"

	apmtype "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	kbtype "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type BuilderTransform func(test.Builder) test.Builder

// YAMLDecoder converts YAML bytes into test.Builder instances.
type YAMLDecoder struct {
	decoder runtime.Decoder
}

func NewYAMLDecoder() *YAMLDecoder {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(estype.GroupVersion, &estype.Elasticsearch{}, &estype.ElasticsearchList{})
	scheme.AddKnownTypes(kbtype.GroupVersion, &kbtype.Kibana{}, &kbtype.KibanaList{})
	scheme.AddKnownTypes(apmtype.GroupVersion, &apmtype.ApmServer{}, &apmtype.ApmServerList{})
	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	return &YAMLDecoder{decoder: decoder}
}

func (yd *YAMLDecoder) ToBuilders(reader *bufio.Reader, transform BuilderTransform) ([]test.Builder, error) {
	var builders []test.Builder

	yamlReader := yaml.NewYAMLReader(reader)
	for {
		yamlBytes, err := yamlReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read YAML: %w", err)
		}
		obj, _, err := yd.decoder.Decode(yamlBytes, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}

		var builder test.Builder

		switch decodedObj := obj.(type) {
		case *estype.Elasticsearch:
			b := elasticsearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Elasticsearch = *decodedObj
			builder = transform(b)
		case *kbtype.Kibana:
			b := kibana.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Kibana = *decodedObj
			builder = transform(b)
		case *apmtype.ApmServer:
			b := apmserver.NewBuilderWithoutSuffix(decodedObj.Name)
			b.ApmServer = *decodedObj
			builder = transform(b)
		default:
			return builders, fmt.Errorf("unexpected object type: %t", decodedObj)
		}

		builders = append(builders, builder)
	}

	return builders, nil
}
