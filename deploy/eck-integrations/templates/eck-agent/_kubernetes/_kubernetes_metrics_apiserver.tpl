{{/*
Config input for kube apiserver
*/}}
{{- define "agent.kubernetes.config.kube_apiserver.input" -}}
{{- if default .control_plane.apiserver.enabled false -}}
- id: kubernetes/kubernetes/metrics-kube-apiserver
  type: kubernetes/metrics
  data_stream:
      namespace: {{ .namespace }}
  use_output: default
  streams:
  - id: kubernetes/metrics-kubernetes.apiserver
    data_stream:
        type: metrics
        dataset: kubernetes.apiserver
    metricsets:
        - apiserver
{{- include "agent.kubernetes.config.kube_apiserver.defaults" .control_plane.apiserver | nindent 4 }}
  meta:
    package:
      name: kubernetes
      version: {{ .version }}
{{- end -}}
{{- end -}}


{{/*
Defaults for kube_apiserver input streams
*/}}
{{- define "agent.kubernetes.config.kube_apiserver.defaults" -}}
hosts:
{{- range dig "vars" "hosts" (list "https://${env.KUBERNETES_SERVICE_HOST}:${env.KUBERNETES_SERVICE_PORT}") . }}
- {{. | quote}}
{{- end }}
period: {{ dig "vars" "period" "30s" . }}
bearer_token_file: {{ dig "vars" "bearer_token_file" "/var/run/secrets/kubernetes.io/serviceaccount/token" .}}
ssl.certificate_authorities:
{{- range dig "vars" "ssl.certificate_authorities" (list "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt") . }}
- {{. | quote}}
{{- end }}
{{- end -}}