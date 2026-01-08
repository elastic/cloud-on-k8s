{{- define "gvList" -}}
{{- $groupVersions := . -}}
---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-reference.html
{{if eq ( markdownTemplateValue "eckVersion" ) "main" -}}
navigation_title: current
applies_to:
  deployment:
    eck: preview
{{- else -}}
navigation_title: {{ markdownTemplateValue "eckVersionShort" }}
applies_to:
  deployment:
    eck: ga ={{ markdownTemplateValue "eckVersionShort" }}
{{- end}}
---
% Generated documentation. Please do not edit.

# {{`{{eck}}`}} API Reference for {{ markdownTemplateValue "eckVersionShort" }} [k8s-api-reference-{{ markdownTemplateValue "eckVersionShort" }}]

## Packages
{{- range $groupVersions }}
* {{ markdownRenderGVLink . }}
{{- end }}

{{ range $groupVersions }}
{{ template "gvDetails" . }}
{{ end }}

{{- end -}}
