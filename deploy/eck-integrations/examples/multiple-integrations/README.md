# Example: Multiple Integrations

In this example we install the built-in `kubernetes` integration and a `nginx` custom integration based on the same cluster-wide agent preset that the `kubernetes` integration utilises. Also, we enable the hints-based autodiscovery supported in the `kubernetes` integration.

## Prerequisites:
1. Installed eck-operator Helm chart
   ```console
   helm repo add elastic https://helm.elastic.co && helm repo update
   helm install elastic-operator elastic/eck-operator --create-namespace
   ```
2. For **non** eck-managed Elasticsearch clusters you need a k8s secret that contains the connection details to it such as:
    ```console
    kubectl create secret generic es-ref-secret \
        --from-literal=username=... \
        --from-literal=password=... \
        --from-literal=url=...
    ```
    Note: specifying an `api-key`, instead of a `username` and `password`, is not supported at the moment but there is an already open PR to add support for it.

3. `kubernetes` integration assets are installed through Kibana
4. `redis` integration assets are installed through Kibana
5. `nginx` integration assets are installed through Kibana

## Run:
1. For **non** eck-managed Elasticsearch clusters
    ```console
    helm install eck-integrations ../../ \
         -f ./agent-kubernetes.yaml \
         -f ./agent-nginx.yaml \
         --set elasticsearchRefs.default.secretName=es-ref-secret 
    ```
    For eck-managed Elasticsearch clusters
    ```console
    helm install eck-integrations ../../ \
         -f ./agent-kubernetes.yaml \
         -f ./agent-nginx.yaml \
         --set elasticsearchRefs.default.name=eck-es-name 
    ```

2. Install a redis pod with the appropriate annotations
    ```console
   kubectl apply -f ./redis.yaml
    ```
3. Install the nginx deployment
    ```console
   kubectl apply -f ./nginx.yaml
    ```
   
## Validate:

1. The Kibana dashboards for the `kubernetes`, `redis` and `nginx` integration should start showing data.