{{- define "type" -}}
[id="{{ anchorIDForType . | safeIdentifier }}"]
[float]
==== {{ .Name.Name }} {{- if eq .Kind "Alias" -}}(`{{.Underlying}}` alias){{ end }}

{{ safe (renderComments .CommentLines) }}


{{ with (typeReferences .) -}}
{{ if . -}}
.Appears in:
****
{{- $prev := "" }}
{{- range . }}
{{- if $prev }}, {{ end -}}
{{- $prev = . }}
- {{ template "link_template" . }}
{{- end }}
****
{{- end }}
{{- end }}


{{- if .Members }}
[cols="20a,80a", options="header"]
|===
|Field |Description

{{- if isExportedType . }}
| *`apiVersion`*  +
_string_
| `{{ apiGroup . }}`

| *`kind`*  +
_string_
| `{{ .Name.Name }}`
{{- end }}
{{ template "members" .}}
|===
{{- end }}
{{- end }}
