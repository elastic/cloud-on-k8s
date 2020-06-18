// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

const (
	appSearchEngine     = "test-engine"
	appSearchSampleDocs = `[
	{
		"id": "park_rocky-mountain",
		"title": "Rocky Mountain",
		"description": "Bisected north to south by the Continental Divide, this portion of the Rockies has ecosystems varying from over 150 riparian lakes to montane and subalpine forests to treeless alpine tundra. Wildlife including mule deer, bighorn sheep, black bears, and cougars inhabit its igneous mountains and glacial valleys. Longs Peak, a classic Colorado fourteener, and the scenic Bear Lake are popular destinations, as well as the historic Trail Ridge Road, which reaches an elevation of more than 12,000 feet (3,700 m).",
		"nps_link": "https://www.nps.gov/romo/index.htm",
		"states": [
			"Colorado"
	],
		"visitors": 4517585,
		"world_heritage_site": false,
		"location": "40.4,-105.58",
		"acres": 265795.2,
		"square_km": 1075.6,
		"date_established": "1915-01-26T06:00:00Z"
	},
	{
		"id": "park_saguaro",
		"title": "Saguaro",
		"description": "Split into the separate Rincon Mountain and Tucson Mountain districts, this park is evidence that the dry Sonoran Desert is still home to a great variety of life spanning six biotic communities. Beyond the namesake giant saguaro cacti, there are barrel cacti, chollas, and prickly pears, as well as lesser long-nosed bats, spotted owls, and javelinas.",
		"nps_link": "https://www.nps.gov/sagu/index.htm",
		"states": [
		"Arizona"
		],
		"visitors": 820426,
		"world_heritage_site": false,
		"location": "32.25,-110.5",
		"acres": 91715.72,
		"square_km": 371.2,
		"date_established": "1994-10-14T05:00:00Z"
	}
	]`
)

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	var entClient EnterpriseSearchClient
	var appSearchClient AppSearchClient
	return test.StepList{
		test.Step{
			Name: "Every secret should be set so that we can build an Enterprise Search client",
			Test: test.Eventually(func() error {
				var err error
				entClient, err = NewEnterpriseSearchClient(b.EnterpriseSearch, k)
				return err
			}),
		},
		test.Step{
			Name: "Enterprise Search health endpoint should eventually respond",
			Test: test.Eventually(func() error { // nolint
				return entClient.HealthCheck()
			}),
		},

		// App Search tests
		test.Step{
			Name: "Retrieve the App Search API Key",
			Test: test.Eventually(func() error {
				appSearchClient = entClient.AppSearch()
				key, err := appSearchClient.GetAPIKey()
				if err != nil {
					return err
				}
				appSearchClient = appSearchClient.WithAPIKey(key)
				return nil
			}),
		},
		test.Step{
			Name: "Create an App Search engine (if not already done)",
			Test: test.Eventually(func() error {
				results, err := appSearchClient.GetEngines()
				if err != nil {
					return err
				}
				if len(results.Results) > 0 {
					// already done
					return nil
				}
				return appSearchClient.CreateEngine(appSearchEngine)
			}),
		},
		test.Step{
			Name: "Index documents in the App Search engine (if not already done)",
			Test: test.Eventually(func() error {
				results, err := appSearchClient.GetDocuments(appSearchEngine)
				if err != nil {
					return err
				}
				if len(results.Results) > 0 {
					// already done
					return nil
				}
				return appSearchClient.IndexDocument(appSearchEngine, appSearchSampleDocs)
			}),
		},
		test.Step{
			Name: "Querying 'mountain' in App Search should eventually return 2 documents",
			// the index operation is not atomic, hence the search retry
			Test: test.Eventually(func() error {
				results, err := appSearchClient.SearchDocuments(appSearchEngine, "mountain")
				if err != nil {
					return err
				}
				if len(results.Results) != 2 {
					return fmt.Errorf("expected %d documents, got %d", 2, len(results.Results))
				}
				return nil
			}),
		},
		// Workplace Search tests are much harder to perform, so there are none so far:
		// - we need a trial license (already manipulated once in other tests)
		// - connecting to an external source (eg. Google Drive) is probably too much work
		// Both the Healthcheck and AppSearch tests already validated the Enterprise Search instance can
		// correctly communicate with Elasticsearch.
	}
}
