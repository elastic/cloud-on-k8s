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

## 3.0.0 [elastic-cloud-kubernetes-300-known-issues]

:::{dropdown} Elastic Maps Server 9.0.0 does not start on certain container runtimes
On May 19th 2025, it was discovered that the Elastic Maps Server container image in version 9.0.0 does not start on OpenShift Container Platform with the following error: `container create failed: open executable: Operation not permitted`.

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