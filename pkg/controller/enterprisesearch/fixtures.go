// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

var existingConfig = `allow_es_settings_modification: true
elasticsearch:
  host: https://mycluster-es-http.default.svc:9200
  password: sc4q5cx8h88ghn7c5zq8wj4s
  ssl:
    certificate_authority: /mnt/elastic-internal/es-certs/tls.crt
    enabled: true
  username: default-entsearch-sample-entsearch-es-user
ent_search:
  auth:
    source: elasticsearch-native
  external_url: https://localhost:3002
  listen_host: 0.0.0.0
  ssl:
    certificate: /mnt/elastic-internal/http-certs/tls.crt
    certificate_authorities:
    - /mnt/elastic-internal/http-certs/ca.crt
    enabled: true
    key: /mnt/elastic-internal/http-certs/tls.key
`

var existingConfigWithReusableSettings = existingConfig +
	`secret_management:
  encryption_keys:
  - alreadysetencryptionkey1
  - alreadysetencryptionkey2
secret_session_key: alreadysetsessionkey
`
