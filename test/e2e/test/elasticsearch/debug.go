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

func printShards(clientFactory func() (client.Client, error)) func() {
	return func() {
		esClient, err := clientFactory()
		if err != nil {
			fmt.Printf("error while creating es client: %v", err)
			return
		}
		catShards, err := http.NewRequest(
			http.MethodGet,
			"/_cat/shards",
			nil,
		)
		if err != nil {
			fmt.Printf("error while creating request: %v \n", err)
			return
		}
		shards, err := esClient.Request(context.Background(), catShards)
		if err != nil {
			fmt.Printf("error while fetching shards: %v \n", err)
			return
		}
		defer shards.Body.Close()
		bytes, err := ioutil.ReadAll(shards.Body)
		if err != nil {
			fmt.Printf("error while reading response body: %v \n", err)
			return
		}
		fmt.Println("GET _cat/shards")
		fmt.Printf("%s \n", bytes)
	}
}
