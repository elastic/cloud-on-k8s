{{- define "members" -}}
{{- range .Members -}}
{{- if not (hiddenMember .)}}
| *`{{ fieldName . }}`* +
{{- if linkForType .Type }}
_link:{{ linkForType .Type}}[$${{ typeDisplayName .Type }}$$]_
{{- else }}
_{{ typeDisplayName .Type }}_
{{- end }}
| {{- if fieldEmbedded . }}
(Members of `{{ fieldName . }}` are embedded into this type.)
{{- end}}
{{- if isOptionalMember .}}
_(Optional)_
{{- end }}
{{- $comments := safe (renderComments .CommentLines) -}}
{{- if $comments }}
{{ $comments }}
{{- else }}
---
{{- end }}

{{- if and (eq (.Type.Name.Name) "ObjectMeta") }}
Refer to the Kubernetes API documentation for the fields of the `metadata` field.
{{- end }}

{{- if or (eq (fieldName .) "spec") }}
{{ template "spec" .Type }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
