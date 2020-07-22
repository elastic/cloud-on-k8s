apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  annotations:
    capabilities: Full Lifecycle
    categories: Database
    certified: 'false'
    containerImage: {{ .OperatorRepo }}:{{ .NewVersion }}
    createdAt: {{ now | date "2006-01-02 15:04:05" }}
    description: Run Elasticsearch, Kibana, APM Server, Enterprise Search, and Beats on Kubernetes and OpenShift
    repository: https://github.com/elastic/cloud-on-k8s
    support: elastic.co
    alm-examples: |-
      [
          {
              "apiVersion": "elasticsearch.k8s.elastic.co/v1",
              "kind": "Elasticsearch",
              "metadata": {
                  "name": "elasticsearch-sample"
              },
              "spec": {
                  "version": "{{ .StackVersion }}",
                  "nodeSets": [
                      {
                          "name": "default",
                          "config": {
                              "node.master": true,
                              "node.data": true,
                              "node.attr.attr_name": "attr_value",
                              "node.store.allow_mmap": false
                          },
                          "podTemplate": {
                              "metadata": {
                                  "labels": {
                                      "foo": "bar"
                                  }
                              },
                              "spec": {
                                  "containers": [
                                      {
                                          "name": "elasticsearch",
                                          "resources": {
                                              "requests": {
                                                  "memory": "4Gi",
                                                  "cpu": 1
                                              },
                                              "limits": {
                                                  "memory": "4Gi",
                                                  "cpu": 2
                                              }
                                          }
                                      }
                                  ]
                              }
                          },
                          "count": 3
                      }
                  ]
              }
          },
          {
              "apiVersion": "kibana.k8s.elastic.co/v1",
              "kind": "Kibana",
              "metadata": {
                  "name": "kibana-sample"
              },
              "spec": {
                  "version": "{{ .StackVersion }}",
                  "count": 1,
                  "elasticsearchRef": {
                      "name": "elasticsearch-sample"
                  },
                  "podTemplate": {
                      "metadata": {
                          "labels": {
                              "foo": "bar"
                          }
                      },
                      "spec": {
                          "containers": [
                              {
                                  "name": "kibana",
                                  "resources": {
                                      "requests": {
                                          "memory": "1Gi",
                                          "cpu": 0.5
                                      },
                                      "limits": {
                                          "memory": "2Gi",
                                          "cpu": 2
                                      }
                                  }
                              }
                          ]
                      }
                  }
              }
          },
          {
              "apiVersion": "apm.k8s.elastic.co/v1",
              "kind": "ApmServer",
              "metadata": {
                  "name": "apmserver-sample"
              },
              "spec": {
                  "version": "{{ .StackVersion }}",
                  "count": 1,
                  "elasticsearchRef": {
                      "name": "elasticsearch-sample"
                  }
              }
          },
          {
              "apiVersion": "enterprisesearch.k8s.elastic.co/v1beta1",
              "kind": "EnterpriseSearch",
              "metadata": {
                  "name": "ent-sample"
              },
              "spec": {
                  "version": "{{ .StackVersion }}",
                  "config": {
                      "ent_search.external_url": "https://localhost:3002"
                  },
                  "count": 1,
                  "elasticsearchRef": {
                      "name": "elasticsearch-sample"
                  }
              }
          }
      ]
  name: elastic-cloud-eck.v{{ .NewVersion }}
  namespace: placeholder
