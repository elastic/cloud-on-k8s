// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Transport certificates
//
// For each Elasticsearch pod, we sign one certificate with the cluster CA.
// The certificate is passed to the pod through a secret volume mount.
// The corresponding private key stays in the ES pod: we request a CSR from the pod,
// and never access the private key directly.
package transport
