# Example: Kubernetes Integration only for container logs

In this example we install the built-in `kubernetes` integration and set it to extract only container logs. 

## Prerequisites:
1. Installed eck-operator helm chart
   ```console
   helm repo add elastic https://helm.elastic.co && helm repo update
   helm install elastic-operator elastic/eck-operator --create-namespace
   ```
2. For **non** eck-managed ElasticSearch clusters you need a k8s secret that contains the connection details to it such as:
    ```console
    kubectl create secret generic es-ref-secret \
        --from-literal=username=... \
        --from-literal=password=... \
        --from-literal=url=...
    ```
    Note: specifying an `api-key`, instead of a `username` and `password`, is not supported at the moment but there is an already open PR to add support for it.

3. `kubernetes` integration Assets are installed through Kibana

## Run:
1. For **non** eck-managed ElasticSearch clusters
    ```console
    helm install eck-integrations ../../ \
         -f ./agent-kubernetes.yaml \
         --set elasticsearchRefs.default.secretName=es-ref-secret 
    ```
    For eck-managed ElasticSearch clusters
    ```console
    helm install eck-integrations ../../ \
         -f ./agent-kubernetes.yaml \
         --set elasticsearchRefs.default.name=eck-es-name 
    ```

## Validate:

1. Container logs should appear in Kibana at Observability=>Logs=>Stream.