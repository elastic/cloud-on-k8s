---
navigation_title: "Elastic Cloud on Kubernetes"
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/release-highlights.html
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/eck-release-notes.html
---

# Elastic Cloud on Kubernetes release notes [elastic-cloud-kubernetes-release-notes]
Review the changes, fixes, and more in each release of Elastic Cloud on Kubernetes. 

% Release notes includes only features, enhancements, and fixes. Add breaking changes, deprecations, and known issues to the applicable release notes sections. 

% ## version.next [elastic-cloud-lubernetes-versionext-release-notes]

% ### Features and enhancements [elastic-cloud-kubernetes-versionext-features-enhancements]

% ### Fixes [elastic-cloud-kubernetes-versionext-fixes]

## 3.0.0 [elastic-cloud-kubernetes-300-release-notes]

### Release Highlights
- ECK 3.0.0 adds support for Elastic Stack version 9.0.0. Elastic Stack version 9.0.0 is not supported on ECK operators running versions earlier than 3.0.0.

### Features and enhancements [elastic-cloud-kubernetes-300-features-enhancements]
- Add support for defining `dnsPolicy` and `dnsConfig` options for the ECK operator StatefulSet [#7999](https://github.com/elastic/cloud-on-k8s/pull/7999)
- Config: Allow escaping dots in keys via `[unsplit.key]` syntax [#8512](https://github.com/elastic/cloud-on-k8s/pull/8512) (issue: [#8499](https://github.com/elastic/cloud-on-k8s/issues/8499))
- Enable copying of ECK images to Amazon ECR to make it easier for users to find our own ECK operator in the AWS marketplace [#8427](https://github.com/elastic/cloud-on-k8s/pull/8427)
- Support new agent image path as of 9.0 [#8518](https://github.com/elastic/cloud-on-k8s/pull/8518)
- Remove ubi suffix for 9.x images [#8509](https://github.com/elastic/cloud-on-k8s/pull/8509)
- Remove support for 6.x Stack version [#8507](https://github.com/elastic/cloud-on-k8s/pull/8507)
- Log resourceVersion on Create and Update [#8503](https://github.com/elastic/cloud-on-k8s/pull/8503)
- Remove policyID validation [#8449](https://github.com/elastic/cloud-on-k8s/pull/8449) (issue: [#8446](https://github.com/elastic/cloud-on-k8s/issues/8446))
- Refactor APM server for 9.0.0 [#8448](https://github.com/elastic/cloud-on-k8s/pull/8448) (issue: [#8447](https://github.com/elastic/cloud-on-k8s/issues/8447))
- Improve error messages and events during Fleet setup [#8350](https://github.com/elastic/cloud-on-k8s/pull/8350)
- Validate updates to 9.0 go through 8.18 [#8559](https://github.com/elastic/cloud-on-k8s/pull/8559) (issue: [#8557](https://github.com/elastic/cloud-on-k8s/issues/8557))

### Fixes [elastic-cloud-kubernetes-300-fixes]
- Correctly parse managed namespaces when specified as an environment variable [#8513](https://github.com/elastic/cloud-on-k8s/pull/8513) (issue: [#7542](https://github.com/elastic/cloud-on-k8s/issues/7542))

### Documentation improvements [elastic-cloud-kubernetes-300-documentation-improvements]
- [DOCS] Updates release notes title ([#8599](https://github.com/elastic/cloud-on-k8s/pull/8599))
- Updates for Istio 1.24 ([#8476](https://github.com/elastic/cloud-on-k8s/pull/8476))
- Fix unresolved attribute in ECK Quickstart ([#8432](https://github.com/elastic/cloud-on-k8s/pull/8432))
- [Docs] Add synthetic monitoring example ([#8385](https://github.com/elastic/cloud-on-k8s/pull/8385)) (issue: [#6294](https://github.com/elastic/cloud-on-k8s/issues/6294))
- [docs] Update heap dump command to use the most recent Java process ([#8294](https://github.com/elastic/cloud-on-k8s/pull/8294))
- [DOC] Document the need for an ingest node for Enterprise Search analytics ([#8271](https://github.com/elastic/cloud-on-k8s/pull/8271))

# Miscellaneous
- Update golang.org/x/net package to 0.38.0 ([#8591](https://github.com/elastic/cloud-on-k8s/pull/8591))
- fix(deps): update module go.elastic.co/apm/v2 to v2.7.0 ([#8576](https://github.com/elastic/cloud-on-k8s/pull/8576))
- chore(deps): update registry.access.redhat.com/ubi9/ubi-minimal docker tag to v9.5-1741850109 ([#8544](https://github.com/elastic/cloud-on-k8s/pull/8544))
- fix(deps): update module golang.org/x/oauth2 to v0.28.0 ([#8528](https://github.com/elastic/cloud-on-k8s/pull/8528))
- fix(deps): update module github.com/gkampitakis/go-snaps to v0.5.11 ([#8524](https://github.com/elastic/cloud-on-k8s/pull/8524))
- fix(deps): update module github.com/spf13/cobra to v1.9.1 ([#8523](https://github.com/elastic/cloud-on-k8s/pull/8523))
- fix(deps): update module github.com/prometheus/client_golang to v1.21.1 ([#8520](https://github.com/elastic/cloud-on-k8s/pull/8520))
- fix(deps): update module github.com/jonboulle/clockwork to v0.5.0 ([#8519](https://github.com/elastic/cloud-on-k8s/pull/8519))
- fix(deps): update module github.com/hashicorp/vault/api to v1.16.0 ([#8517](https://github.com/elastic/cloud-on-k8s/pull/8517))
- fix(deps): update module github.com/google/go-cmp to v0.7.0 ([#8516](https://github.com/elastic/cloud-on-k8s/pull/8516))
- fix(deps): update module helm.sh/helm/v3 to v3.17.1 ([#8505](https://github.com/elastic/cloud-on-k8s/pull/8505))
- chore(deps): update wolfi (versioned) ([#8504](https://github.com/elastic/cloud-on-k8s/pull/8504))
- Bump github.com/go-jose/go-jose/v4 from 4.0.1 to 4.0.5 ([#8488](https://github.com/elastic/cloud-on-k8s/pull/8488))
- fix(deps): update module github.com/go-git/go-git/v5 to v5.14.0 ([#8487](https://github.com/elastic/cloud-on-k8s/pull/8487))
- fix(deps): update k8s ([#8486](https://github.com/elastic/cloud-on-k8s/pull/8486))
- chore(deps): update docker.elastic.co/ci-agent-images/serverless-docker-builder:0.6.1 docker digest to 730f062 ([#8483](https://github.com/elastic/cloud-on-k8s/pull/8483))
- fix(deps): update module github.com/spf13/pflag to v1.0.6 ([#8468](https://github.com/elastic/cloud-on-k8s/pull/8468))
- Update module github.com/gkampitakis/go-snaps to v0.5.10 ([#8467](https://github.com/elastic/cloud-on-k8s/pull/8467))
- chore(deps): update dependency go to v1.24.1 ([#8454](https://github.com/elastic/cloud-on-k8s/pull/8454))
- chore(deps): update docker.elastic.co/wolfi/go docker tag to v1.24 ([#8453](https://github.com/elastic/cloud-on-k8s/pull/8453))
- fix(deps): update module go.elastic.co/apm/v2/* to v2.6.3 ([#8440](https://github.com/elastic/cloud-on-k8s/pull/8440))
- chore(deps): update wolfi to v1.23.5-r1 ([#8434](https://github.com/elastic/cloud-on-k8s/pull/8434))
- Bump golang.org/x/crypto from 0.29.0 to 0.31.0 ([#8334](https://github.com/elastic/cloud-on-k8s/pull/8334))
- fix(deps): update module github.com/prometheus/common to v0.61.0 ([#8333](https://github.com/elastic/cloud-on-k8s/pull/8333))
- fix(deps): update module github.com/magiconair/properties to v1.8.9 ([#8307](https://github.com/elastic/cloud-on-k8s/pull/8307))
- chore(deps): update docker.elastic.co/wolfi/go docker tag to v1.23.4 ([#8306](https://github.com/elastic/cloud-on-k8s/pull/8306))
- fix(deps): update module github.com/stretchr/testify to v1.10.0 ([#8282](https://github.com/elastic/cloud-on-k8s/pull/8282))
