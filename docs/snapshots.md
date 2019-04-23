# Automated snapshots example

Automated snapshots for Elasticsearch in Kubernetes can be easily achieved by:

1. Providing snapshot repository credentials in Elasticsearch keystore
2. Registering the repository with Elasticsearch API
3. Setting up a [CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) to perform the snapshots request

This example will guide you through an example using the [Google Cloud Storage Repository Plugin](https://www.elastic.co/guide/en/elasticsearch/plugins/master/repository-gcs.html).

For more information on Elasticsearch snapshots, see the official [documentation](https://www.elastic.co/guide/en/elasticsearch/reference/current/modules-snapshots.html).

## Provide GCS credentials to Elasticsearch keystore

Elasticsearch GCS repository plugin requires a JSON file containing service account credentials in Elasticsearch keystore (see [the documentation for more details](https://www.elastic.co/guide/en/elasticsearch/plugins/master/repository-gcs-usage.html)).

Using the operator, you can automatically inject secure settings into a cluster nodes by providing them through a secret in the Elasticsearch Spec.

First, create a file containing GCS credentials, that we'll name `es.file.gcs.client.default.credentials_file`:

```json
{
  "type": "service_account",
  "project_id": "your-project-id",
  "private_key_id": "...",
  "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
  "client_email": "service-account-for-your-repository@your-project-id.iam.gserviceaccount.com",
  "client_id": "...",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://accounts.google.com/o/oauth2/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/your-bucket@your-project-id.iam.gserviceaccount.com"
}
```

The `es.file` prefix indicates this file will be used as an Elasticsearch secure setting with the `file` type.

Create a Kubernetes secret from that file:
```bash
kubectl create secret generic gcs-credentials --from-file=es.file.gcs.client.default.credentials_file
```

Then, edit the `secureSettings` section of the Elasticsearch cluster resource:

```yaml
kind: Elasticsearch
spec:
    # ...
    # Inject secure settings into Elasticsearch nodes from a k8s secret reference
    secureSettings:
      secretName: "gcs-credentials"
```

And apply the modifications:

````bash
kubectl apply -f elasticsearch.yml
````

GCS credentials will be propagated into each node's keystore automatically.

## Register the repository in Elasticsearch

Following the [snapshot documentation](https://www.elastic.co/guide/en/elasticsearch/reference/current/modules-snapshots.html), create the GCS snapshot repository in Elasticsearch:

```
PUT _snapshot/my_gcs_repository
{
  "type": "gcs",
  "settings": {
    "bucket": "my_bucket",
    "client": "default"
  }
}
```

You can then perform a snapshot via a simple http request:

```
PUT /_snapshot/my_gcs_repository/test-snapshot
```

## Periodic snapshots with a CronJob

You can specify a simple CronJob to perform a snapshot every day. We simply perform an HTTP request against the appropriate endpoint, using a daily snapshot naming format. Elasticsearch credentials are mounted as a volume into the job's pod:

```yml
# snapshotter.yml
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: my-cluster-snapshotter
spec:
  schedule: "@daily"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: hello
            image: centos:7
            volumeMounts:
              - name: es-basic-auth
                mountPath: /mnt/elastic/es-basic-auth
            command:
            - /bin/bash
            args:
            - -c
            - 'curl -s -i -k -u "elastic:$(</mnt/elastic/es-basic-auth/elastic)" -XPUT "https://elasticsearch-sample-es:9200/_snapshot/my_gcs_repository/%3Csnapshot-%7Bnow%2Fd%7D%3E" | tee /dev/stderr | grep "200 OK"'
          restartPolicy: OnFailure
          volumes:
          - name: es-basic-auth
            secret:
              secretName: elasticsearch-sample-elastic-user
```

Then apply it to the kubernetes cluster:

```
kubectl apply -f snapshotter.yml
```

For more details on Kubernetes CronJobs, please visit the [official documentation](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/).
