{{- define "agent.clouddefend.pernode.preset" -}}
{{- include "elasticagent.preset.mutate.rules" (list $ $.Values.eck_agent.presets.perNode "agent.clouddefend.pernode.preset.rules") -}}
{{- include "elasticagent.preset.mutate.volumemounts" (list $ $.Values.eck_agent.presets.perNode "agent.clouddefend.pernode.preset.volumemounts") -}}
{{- include "elasticagent.preset.mutate.volumes" (list $ $.Values.eck_agent.presets.perNode "agent.clouddefend.pernode.preset.volumes") -}}
{{- include "elasticagent.preset.mutate.securityContext.capabilities.add" (list $ $.Values.eck_agent.presets.perNode "agent.clouddefend.pernode.securityContext.capabilities.add") -}}
{{- include "elasticagent.preset.mutate.elasticsearchrefs.byname" (list $ $.Values.eck_agent.presets.perNode $.Values.cloudDefend.output)}}
{{- end -}}

{{- define "agent.clouddefend.pernode.preset.rules" -}}
rules:
- apiGroups: [""] # "" indicates the core API group
  resources:
  - namespaces
  - pods
  - persistentvolumes
  - persistentvolumeclaims
  - persistentvolumeclaims/status
  - nodes
  - nodes/metrics
  - configmaps
  - nodes/proxy
  - nodes/stats
  - services
  - events
  verbs:
  - get
  - watch
  - list
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - get
  - watch
  - list
- nonResourceURLs:
  - /metrics
  verbs:
  - get
  - watch
  - list
- apiGroups: ["coordination.k8s.io"]
  resources:
  - leases
  verbs:
  - get
  - create
  - update
- nonResourceURLs:
  - /healthz
  - /healthz/*
  - /livez
  - /livez/*
  - /metrics
  - /metrics/slis
  - /readyz
  - /readyz/*
  verbs:
  - get
- apiGroups: ["apps"]
  resources:
  - replicasets
  - deployments
  - daemonsets
  - statefulsets
  verbs:
  - get
  - list
  - watch
- apiGroups: ["batch"]
  resources:
  - jobs
  - cronjobs
  verbs:
  - get
  - list
  - watch
{{- end -}}

{{- define "agent.clouddefend.pernode.preset.volumemounts" -}}
extraVolumeMounts:
- name: boot
  mountPath: /boot
  readOnly: true
- name: sys-kernel-debug
  mountPath: /sys/kernel/debug
- name: sys-fs-bpf
  mountPath: /sys/fs/bpf
- name: sys-kernel-security
  mountPath: /sys/kernel/security
  readOnly: true
{{- end -}}

{{- define "agent.clouddefend.pernode.preset.volumes" -}}
extraVolumes:
- name: boot
  hostPath:
    path: /boot
- name: sys-kernel-debug
  hostPath:
    path: /sys/kernel/debug
- name: sys-fs-bpf
  hostPath:
    path: /sys/fs/bpf
- name: sys-kernel-security
  hostPath:
    path: /sys/kernel/security
{{- end -}}

{{- define "agent.clouddefend.pernode.securityContext.capabilities.add" -}}
securityContext:
  capabilities:
    add:
      - BPF # (since Linux 5.8) allows loading of BPF programs, create most map types, load BTF, iterate programs and maps.
      - PERFMON # (since Linux 5.8) allows attaching of BPF programs used for performance metrics and observability operations.
      - SYS_RESOURCE # Allow use of special resources or raising of resource limits. Used by 'Defend for Containers' to modify 'rlimit_memlock'
{{- end -}}
