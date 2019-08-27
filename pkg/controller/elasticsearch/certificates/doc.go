// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Package certificates contains the main entry points for certificates and reconciliation of these related to
// running Elasticsearch clusters.
//
// For each Elasticsearch cluster, we manage these CAs:
//   - Transport: Used to issue an unique certificate to every node, used for internal node-to-node communication
//     (Elasticsearch transport protocol).
//   - HTTP: Used to issue a single certificate (signed by the CA) that is shared between the Elasticsearch nodes if
//     no user provided certificate exists.
package certificates
