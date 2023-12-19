{{- define "agent.kubernetes.config.kube_proxy.enabled" -}}
enabled: {{ .Values.kubernetes.proxy.enabled }}
{{- end -}}

{{/*
Config input for kube proxy
*/}}
{{- define "agent.kubernetes.config.kube_proxy.input" -}}
{{- $vars := (include "agent.kubernetes.config.kube_proxy.default_vars" .) | fromYaml -}}
- id: kubernetes/metrics-kube-proxy
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .Values.kubernetes.namespace }}
  use_output: default
  streams:
    - id: kubernetes/metrics-kubernetes.proxy
      data_stream:
        type: metrics
        dataset: kubernetes.proxy
      metricsets:
        - proxy
{{- mergeOverwrite $vars .Values.kubernetes.proxy.vars | toYaml | nindent 4 }}
  meta:
    package:
      name: kubernetes
      version: 1.51.0
{{- end -}}


{{/*
Defaults for kube_proxy input streams
*/}}
{{- define "agent.kubernetes.config.kube_proxy.default_vars" -}}
hosts:
- "localhost:10249"
period: "10s"
{{- end -}}