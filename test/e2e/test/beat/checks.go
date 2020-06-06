// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

func HasEventFromBeat(name beatcommon.Type) ValidationFunc {
	return HasEvent(fmt.Sprintf("agent.type:%s", name))
}

func HasEventFromPod(name string) ValidationFunc {
	return HasEvent(fmt.Sprintf("kubernetes.pod.name:%s", name))
}

func HasMessageContaining(message string) ValidationFunc {
	return HasEvent(fmt.Sprintf("message:%s", message))
}

func HasEvent(query string) ValidationFunc {
	return hasEvent(fmt.Sprintf("/*beat*/_search?q=%s", query))
}

func hasEvent(url string) ValidationFunc {
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
		if len(results.Hits.Hits) == 0 {
			return fmt.Errorf("hit count should be more than 0 for %s", url)
		}

		return nil
	}
}
