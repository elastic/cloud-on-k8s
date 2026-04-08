// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/keystorepassword"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
)

// ElasticsearchKeystoreListArgv returns `sh -c` arguments to run `elasticsearch-keystore list` inside the Elasticsearch
// container. Password-protected keystores prompt on a TTY; without one, the CLI reads the password from stdin (see
// org.elasticsearch.cli.Terminal). KEYSTORE_PASSWORD_FILE is for docker-entrypoint, not for elasticsearch-keystore.
// Mirror the operator init pattern: pipe the password bytes into the CLI (same idea as add-file).
func ElasticsearchKeystoreListArgv() []string {
	script := fmt.Sprintf(`set -e
PWFILE=""
if [ -n "${%[1]s:-}" ] && [ -f "$%[1]s" ]; then
  PWFILE="$%[1]s"
elif [ -f "%[2]s" ]; then
  PWFILE="%[2]s"
fi
if [ -n "$PWFILE" ]; then
  printf '%%s\n' "$(cat "$PWFILE")" | %[3]s list
else
  %[3]s list
fi
`,
		settings.KeystorePasswordFileEnvVar,
		keystorepassword.PasswordFile,
		initcontainer.KeystoreBinPath,
	)
	return []string{"sh", "-c", script}
}