spec:
  customresourcedefinitions:
    owned:
    {{- range .CRDList }}
    - description: {{ .Description }}
      displayName: {{ .DisplayName }}
      group: {{ .Group }}
      kind: {{ .Kind }}
      name: {{ .Name }}
      version: {{ .Version }}
    {{- end }}
  description: 'Elastic Cloud on Kubernetes automates the deployment, provisioning,
    management, and orchestration of Elasticsearch, Kibana, APM Server, Beats, and 
    Enterprise Search on Kubernetes.


    Current features:


    *  Elasticsearch, Kibana, APM Server, Enterprise Search, and Beats deployments

    *  TLS Certificates management

    *  Safe Elasticsearch cluster configuration and topology changes

    *  Persistent volumes usage

    *  Custom node configuration and attributes

    *  Secure settings keystore updates


    Supported versions:


    * Kubernetes: 1.12+ or OpenShift 3.11+

    * Elasticsearch: 6.8+, 7.1+


    See the [Quickstart](https://www.elastic.co/guide/en/cloud-on-k8s/{{ .ShortVersion }}/k8s-quickstart.html)
    to get started with ECK.'
  displayName: Elastic Cloud on Kubernetes
  icon:
  - base64data: PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI2NCIgaGVpZ2h0PSI2NCIgdmlld0JveD0iMCAwIDY0IDY0Ij4KICA8ZyBmaWxsPSJub25lIiBmaWxsLXJ1bGU9ImV2ZW5vZGQiPgogICAgPHBhdGggZmlsbD0iIzM0Mzc0MSIgZD0iTTM3LjA2MjUsMzIgQzM3LjA2MjUsMzQuNzk2IDM0Ljc5NTUsMzcuMDYyIDMyLjAwMDUsMzcuMDYyIEMyOS4yMDQ1LDM3LjA2MiAyNi45Mzc1LDM0Ljc5NiAyNi45Mzc1LDMyIEMyNi45Mzc1LDI5LjIwNCAyOS4yMDQ1LDI2LjkzOCAzMi4wMDA1LDI2LjkzOCBDMzQuNzk1NSwyNi45MzggMzcuMDYyNSwyOS4yMDQgMzcuMDYyNSwzMiIvPgogICAgPHBhdGggZmlsbD0iI0YwNEU5OCIgZD0iTTM4LjIyMjcsMjQuMTA0NSBDMzguMjY1NywyNC4xMzg1IDM4LjMwOTcsMjQuMTczNSAzOC4zNTI3LDI0LjIwODUgQzM5LjI5MDcsMjQuOTcyNSA0MC41ODU3LDI1LjEyMDUgNDEuNjc3NywyNC41OTc1IEw1NS4xNzE3LDE4LjE0MTUgQzUxLjg3NDcsMTIuNjQwNSA0Ni42NzU3LDguNDE2NSA0MC40ODI3LDYuMzY3NSBMMzcuMTE4NywyMC45NDI1IEMzNi44NDU3LDIyLjEyMTUgMzcuMjcxNywyMy4zNTU1IDM4LjIyMjcsMjQuMTA0NSIvPgogICAgPHBhdGggZmlsbD0iIzA3QyIgZD0iTTMyLjE4NTUsNDQuMTgzNiBDMzEuOTE4NSw0My4wMDI2IDMwLjk5OTUsNDIuMDc1NiAyOS44MTg1LDQxLjgxMzYgQzI1LjMxNjUsNDAuODE1NiAyMS45Mzc1LDM2Ljc5ODYgMjEuOTM3NSwzMS45OTk2IEMyMS45Mzc1LDI3LjE4MjYgMjUuMzQxNSwyMy4xNTI2IDI5Ljg3MDUsMjIuMTczNiBDMzEuMDUyNSwyMS45MTg2IDMxLjk3NTUsMjAuOTk0NiAzMi4yNDc1LDE5LjgxNTYgTDM1LjYxMDUsNS4yNDc2IEMzNC40Mjg1LDUuMDg5NiAzMy4yMjQ1LDQuOTk5NiAzMS45OTk1LDQuOTk5NiBDMTcuMDg3NSw0Ljk5OTYgNC45OTk1LDE3LjA4ODYgNC45OTk1LDMxLjk5OTYgQzQuOTk5NSw0Ni45MTE2IDE3LjA4NzUsNTguOTk5NiAzMS45OTk1LDU4Ljk5OTYgQzMzLjE3ODUsNTguOTk5NiAzNC4zMzY1LDU4LjkxNTYgMzUuNDc0NSw1OC43Njk2IEwzMi4xODU1LDQ0LjE4MzYgWiIvPgogICAgPHBhdGggZmlsbD0iIzM0Mzc0MSIgZD0iTTU3LjI4NTIsNDEuNDc4IEM1OC4zOTAyLDM4LjUyOCA1OS4wMDAyLDM1LjMzNiA1OS4wMDAyLDMyIEM1OS4wMDAyLDI4LjcxMyA1OC40MTEyLDI1LjU2MyA1Ny4zMzUyLDIyLjY0OSBMNDMuNzUyMiwyOS4xNDggQzQyLjY1MzIsMjkuNjc0IDQxLjk2MjIsMzAuNzg2IDQxLjk2MTE5MDIsMzIuMDA0IEw0MS45NjExOTAyLDMyLjA1NCBDNDEuOTU4MiwzMy4yNyA0Mi42NDMyLDM0LjM4MSA0My43MzcyLDM0LjkxMSBMNTcuMjg1Miw0MS40NzggWiIvPgogICAgPHBhdGggZmlsbD0iI0YwNEU5OCIgZD0iTTM4LjMxMDUsMzkuODIzNyBDMzguMjY3NSwzOS44NTc3IDM4LjIyNDUsMzkuODkxNyAzOC4xODE1LDM5LjkyNTcgQzM3LjIyNzUsNDAuNjY5NyAzNi43OTU1LDQxLjkwMDcgMzcuMDYxNSw0My4wODE3IEw0MC4zNTM1LDU3LjY3NjcgQzQ2LjU1OTUsNTUuNjU3NyA1MS43ODE1LDUxLjQ1OTcgNTUuMTA0NSw0NS45Nzc3IEw0MS42Mzk1LDM5LjQ1MDcgQzQwLjU0OTUsMzguOTIyNyAzOS4yNTI1LDM5LjA2MzcgMzguMzEwNSwzOS44MjM3Ii8+CiAgPC9nPgo8L3N2Zz4K
    mediatype: image/svg+xml
  install:
    spec:
      deployments:
      - name: elastic-operator
        spec:
          replicas: 1
          selector:
            matchLabels:
              control-plane: elastic-operator
          template:
            metadata:
              annotations:
                "co.elastic.logs/raw": "[{\"type\":\"container\",\"json.keys_under_root\":true,\"paths\":[\"/var/log/containers/*${data.kubernetes.container.id}.log\"],\"processors\":[{\"convert\":{\"mode\":\"rename\",\"ignore_missing\":true,\"fields\":[{\"from\":\"error\",\"to\":\"_error\"}]}},{\"convert\":{\"mode\":\"rename\",\"ignore_missing\":true,\"fields\":[{\"from\":\"_error\",\"to\":\"error.message\"}]}},{\"convert\":{\"mode\":\"rename\",\"ignore_missing\":true,\"fields\":[{\"from\":\"source\",\"to\":\"_source\"}]}},{\"convert\":{\"mode\":\"rename\",\"ignore_missing\":true,\"fields\":[{\"from\":\"_source\",\"to\":\"event.source\"}]}}]}]"
              labels:
                control-plane: elastic-operator
            spec:
              serviceAccountName: elastic-operator
              containers:
              - image: {{ .OperatorRepo }}:{{ .NewVersion }}
                name: manager
                args: ["manager", "--log-verbosity=0"]
                env:
                - name: NAMESPACES
                  valueFrom:
                    fieldRef:
                      fieldPath: metadata.annotations['olm.targetNamespaces']
                - name: OPERATOR_NAMESPACE
                  valueFrom:
                    fieldRef:
                      fieldPath: metadata.annotations['olm.operatorNamespace']
                - name: OPERATOR_IMAGE
                  value: {{ .OperatorRepo }}:{{ .NewVersion }}
                resources:
                  limits:
                    cpu: 1
                    memory: 512Mi
                  requests:
                    cpu: 100m
                    memory: 150Mi
              terminationGracePeriodSeconds: 10
      permissions:
      - rules:
{{ .OperatorRBAC | indent 8 }}
        serviceAccountName: elastic-operator
    strategy: deployment
  installModes:
  - supported: true
    type: OwnNamespace
  - supported: true
    type: SingleNamespace
  - supported: true
    type: MultiNamespace
  - supported: true
    type: AllNamespaces
  keywords:
  - elasticsearch
  - kibana
  - analytics
  - search
  - database
  - apm
  links:
  - name: Documentation
    url: https://www.elastic.co/guide/en/cloud-on-k8s/{{ .ShortVersion }}/index.html
  maintainers:
  - email: eck@elastic.co
    name: Elastic
  maturity: stable
  minKubeVersion: 1.11.0
  provider:
    name: Elastic
  replaces: elastic-cloud-eck.v{{ .PrevVersion }}
  version: {{ .NewVersion }}
