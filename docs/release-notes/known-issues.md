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

## 3.4.1 [elastic-cloud-kubernetes-341-known-issues]

:::{dropdown} Logstash pods rejected by OpenShift SCC when using a non-default security context constraint
In ECK 3.4.x, the Logstash controller unconditionally injects `seccompProfile: RuntimeDefault` and `fsGroup: 1000` into the pod security context, ignoring the `--set-default-security-context=auto-detect` operator flag. On OpenShift, this flag should suppress the injection, as it does for all other ECK-managed workloads ({{es}}, {{product.kibana}}, APM Server). Clusters using a non-default SCC such as `anyuid` — which does not permit explicit seccomp configuration — will see Logstash pods rejected at admission after upgrading.

For more information, check this [Issue #9550](https://github.com/elastic/cloud-on-k8s/issues/9550).

**Workaround**

This workaround applies only to namespaces governed by an `anyuid`-style SCC (`RunAsAny` fsGroup, seccomp forbidden) that are **not** also enforcing [restricted Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/).

To check whether your namespace enforces restricted PSA, run:

```bash
kubectl get namespace <your-namespace> -o jsonpath='{.metadata.labels}'

# or on OpenShift:
oc get namespace <your-namespace> -o jsonpath='{.metadata.labels}'
```

If you see `pod-security.kubernetes.io/enforce: restricted`, do **not** use this workaround — in restricted-PSA namespaces, `seccompProfile` is required and must not be omitted, and `fsGroup` must fall within the namespace's allocated `supplemental-groups` range rather than a fixed value like `1000`.

For namespaces using `anyuid`-style SCC without restricted PSA enforcement, override the Logstash pod security context as follows to prevent ECK from injecting its defaults:

```yaml
spec:
  podTemplate:
    spec:
      securityContext:
        fsGroup: 1000
        # seccompProfile intentionally omitted — not permitted by anyuid SCC
```

:::{important}
The `podTemplate.spec.securityContext` override must be applied to existing Logstash CRs before upgrading to ECK 3.4.0. If the upgrade happens first and the Logstash pods enter the SCC rejection loop, updating the CR afterwards won't help. For the latter, the only recovery is to delete and re-create the Logstash CR with the security context as described in place.
:::

This override can be removed once you upgrade to ECK 3.5.0 or later when available, which includes the fix for [#9550](https://github.com/elastic/cloud-on-k8s/issues/9550).
:::

## 3.4.0 [elastic-cloud-kubernetes-340-known-issues]

:::{dropdown} Logstash pods rejected by OpenShift SCC when using a non-default security context constraint
In ECK 3.4.x, the Logstash controller unconditionally injects `seccompProfile: RuntimeDefault` and `fsGroup: 1000` into the pod security context, ignoring the `--set-default-security-context=auto-detect` operator flag. On OpenShift, this flag should suppress the injection, as it does for all other ECK-managed workloads ({{es}}, {{product.kibana}}, APM Server). Clusters using a non-default SCC such as `anyuid` — which does not permit explicit seccomp configuration — will see Logstash pods rejected at admission after upgrading.

For more information, check this [Issue #9550](https://github.com/elastic/cloud-on-k8s/issues/9550).

**Workaround**

This workaround applies only to namespaces governed by an `anyuid`-style SCC (`RunAsAny` fsGroup, seccomp forbidden) that are **not** also enforcing [restricted Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/).

To check whether your namespace enforces restricted PSA, run:

```bash
kubectl get namespace <your-namespace> -o jsonpath='{.metadata.labels}'

# or on OpenShift:
oc get namespace <your-namespace> -o jsonpath='{.metadata.labels}'
```

If you see `pod-security.kubernetes.io/enforce: restricted`, do **not** use this workaround — in restricted-PSA namespaces, `seccompProfile` is required and must not be omitted, and `fsGroup` must fall within the namespace's allocated `supplemental-groups` range rather than a fixed value like `1000`.

For namespaces using `anyuid`-style SCC without restricted PSA enforcement, override the Logstash pod security context as follows to prevent ECK from injecting its defaults:

```yaml
spec:
  podTemplate:
    spec:
      securityContext:
        fsGroup: 1000
        # seccompProfile intentionally omitted — not permitted by anyuid SCC
```

:::{important}
The `podTemplate.spec.securityContext` override must be applied to existing Logstash CRs before upgrading to ECK 3.4.0. If the upgrade happens first and the Logstash pods enter the SCC rejection loop, updating the CR afterwards won't help. For the latter, the only recovery is to delete and re-create the Logstash CR with the security context as described in place.
:::

This override can be removed once you upgrade to ECK 3.5.0 or later when available, which includes the fix for [#9550](https://github.com/elastic/cloud-on-k8s/issues/9550).
:::

## 3.3.2 [elastic-cloud-kubernetes-332-known-issues]

:::{dropdown} Certificate mismatch causing {{es}} and {{product.kibana}} connection failure during ECK operator upgrade

During or after upgrading the ECK operator to 3.3.0–3.3.2, HTTP and transport certificate issues can arise due to mismatched Authority Key Identifier (AKI) and Subject Key Identifier (SKI) values. This results in SSL handshake failures, preventing ES nodes from joining the cluster and Kibana, Fleet, and other HTTP clients from connecting to it.

For more information, check [PR #9197](https://github.com/elastic/cloud-on-k8s/pull/9197).

**Workaround**

Delete the transport certificate secret (`<cluster>-es-<nodeset>-es-transport-certs`) and the HTTP certificate secret (`<cluster>-es-http-certs-internal`) to force ECK to regenerate all certificates. For more details, refer to the [KB article](https://ela.st/eck-operator-upgrade-cert-issue). Alternatively, upgrade to ECK 3.4.0 or later once available.

:::

## 3.3.1 [elastic-cloud-kubernetes-331-known-issues]

:::{dropdown} Certificate mismatch causing {{es}} and {{product.kibana}} connection failure during ECK operator upgrade

During or after upgrading the ECK operator to 3.3.0–3.3.2, HTTP and transport certificate issues can arise due to mismatched Authority Key Identifier (AKI) and Subject Key Identifier (SKI) values. This results in SSL handshake failures, preventing ES nodes from joining the cluster and Kibana, Fleet, and other HTTP clients from connecting to it.

For more information, check [PR #9197](https://github.com/elastic/cloud-on-k8s/pull/9197).

**Workaround**

Delete the transport certificate secret (`<cluster>-es-<nodeset>-es-transport-certs`) and the HTTP certificate secret (`<cluster>-es-http-certs-internal`) to force ECK to regenerate all certificates. For more details, refer to the [KB article](https://ela.st/eck-operator-upgrade-cert-issue). Alternatively, upgrade to ECK 3.4.0 or later once available.

:::

:::{dropdown} FIPS operator images use standard Go cryptography instead of BoringCrypto
Due to a build configuration issue, ECK operator FIPS images published between versions 2.9.0 and 3.3.1 use the standard Go cryptography library instead of BoringCrypto. Standard Go does not use FIPS 140-2/3 validated cryptographic libraries. Upgrade to version 3.3.2 or later to get images built using FIPS 140-2/3 validated cryptographic libraries.

For more information, check [PR #9263](https://github.com/elastic/cloud-on-k8s/pull/9263).

**Workaround**

Upgrade to ECK 3.3.2 or later.

:::

:::{dropdown} AutoOps - Enterprise license expiring may cause policy phase to be set to `Invalid` prior to 9.2.4

In clusters running AutoOps Agent versions earlier than 9.2.4, an Enterprise license expiring may cause the policy phase to be set to `Invalid`. In this state, the AutoOps Agent stops sending data to AutoOps because the policy no longer passes validation on the controller.

**Workaround**

Renew or restore the Enterprise license so that the AutoOps policy can be validated again. To prevent this issue in the future, upgrade the AutoOps Agent to version 9.2.4 or later.

:::

## 3.3.0 [elastic-cloud-kubernetes-330-known-issues]

:::{dropdown} Certificate mismatch causing {{es}} and {{product.kibana}} connection failure during ECK operator upgrade

During or after upgrading the ECK operator to 3.3.0–3.3.2, HTTP and transport certificate issues can arise due to mismatched Authority Key Identifier (AKI) and Subject Key Identifier (SKI) values. This results in SSL handshake failures, preventing ES nodes from joining the cluster and Kibana, Fleet, and other HTTP clients from connecting to it.

For more information, check [PR #9197](https://github.com/elastic/cloud-on-k8s/pull/9197).

**Workaround**

Delete the transport certificate secret (`<cluster>-es-<nodeset>-es-transport-certs`) and the HTTP certificate secret (`<cluster>-es-http-certs-internal`) to force ECK to regenerate all certificates. For more details, refer to the [KB article](https://ela.st/eck-operator-upgrade-cert-issue). Alternatively, upgrade to ECK 3.4.0 or later once available.

:::

:::{dropdown} FIPS operator images use standard Go cryptography instead of BoringCrypto
Due to a build configuration issue, ECK operator FIPS images published between versions 2.9.0 and 3.3.1 use the standard Go cryptography library instead of BoringCrypto. Standard Go does not use FIPS 140-2/3 validated cryptographic libraries. Upgrade to version 3.3.2 or later to get images built using FIPS 140-2/3 validated cryptographic libraries.

For more information, check [PR #9263](https://github.com/elastic/cloud-on-k8s/pull/9263).

**Workaround**

Upgrade to ECK 3.3.2 or later.

:::

:::{dropdown} Stack Config Policies - File settings may not reload correctly on {{es}} versions prior to 8.11.0

{{es}} versions prior to 8.11.0 contain a bug where updates to file-based cluster settings may not be reloaded correctly when the file changes. This is caused by an {{es}} issue where new keys in file-settings are incorrectly deleted during file monitoring and reload operations.

When using Stack Config Policies with affected {{es}} versions, updated settings may not appear correctly when querying the `_cluster/settings` endpoint, even though the Stack Config Policy has been updated. Making an additional manual update to the Stack Config Policy may trigger the settings to reload correctly.

This issue was fixed in {{es}} 8.11.0 via [elasticsearch#99212](https://github.com/elastic/elasticsearch/pull/99212).

**Workaround**

Use {{es}} version 8.11.0 or later when deploying Stack Config Policies.

:::

## 3.2.0 [elastic-cloud-kubernetes-320-known-issues]

:::{dropdown} FIPS operator images use standard Go cryptography instead of BoringCrypto
Due to a build configuration issue, ECK operator FIPS images published between versions 2.9.0 and 3.3.1 use the standard Go cryptography library instead of BoringCrypto. Standard Go does not use FIPS 140-2/3 validated cryptographic libraries. Upgrade to version 3.3.2 or later to get images built using FIPS 140-2/3 validated cryptographic libraries.

For more information, check [PR #9263](https://github.com/elastic/cloud-on-k8s/pull/9263).

**Workaround**

Upgrade to ECK 3.3.2 or later.

:::

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

:::{dropdown} FIPS operator images use standard Go cryptography instead of BoringCrypto
Due to a build configuration issue, ECK operator FIPS images published between versions 2.9.0 and 3.3.1 use the standard Go cryptography library instead of BoringCrypto. Standard Go does not use FIPS 140-2/3 validated cryptographic libraries. Upgrade to version 3.3.2 or later to get images built using FIPS 140-2/3 validated cryptographic libraries.

For more information, check [PR #9263](https://github.com/elastic/cloud-on-k8s/pull/9263).

**Workaround**

Upgrade to ECK 3.3.2 or later.

:::

## 3.0.0 [elastic-cloud-kubernetes-300-known-issues]

:::{dropdown} FIPS operator images use standard Go cryptography instead of BoringCrypto
Due to a build configuration issue, ECK operator FIPS images published between versions 2.9.0 and 3.3.1 use the standard Go cryptography library instead of BoringCrypto. Standard Go does not use FIPS 140-2/3 validated cryptographic libraries. Upgrade to version 3.3.2 or later to get images built using FIPS 140-2/3 validated cryptographic libraries.

For more information, check [PR #9263](https://github.com/elastic/cloud-on-k8s/pull/9263).

**Workaround**

Upgrade to ECK 3.3.2 or later.

:::

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