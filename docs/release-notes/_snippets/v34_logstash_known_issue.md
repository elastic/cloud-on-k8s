::::{dropdown} Logstash pods rejected by OpenShift SCC when using a non-default security context constraint
In ECK 3.4.x, the Logstash controller always injects `seccompProfile: RuntimeDefault` and `fsGroup: 1000` into the pod security context, ignoring the `--set-default-security-context=auto-detect` flag. On OpenShift, this flag should suppress the injection — as it does for {{es}}, {{product.kibana}}, and APM Server. As a result, clusters using a non-default SCC such as `anyuid`, which forbids explicit seccomp settings, reject Logstash pods at admission after upgrading.

For more information, check this [Issue #9550](https://github.com/elastic/cloud-on-k8s/issues/9550).

**Workaround**

This workaround applies **only** to namespaces using an `anyuid`-style SCC (`RunAsAny` fsGroup, seccomp forbidden) that do **not** enforce [restricted Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/). Check your namespace's labels first:

```bash
kubectl get namespace <your-namespace> -o jsonpath='{.metadata.labels}'

# or on OpenShift:
oc get namespace <your-namespace> -o jsonpath='{.metadata.labels}'
```

If the output contains `pod-security.kubernetes.io/enforce: restricted`, do **not** apply this workaround: restricted PSA requires `seccompProfile` and expects `fsGroup` within the namespace's allocated `supplemental-groups` range, not a fixed `1000`.

Otherwise, set the security context on the Logstash resource so ECK skips its own injection:

```yaml
spec:
  podTemplate:
    spec:
      securityContext:
        fsGroup: 1000
        # seccompProfile intentionally omitted — not permitted by anyuid SCC
```

:::{important}
Apply this override to existing Logstash resources before upgrading to ECK 3.4. If you are already running ECK 3.4 and the pods are in the SCC rejection loop, delete the Logstash resource and re-create it with the override described in this workaround.
:::

You can remove this override after upgrading to ECK 3.5.0 or later (once available), which fixes [#9550](https://github.com/elastic/cloud-on-k8s/issues/9550).
::::