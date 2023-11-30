{{/*
Config input for kube audit_logs_filestream
*/}}
{{- define "kubernetes.config.audit_logs_filestream.input" -}}
{{- if default .control_plane.audit_logs_filestream.enabled false -}}
- id: filestream-audit-logs
  revision: 1
  name: kubernetes
  type: filestream
  data_stream:
    namespace: {{.namespace}}
  use_output: default
  package_policy_id: {{.integrationID}}
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
  meta:
    package:
      name: kubernetes
      version: {{ .version }}
{{- end -}}
{{- end -}}