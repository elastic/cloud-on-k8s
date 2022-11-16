// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
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
	request, err := http.NewRequest( //nolint:noctx
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
	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("error while reading response body: %v \n", err)
		return
	}
	fmt.Printf("%s \n", bytes)
}
