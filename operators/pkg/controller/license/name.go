/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import "strings"

const licenseSuffix = "-license"

func licenseNameFromCluster(c string) string {
	return c + licenseSuffix
}

func clusterNameFromLicense(l string) string {
	return strings.TrimSuffix(l, licenseSuffix)
}
