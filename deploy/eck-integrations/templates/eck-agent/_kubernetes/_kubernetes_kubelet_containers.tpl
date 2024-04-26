{{- define "elasticagent.kubernetes.config.kubelet.containers.init" -}}
{{- if eq ((.Values.kubernetes.metrics).enabled) false -}}
{{- $_ := set $.Values.kubernetes.containers.metrics "enabled" false -}}
{{- else -}}
{{- if eq $.Values.kubernetes.containers.metrics.enabled true -}}
{{- $preset := $.Values.agent.presets.perNode -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.kubelet.containers.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.kubernetes.config.kubelet.containers.input" -}}
- id: kubernetes/metrics-kubernetes.container
  type: kubernetes/metrics
  data_stream:
    namespace: {{ $.Values.kubernetes.namespace }}
  use_output: {{ $.Values.kubernetes.output }}
  streams:
  - id: kubernetes/metrics-kubernetes.container
    data_stream:
      type: metrics
      dataset: kubernetes.container
    metricsets:
      - container
{{- $defaults := (include "elasticagent.kubernetes.config.kubelet.containers.default_vars" . ) | fromYaml -}}
{{- mergeOverwrite $defaults .Values.kubernetes.containers.metrics.vars | toYaml | nindent 4 }}
{{- end -}}

{{- define "elasticagent.kubernetes.config.kubelet.containers.default_vars" -}}
add_metadata: true
hosts:
- "https://${env.NODE_NAME}:10250"
period: "10s"
bearer_token_file: "/var/run/secrets/kubernetes.io/serviceaccount/token"
ssl.verification_mode: "none"
{{- end -}}