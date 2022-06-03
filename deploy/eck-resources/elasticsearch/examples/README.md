# Examples

This directory contains helm charts with custom example configurations. 

# Usage

### Advanced Elasticsearch node scheduling
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-advanced-node-scheduling.html

**affinity-and-selector.yaml** 

This chart will restrict the scheduling to a particular set of Kubernetes nodes based on labels, use a `NodeSelector`. The example schedules Elasticsearch Pods on Kubernetes nodes tagged with `environment: production`. To install it:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/affinity-and-selector.yaml`

### Autoscaling
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-autoscaling.html

**autoscaling.yaml** 

This chart define autoscaling policy that applies to one or more NodeSets. To install

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/autoscaling.yaml`

### Volume Claim Template
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-volume-claim-templates.html

**custom-persistent-volume-claim.yaml**

Define your own volume claim template with the desired storage capacity and (optionally) the Kubernetes storage class to associate with the persistent volume. To install it:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/custom-persistent-volume-claim.yaml`

### Hot-Warm-Cold topologies
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-advanced-node-scheduling.html#k8s-hot-warm-topologies

**hot-warm-cold.yaml**

By combining Elasticsearch shard allocation awareness with Kubernetes node affinity, you can set up an Elasticsearch cluster with hot-warm topology:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/hot-warm-cold.yaml`

### Mounting Volumes

**mounting-volumes.yaml**

If you wish mount Volumes into Elasticsearch resource, you can have a look at this chart, in this example, we are mounting a volume which store a `metadata.xml` file. To install:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/mounting-volumes.yaml`

### Pod Disruption Budget
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-pod-disruption-budget.html

**pod-disruption-budget.yaml**

This chart will limit the disruption to your application when its pods need to be rescheduled for some reason such as upgrades or routine maintenance work on the Kubernetes nodes. To install

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/pod-disruption-budget.yaml`

### Pod PreStop hook
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-prestop.html 

**pod-prestop-hook.yaml**

This chart will minimize unavailability when pods are terminated. To install it:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/pod-prestop-hook.yaml`

### Readiness probe
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-readiness.html

**readiness-probe.yaml**

By default, the readiness probe checks that the Pod responds to HTTP requests within a timeout of three seconds. This is acceptable in most cases. But if for your use case you need to adjust it, you can install this chart by running:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/readiness-probe.yaml`

### Remote Cluster
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-remote-clusters.html

**remote-cluster.yaml**

This chart will enables you to establish uni-directional connections to a remote cluster. To install it:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/remote-cluster.yaml`

### SAML
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/2.2/k8s-saml-authentication.html

**saml-settings.yaml**

This chart is an example on how to setup SAML with ECK. To install it:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/saml-settings.yaml`

### Secure Setting
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-es-secure-settings.html

**secure-settings.yaml**

To add reference of existing Secrets into the Elasticsearch resource, you can run:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/secure-settings.yaml`

### Security Context
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-security-context.html

**security-context.yaml**

Defines privilege and access control settings for a Pod. To install it:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/security-context.yaml`

### Topology spread constrains and availability zone awareness
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-advanced-node-scheduling.html#k8s-availability-zone-awareness

**spread-constrains-allocation-awareness.yaml**

This chart will deal with logical failure domains, available in a running Pod. Combined with Elasticsearch shard allocation awareness and Kubernetes topology spread constraints, you can create an availability zone-aware Elasticsearch cluster. To install it:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/spread-constrains-allocation-awareness.yaml`

### Stack Monitoring
For more info, check https://www.elastic.co/guide/en/cloud-on-k8s/2.2/k8s-stack-monitoring.html 

**stack-monitoring.yaml**

To enable stack monitoring, simply reference the monitoring Elasticsearch cluster in the spec.monitoring section of their specification.

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/stack-monitoring.yaml`

### Transport Settings
For more info check, https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-transport-settings.html

**transport-settings.yaml**

In the spec.transport.service. section, you can change the Kubernetes service used to expose the Elasticsearch transport module:

`helm install elasticsearch deploy/eck-resources/elasticsearch/ --values deploy/eck-resources/elasticsearch/examples/transport-settings.yaml`

