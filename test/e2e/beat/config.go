// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

var (
	e2eFilebeatConfig = `filebeat:
  autodiscover:
    providers:
    - type: kubernetes
      host: ${HOSTNAME}
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

	e2eFilebeatPodTemplate = `spec:
  automountServiceAccountToken: true
  containers:
  - name: filebeat
    volumeMounts:
    - mountPath: /var/lib/docker/containers
      name: varlibdockercontainers
    - mountPath: /var/log/containers
      name: varlogcontainers
    - mountPath: /var/log/pods
      name: varlogpods
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
      host: ${HOSTNAME}
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
    host: ${HOSTNAME}
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
  automountServiceAccountToken: true
  containers:
  - args:
    - -e
    - -c
    - /etc/beat.yml
    - -system.hostfs=/hostfs
    name: metricbeat
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
)
