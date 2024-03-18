{{- define "elasticagent.kubernetes.config.audit_logs.init" -}}
{{- if eq $.Values.kubernetes.containers.audit_logs.enabled true -}}
{{- $preset := $.Values.eck_agent.presets.perNode -}}
{{- $inputVal := (include "elasticagent.kubernetes.config.audit_logs.input" $ | fromYamlArray) -}}
{{- include "elasticagent.preset.mutate.inputs" (list $ $preset $inputVal) -}}
{{- include "elasticagent.preset.applyOnce" (list $ $preset "elasticagent.kubernetes.pernode.preset") -}}
{{- end -}}
{{- end -}}

{{/*
Config input for kube audit_logs_filestream
*/}}
{{- define "elasticagent.kubernetes.config.audit_logs.input" -}}
- id: filestream-kubernetes.audit_logs
  type: filestream
  data_stream:
    namespace: {{.Values.kubernetes.namespace}}
  use_output: {{ .Values.kubernetes.output }}
  streams:
  - id: filestream-kubernetes.audit_logs
    data_stream:
      type: logs
      dataset: kubernetes.audit_logs
    paths:
      - /var/log/kubernetes/kube-apiserver-audit.log
    exclude_files:
      - .gz$
    parsers:
      - ndjson:
          add_error_key: true
          target: kubernetes_audit
    processors:
      - rename:
          fields:
            - from: kubernetes_audit
              to: kubernetes.audit
      - drop_fields:
          when:
            has_fields: kubernetes.audit.responseObject
          fields:
            - kubernetes.audit.responseObject.metadata
      - drop_fields:
          when:
            has_fields: kubernetes.audit.requestObject
          fields:
            - kubernetes.audit.requestObject.metadata
      - script:
          lang: javascript
          id: dedot_annotations
          source: |
            function process(event) {
              var audit = event.Get("kubernetes.audit");
              for (var annotation in audit["annotations"]) {
                var annotation_dedoted = annotation.replace(/\./g,'_')
                event.Rename("kubernetes.audit.annotations."+annotation, "kubernetes.audit.annotations."+annotation_dedoted)
              }
              return event;
            } function test() {
              var event = process(new Event({ "kubernetes": { "audit": { "annotations": { "authorization.k8s.io/decision": "allow", "authorization.k8s.io/reason": "RBAC: allowed by ClusterRoleBinding \"system:kube-scheduler\" of ClusterRole \"system:kube-scheduler\" to User \"system:kube-scheduler\"" } } } }));
              if (event.Get("kubernetes.audit.annotations.authorization_k8s_io/decision") !== "allow") {
                  throw "expected kubernetes.audit.annotations.authorization_k8s_io/decision === allow";
              }
            }
{{- end -}}