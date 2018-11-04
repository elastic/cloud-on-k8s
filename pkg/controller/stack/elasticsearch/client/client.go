package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/elastic/stack-operators/pkg/controller/stack/common"
)

type Client struct {
	HTTP     *http.Client
	Endpoint string
} //TODO credentials, TLS

func checkError(response *http.Response) error {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New(fmt.Sprintf("%s returned %s, %v", response.Request.URL, response.Status, response.Header))
	}
	return nil
}

func parseRoutingTable(raw interface{}) ([]Shard, error) {
	var result []Shard
	table, ok := raw.(map[string]interface{})
	if !ok {
		return result, errors.New("cluster state was not a map")
	}
	nodes, ok := table["nodes"].(map[string]interface{})
	if nodes == nil || !ok {
		return result, errors.New("cluster state did not contain nodes")
	}
	routingTable, ok := table["routing_table"].(map[string]interface{})
	if routingTable == nil || !ok {
		return result, errors.New("cluster state did not contain routing table")
	}

	indices, ok := routingTable["indices"].(map[string]interface{})
	if indices == nil || !ok {
		return result, nil // no indices
	}

	for k1, v := range indices {
		index, ok := v.(map[string]interface{})
		if !ok {
			return result, errors.New(common.Concat("Unexpected type ", reflect.TypeOf(index).String(), " at [", k1, "]"))
		}
		for k2, shardMap := range index {
			shards, ok := shardMap.(map[string]interface{})
			if !ok {
				return result, errors.New(common.Concat(
					"Expected a shard map at [",
					k1, "/", k2,
					"] but was [",
					reflect.TypeOf(shardMap).String(), "]"))
			}
			for k3, shardArray := range shards {
				rawShards, ok := shardArray.([]interface{})
				if !ok {
					return result, errors.New(common.Concat(
						"Expected a shard map at [",
						k1, "/", k2, "/", k3,
						"] but was [",
						reflect.TypeOf(shardArray).String(), "]"))
				}

				for _, rawShard := range rawShards {
					shardMap := rawShard.(map[string]interface{})
					parsedShard := Shard{}
					for k4, val := range shardMap {
						switch val.(type) {
						case string:
							switch k4 {
							case "state":
								parsedShard.State = val.(string)
							case "node":
								parsedShard.Node = nodes[val.(string)].(map[string]interface{})["name"].(string) //panics
							case "index":
								parsedShard.Index = val.(string)
							case "shard":

							default:
								//ignore
							}
						case float64:
							if k4 == "shard" {
								parsedShard.Shard = int(val.(float64))
							}
						case bool:
							if k4 == "primary" {
								parsedShard.Primary = val.(bool)
							}
						default:
							//if val != nil {
							//	fmt.Printf("Ignoring %v (%s) -> %v", k4, reflect.TypeOf(val).String(), val)
							//}

							//ignore
						}

					}
					result = append(result, parsedShard)

				}
			}

		}
	}

	return result, nil

}

func (c *Client) GetShards() ([]Shard, error) {
	result := []Shard{}
	resp, err := c.HTTP.Get(fmt.Sprintf("%s/_cluster/state", c.Endpoint))
	if err != nil {
		return result, err
	}

	err = checkError(resp)
	if err != nil {
		return result, err
	}

	defer resp.Body.Close()
	var raw interface{}
	err = json.NewDecoder(resp.Body).Decode(&raw)
	if err != nil {
		return result, err
	}
	return parseRoutingTable(raw)
}

func (c *Client) ExcludeFromShardAllocation(nodes string) error {
	allocationSetting := ClusterRoutingAllocation{TransientSettings{ExcludeName: nodes, Enable: "all"}}
	body, err := json.Marshal(allocationSetting)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/_cluster/settings", c.Endpoint), bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")

	response, err := c.HTTP.Do(request)
	if err != nil {
		return err
	}

	err = checkError(response)
	if err != nil {
		return err
	}
	return nil
}
