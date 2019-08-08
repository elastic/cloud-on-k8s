{{- define "members" -}}
{{- range .Members -}}
{{- if not (hiddenMember .)}}
| `*{{ fieldName . }}*` +
_{{- if linkForType .Type -}}link:{{ linkForType .Type}}[{{ typeDisplayName .Type }}] {{- else -}} {{ typeDisplayName .Type }} {{- end -}}_
| {{- if fieldEmbedded . }}
(Members of `{{ fieldName . }}` are embedded into this type.)
{{- end}}
{{- if isOptionalMember .}}
_(Optional)_
{{- end }}
{{ safe (renderComments .CommentLines) }}

{{- if and (eq (.Type.Name.Name) "ObjectMeta") }}
Refer to the Kubernetes API documentation for the fields of the `metadata` field.
{{- end }}

{{- if or (eq (fieldName .) "spec") }}
{{ template "spec" .Type }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
