{{- define "elasticagent.kubernetes.config.kubelet.nodes.init" -}}
{{- if eq ((.Values.kubernetes.metrics).enabled) false -}}
{{- $_ := set $.Values.kubernetes.nodes.metrics "enabled" false -}}
{{- else -}}
{{- if eq $.Values.kubernetes.nodes.metrics.enabled true -}}
{{- $preset := $.Values.agent.presets.perNode -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.kubelet.nodes.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.kubernetes.config.kubelet.nodes.input" -}}
- id: kubernetes/metrics-kubernetes.node
  type: kubernetes/metrics
  data_stream:
    namespace: {{ $.Values.kubernetes.namespace }}
  use_output: {{ $.Values.kubernetes.output }}
  streams:
  - id: kubernetes/metrics-kubernetes.node
    data_stream:
      type: metrics
      dataset: kubernetes.node
    metricsets:
      - node
{{- $defaults := (include "elasticagent.kubernetes.config.kubelet.nodes.default_vars" . ) | fromYaml -}}
{{- mergeOverwrite $defaults .Values.kubernetes.nodes.metrics.vars | toYaml | nindent 4 }}
{{- end -}}

{{- define "elasticagent.kubernetes.config.kubelet.nodes.default_vars" -}}
add_metadata: true
hosts:
- "https://${env.NODE_NAME}:10250"
period: "10s"
bearer_token_file: "/var/run/secrets/kubernetes.io/serviceaccount/token"
ssl.verification_mode: "none"
{{- end -}}