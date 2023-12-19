{{- define "agent.kubernetes.config.container_logs.enabled" -}}
enabled: {{ .Values.kubernetes.containers.logs.enabled }}
{{- end -}}

{{/*
Config input for container logs
*/}}
{{- define "agent.kubernetes.config.container_logs.input" -}}
- id: filestream-container-logs
  type: filestream
  data_stream:
    namespace: {{ .Values.kubernetes.namespace }}
  use_output: default
  streams:
  - id: kubernetes-container-logs-${kubernetes.pod.name}-${kubernetes.container.id}
    data_stream:
      dataset: kubernetes.container_logs
    paths:
      - '/var/log/containers/*${kubernetes.container.id}.log'
    prospector.scanner.symlinks: {{ dig "vars" "symlinks" true .Values.kubernetes.containers.logs }}
    parsers:
      - container:
          stream: {{ dig "vars" "stream" "all" .Values.kubernetes.containers.logs }}
          format: {{ dig "vars" "format" "auto" .Values.kubernetes.containers.logs }}
    processors:
      - add_fields:
          target: kubernetes
          fields:
            annotations.elastic_co/dataset: '${kubernetes.annotations.elastic.co/dataset|""}'
            annotations.elastic_co/namespace: '${kubernetes.annotations.elastic.co/namespace|""}'
      - drop_fields:
          fields:
            - kubernetes.annotations.elastic_co/dataset
          when:
            equals:
              kubernetes.annotations.elastic_co/dataset: ''
          ignore_missing: true
      - drop_fields:
          fields:
            - kubernetes.annotations.elastic_co/namespace
          when:
            equals:
              kubernetes.annotations.elastic_co/namespace: ''
          ignore_missing: true
  meta:
    package:
      name: kubernetes
      version: {{ .Values.kubernetes.version }}
{{- end -}}