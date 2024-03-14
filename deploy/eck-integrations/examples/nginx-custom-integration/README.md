# Example: Nginx Custom Integration

In this example we define a `nginx` custom integration bound against a custom agent preset as shown in [agent-nginx.yaml](agent-nginx.yaml).

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

3. `nginx` integration Assets are installed through Kibana

## Run:
1. For **non** eck-managed ElasticSearch clusters
    ```console
    helm install eck-integrations ../../ \
         -f ./agent-nginx.yaml \
         --set elasticsearchRefs.default.secretName=es-ref-secret 
    ```
    For eck-managed ElasticSearch clusters
    ```console
    helm install eck-integrations ../../ \
         -f ./agent-nginx.yaml \
         --set elasticsearchRefs.default.name=eck-es-name 
    ```

2. Install the nginx deployment
    ```console
   kubectl apply -f ./nginx.yaml
    ```
   
## Validate:

1. The Kibana `nginx`-related dashboards should start showing up the respective info.