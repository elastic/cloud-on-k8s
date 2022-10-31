// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v7"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	esuser "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

type LoadTest struct {
	client         *elasticsearch.Client
	start          time.Time
	stop           chan bool
	requestsPerSec int64
	errors         []error
	numberRequests int64
	sync.RWMutex
}

type LoadTestResult struct {
	Success      bool
	NumRequests  int64
	TestDuration time.Duration
	ReqPerSecond float64
	Errors       map[string]int
}

func (ltr LoadTestResult) String() string {
	return fmt.Sprintf("succes: %v, duration: %v, number of requests: %d, requests/s %f, errors: %v",
		ltr.Success, ltr.TestDuration, ltr.NumRequests, ltr.ReqPerSecond, ltr.Errors)
}

func NewLoadTest(k *test.K8sClient, es esv1.Elasticsearch, requestPerSec int) (*LoadTest, error) {
	caCert, err := k.GetHTTPCertsBytes(esv1.ESNamer, es.Namespace, es.Name)
	if err != nil {
		return nil, err
	}
	password, err := k.GetElasticPassword(k8s.ExtractNamespacedName(&es))
	if err != nil {
		return nil, err
	}
	url := services.InternalServiceURL(es)
	cfg := elasticsearch.Config{
		Addresses: []string{url},
		Username:  esuser.ElasticUserName,
		Password:  password,
		CACert:    caCert,
	}
	if test.Ctx().AutoPortForwarding {
		cfg.Transport = &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         portforward.NewForwardingDialer().DialContext,
			MaxIdleConnsPerHost: 0,
			DisableKeepAlives:   true,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &LoadTest{
		client:         client,
		stop:           make(chan bool, 1),
		requestsPerSec: int64(requestPerSec),
	}, nil
}

func (lt *LoadTest) Start() {
	lt.start = time.Now()
	// TODO more than one worker if we think it would be beneficial
	go func() {
		for {
			select {
			case <-lt.stop:
				return
			default:
				time.Sleep(lt.timeUntilNextReq())
				lt.req()
				continue
			}
		}
	}()
}

func (lt *LoadTest) req() {
	info, err := lt.client.Info()
	lt.RWMutex.Lock()
	defer lt.RWMutex.Unlock()
	lt.numberRequests++
	if err != nil {
		lt.errors = append(lt.errors, err)
		// give immediate feedback if things are going south
		println(err.Error())
		return
	}
	if info.IsError() {
		lt.errors = append(lt.errors, fmt.Errorf("failed request: %s", info.Status()))
	}
	defer info.Body.Close()
	_, _ = io.Copy(io.Discard, info.Body)
}

func (lt *LoadTest) Stop() LoadTestResult {
	lt.stop <- true
	stopped := time.Now()
	lt.RLock()
	defer lt.RUnlock()

	testDur := stopped.Sub(lt.start)
	errReport := map[string]int{}
	for _, e := range lt.errors {
		errStr := e.Error()
		_, exists := errReport[errStr]
		if !exists {
			errReport[errStr] = 0
		}
		errReport[errStr]++
	}
	return LoadTestResult{
		Success:      len(lt.errors) == 0,
		Errors:       errReport,
		NumRequests:  lt.numberRequests,
		TestDuration: testDur,
		ReqPerSecond: float64(lt.numberRequests) / testDur.Seconds(),
	}
}

func (lt *LoadTest) timeUntilNextReq() time.Duration {
	lt.RWMutex.RLock()
	defer lt.RWMutex.RUnlock()

	elapsed := time.Since(lt.start)
	expectedNumRequests := lt.requestsPerSec * int64(elapsed.Seconds())
	if lt.numberRequests < expectedNumRequests {
		// we made fewer requests that expected probably due to slow responses from server continue immediately
		return 0
	}
	every := int64(time.Second) / lt.requestsPerSec
	expectedTimeSpend := time.Duration((lt.numberRequests + 1) * every)
	return expectedTimeSpend - elapsed
}
