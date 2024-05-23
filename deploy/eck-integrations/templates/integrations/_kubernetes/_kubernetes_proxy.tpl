{{- define "elasticagent.kubernetes.config.kube_proxy.init" -}}
{{- if eq $.Values.kubernetes.proxy.enabled true -}}
{{- $preset := $.Values.agent.presets.perNode -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.kube_proxy.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}

{{/*
Config input for kube proxy
*/}}
{{- define "elasticagent.kubernetes.config.kube_proxy.input" -}}
{{- $vars := (include "elasticagent.kubernetes.config.kube_proxy.default_vars" .) | fromYaml -}}
- id: kubernetes/metrics-kubernetes.proxy
  type: kubernetes/metrics
  data_stream:
    namespace: {{ .Values.kubernetes.namespace }}
  use_output: {{ .Values.kubernetes.output }}
  streams:
    - id: kubernetes/metrics-kubernetes.proxy
      data_stream:
        type: metrics
        dataset: kubernetes.proxy
      metricsets:
        - proxy
{{- mergeOverwrite $vars .Values.kubernetes.proxy.vars | toYaml | nindent 4 }}
{{- end -}}


{{/*
Defaults for kube_proxy input streams
*/}}
{{- define "elasticagent.kubernetes.config.kube_proxy.default_vars" -}}
hosts:
- "localhost:10249"
period: "10s"
{{- end -}}