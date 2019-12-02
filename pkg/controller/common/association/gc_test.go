// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	kibanatype "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	testclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	fakerest "k8s.io/client-go/rest/fake"
)

const (
	ApmAssociationLabelName         = "apmassociation.k8s.elastic.co/name"
	ApmAssociationLabelNamespace    = "apmassociation.k8s.elastic.co/namespace"
	KibanaAssociationLabelName      = "kibanaassociation.k8s.elastic.co/name"
	KibanaAssociationLabelNamespace = "kibanaassociation.k8s.elastic.co/namespace"
)

// fakeClientFactory returns a rest client which relies on newFakeRoundTripper to serve the response
func fakeClientFactory(baseConfig *rest.Config, gv schema.GroupVersion) (rest.Interface, error) {
	codecs := serializer.NewCodecFactory(k8s.Scheme())
	return &fakerest.RESTClient{
		Client:               fakerest.CreateHTTPClient(newFakeRoundTripper()),
		NegotiatedSerializer: codecs.WithoutConversion(),
		GroupVersion:         gv,
		// not strictly necessary here, but let's try to have something similar to the reality
		VersionedAPIPath: "apis" + "/" + gv.Group + "/" + gv.Version,
	}, nil
}

// newFakeRoundTripper returns a fixed list values to be used in a fake REST client
func newFakeRoundTripper() func(req *http.Request) (*http.Response, error) {
	fakeResources := make(map[string]interface{})
	fakeResources["/apis/apm.k8s.elastic.co/v1beta1/apmservers"] = &apmtype.ApmServerList{
		TypeMeta: v1.TypeMeta{},
		ListMeta: v1.ListMeta{},
		Items: []apmtype.ApmServer{
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "apm1",
					Namespace: "ns1",
				},
			},
		},
	}
	fakeResources["/apis/kibana.k8s.elastic.co/v1beta1/kibanas"] = &kibanatype.KibanaList{
		TypeMeta: v1.TypeMeta{},
		ListMeta: v1.ListMeta{},
		Items: []kibanatype.Kibana{
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "kibana1",
					Namespace: "ns1",
				},
			},
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "kibana2",
					Namespace: "ns2",
				},
			},
		},
	}

	fakeReqHandler := func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case "GET":
			res, err := json.Marshal(fakeResources[req.URL.Path])
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: 200, Body: ioutil.NopCloser(bytes.NewReader(res))}, nil
		default:
			return nil, fmt.Errorf("unexpected request for URL %q with method %q", req.URL.String(), req.Method)
		}
	}
	return fakeReqHandler
}

func newUserSecret(
	namespace, name,
	associationNamespaceLabel, associationNameLabel,
	associationNamespaceValue, associationNameValue string,
) runtime.Object {
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				associationNameLabel:      associationNameValue,
				associationNamespaceLabel: associationNamespaceValue,
				common.TypeLabelName:      user.UserType,
			},
		},
	}
}

func TestUsersGarbageCollector_GC(t *testing.T) {
	// Create 5 secrets, 3 actually used and 2 orphaned
	clientset := testclient.NewSimpleClientset(
		newUserSecret("es", "ns1-kb-orphaned-xxxx-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns1", "orphaned-kibana"),
		newUserSecret("es", "ns1-kb-kibana1-w2fz-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns1", "kibana1"),
		newUserSecret("es", "ns1-kb-kibana2-fy8i-kibana-user", KibanaAssociationLabelNamespace, KibanaAssociationLabelName, "ns2", "kibana2"),
		newUserSecret("es", "ns1-kb-orphaned-xxxx-apm-user", ApmAssociationLabelNamespace, ApmAssociationLabelName, "ns1", "orphaned-apm"),
		newUserSecret("es", "ns1-kb-apm1-yrfa-apm-user", ApmAssociationLabelNamespace, ApmAssociationLabelName, "ns1", "apm1"),
	)

	restMapper := &fakeRESTMapper{}
	ugc := &UsersGarbageCollector{
		clientset:     clientset,
		baseConfig:    &rest.Config{},
		mapper:        restMapper,
		scheme:        k8s.Scheme(),
		clientFactory: fakeClientFactory,
	}

	// register some resources
	ugc.RegisterForUserGC(&apmtype.ApmServerList{}, ApmAssociationLabelNamespace, ApmAssociationLabelName)
	ugc.RegisterForUserGC(&kibanatype.KibanaList{}, KibanaAssociationLabelNamespace, KibanaAssociationLabelName)

	err := ugc.GC()
	if err != nil {
		t.Errorf("UsersGarbageCollector.GC() error = %v", err)
		return
	}

	// kibana1, kibana2 and apm1 user Secret must still be present
	_, err = clientset.CoreV1().Secrets("es").Get("ns1-kb-kibana1-w2fz-kibana-user", v1.GetOptions{})
	assert.NoError(t, err)
	_, err = clientset.CoreV1().Secrets("es").Get("ns1-kb-kibana2-fy8i-kibana-user", v1.GetOptions{})
	assert.NoError(t, err)
	_, err = clientset.CoreV1().Secrets("es").Get("ns1-kb-apm1-yrfa-apm-user", v1.GetOptions{})
	assert.NoError(t, err)

	// Orphaned secret must have been deleted
	_, err = clientset.CoreV1().Secrets("es").Get("ns1-kb-orphaned-xxxx-kibana-user", v1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
	_, err = clientset.CoreV1().Secrets("es").Get("ns1-kb-orphaned-xxxx-apm-user", v1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

type fakeRESTMapper struct {
	meta.DefaultRESTMapper
}

// RESTMapping is a fake implementation that returns the plural of a Kind
func (f *fakeRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return &meta.RESTMapping{
		Resource: schema.GroupVersionResource{
			Group:    gk.Group,
			Resource: strings.ToLower(gk.Kind) + "s",
		},
		GroupVersionKind: schema.GroupVersionKind{},
	}, nil
}
