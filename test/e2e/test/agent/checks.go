// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

const (
	MetricsType = "metrics"
	LogsType    = "logs"
)

type DataStreamResult struct {
	DataStreams []DataStream `json:"data_streams"`
	Error       interface{}  `json:"error"`
}

type DataStream struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func HasEvent(query string) ValidationFunc {
	return checkEvent(query, func(hitsCount int) error {
		if hitsCount == 0 {
			return fmt.Errorf("hit count should be more than 0 for %s", query)
		}
		return nil
	})
}

func NoEvent(query string) ValidationFunc {
	return checkEvent(query, func(hitsCount int) error {
		if hitsCount != 0 {
			return fmt.Errorf("hit count should be 0 for %s", query)
		}
		return nil
	})
}

func HasWorkingDataStream(typ, dataset, namespace string) ValidationFunc {
	dsName := fmt.Sprintf("%s-%s-%s", typ, dataset, namespace)
	return and(
		HasDataStream(dsName),
		HasEvent(fmt.Sprintf("/%s/_search?q=!error.message:*", dsName)),
	)
}

func HasAnyDataStream() ValidationFunc {
	return func(esClient client.Client) error {
		resultBytes, err := request(esClient, "/_data_stream")
		if err != nil {
			return err
		}

		var results DataStreamResult
		err = json.Unmarshal(resultBytes, &results)
		if err != nil {
			return err
		}

		if results.Error != nil {
			return fmt.Errorf("error %v while checking for data streams", results.Error)
		}

		if len(results.DataStreams) == 0 {
			return errors.New("no data streams found")
		}

		return nil
	}
}

func HasDataStream(name string) ValidationFunc {
	return func(esClient client.Client) error {
		resultBytes, err := request(esClient, fmt.Sprintf("/_data_stream/%s", name))
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

func checkEvent(url string, check func(int) error) ValidationFunc {
	return func(esClient client.Client) error {
		resultBytes, err := request(esClient, url)
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

func request(esClient client.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	res, err := esClient.Request(context.Background(), req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	resultBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return resultBytes, nil
}

func and(validationFuncs ...ValidationFunc) ValidationFunc {
	return func(esClient client.Client) error {
		for _, vf := range validationFuncs {
			if err := vf(esClient); err != nil {
				return err
			}
		}
		return nil
	}
}
