{{- define "type" -}}
[id="{{ anchorIDForType . | safeIdentifier }}"]
==== {{ .Name.Name }} {{- if eq .Kind "Alias" -}}(`{{.Underlying}}` alias){{ end }}
{{- with (typeReferences .) }}
(*Appears on:*
{{- $prev := "" }}
{{- range . }}
{{- if $prev }}, {{ end -}}
{{- $prev = . }}
link:{{ linkForType . }}[{{ typeDisplayName . }}]
{{- end }}
)
{{- end }}

{{ safe (renderComments .CommentLines) }}

{{- if .Members }}
[cols="1a,5a", options="header"]
|===
|Field |Description

{{- if isExportedType . }}
|`*apiVersion*` +
_string_
|`{{ apiGroup .  }}`

|`*kind*` +
_string_
|`{{ .Name.Name }}`
{{- end }}
{{ template "members" .}}
|===
{{- end }}
{{- end }}
