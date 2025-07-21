{{- define "gvList" -}}
{{- $groupVersions := . -}}
---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-reference.html
{{if eq ( markdownTemplateValue "eckVersion" ) "main" -}}
navigation_title: API Reference for the main branch
applies_to:
  deployment:
    eck: preview
{{- else -}}
navigation_title: API Reference for {{ markdownTemplateValue "eckVersion" }}
applies_to:
  deployment:
    eck: ga {{ markdownTemplateValue "eckVersion" }}
{{- end}}
---
% Generated documentation. Please do not edit.

# {{`{{eck}}`}} API Reference [k8s-api-reference]

## Packages
{{- range $groupVersions }}
* {{ markdownRenderGVLink . }}
{{- end }}

{{ range $groupVersions }}
{{ template "gvDetails" . }}
{{ end }}

{{- end -}}
