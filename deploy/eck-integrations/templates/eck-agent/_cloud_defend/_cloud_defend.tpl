{{- define "agent.cloud_defend.config.enabled" -}}
enabled: {{ .Values.cloudDefend.enabled }}
{{- end -}}

{{- define "agent.cloud_defend.config.input" -}}
- id: cloud_defend/control-cloud_defend
  type: cloud_defend/control
  data_stream:
    namespace: {{ .Values.kubernetes.namespace }}
  use_output: default
  streams:
    - id: cloud_defend/control-cloud_defend.alerts
      data_stream:
        type: logs
        dataset: cloud_defend.alerts
      security-policy:
        process:
          {{- .Values.cloudDefend.process | toYaml | nindent 10 }}
        file:
          {{- .Values.cloudDefend.file | toYaml | nindent 10 }}
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