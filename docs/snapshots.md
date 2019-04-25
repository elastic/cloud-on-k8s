# How to create automated snapshots

To create automated snapshots for Elasticsearch in Kubernetes you have to:

1. Provide snapshot repository credentials in Elasticsearch keystore.
2. Register the snapshot repository with Elasticsearch API.
3. Set up a [CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) to perform the snapshots request.

The example in this section uses the [Google Cloud Storage Repository Plugin](https://www.elastic.co/guide/en/elasticsearch/plugins/master/repository-gcs.html).

For more information on Elasticsearch snapshots, see [Snapshot and Restore](https://www.elastic.co/guide/en/elasticsearch/reference/current/modules-snapshots.html).

## Provide GCS credentials to Elasticsearch keystore

Elasticsearch GCS repository plugin requires a JSON file that contains service account credentials in Elasticsearch keystore. For more details, see [Google Cloud Storage Repository Plugin](https://www.elastic.co/guide/en/elasticsearch/plugins/master/repository-gcs-usage.html)).

Using the operator, you can automatically inject secure settings into a cluster node by providing them through a secret in the Elasticsearch Spec.

1. Create a file containing GCS credentials. For this exercise, name it `es.file.gcs.client.default.credentials_file`:

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

The `es.file` prefix indicates that this file is used as an Elasticsearch secure setting with the `file` type.

2. Create a Kubernetes secret from that file:
```bash
kubectl create secret generic gcs-credentials --from-file=es.file.gcs.client.default.credentials_file
```

3. Edit the `secureSettings` section of the Elasticsearch resource:

```yaml
kind: Elasticsearch
spec:
    # ...
    # Inject secure settings into Elasticsearch nodes from a k8s secret reference
    secureSettings:
      secretName: "gcs-credentials"
```

4. Apply the modifications:

````bash
kubectl apply -f elasticsearch.yml
````

GCS credentials are automatically propagated into each node keystore. It can take up to a few minutes, depending on the number of secrets in the keystore. You don't have to restart the nodes. 

## Register the repository in Elasticsearch

1. Create the GCS snapshot repository in Elasticsearch according to the procedure described in [Snapshot and Restore](https://www.elastic.co/guide/en/elasticsearch/reference/current/modules-snapshots.html):

```
PUT /_snapshot/my_gcs_repository
{
  "type": "gcs",
  "settings": {
    "bucket": "my_bucket",
    "client": "default"
  }
}
```

2. Perform a snapshot with the following HTTP request:

```
PUT /_snapshot/my_gcs_repository/test-snapshot
```

## Periodic snapshots with a CronJob

You can specify a simple CronJob to perform a snapshot every day.

1. Make an HTTP request against the appropriate endpoint, using a daily snapshot naming format. Elasticsearch credentials are mounted as a volume into the job's pod:

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

2. Apply it to the Kubernetes cluster:

```
kubectl apply -f snapshotter.yml
```

For more details see [Kubernetes CronJobs](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/).
