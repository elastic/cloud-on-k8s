// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// package portforward provides a dialer that uses the Kubernetes API to automatically forward connections to
// Kubernetes services and pods using the "port-forward" feature of kubectl.
//
// This is convenient when running outside of Kubernetes while still requiring having TCP-level access to certain
// services and pods running inside of Kubernetes.
//
// Note: It is intended for development use only.
package portforward

import ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"

var (
	log = ulog.Log.WithName("dev-portforward")
)
