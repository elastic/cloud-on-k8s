// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Transport certificates
//
// For each Elasticsearch pod, we issue one certificate from the cluster CA.
// The certificate and associated private key are passed to the pod through a secret volume mount.
package transport
