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

func NoMessageContaining(message string) ValidationFunc {
	return NoEvent(fmt.Sprintf("message:%s", message))
}

func HasEvent(query string) ValidationFunc {
	return hasEvent(fmt.Sprintf("/*beat*/_search?q=%s", query))
}

func NoEvent(query string) ValidationFunc {
	return noEvent(fmt.Sprintf("/*beat*/_search?q=%s", query))
}

// HasMonitoringEvent is the same as HasEvent, but checks the stack monitoring indices
// Note that event.dataset is not indexed in these indices
func HasMonitoringEvent(query string) ValidationFunc {
	return hasEvent(fmt.Sprintf("/.monitoring-*/_search?q=%s", query))
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
