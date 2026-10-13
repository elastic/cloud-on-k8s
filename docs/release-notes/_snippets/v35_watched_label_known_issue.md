::::{dropdown} Internal `eck.k8s.elastic.co/watched` label leaks into {{es}} and {{product.kibana}} pod templates

In ECK 3.5.0, the operator-internal `eck.k8s.elastic.co/watched: "true"` label, intended only for the Secrets, Services, and ConfigMaps that the operator manages, also leaks into the {{es}} StatefulSet and pod template labels and into the {{product.kibana}} Deployment and pod template labels. The leaked label is harmless, but adding it changes the pod specs, which restarts all {{es}} and {{product.kibana}} pods during the upgrade to 3.5.0.

The issue is fixed in ECK 3.6.0. Removing the label changes the pod specs again, so upgrading to 3.6.0 from 3.5.0 restarts the {{es}} and {{product.kibana}} pods one more time.

For more information, check [Issue #9786](https://github.com/elastic/cloud-on-k8s/issues/9786).

**Workaround**

No workaround is needed. The leaked label does not affect how the operator or the workloads function.

::::
