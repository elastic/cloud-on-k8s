{{- define "elasticagent.kubernetes.config.kubelet.pods.init" -}}
{{- if eq ((.Values.kubernetes.metrics).enabled) false -}}
{{- $_ := set $.Values.kubernetes.pods.metrics "enabled" false -}}
{{- else -}}
{{- if eq $.Values.kubernetes.pods.metrics.enabled true -}}
{{- $preset := $.Values.agent.presets.perNode -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.kubelet.pods.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "elasticagent.kubernetes.config.kubelet.pods.input" -}}
- id: kubernetes/metrics-kubernetes.pod
  type: kubernetes/metrics
  data_stream:
    namespace: {{ $.Values.kubernetes.namespace }}
  use_output: {{ $.Values.kubernetes.output }}
  streams:
  - id: kubernetes/metrics-kubernetes.pod
    data_stream:
      type: metrics
      dataset: kubernetes.pod
    metricsets:
      - pod
{{- $defaults := (include "elasticagent.kubernetes.config.kubelet.pods.default_vars" . ) | fromYaml -}}
{{- mergeOverwrite $defaults .Values.kubernetes.pods.metrics.vars | toYaml | nindent 4 }}
{{- end -}}

{{- define "elasticagent.kubernetes.config.kubelet.pods.default_vars" -}}
add_metadata: true
hosts:
- "https://${env.NODE_NAME}:10250"
period: "10s"
bearer_token_file: "/var/run/secrets/kubernetes.io/serviceaccount/token"
ssl.verification_mode: "none"
{{- end -}}