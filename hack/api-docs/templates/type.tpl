{{- define "type" -}}
{{- $type := . -}}
{{- if markdownShouldRenderType $type -}}

### {{ $type.Name  }} {{ if $type.IsAlias }}({{ markdownRenderTypeLink $type.UnderlyingType  }}) {{ end }} [#{{ markdownTypeID $type }}]

{{ $type.Doc }}

{{ if $type.References -}}
:::{admonition} Appears In:
{{- range $type.SortedReferences }}
* {{ markdownRenderTypeLink . }}
{{- end }}

:::
{{- end }}

{{ if $type.Members -}}
| Field | Description |
| --- | --- |
{{ if $type.GVK -}}
| *`apiVersion`* __string__ | `{{ $type.GVK.Group }}/{{ $type.GVK.Version }}` |
| *`kind`* __string__ | `{{ $type.GVK.Kind }}` | 
{{ end -}}

{{ range $type.Members -}}
| *`{{ .Name  }}`* __{{ markdownRenderType .Type }}__ | {{ template "type_members" . }} |
{{ end -}}
{{ end -}}

{{- end -}}
{{- end -}}
