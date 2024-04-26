{{- define "elasticagent.kubernetes.config.kubelet.system.init" -}}
{{- if eq ((.Values.kubernetes.metrics).enabled) false -}}
{{- $_ := set $.Values.kubernetes.system.metrics "enabled" false -}}
{{- else -}}
{{- if eq $.Values.kubernetes.system.metrics.enabled true -}}
{{- $preset := $.Values.agent.presets.perNode -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.kubelet.system.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.kubernetes.config.kubelet.system.input" -}}
- id: kubernetes/metrics-kubernetes.system
  type: kubernetes/metrics
  data_stream:
    namespace: {{ $.Values.kubernetes.namespace }}
  use_output: {{ $.Values.kubernetes.output }}
  streams:
  - id: kubernetes/metrics-kubernetes.system
    data_stream:
      type: metrics
      dataset: kubernetes.system
    metricsets:
      - system
{{- $defaults := (include "elasticagent.kubernetes.config.kubelet.system.default_vars" . ) | fromYaml -}}
{{- mergeOverwrite $defaults .Values.kubernetes.system.metrics.vars | toYaml | nindent 4 }}
{{- end -}}

{{- define "elasticagent.kubernetes.config.kubelet.system.default_vars" -}}
add_metadata: true
hosts:
- "https://${env.NODE_NAME}:10250"
period: "10s"
bearer_token_file: "/var/run/secrets/kubernetes.io/serviceaccount/token"
ssl.verification_mode: "none"
{{- end -}}