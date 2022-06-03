# ECK Elasticsearch resource Helm Charts Usage

This Helm chart is a lightweight way to configure Elasticsearch resources managed by ECK Operator.

## Usage

- This repo includes a number of examples configurations which can be used as a reference. They are also used in the automated testing of this chart.
- This chart deploy an Elasticsearch resource using the [quickstart example](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-deploy-elasticsearch.html)

## Installing

- Make sure you have the ECK Operator up and running
- Clone this repo
  `git clone git@github.com:elastic/cloud-on-k8s.git`
- Install the Elasticsearch chart by running:
    `helm install elasticsearch deploy/eck-resources/elasticsearch/`

This will deploy 1 Elasticsearch instance containing all the roles. 

## Configuration

| Parameter                          | Description                                                                                                                                                                                                                                                                                                       | Default                                          |
|------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------|
| `version`          | The version will define the Elasticsearch version that will be deployed | 8.2.0
| `esName`                     | This will be used as the Elasticsearch resource name | quickstart
| `labels` | Configurable labels applied to all Elasticsearch pods | {}
| `annotations` | Configurable annotations applied to all Elasticsearch pods | {}
| `monitoring` | Configure the stack monitoring, for more info check this [doc page](https://www.elastic.co/guide/en/cloud-on-k8s/master/k8s-stack-monitoring.html) | {}
| `transport` | This setting deal with how you want to expose Elasticsearch services, check the [docs](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-transport-settings.html) | {}
| `http` |  This settings deals with the http session, check this [doc page](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-kibana-http-configuration.html) | {}
| `secureSettings` | Configure secure settings with Kubernetes Secrets, see the [docs](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-es-secure-settings.html) | {}
| `updateStrategy` | This setting will limit the number of simultaneous changes, more info [here](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-update-strategy.html) | {}
| `esSpec` | This setting define the main `nodeSet` block |
