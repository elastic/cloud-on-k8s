{{- range $envName, $sortedTests := .Tests }}
{{- range $test := $sortedTests.Failed }}

<p>
<details>
<summary>🐞 <code>{{ $test.Name }}</code> ~ {{ $envName }}</summary>

```
{{ $test.Error }}
```

</details>
</p>

{{- end }}
{{- end }}
