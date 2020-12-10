// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

func HasEventFromAgent() ValidationFunc {
	return HasEvent("event.dataset:elastic_agent AND elastic_agent:*")
}

func HasEvent(query string) ValidationFunc {
	return hasEvent(fmt.Sprintf("/*agent*/_search?q=%s", url.QueryEscape(query))) //todo agent
}

func NoEvent(query string) ValidationFunc {
	return noEvent(fmt.Sprintf("/*agent*/_search?q=%s", query))
}

type DataStreamResult struct {
	DataStreams []DataStream `json:"data_streams"`
	Error       interface{}  `json:"error"`
}

type DataStream struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func HasDataStream(name string) ValidationFunc {
	return func(esClient client.Client) error {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/_data_stream/%s", name), nil)
		if err != nil {
			return err
		}

		res, err := esClient.Request(context.Background(), req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		resultBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		var results DataStreamResult
		err = json.Unmarshal(resultBytes, &results)
		if err != nil {
			return err
		}

		if results.Error != nil {
			return fmt.Errorf("error %v while checking for data stream %s", results.Error, name)
		}

		if len(results.DataStreams) != 1 {
			return fmt.Errorf(
				"unexpected count %v of data streams returned when looking for %s",
				len(results.DataStreams),
				name)
		}

		if results.DataStreams[0].Name != name {
			return fmt.Errorf("unexpected data stream %s returned when looking for %s",
				results.DataStreams[0].Name,
				name)
		}

		if results.DataStreams[0].Status != "GREEN" {
			return fmt.Errorf("data stream status is %s instead of GREEN", results.DataStreams[0].Status)
		}

		return nil
	}
}

func hasEvent(url string) ValidationFunc {
	return checkEvent(url, func(hitsCount int) error {
		if hitsCount == 0 {
			return fmt.Errorf("hit count should be more than 0 for %s", url)
		}
		return nil
	})
}

func noEvent(url string) ValidationFunc {
	return checkEvent(url, func(hitsCount int) error {
		if hitsCount != 0 {
			return fmt.Errorf("hit count should be 0 for %s", url)
		}
		return nil
	})
}

func checkEvent(url string, check func(int) error) ValidationFunc {
	return func(esClient client.Client) error {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		res, err := esClient.Request(context.Background(), req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		resultBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		var results client.SearchResults
		err = json.Unmarshal(resultBytes, &results)
		if err != nil {
			return err
		}
		if err := check(len(results.Hits.Hits)); err != nil {
			return err
		}

		return nil
	}
}
