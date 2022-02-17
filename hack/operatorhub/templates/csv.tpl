apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  annotations:
    capabilities: Full Lifecycle
    categories: Database
    certified: 'false'
    containerImage: {{ .OperatorRepo }}:{{ .NewVersion }}
    createdAt: {{ now | date "2006-01-02 15:04:05" }}
    description: Run Elasticsearch, Kibana, APM Server, Beats, Enterprise Search, Elastic Agent and Elastic Maps Server on Kubernetes and OpenShift
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
                              "node.roles": ["master", "data"],
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
              "apiVersion": "enterprisesearch.k8s.elastic.co/v1",
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
          },
          {
            "apiVersion": "beat.k8s.elastic.co/v1beta1",
            "kind": "Beat",
            "metadata": {
              "name": "heartbeat-sample"
            },
            "spec": {
              "type": "heartbeat",
              "version": "{{ .StackVersion }}",
              "elasticsearchRef": {
                "name": "elasticsearch-sample"
              },
              "config": {
                "heartbeat.monitors": [
                  {
                    "type": "tcp",
                    "schedule": "@every 5s",
                    "hosts": [
                      "elasticsearch-sample-es-http.default.svc:9200"
                    ]
                  }
                ]
              },
              "deployment": {
                "replicas": 1,
                "podTemplate": {
                  "spec": {
                    "securityContext": {
                      "runAsUser": 0
                    }
                  }
                }
              }
            }
          },
          {
            "apiVersion": "agent.k8s.elastic.co/v1alpha1",
            "kind": "Agent",
            "metadata": {
              "name": "agent-sample"
            },
            "spec": {
              "version": "{{ .StackVersion }}",
              "elasticsearchRefs": [
                {
                  "name": "elasticsearch-sample"
                }
              ],
              "daemonSet": {},
              "config": {
                "inputs": [
                  {
                    "name": "system-1",
                    "revision": 1,
                    "type": "system/metrics",
                    "use_output": "default",
                    "meta": {
                      "package": {
                        "name": "system",
                        "version": "0.9.1"
                      }
                    },
                    "data_stream": {
                      "namespace": "default"
                    },
                    "streams": [
                      {
                        "id": "system/metrics-system.cpu",
                        "data_stream": {
                          "dataset": "system.cpu",
                          "type": "metrics"
                        },
                        "metricsets": [
                          "cpu"
                        ],
                        "cpu.metrics": [
                          "percentages",
                          "normalized_percentages"
                        ],
                        "period": "10s"
                      }
                    ]
                  }
                ]
              }
            }
          },
          {
              "apiVersion": "maps.k8s.elastic.co/v1alpha1",
              "kind": "ElasticMapsServer",
              "metadata": {
                  "name": "ems-sample"
              },
              "spec": {
                  "version": "{{ .StackVersion }}",
                  "count": 1,
                  "elasticsearchRef": {
                      "name": "elasticsearch-sample"
                  }
              }
          }
      ]
  name: {{ .PackageName }}.v{{ .NewVersion }}
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
  description: 'Elastic Cloud on Kubernetes (ECK) is the official operator by Elastic for automating the deployment, provisioning,
    management, and orchestration of Elasticsearch, Kibana, APM Server, Beats, Enterprise Search, Elastic Agent and Elastic Maps Server
    on Kubernetes.


    Current features:


    *  Elasticsearch, Kibana, APM Server, Enterprise Search, Beats, Elastic Agent and Elastic Maps Server deployments

    *  TLS Certificates management

    *  Safe Elasticsearch cluster configuration and topology changes

    *  Persistent volumes usage

    *  Custom node configuration and attributes

    *  Secure settings keystore updates


    Supported versions:


    * Kubernetes 1.19-1.23

    * OpenShift 4.6-4.10

    * Google Kubernetes Engine (GKE), Azure Kubernetes Service (AKS), and Amazon Elastic Kubernetes Service (EKS)

    * Elasticsearch, Kibana, APM Server: {{ if .UbiOnly }}7.10+{{ else }}6.8+, 7.1+{{ end }}

    * Enterprise Search: {{ if .UbiOnly }}7.10+{{ else }}7.7+{{ end }}

    * Beats: {{ if .UbiOnly }}7.10+{{ else }}7.0+{{ end }}

    * Elastic Agent: 7.10+

    * Elastic Maps Server: 7.11+


    ECK should work with all conformant installers as listed in these [FAQs](https://github.com/cncf/k8s-conformance/blob/master/faq.md#what-is-a-distribution-hosted-platform-and-an-installer). Distributions include source patches and so may not work as-is with ECK.

    Alpha, beta, and stable API versions follow the same [conventions used by Kubernetes](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning).

    See the full [Elastic support matrix](https://www.elastic.co/support/matrix#matrix_kubernetes) for more information.

    See the [Quickstart](https://www.elastic.co/guide/en/cloud-on-k8s/{{ .ShortVersion }}/k8s-quickstart.html)
    to get started with ECK.'
  displayName: Elasticsearch (ECK) Operator
  icon:
  - base64data: PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0iVVRGLTgiIHN0YW5kYWxvbmU9Im5vIj8+CjxzdmcKICAgeG1sbnM6ZGM9Imh0dHA6Ly9wdXJsLm9yZy9kYy9lbGVtZW50cy8xLjEvIgogICB4bWxuczpjYz0iaHR0cDovL2NyZWF0aXZlY29tbW9ucy5vcmcvbnMjIgogICB4bWxuczpyZGY9Imh0dHA6Ly93d3cudzMub3JnLzE5OTkvMDIvMjItcmRmLXN5bnRheC1ucyMiCiAgIHhtbG5zOnN2Zz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciCiAgIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyIKICAgeG1sbnM6c29kaXBvZGk9Imh0dHA6Ly9zb2RpcG9kaS5zb3VyY2Vmb3JnZS5uZXQvRFREL3NvZGlwb2RpLTAuZHRkIgogICB4bWxuczppbmtzY2FwZT0iaHR0cDovL3d3dy5pbmtzY2FwZS5vcmcvbmFtZXNwYWNlcy9pbmtzY2FwZSIKICAgaW5rc2NhcGU6dmVyc2lvbj0iMS4wICg0MDM1YTRmYjQ5LCAyMDIwLTA1LTAxKSIKICAgaGVpZ2h0PSI2NCIKICAgd2lkdGg9IjY0IgogICBzb2RpcG9kaTpkb2NuYW1lPSJjbHVzdGVyLWNvbG9yLTY0eDY0LnN2ZyIKICAgeG1sOnNwYWNlPSJwcmVzZXJ2ZSIKICAgdmlld0JveD0iMCAwIDY0IDY0IgogICB5PSIwcHgiCiAgIHg9IjBweCIKICAgaWQ9IkxheWVyXzEiCiAgIHZlcnNpb249IjEuMSI+PG1ldGFkYXRhCiAgIGlkPSJtZXRhZGF0YTExOCI+PHJkZjpSREY+PGNjOldvcmsKICAgICAgIHJkZjphYm91dD0iIj48ZGM6Zm9ybWF0PmltYWdlL3N2Zyt4bWw8L2RjOmZvcm1hdD48ZGM6dHlwZQogICAgICAgICByZGY6cmVzb3VyY2U9Imh0dHA6Ly9wdXJsLm9yZy9kYy9kY21pdHlwZS9TdGlsbEltYWdlIiAvPjxkYzp0aXRsZT48L2RjOnRpdGxlPjwvY2M6V29yaz48L3JkZjpSREY+PC9tZXRhZGF0YT48ZGVmcwogICBpZD0iZGVmczExNiIgLz48c29kaXBvZGk6bmFtZWR2aWV3CiAgIGlua3NjYXBlOmN1cnJlbnQtbGF5ZXI9IkxheWVyXzEiCiAgIGlua3NjYXBlOndpbmRvdy1tYXhpbWl6ZWQ9IjEiCiAgIGlua3NjYXBlOndpbmRvdy15PSIwIgogICBpbmtzY2FwZTp3aW5kb3cteD0iMCIKICAgaW5rc2NhcGU6Y3k9IjUwLjkwMzQ1NiIKICAgaW5rc2NhcGU6Y3g9IjEyIgogICBpbmtzY2FwZTp6b29tPSIzNC45NTgzMzMiCiAgIGZpdC1tYXJnaW4tYm90dG9tPSIwIgogICBmaXQtbWFyZ2luLXJpZ2h0PSIwIgogICBmaXQtbWFyZ2luLWxlZnQ9IjAiCiAgIGZpdC1tYXJnaW4tdG9wPSIwIgogICBzaG93Z3JpZD0iZmFsc2UiCiAgIGlkPSJuYW1lZHZpZXcxMTQiCiAgIGlua3NjYXBlOndpbmRvdy1oZWlnaHQ9IjEzODgiCiAgIGlua3NjYXBlOndpbmRvdy13aWR0aD0iMjU2MCIKICAgaW5rc2NhcGU6cGFnZXNoYWRvdz0iMiIKICAgaW5rc2NhcGU6cGFnZW9wYWNpdHk9IjAiCiAgIGd1aWRldG9sZXJhbmNlPSIxMCIKICAgZ3JpZHRvbGVyYW5jZT0iMTAiCiAgIG9iamVjdHRvbGVyYW5jZT0iMTAiCiAgIGJvcmRlcm9wYWNpdHk9IjEiCiAgIGJvcmRlcmNvbG9yPSIjNjY2NjY2IgogICBwYWdlY29sb3I9IiNmZmZmZmYiIC8+CjxzdHlsZQogICBpZD0ic3R5bGU5MSIKICAgdHlwZT0idGV4dC9jc3MiPgoJLnN0MHtmaWxsOiNGRkQxMDY7fQoJLnN0MXtmaWxsOiMyMUJBQjA7fQoJLnN0MntmaWxsOiNFRTRGOTc7fQoJLnN0M3tmaWxsOiMxNEE3REY7fQoJLnN0NHtmaWxsOiM5MUM3M0U7fQoJLnN0NXtmaWxsOiMwMjc5QTA7fQoJLnN0NntmaWxsOm5vbmU7fQo8L3N0eWxlPgo8ZwogICB0cmFuc2Zvcm09InNjYWxlKDIuNjU1NjAxNywyLjY2NjY2NjcpIgogICBpZD0iZzEwOSI+Cgk8ZwogICBpZD0iZzEwNyI+CgkJPGcKICAgaWQ9ImcxMDUiPgoJCQk8cGF0aAogICBpZD0icGF0aDkzIgogICBkPSJtIDkuMiwxMC4yIDUuNywyLjYgNS43LC01IEMgMjAuNyw3LjQgMjAuNyw3IDIwLjcsNi41IDIwLjcsMyAxNy44LDAuMSAxNC4zLDAuMSAxMi4yLDAuMSAxMC4yLDEuMSA5LDIuOSBsIC0xLDUgeiIKICAgY2xhc3M9InN0MCIgLz4KCQkJPHBhdGgKICAgaWQ9InBhdGg5NSIKICAgZD0ibSAzLjMsMTYuMiBjIC0wLjEsMC40IC0wLjEsMC44IC0wLjEsMS4zIDAsMy41IDIuOSw2LjQgNi40LDYuNCAyLjEsMCA0LjEsLTEuMSA1LjMsLTIuOCBsIDAuOSwtNC45IC0xLjMsLTIuNCAtNS43LC0yLjYgeiIKICAgY2xhc3M9InN0MSIgLz4KCQkJPHBhdGgKICAgaWQ9InBhdGg5NyIKICAgZD0iTSAzLjMsNi40IDcuMiw3LjMgOCwyLjkgQyA3LjUsMi40IDYuOSwyLjIgNi4yLDIuMiA0LjUsMi4yIDMuMSwzLjYgMy4xLDUuMyAzLjEsNS43IDMuMiw2IDMuMyw2LjQiCiAgIGNsYXNzPSJzdDIiIC8+CgkJCTxwYXRoCiAgIGlkPSJwYXRoOTkiCiAgIGQ9Im0gMyw3LjMgYyAtMS43LDAuNiAtMywyLjIgLTMsNC4xIDAsMS44IDEuMSwzLjQgMi44LDQgbCA1LjUsLTQuOSAtMSwtMi4xIHoiCiAgIGNsYXNzPSJzdDMiIC8+CgkJCTxwYXRoCiAgIGlkPSJwYXRoMTAxIgogICBkPSJtIDE2LDIxLjEgYyAwLjUsMC40IDEuMiwwLjYgMS45LDAuNiAxLjcsMCAzLjEsLTEuNCAzLjEsLTMuMSAwLC0wLjQgLTAuMSwtMC43IC0wLjIsLTEuMSBsIC0zLjksLTAuOSB6IgogICBjbGFzcz0ic3Q0IiAvPgoJCQk8cGF0aAogICBpZD0icGF0aDEwMyIKICAgZD0ibSAxNi44LDE1LjcgNC4zLDEgYyAxLjcsLTAuNiAzLC0yLjIgMywtNC4xIDAsLTEuOCAtMS4xLC0zLjQgLTIuOCwtNCBsIC01LjYsNC45IHoiCiAgIGNsYXNzPSJzdDUiIC8+CgkJPC9nPgoJPC9nPgo8L2c+CjxyZWN0CiAgIHN0eWxlPSJzdHJva2Utd2lkdGg6Mi42NjExMyIKICAgeT0iMCIKICAgeD0iMCIKICAgaWQ9InJlY3QxMTEiCiAgIGhlaWdodD0iNjQiCiAgIHdpZHRoPSI2My43MzQ0NCIKICAgY2xhc3M9InN0NiIgLz4KPC9zdmc+Cg==
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
                args:
                  - "manager"
                  - "--config=/conf/eck.yaml"
                  - "--manage-webhook-certs=false"
                  - "--enable-webhook"
                  {{- range  .AdditionalArgs }}
                  - "{{.}}"
                  {{- end }}
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
                    memory: 1Gi
                  requests:
                    cpu: 100m
                    memory: 150Mi
                ports:
                - containerPort: 9443
                  name: https-webhook
                  protocol: TCP
              terminationGracePeriodSeconds: 10
      permissions:
      - rules:
{{ .OperatorRBAC | indent 8 -}}
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
  minKubeVersion: 1.16.0
  provider:
    name: Elastic
  replaces: {{ .PackageName }}.v{{ .PrevVersion }}
  version: {{ .NewVersion }}
  webhookdefinitions:
{{ .OperatorWebhooks | trim | indent 4 }}
