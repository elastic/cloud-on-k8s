// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build beat || e2e

package beat

var (
	E2EFilebeatConfig = `filebeat:
  autodiscover:
    providers:
    - type: kubernetes
      node: ${NODE_NAME}
      hints:
        enabled: true
        default_config:
          type: container
          paths:
          - /var/log/containers/*${data.kubernetes.container.id}.log
processors:
- add_cloud_metadata: {}
- add_host_metadata: {}
`

	E2EFilebeatPodTemplate = `spec:
  automountServiceAccountToken: true # some older Beat versions are depending on this settings presence in k8s context
  containers:
  - name: filebeat
    resources:
      requests:
        cpu: 100m
        memory: 512Mi
      limits:
        cpu: 100m
        memory: 512Mi
    volumeMounts:
    - mountPath: /var/lib/docker/containers
      name: varlibdockercontainers
    - mountPath: /var/log/containers
      name: varlogcontainers
    - mountPath: /var/log/pods
      name: varlogpods
    env:
    - name: NODE_NAME 
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
  dnsPolicy: ClusterFirstWithHostNet
  hostNetwork: true
  securityContext:
    runAsUser: 0
  serviceAccount: elastic-beat-filebeat-sample
  terminationGracePeriodSeconds: 30
  volumes:
  - hostPath:
      path: /var/lib/docker/containers
    name: varlibdockercontainers
  - hostPath:
      path: /var/log/containers
    name: varlogcontainers
  - hostPath:
      path: /var/log/pods
    name: varlogpods
`

	e2eHeartBeatConfigTpl = `
heartbeat.monitors:
- type: tcp
  schedule: '@every 5s'
  hosts: ["%s.%s.svc:9200"]
`

	e2eHeartbeatPodTemplate = `spec:
  dnsPolicy: ClusterFirstWithHostNet
  hostNetwork: true
  securityContext:
    runAsUser: 0
`

	e2eMetricbeatConfig = `metricbeat:
  autodiscover:
    providers:
    - hints:
        default_config: {}
        enabled: "true"
      node: ${NODE_NAME}
      type: kubernetes
  modules:
  - module: system
    period: 10s
    metricsets:
    - cpu
    - load
    - memory
    - network
    - process
    - process_summary
    process:
      include_top_n:
        by_cpu: 5
        by_memory: 5
    processes:
    - .*
  - module: system
    period: 1m
    metricsets:
    - filesystem
    - fsstat
    processors:
    - drop_event:
        when:
          regexp:
            system:
              filesystem:
                mount_point: ^/(sys|cgroup|proc|dev|etc|host|lib)($|/)
  - module: kubernetes
    period: 10s
    node: ${NODE_NAME}
    hosts:
    - https://${HOSTNAME}:10250
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    ssl:
      verification_mode: none
    metricsets:
    - node
    - system
    - pod
    - container
    - volume
processors:
- add_cloud_metadata: {}
- add_host_metadata: {}
`

	e2eMetricbeatPodTemplate = `spec:
  automountServiceAccountToken: true # some older Beat versions are depending on this settings presence in k8s context
  containers:
  - args:
    - -e
    - -c
    - /etc/beat.yml
    - -system.hostfs=/hostfs
    name: metricbeat
    env:
    - name: NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    volumeMounts:
    - mountPath: /hostfs/sys/fs/cgroup
      name: cgroup
    - mountPath: /var/run/docker.sock
      name: dockersock
    - mountPath: /hostfs/proc
      name: proc
  dnsPolicy: ClusterFirstWithHostNet
  hostNetwork: true
  securityContext:
    runAsUser: 0
  serviceAccount: elastic-beat-metricbeat-sample
  terminationGracePeriodSeconds: 30
  volumes:
  - hostPath:
      path: /sys/fs/cgroup
    name: cgroup
  - hostPath:
      path: /var/run/docker.sock
    name: dockersock
  - hostPath:
      path: /proc
    name: proc`

	e2eAuditbeatConfig = `auditbeat.modules:
- module: file_integrity
  paths:
  - /hostfs/bin
  - /hostfs/usr/bin
  - /hostfs/sbin
  - /hostfs/usr/sbin
  - /hostfs/etc
  exclude_files:
  - '(?i)\.sw[nop]$'
  - '~$'
  - '/\.git($|/)'
  scan_at_start: true
  scan_rate_per_sec: 50 MiB
  max_file_size: 100 MiB
  hash_types: [sha1]
  recursive: true
- module: auditd
  audit_rules: |
    # Executions
    -a always,exit -F arch=b64 -S execve,execveat -k exec

    # Unauthorized access attempts (adjusted to be compatible with amd64 and arm64)
    -a always,exit -F arch=b64 -S truncate,ftruncate,openat,open_by_handle_at -F exit=-EACCES -k access
    -a always,exit -F arch=b64 -S truncate,ftruncate,openat,open_by_handle_at -F exit=-EPERM -k access

processors:
  - add_cloud_metadata: {}
  - add_process_metadata:
      match_pids: ['process.pid']
  - add_kubernetes_metadata:
      node: ${NODE_NAME}
      default_indexers.enabled: false
      default_matchers.enabled: false
      indexers:
        - container:
      matchers:
        - fields.lookup_fields: ['container.id']
`

	e2eAuditbeatPodTemplate = `spec:
  hostPID: true  # Required by auditd module
  dnsPolicy: ClusterFirstWithHostNet
  hostNetwork: true
  automountServiceAccountToken: true # some older Beat versions are depending on this settings presence in k8s context
  securityContext:
    runAsUser: 0
  volumes:
  - name: bin
    hostPath:
      path: /bin
  - name: usrbin
    hostPath:
      path: /usr/bin
  - name: sbin
    hostPath:
      path: /sbin
  - name: usrsbin
    hostPath:
      path: /usr/sbin
  - name: etc
    hostPath:
      path: /etc
  - name: run-containerd
    hostPath:
      path: /run/containerd
      type: DirectoryOrCreate
  containers:
  - name: auditbeat
    env:
    - name: NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    securityContext:
      capabilities:
        add:
        # Capabilities needed for auditd module
        - 'AUDIT_READ'
        - 'AUDIT_WRITE'
        - 'AUDIT_CONTROL'
    volumeMounts:
    - name: bin
      mountPath: /hostfs/bin
      readOnly: true
    - name: sbin
      mountPath: /hostfs/sbin
      readOnly: true
    - name: usrbin
      mountPath: /hostfs/usr/bin
      readOnly: true
    - name: usrsbin
      mountPath: /hostfs/usr/sbin
      readOnly: true
    - name: etc
      mountPath: /hostfs/etc
      readOnly: true
    # Directory with root filesystems of containers executed with containerd, this can be
    # different with other runtimes. This volume is needed to monitor the file integrity
    # of files in containers.
    - name: run-containerd
      mountPath: /run/containerd
      readOnly: true
`

	e2ePacketbeatConfig = `packetbeat.interfaces.device: any
packetbeat.protocols:
- type: dns
  ports: [53]
  include_authorities: true
  include_additionals: true
- type: http
  ports: [80, 8000, 8080, 9200]
packetbeat.flows:
  timeout: 30s
  period: 10s
processors:
  - add_cloud_metadata:
  - add_kubernetes_metadata:
      node: ${NODE_NAME}
      indexers:
      - ip_port:
      matchers:
      - field_format:
          format: '%{[ip]}:%{[port]}'`

	e2ePacketbeatPodTemplate = `
spec:
  terminationGracePeriodSeconds: 30
  hostNetwork: true
  automountServiceAccountToken: true # some older Beat versions are depending on this settings presence in k8s context
  dnsPolicy: ClusterFirstWithHostNet
  containers:
  - name: packetbeat
    env:
    - name: NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    securityContext:
      runAsUser: 0
      capabilities:
        add:
        - NET_ADMIN
`
	e2eJournalbeatConfig = `journalbeat.inputs:
- paths: []
  seek: cursor
  cursor_seek_fallback: tail
processors:
- add_kubernetes_metadata:
    node: "${NODE_NAME}"
    in_cluster: true
    default_indexers.enabled: false
    default_matchers.enabled: false
    indexers:
      - container:
    matchers:
      - fields:
          lookup_fields: ["container.id"]
- decode_json_fields:
    fields: ["message"]
    process_array: false
    max_depth: 1
    target: ""
    overwrite_keys: true
`

	e2eJournalbeatPodTemplate = `
spec:
  automountServiceAccountToken: true # some older Beat versions are depending on this settings presence in k8s context
  dnsPolicy: ClusterFirstWithHostNet
  containers:
  - name: journalbeat
    volumeMounts:
    - mountPath: /var/log/journal
      name: var-journal
    - mountPath: /run/log/journal
      name: run-journal
    - mountPath: /etc/machine-id
      name: machine-id
    env:
    - name: NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
  hostNetwork: true
  securityContext:
    runAsUser: 0
  terminationGracePeriodSeconds: 30
  volumes:
  - hostPath:
      path: /var/log/journal
    name: var-journal
  - hostPath:
      path: /run/log/journal
    name: run-journal
  - hostPath:
      path: /etc/machine-id
    name: machine-id
`
)
