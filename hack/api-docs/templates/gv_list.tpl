{{- define "gvList" -}}
{{- $groupVersions := . -}}

// Generated documentation. Please do not edit.
:page_id: api-reference
:anchor_prefix: k8s-api

ifdef::env-github[]
****
link:https://www.elastic.co/guide/en/cloud-on-k8s/master/k8s-{page_id}.html[View this document on the Elastic website]
****
endif::[]

[id="{p}-{page_id}"]
= API Reference

.Packages
{{- range $groupVersions }}
- {{ asciidocRenderGVLink . }}
{{- end }}

{{ range $groupVersions }}
{{ template "gvDetails" . }}
{{ end }}

{{- end -}}
