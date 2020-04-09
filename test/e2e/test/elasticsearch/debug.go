// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

func printShardsAndAllocation(clientFactory func() (client.Client, error)) func() {
	return func() {
		esClient, err := clientFactory()
		if err != nil {
			fmt.Printf("error while creating es client: %v", err)
			return
		}
		printResponse(esClient, "/_cat/shards?v")
		printResponse(esClient, "/_cluster/allocation/explain")
	}
}

func printResponse(esClient client.Client, url string) {
	request, err := http.NewRequest(
		http.MethodGet,
		url,
		nil,
	)
	if err != nil {
		fmt.Printf("error while creating request: %v \n", err)
		return
	}
	fmt.Println("GET " + url)
	response, err := esClient.Request(context.Background(), request)
	if err != nil {
		fmt.Printf("error while making request %s: %v \n", url, err)
		return
	}
	defer response.Body.Close()
	bytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("error while reading response body: %v \n", err)
		return
	}
	fmt.Printf("%s \n", bytes)
}
