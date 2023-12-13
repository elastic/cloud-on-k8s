{{/*
Config input for kube proxy
*/}}
{{- define "agent.kubernetes.config.kube_proxy.input" -}}
{{- if default .proxy.metrics.enabled false -}}
- id: kubernetes/metrics-kube-proxy
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .namespace }}
  use_output: default
  streams:
    - id: kubernetes/metrics-kubernetes.proxy
      data_stream:
        type: metrics
        dataset: kubernetes.proxy
      metricsets:
        - proxy
{{- include "agent.kubernetes.config.kube_proxy.defaults" .proxy | nindent 4 }}
  meta:
    package:
      name: kubernetes
      version: 1.51.0
{{- end -}}
{{- end -}}


{{/*
Defaults for kube_proxy input streams
*/}}
{{- define "agent.kubernetes.config.kube_proxy.defaults" -}}
hosts:
{{- range dig "vars" "hosts" (list "localhost:10249") . }}
- {{. | quote}}
{{- end }}
period: {{ dig "vars" "period" "10s" . }}
{{- end -}}