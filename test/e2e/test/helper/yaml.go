// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helper

import (
	"bufio"
	"fmt"
	"io"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/enterprisesearch"
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
	scheme.AddKnownTypes(esv1.GroupVersion, &esv1.Elasticsearch{}, &esv1.ElasticsearchList{})
	scheme.AddKnownTypes(kbv1.GroupVersion, &kbv1.Kibana{}, &kbv1.KibanaList{})
	scheme.AddKnownTypes(apmv1.GroupVersion, &apmv1.ApmServer{}, &apmv1.ApmServerList{})
	scheme.AddKnownTypes(beatv1beta1.GroupVersion, &beatv1beta1.Beat{}, &beatv1beta1.BeatList{})
	scheme.AddKnownTypes(entv1beta1.GroupVersion, &entv1beta1.EnterpriseSearch{}, &entv1beta1.EnterpriseSearchList{})
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
		case *esv1.Elasticsearch:
			b := elasticsearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Elasticsearch = *decodedObj
			builder = transform(b)
		case *kbv1.Kibana:
			b := kibana.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Kibana = *decodedObj
			builder = transform(b)
		case *apmv1.ApmServer:
			b := apmserver.NewBuilderWithoutSuffix(decodedObj.Name)
			b.ApmServer = *decodedObj
			builder = transform(b)
		case *beatv1beta1.Beat:
			b := beat.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Beat = *decodedObj
			// adjust builder to compensate for overwriting Beat struct
			// RBAC objects were populated using a name that doesn't have proper test suffix,
			// hence clearing them here so the next calls can populate them properly
			b.RBACObjects = nil

			// Since b.Beat was overwritten, b.PodTemplate is pointing to wrong struct, fixing it here
			if b.Beat.Spec.DaemonSet != nil {
				b.PodTemplate = &b.Beat.Spec.DaemonSet.PodTemplate
			} else if b.Beat.Spec.Deployment != nil {
				b.PodTemplate = &b.Beat.Spec.Deployment.PodTemplate
			}
			builder = transform(b)
		case *entv1beta1.EnterpriseSearch:
			b := enterprisesearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.EnterpriseSearch = *decodedObj
			builder = transform(b)
		default:
			return builders, fmt.Errorf("unexpected object type: %t", decodedObj)
		}

		builders = append(builders, builder)
	}

	return builders, nil
}
