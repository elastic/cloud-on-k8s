{{- define "elasticagent.kubernetes.config.kube_apiserver.init" -}}
{{- if $.Values.kubernetes.apiserver.enabled}}
{{- $preset := $.Values.agent.presets.clusterWide -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.kube_apiserver.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.clusterwide.preset") -}}
{{- end -}}
{{- end -}}

{{/*
Config input for kube apiserver
*/}}
{{- define "elasticagent.kubernetes.config.kube_apiserver.input" -}}
- id: kubernetes/metrics-kubernetes.apiserver
  type: kubernetes/metrics
  data_stream:
    namespace: {{ $.Values.kubernetes.namespace }}
  use_output: {{ $.Values.kubernetes.output }}
  streams:
  - id: kubernetes/metrics-kubernetes.apiserver
    data_stream:
      type: metrics
      dataset: kubernetes.apiserver
    metricsets:
    - apiserver
{{- $vars := (include "elasticagent.kubernetes.config.kube_apiserver.default_vars" .) | fromYaml -}}
{{- mergeOverwrite $vars $.Values.kubernetes.apiserver.vars | toYaml | nindent 4 }}
{{- end -}}


{{/*
Defaults for kube_apiserver input streams
*/}}
{{- define "elasticagent.kubernetes.config.kube_apiserver.default_vars" -}}
hosts:
- 'https://${env.KUBERNETES_SERVICE_HOST}:${env.KUBERNETES_SERVICE_PORT}'
period: "30s"
bearer_token_file: '/var/run/secrets/kubernetes.io/serviceaccount/token'
ssl.certificate_authorities:
- '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt'
{{- end -}}