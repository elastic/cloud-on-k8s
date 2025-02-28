{{- define "gvList" -}}
{{- $groupVersions := . -}}
---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-reference.html
navigation_title: API reference
applies_to:
  deployment:
    eck: all
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
