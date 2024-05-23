# Example: Nginx Custom Integration

In this example we define a `nginx` custom integration alongside a custom agent preset defined in [agent-nginx.yaml](agent-nginx.yaml).

## Prerequisites:
1. Installed eck-operator Helm chart
   ```console
   helm repo add elastic https://helm.elastic.co && helm repo update
   helm install elastic-operator elastic/eck-operator --create-namespace
   ```
2. For **non** eck-managed Elasticsearch clusters you need a k8s secret that contains the connection details to it such as:
   1. with a Username and Password ([Kibana - Create roles and users](https://www.elastic.co/guide/en/kibana/current/using-kibana-with-security.html#security-create-roles)):
      ```console
       kubectl create secret generic es-ref-secret \
           --from-literal=username=... \
           --from-literal=password=... \
           --from-literal=url=...
       ```
   2. with an API key ([Kibana - Creating API Keys](https://www.elastic.co/guide/en/kibana/current/api-keys.html)):
      ```console
       kubectl create secret generic es-ref-secret \
           --from-literal=api-key=... \
           --from-literal=url=...
       ```

3. `nginx` integration assets are installed through Kibana

## Run:
1. For **non** eck-managed Elasticsearch clusters
    ```console
    helm install eck-integrations ../../ \
         -f ./agent-nginx.yaml \
         --set elasticsearchRefs.default.secretName=es-ref-secret 
    ```
    For eck-managed Elasticsearch clusters
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

1. The Kibana `nginx`-related dashboards should start showing nginx related data.