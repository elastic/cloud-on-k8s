{{- define "agent.cloud_defend.config.enabled" -}}
enabled: {{ .Values.cloudDefend.enabled }}
{{- end -}}

{{- define "agent.cloud_defend.config.input" -}}
- id: cloud_defend/control-cloud_defend
  {{/* revision field is validated and required for defend for containers */}}
  revision: 1
  name: D4C
  type: cloud_defend/control
  data_stream:
    namespace: {{ .Values.kubernetes.namespace }}
  use_output: default
  {{/* package_policy_id field is validated and required for defend 4 containers */}}
  package_policy_id: 05c82775-6f4a-4531-9907-55f958e8d5e4
  streams:
    - id: cloud_defend/control-cloud_defend.alerts
      data_stream:
        type: logs
        dataset: cloud_defend.alerts
      security-policy:
        {{- if and .Values.cloudDefend.process.responses .Values.cloudDefend.process.selectors}}
        process:
          {{- .Values.cloudDefend.process | toYaml | nindent 10 }}
        {{- end }}
        {{- if and .Values.cloudDefend.file.responses .Values.cloudDefend.file.selectors}}
        file:
          {{- .Values.cloudDefend.file | toYaml | nindent 10 }}
        {{- end }}
    - id: cloud_defend/control-cloud_defend.file
      data_stream:
        type: logs
        dataset: cloud_defend.file
      file-config: null
    - id: cloud_defend/control-cloud_defend.metrics
      data_stream:
        type: metrics
        dataset: cloud_defend.metrics
      metricsets:
        - cloud_defend
      hosts: null
      period: 24h
    - id: cloud_defend/control-cloud_defend.process
      data_stream:
        type: logs
        dataset: cloud_defend.process
      process-config: null
  meta:
    package:
      name: cloud_defend
      version: {{.Values.cloudDefend.version}}
{{- end -}}