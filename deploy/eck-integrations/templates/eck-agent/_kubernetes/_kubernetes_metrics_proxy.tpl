{{/*
Config input for kube proxy
*/}}
{{- define "kubernets.config.kube_proxy.input" -}}
{{- if default .control_plane.proxy.enabled false -}}
- id: kubernetes/metrics-kube-proxy
  revision: 1
  name: kubernetes
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .namespace }}
  use_output: default
  package_policy_id: {{.integrationID}}
  streams:
    - id: kubernetes/metrics-kubernetes.proxy
      data_stream:
        type: metrics
        dataset: kubernetes.proxy
      metricsets:
        - proxy
{{- include "kubernets.config.kube_proxy.defaults" .control_plane.proxy | nindent 4 }} 
  meta:
    package:
      name: kubernetes
      version: 1.51.0
{{- end -}}
{{- end -}}


{{/*
Defaults for kube_proxy input streams
*/}}
{{- define "kubernets.config.kube_proxy.defaults" -}}
hosts:
{{- range dig "vars" "hosts" (list "localhost:10249") . }}
- {{. | quote}}
{{- end }}
period: {{ dig "vars" "period" "10s" . }}
{{- end -}}