// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package tracing

import (
	"net/http"

	"go.elastic.co/apm/v2"
	"k8s.io/client-go/transport"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing/apmclientgo"
)

func ClientGoTransportWrapper(o ...apmclientgo.ClientOption) transport.WrapperFunc {
	return func(rt http.RoundTripper) http.RoundTripper {
		return apmclientgo.WrapRoundTripper(rt, o...)
	}
}

// ClientGoCacheTx creates a new apm.Transaction for cache refresh requests. If no APM transaction is running
// we assume the transaction is used to manage the client-go cache, as we cannot inject a context into those
// requests. All other requests should have a properly initialised transaction originating in the operator code.
func ClientGoCacheTx(tracer *apm.Tracer) func() *apm.Transaction {
	return func() *apm.Transaction {
		if tracer == nil {
			return nil
		}
		// if no transaction is running assume we are tracing cache refresh requests from client-go
		return tracer.StartTransaction("client-go", string(PeriodicTxType))
	}
}
