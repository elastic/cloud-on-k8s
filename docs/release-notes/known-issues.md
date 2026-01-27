---
navigation_title: "Known issues"
---

# Elastic Cloud on Kubernetes known issues [elastic-cloud-kubernetes-known-issues]

Known issues are significant defects or limitations that may impact your implementation. These issues are actively being worked on and will be addressed in a future release. Review the Elastic Cloud on Kubernetes known issues to help you make informed decisions, such as upgrading to a new version.

% Use the following template to add entries to this page.

% :::{dropdown} Title of known issue 
% Applicable versions for the known issue and the version for when the known issue was fixed % On [Month Day, Year], a known issue was discovered that [description of known issue]. 
% For more information, check [Issue #](Issue link).

% Workaround 
% Workaround description.

:::

## 3.3.0 [elastic-cloud-kubernetes-330-known-issues]

:::{dropdown} Stack Config Policies - File settings may not reload correctly on {{es}} versions prior to 8.11.0

{{es}} versions prior to 8.11.0 contain a bug where updates to file-based cluster settings may not be reloaded correctly when the file changes. This is caused by an {{es}} issue where new keys in file-settings are incorrectly deleted during file monitoring and reload operations.

When using Stack Config Policies with affected versions, updated settings may not appear correctly when querying the `_cluster/settings` endpoint, even though the Stack Config Policy has been updated. Making an additional manual update to the Stack Config Policy may trigger the settings to reload correctly.

This issue was fixed in {{es}} 8.11.0 via [elasticsearch#99212](https://github.com/elastic/elasticsearch/pull/99212).

**Workaround**

Use {{es}} version 8.11.0 or later when deploying Stack Config Policies.

:::

## 3.2.0 [elastic-cloud-kubernetes-320-known-issues]

:::{dropdown} Elastic Agent fails with "cipher: message authentication failed" on ECK 3.2.0 re-upgrade

Elastic Agent fails to start with "cipher: message authentication failed" after re-upgrading to ECK 3.2.0, the CONFIG_PATH for Elastic Agent in Fleet mode was changed to align with the STATE_PATH (tracking [Issue #8819](https://github.com/elastic/cloud-on-k8s/issues/8819)).

If you upgrade to 3.2.0, downgrade to a previous version (like 3.1.0), and then upgrade back to 3.2.0, the Elastic Agent Pods may fail to start. This occurs because the agent, using the new CONFIG_PATH, is unable to decrypt the existing state files encrypted with keys from the old path.

You will see errors in the agent logs similar to one of the following:

`Error: fail to read state store '/usr/share/elastic-agent/state/data/state.enc': failed migrating YAML store JSON store: could not parse YAML `
`fail to decode bytes: cipher: message authentication failed`

or

`Error: fail to read action store '/usr/share/elastic-agent/state/data/action_store.yml': yaml: input error: fail to decode bytes: cipher: message authentication failed`

For more information, check [PR #8856](https://github.com/elastic/cloud-on-k8s/pull/8856).

**Workaround**

To work around this issue, you must force the Agent to re-enroll. Add the `FLEET_FORCE=true` environment variable to your Agent's podTemplate specification. This will cause the agent to start fresh and re-enroll with Fleet.

This environment variable can be removed once the Agent has successfully started and re-enrolled.

```
apiVersion: agent.k8s.elastic.co/v1alpha1
kind: Agent
metadata:
  name: eck-agent # Your Agent resource name
spec:
  # ... other agent specs
  podTemplate:
    spec:
      containers:
        - name: agent
          env:
            - name: FLEET_FORCE
              value: "true"
```
:::

## 3.1.0 [elastic-cloud-kubernetes-310-known-issues]

There are no known issues in ECK 3.1

## 3.0.0 [elastic-cloud-kubernetes-300-known-issues]

:::{dropdown} Elastic Maps Server does not start on certain container runtimes
On May 19th 2025, it was discovered that the Elastic Maps Server container image in versions 7.17.28, 8.18.0, 8.18.1, 9.0.0 and 9.0.1 does not start on OpenShift Container Platform with the following error: `container create failed: open executable: Operation not permitted`.

For more information, check [Issue #8655](https://github.com/elastic/cloud-on-k8s/issues/8655).

**Workaround**

To workaround the issue override the container command for Elastic Maps Server: 

```
apiVersion: maps.k8s.elastic.co/v1alpha1
kind: ElasticMapsServer
metadata:
  name: ems-sample
spec:
  version: 9.0.0
  count: 1
  podTemplate:
    spec:
      containers:
        - name: maps
          command: [ /bin/sh, -c, "node app/index.js"]
```