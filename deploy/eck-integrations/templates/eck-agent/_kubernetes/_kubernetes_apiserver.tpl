{{- define "agent.kubernetes.config.kube_apiserver.enabled" -}}
enabled: {{ .Values.kubernetes.apiserver.enabled }}
{{- end -}}

{{/*
Config input for kube apiserver
*/}}
{{- define "agent.kubernetes.config.kube_apiserver.input" -}}
{{- $vars := (include "agent.kubernetes.config.kube_apiserver.default_vars" .) | fromYaml -}}
- id: kubernetes/kubernetes/metrics-kube-apiserver
  type: kubernetes/metrics
  data_stream:
      namespace: {{ .Values.kubernetes.namespace }}
  use_output: default
  streams:
  - id: kubernetes/metrics-kubernetes.apiserver
    data_stream:
        type: metrics
        dataset: kubernetes.apiserver
    metricsets:
        - apiserver
{{- mergeOverwrite $vars .Values.kubernetes.apiserver.vars | toYaml | nindent 4 }}
  meta:
    package:
      name: kubernetes
      version: {{ .Values.kubernetes.version }}
{{- end -}}


{{/*
Defaults for kube_apiserver input streams
*/}}
{{- define "agent.kubernetes.config.kube_apiserver.default_vars" -}}
hosts:
- 'https://${env.KUBERNETES_SERVICE_HOST}:${env.KUBERNETES_SERVICE_PORT}'
period: "30s"
bearer_token_file: '/var/run/secrets/kubernetes.io/serviceaccount/token'
ssl.certificate_authorities:
- '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt'
{{- end -}}