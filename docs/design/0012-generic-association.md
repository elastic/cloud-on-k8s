# Generic Application Association

## Definitions

  - In the rest of this document an **Elastic stack application** is an application managed by ECK. For instance, it could be Elasticsearch, the APM Server, Kibana or EnterpriseSearch.

  - An **associated application** describes any user application, running in a `Pod`, in a namespace managed by ECK.

## Context and Problem Statement

Establishing a trusted connection to an Elastic stack application requires gathering and keeping in sync multiple moving parts. Each Elastic stack application comes with its certificates and different kind of credentials. A credential could be a token, in the case of the APM Server, or a set of user/password. Manually copying certificates, credentials and tokens [can be error prone](https://github.com/elastic/cloud-on-k8s/issues/3032). Moreover, if they are not kept in sync the association could be broken.

This document describes how ECK could make available to an associated application the information needed to establish a trusted association with an Elastic stack application.

It is worth to note that the problem could be divided into 2 different sub-problems:

  - A first problem is to let a user specify the association information required by one or several of its applications, and make it available in the application namespace.
  - A second problem is how ECK can automatically inject this information, either by managing a part of the `Spec` of a top-level deployment resource _(`Deployment`, `DaemonSet`, `StatefulSet`, `Job`)_ or by mutating the `Pod` spec before its creation.

## Decision Drivers

### Security

A user should not be able to create an association without the agreement of the Elastic stack application owner. It should not be possible to create an Elastic user with an arbitrary role if a user does not have the ownership of the Elastic stack application. Similarly, it should not be possible for an Elastic stack application owner to modify the spec of a `Pod` without the agreement of the associated application owner.

### Pod spec management

Ideally the solution would be able to update the `Pod` specification, injecting the association data automatically and making it available through some "well-known" environment variables or files in the `Pod`. This would allow the user to delegate the binding of the association variables to ECK. This behavior is similar to what is described in the issue [Facilitate ECK integration with MutatingAdmissionWebhook](https://github.com/elastic/cloud-on-k8s/issues/2418).

## Artifacts involved in an association

### Certificates

An application must be able to validate the certificates exposed by the Elastic stack applications. This means:
    - Copying the certificates of the Elastic stack applications into the associated application namespace.
    - Mounting the certificates in a Pod so they can be read and consume.
    - Handling certificates rotation.

### APMServer token

In the context of the APM Server ECK would need to reconcile the APM token.

### User management

In the context of the problem stated here it could be worth to think about user provisioning. User may want to specify a required `Role` for an associated application and let ECK automatically generates a user and the credentials. 

## Detailed Problem Statement

### Specify how an application is associated with an Elastic Stack Application

The first problem is to make the association information available to the associated application. Each associated application may require a different set of information. For instance some of them may want to establish a connection to Elasticsearch while others may require the APM token and the APM Server certs to upload some metrics.

Two kind of artifacts can be used to describe an association _(not exclusive)_:
  1. A **new CRD** which would allow to specify the association between the associated application and some Elastic stack applications 
  2. One or several ***annotations***, set on top-level objects resources that manage the associated application Pods. 

### Inject the required artifacts into a Pod

For the record there a 3 ways to inject some data into an application:

  - [Environment variables](https://kubernetes.io/docs/tasks/inject-data-application/define-environment-variable-container/), examples of good candidates to be injected with environment variables are:
    - Elastic stack application urls
    - Username and Password
    - APM Server token

  - [Secrets](https://kubernetes.io/docs/tasks/inject-data-application/distribute-credentials-secure/) _(not mutually exclusive with environment variables)_, they naturally address the case of the certificates, but they can also contain the variables exposed as environment variables.
 
  - [PodPreset](https://kubernetes.io/docs/tasks/inject-data-application/podpreset/), it is an interesting solution, designed to solve the problem described here. Unfortunately this is a recent addition in the Kubernetes API and it's not widely available yet.

## Considered Options

Since [PodPreset](https://kubernetes.io/docs/tasks/inject-data-application/podpreset/) is not available yet we are only considering here a mixed usage of a new CRD to specify the association information to be exposed plus some annotations on the top-level resources to let user specify how ECK should take over some part of the `Pod` spec.

### Combine a new CRD with an annotation

In this option a new CRD is introduced. It allows an Elastic stack application owner to specify for some third party applications which association data is required.

#### Association specification as a new CRD

```yaml
apiVersion: association.k8s.elastic.co/v1alpha1
kind: Association
metadata:
  name: monitor
  namespace: monitor-ns # the Elastic stack applicaitons must live in this namespace
spec:

  ## targetNamespaces are the namespaces where the association information are copied and reconciled
  targetNamespaces:
    - ns1

  ## These are the Elastic applications.
  ## The namespace is automatically inherited from this custom resource.
  elasticsearchName: monitor
  kibanaName: monitor
  # apmServerName: monitor
  
  ## User field lets the user specify a user or a Secret which contains a user and a password.
  ## 2 versions are maintained:
  ## 1. One in the target namespaces, as a part of the association information, it contains the username and the clear-text password
  ## 2. An other one in the Elasticsearch namespace, it contains the hashed password, ready to be used by Elasticsearch
  ## If empty then no user is generated, only certificates are copied as part of this association
  user:
    ## Role must already exists.
    ## See https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-users-and-roles.html#k8s_creating_custom_roles
    role: superuser
    ## Optional and proposed here as a later improvement: user may specify a spec of the password.
    ## Some sane defaults would be: 64 chars, mixed cases, no special characters and no ttl.
    # passwordSpec:
    #   length: 64
    #   mixedCases: true
    #   symbols: false
    #   ttl: "24h"
    # Or just reference a password
    # secretName: secret-with-user-and-password
```

#### Automatic injection of the association data

The new CRD allows creating and maintaining a copy of the information needed to establish an association. But, it might be worth to propose a mechanism to inject the variables automatically. Optionally, it would be possible to add some annotations to automatically inject the required data into the `Pods` _(in the targeted namespaces)_ :

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-application
  annotations:
    association.k8s.elastic.co/name: "monitor-ns/monitor" # namespaced name of the association CR
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: container1
        image: ...
      - name: container2
        image: ...
```

## Making association variables available

### The Association Secret

The proposal here is to define the content of the association secret. This `Secret` is managed by a generic association controller. According to what the user specify in the generic association custom resource this `Secret` may be filled with the following items:

 * `username`: username of the user as it exists in Elasticsearch
 * `password`: clear text version of the password, the hashed version is maintained, reconciled and is made available by the generic association controller to the Elasticsearch controller.
 * `elasticsearch-host`: hostname of the Elasticsearch cluster
 * `elasticsearch-port`: port of the Elasticsearch cluster
 * `elasticsearch-ca`: Certificate of the Elasticsearch CA
 * `kibana-host`: hostname to be used to access Kibana
 * `kibana-port`: port of Kibana
 * `kibana-ca`: certificate of the Kibana CA
 * `apmserver-url`: url to be used to access the APM Server
 * `apmserver-token`: token to be used to access the APM Server
 * `apmserver-ca`: certificate of the APM Server CA

## Injecting association data automatically

#### Option #1: Updating the Pod's parent resource

Two different approaches might be used in order to populate the environment variables and bind the `Secret` automatically in a `Pod`.

In the first one ECK would insert some environment variables in the `Pod`'s parent resource:

Here is the `Deployment` as it would be created by a user:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-application
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: container1
        image: ...
      - name: container2
        image: ...
```

Here is the `Deployment` as it would be updated by ECK:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-application
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      annotations:
        elastic.config.checksum: 585696 # allows an automatic rotation
      labels:
        app: test
    spec:
      containers:
      - name: container1
        image: ...
        env:
          - name: ELASTICSEARCH_USERNAME
            valueFrom:
              secretKeyRef:
                name: <automatic-generated-name>
                key: username
        volumeMounts:
        - name: elastic-volume
          mountPath: /mnt/elastic
      - name: container2
        image: ...
        volumeMounts:
        - name: elastic-volume
          mountPath: /mnt/elastic
      volumes:
      - name: elastic-volume
        secret:
          secretName: <automatic-generated-name>
```

  * Pros:
    * The configuration checksum can be injected in the pod template to allow an automatic restart of the `Pods` 
    * ECK can inject and manage some "well-known" environment variables, some good examples are the ones used in the context of the APM Server (`ELASTIC_APM_SERVER_URLS`, `ELASTIC_APM_SECRET_TOKEN`...) 
  * Cons:
    * It could be considered as "intrusive"
    * It could be a lot of resources to watch

#### Option #2: Using a MutatingAdmissionWebhook

  * Pros:
    * Less intrusive than updating the parent resource
  * Cons:
    * Cannot handle the lifecycle of the Secret content. The associated application must detect than something has changed in the `Secret`. In case of an environment variable the application must handle a `Pod` restart itself.
    * Yet another webhook

#### Environment variables

The following section describes the names of some environment variables which, for some of them, are already considered as some "well-known" environment variables.

##### Elasticsearch related variables

  * `ELASTICSEARCH_URL`: Reference to the `Secret` key `apmserver-url` 
  * `ELASTICSEARCH_HOST`: Reference to the `Secret` key `elasticsearch-host`
  * `ELASTICSEARCH_PORT`: Reference to the `Secret` key `elasticsearch-port` 
  * `ELASTICSEARCH_USERNAME`: Reference to the `Secret` key `username` 
  * `ELASTICSEARCH_PASSWORD`: Reference to the `Secret` key `password` 
  * `ELASTICSEARCH_CA`: Reference to the `Secret` key `elasticsearch-ca` 

##### APM related variables

  * `ELASTIC_APM_SERVER_URLS` and `ELASTIC_APM_SERVER_URL`: Reference to the `Secret` key `apmserver-url` This variable is compatible with at least the [Java Agent](https://github.com/elastic/apm-agent-java/blob/master/docs/configuration.asciidoc#server_urls), the [Go Agent](https://github.com/elastic/apm-agent-go/blob/95e54b2e24ea813e0d059e56dfe6c740681564cb/transport/http.go#L57), 
  * `ELASTIC_APM_SECRET_TOKEN`: Reference to the `Secret` key `apmserver-token`
  * `ELASTIC_APM_SERVER_CERT`: path to `APMServer` CA certificate


### Related works

 - [k8s/k8s - Introduce ClusterSecret for (among others) wildcard certificates](https://github.com/kubernetes/kubernetes/issues/70147)
 - [jetstack/cert-manager - Add advice on how to sync a secret between namespaces to documentation
](https://github.com/jetstack/cert-manager/issues/494)
 - [Enabling mTLS of Pods using the cert-manager CSI Driver](https://cert-manager.io/docs/usage/csi/)