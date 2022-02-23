// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

const (
	E2EFleetPolicies = `
    xpack.fleet.packages:
    - name: system
      version: latest
    - name: elastic_agent
      version: latest
    - name: fleet_server
      version: latest
    - name: kubernetes
      # pinning this version as the next one introduced a kube-proxy host setting default that breaks this recipe,
      # see https://github.com/elastic/integrations/pull/1565 for more details
      version: 0.14.0
    xpack.fleet.agentPolicies:
    - name: Fleet Server on ECK policy
      id: eck-fleet-server
      namespace: default
      monitoring_enabled:
      - logs
      - metrics
      is_default_fleet_server: true
      package_policies:
      - name: fleet_server-1
        id: fleet_server-1
        package:
          name: fleet_server
    - name: Elastic Agent on ECK policy
      id: eck-agent
      namespace: default
      monitoring_enabled:
      - logs
      - metrics
      unenroll_timeout: 900
      is_default: true
      package_policies:
      - package:
          name: system
        name: system-1
      - package:
          name: kubernetes
        name: kubernetes-1`

	E2EAgentSystemIntegrationConfig = `id: 2d70a6f0-33a5-11eb-bb2f-418d0388a8cf
revision: 2
agent:
  monitoring:
    enabled: true
    use_output: default
    logs: true
    metrics: true
inputs:
  - id: 2e187fb0-33a5-11eb-bb2f-418d0388a8cf
    name: system-1
    revision: 1
    type: logfile
    use_output: default
    meta:
      package:
        name: system
        version: 0.9.1
    data_stream:
      namespace: default
    streams:
      - id: logfile-system.auth
        data_stream:
          dataset: system.auth
          type: logs
        paths:
          - /var/log/auth.log*
          - /var/log/secure*
        exclude_files:
          - .gz$
        multiline:
          pattern: ^\s
          match: after
        processors:
          - add_locale: {}
          - add_fields:
              target: ''
              fields:
                ecs.version: 1.5.0
      - id: logfile-system.syslog
        data_stream:
          dataset: system.syslog
          type: logs
        paths:
          - /var/log/messages*
          - /var/log/syslog*
        exclude_files:
          - .gz$
        multiline:
          pattern: ^\s
          match: after
        processors:
          - add_locale: {}
          - add_fields:
              target: ''
              fields:
                ecs.version: 1.5.0
  - id: 2e187fb0-33a5-11eb-bb2f-418d0388a8cf
    name: system-1
    revision: 1
    type: system/metrics
    use_output: default
    meta:
      package:
        name: system
        version: 0.9.1
    data_stream:
      namespace: default
    streams:
      - id: system/metrics-system.cpu
        data_stream:
          dataset: system.cpu
          type: metrics
        metricsets:
          - cpu
        cpu.metrics:
          - percentages
          - normalized_percentages
        period: 10s
      - id: system/metrics-system.diskio
        data_stream:
          dataset: system.diskio
          type: metrics
        metricsets:
          - diskio
        diskio.include_devices: {}
        period: 10s
      - id: system/metrics-system.filesystem
        data_stream:
          dataset: system.filesystem
          type: metrics
        metricsets:
          - filesystem
        period: 1m
        processors:
          - drop_event.when.regexp:
              system.filesystem.mount_point: ^/(sys|cgroup|proc|dev|etc|host|lib|snap)($|/)
      - id: system/metrics-system.fsstat
        data_stream:
          dataset: system.fsstat
          type: metrics
        metricsets:
          - fsstat
        period: 1m
        processors:
          - drop_event.when.regexp:
              system.fsstat.mount_point: ^/(sys|cgroup|proc|dev|etc|host|lib|snap)($|/)
      - id: system/metrics-system.load
        data_stream:
          dataset: system.load
          type: metrics
        metricsets:
          - load
        period: 10s
      - id: system/metrics-system.memory
        data_stream:
          dataset: system.memory
          type: metrics
        metricsets:
          - memory
        period: 10s
      - id: system/metrics-system.network
        data_stream:
          dataset: system.network
          type: metrics
        metricsets:
          - network
        period: 10s
        network.interfaces: {}
      - id: system/metrics-system.process
        data_stream:
          dataset: system.process
          type: metrics
        metricsets:
          - process
        period: 10s
        process.include_top_n.by_cpu: 5
        process.include_top_n.by_memory: 5
        process.cmdline.cache.enabled: true
        process.cgroups.enabled: false
        process.include_cpu_ticks: false
        processes:
          - .*
      - id: system/metrics-system.process_summary
        data_stream:
          dataset: system.process_summary
          type: metrics
        metricsets:
          - process_summary
        period: 10s
      - id: system/metrics-system.socket_summary
        data_stream:
          dataset: system.socket_summary
          type: metrics
        metricsets:
          - socket_summary
        period: 10s
      - id: system/metrics-system.uptime
        data_stream:
          dataset: system.uptime
          type: metrics
        metricsets:
          - uptime
        period: 10s
`

	E2EAgentSystemIntegrationPodTemplate = `spec:
  containers:
  - name: agent
    volumeMounts:
    - mountPath: /var/log
      name: varlog
  dnsPolicy: ClusterFirstWithHostNet
  terminationGracePeriodSeconds: 30
  volumes:
  - hostPath:
      path: /var/log
    name: varlog
`

	E2EAgentMultipleOutputConfig = `id: 2d70a6f0-33a5-11eb-bb2f-418d0388a8cf
revision: 2
agent:
  monitoring:
    enabled: true
    use_output: monitoring
    logs: true
    metrics: true
inputs:
  - id: 2e187fb0-33a5-11eb-bb2f-418d0388a8cf
    name: system-1
    revision: 1
    type: logfile
    use_output: default
    meta:
      package:
        name: system
        version: 0.9.1
    data_stream:
      namespace: default
    streams:
      - id: logfile-system.auth
        data_stream:
          dataset: system.auth
          type: logs
        paths:
          - /var/log/auth.log*
          - /var/log/secure*
        exclude_files:
          - .gz$
        multiline:
          pattern: ^\s
          match: after
        processors:
          - add_locale: {}
          - add_fields:
              target: ''
              fields:
                ecs.version: 1.5.0
      - id: logfile-system.syslog
        data_stream:
          dataset: system.syslog
          type: logs
        paths:
          - /var/log/messages*
          - /var/log/syslog*
        exclude_files:
          - .gz$
        multiline:
          pattern: ^\s
          match: after
        processors:
          - add_locale: {}
          - add_fields:
              target: ''
              fields:
                ecs.version: 1.5.0
  - id: 2e187fb0-33a5-11eb-bb2f-418d0388a8cf
    name: system-1
    revision: 1
    type: system/metrics
    use_output: default
    meta:
      package:
        name: system
        version: 0.9.1
    data_stream:
      namespace: default
    streams:
      - id: system/metrics-system.cpu
        data_stream:
          dataset: system.cpu
          type: metrics
        metricsets:
          - cpu
        cpu.metrics:
          - percentages
          - normalized_percentages
        period: 10s
      - id: system/metrics-system.diskio
        data_stream:
          dataset: system.diskio
          type: metrics
        metricsets:
          - diskio
        diskio.include_devices: {}
        period: 10s
      - id: system/metrics-system.filesystem
        data_stream:
          dataset: system.filesystem
          type: metrics
        metricsets:
          - filesystem
        period: 1m
        processors:
          - drop_event.when.regexp:
              system.filesystem.mount_point: ^/(sys|cgroup|proc|dev|etc|host|lib|snap)($|/)
      - id: system/metrics-system.fsstat
        data_stream:
          dataset: system.fsstat
          type: metrics
        metricsets:
          - fsstat
        period: 1m
        processors:
          - drop_event.when.regexp:
              system.fsstat.mount_point: ^/(sys|cgroup|proc|dev|etc|host|lib|snap)($|/)
      - id: system/metrics-system.load
        data_stream:
          dataset: system.load
          type: metrics
        metricsets:
          - load
        period: 10s
      - id: system/metrics-system.memory
        data_stream:
          dataset: system.memory
          type: metrics
        metricsets:
          - memory
        period: 10s
      - id: system/metrics-system.network
        data_stream:
          dataset: system.network
          type: metrics
        metricsets:
          - network
        period: 10s
        network.interfaces: {}
      - id: system/metrics-system.process
        data_stream:
          dataset: system.process
          type: metrics
        metricsets:
          - process
        period: 10s
        process.include_top_n.by_cpu: 5
        process.include_top_n.by_memory: 5
        process.cmdline.cache.enabled: true
        process.cgroups.enabled: false
        process.include_cpu_ticks: false
        processes:
          - .*
      - id: system/metrics-system.process_summary
        data_stream:
          dataset: system.process_summary
          type: metrics
        metricsets:
          - process_summary
        period: 10s
      - id: system/metrics-system.socket_summary
        data_stream:
          dataset: system.socket_summary
          type: metrics
        metricsets:
          - socket_summary
        period: 10s
      - id: system/metrics-system.uptime
        data_stream:
          dataset: system.uptime
          type: metrics
        metricsets:
          - uptime
        period: 10s
`

	E2EAgentFleetModePodTemplate = `spec:
  automountServiceAccountToken: true
  securityContext:
    runAsUser: 0
`
)
