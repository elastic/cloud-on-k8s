# Example: Kubernetes Integration with hint-based autodiscover

In this example we install the built-in `kubernetes` integration and enable the feature of hint-based autodiscover. With this feature, the kubernetes integration can monitor the creation of pods that bear specific annotations based on which the agent loads dynamically the respective integration. In the context of this example, we showcase hint-based autodiscover with `redis` integration.  

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

3. `redis` integration assets are installed through Kibana ([Kibana - Install and uninstall Elastic Agent integration assets](https://www.elastic.co/guide/en/fleet/current/install-uninstall-integration-assets.html))

## Run:
1. For **non** eck-managed Elasticsearch clusters
    ```console
    helm install eck-integrations ../../ \
         -f ./agent-kubernetes.yaml \
         --set elasticsearchRefs.default.secretName=es-ref-secret 
    ```
    For eck-managed Elasticsearch clusters
    ```console
    helm install eck-integrations ../../ \
         -f ./agent-kubernetes.yaml \
         --set elasticsearchRefs.default.name=eck-es-name 
    ```

2. Install a redis pod with the appropriate annotations
    ```console
   kubectl apply -f ./redis.yaml
    ```

## Validate:

1. The Kibana `redis`-related dashboards should start showing up the respective info.