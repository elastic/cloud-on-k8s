---
navigation_title: "Release Notes"
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
- Fix unresolved attribute in ECK Quickstart [#8432](https://github.com/elastic/cloud-on-k8s/pull/8432)
- Add synthetic monitoring example [#8385](https://github.com/elastic/cloud-on-k8s/pull/8385) (issue: [#6294](https://github.com/elastic/cloud-on-k8s/issues/6294))
- Update heap dump command to use the most recent Java process [#8294](https://github.com/elastic/cloud-on-k8s/pull/8294)
- Document the need for an ingest node for Enterprise Search analytics [#8271](https://github.com/elastic/cloud-on-k8s/pull/8271)

### Miscellaneous
- chore(deps): update Docker tag `registry.access.redhat.com/ubi9/ubi-minimal` to `v9.5-1741850109` [#8544](https://github.com/elastic/cloud-on-k8s/pull/8544)
- Update `golang.org/x/net` package to `0.37.0` [#8521](https://github.com/elastic/cloud-on-k8s/pull/8521)
- chore(deps): update Docker tag `docker.elastic.co/wolfi/go` to `v1.24` [#8453](https://github.com/elastic/cloud-on-k8s/pull/8453)
- fix(deps): update module `go.elastic.co/apm/v2/*` to `v2.6.3` [#8440](https://github.com/elastic/cloud-on-k8s/pull/8440)
- chore(deps): update Wolfi to `v1.23.5-r1` [#8434](https://github.com/elastic/cloud-on-k8s/pull/8434)
- fix(deps): update k8s [#8400](https://github.com/elastic/cloud-on-k8s/pull/8400)
- fix(deps): update module `github.com/gkampitakis/go-snaps` to `v0.5.8` [#8393](https://github.com/elastic/cloud-on-k8s/pull/8393)
- Bump `golang.org/x/crypto` from `0.29.0` to `0.31.0` [#8334](https://github.com/elastic/cloud-on-k8s/pull/8334)
- fix(deps): update module `github.com/prometheus/common` to `v0.61.0` [#8333](https://github.com/elastic/cloud-on-k8s/pull/8333)
- fix(deps): update Kubernetes dependencies to `v0.32.0` and controller-runtime to `v0.19.3` [#8330](https://github.com/elastic/cloud-on-k8s/pull/8330)
- fix(deps): update module `github.com/magiconair/properties` to `v1.8.9` [#8307](https://github.com/elastic/cloud-on-k8s/pull/8307)
- chore(deps): update Docker tag `docker.elastic.co/wolfi/go` to `v1.23.4` [#8306](https://github.com/elastic/cloud-on-k8s/pull/8306)
- fix(deps): update module `github.com/stretchr/testify` to `v1.10.0` [#8282](https://github.com/elastic/cloud-on-k8s/pull/8282)
