## Cluster cha
```yaml
apiVersion: everest.example.com/v1alpha1
kind: Cluster
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: >
      {"apiVersion":"everest.example.com/v1alpha1","kind":"Cluster","metadata":{"annotations":{},"name":"demo-cluster-4","namespace":"default"},"spec":{"engines":[{"category":"database","config":{"kv":{"archive_mode":"on","archive_timeout":"6min","dynamic_shared_memory_type":"posix","log_destination":"csvlog","log_directory":"/controller/log"}},"credentialsSecretNames":["app-user-secret-cnpg","readonly-user-secret-cnpg"],"engineType":"cnpg","monitoring":{"enabled":true},"name":"demo-pg-4","priority":1,"replicas":2,"storage":{"class":"csi-sc-viettelplugin-hdd","enabled":true,"size":"10Gi"},"version":"16.1"}]}}
  creationTimestamp: '2025-12-29T07:49:17Z'
  finalizers:
    - everest.example.com/cluster-finalizer
  generation: 6
  name: demo-cluster-4
  namespace: default
  resourceVersion: '75376958'
  selfLink: >-
    /apis/everest.example.com/v1alpha1/namespaces/default/clusters/demo-cluster-4
  uid: 1be893ed-eeab-40d6-a38b-c6c687a59180
spec:
  engines:
    - backup:
        enabled: false
      category: database
      config:
        kv:
          archive_mode: 'on'
          archive_timeout: 6min
          dynamic_shared_memory_type: posix
          log_destination: csvlog
          log_directory: /controller/log
      credentialsSecretNames:
        - app-user-secret-cnpg
        - readonly-user-secret-cnpg
      engineType: cnpg
      expose:
        enabled: false
      monitoring:
        enabled: true
      name: demo-pg-4
      priority: 1
      replicas: 4
      resources:
        cpu: ''
        memory: ''
      storage:
        class: csi-sc-viettelplugin-hdd
        enabled: true
        size: 10Gi
      version: '16.1'
status:
  engines:
    - conditions:
        - lastTransitionTime: '2025-12-29T09:22:19Z'
          message: Cluster is Ready
          reason: ClusterIsReady
          status: 'True'
          type: Ready
        - lastTransitionTime: '2025-12-29T07:50:23Z'
          message: Continuous archiving is working
          reason: ContinuousArchivingSuccess
          status: 'True'
          type: ContinuousArchiving
      name: demo-pg-4
      phase: Ready
      ready: 4
      total: 4
      type: cnpg
  phase: Ready
```


## Cluster CNPG
```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  annotations:
    cnpg.io/forceSwitchoverTo: demo-pg-4-2
    cnpg.io/switchover: demo-pg-4-2
    freelens.app/resource-version: v1
  creationTimestamp: '2025-12-29T07:49:38Z'
  finalizers:
    - everest.example.com/finalizer
  generation: 4
  labels:
    app.kubernetes.io/managed-by: db-operator
    everest.example.com/cluster: demo-cluster-4
    everest.example.com/engine: cnpg
    ops.example.com/switchover-target: demo-pg-4-2
  name: demo-pg-4
  namespace: default
  ownerReferences:
    - apiVersion: everest.example.com/v1alpha1
      blockOwnerDeletion: true
      controller: true
      kind: Cluster
      name: demo-cluster-4
      uid: 1be893ed-eeab-40d6-a38b-c6c687a59180
  resourceVersion: '75395797'
  selfLink: /apis/postgresql.cnpg.io/v1/namespaces/default/clusters/demo-pg-4
  uid: 84efa4df-0acb-4916-9849-7add2f139ad5
spec:
  affinity:
    podAntiAffinityType: preferred
  bootstrap:
    initdb:
      database: app
      encoding: UTF8
      localeCType: C
      localeCollate: C
      owner: app
  enablePDB: true
  enableSuperuserAccess: false
  failoverDelay: 0
  imageName: ghcr.io/cloudnative-pg/postgresql:16.1
  instances: 4
  logLevel: info
  managed:
    roles:
      - connectionLimit: -1
        ensure: present
        inherit: true
        login: true
        name: app_user
        passwordSecret:
          name: app-user-secret-cnpg
      - connectionLimit: -1
        ensure: present
        inherit: true
        login: true
        name: readonly_user
        passwordSecret:
          name: readonly-user-secret-cnpg
  maxSyncReplicas: 0
  minSyncReplicas: 0
  monitoring:
    customQueriesConfigMap:
      - key: queries
        name: cnpg-default-monitoring
    disableDefaultQueries: false
    enablePodMonitor: true
  postgresGID: 26
  postgresUID: 26
  postgresql:
    parameters:
      archive_mode: 'on'
      archive_timeout: 6min
      dynamic_shared_memory_type: posix
      log_destination: csvlog
      log_directory: /controller/log
      log_filename: postgres
      log_rotation_age: '0'
      log_rotation_size: '0'
      log_truncate_on_rotation: 'false'
      logging_collector: 'on'
      max_parallel_workers: '32'
      max_replication_slots: '32'
      max_worker_processes: '32'
      shared_memory_type: mmap
      shared_preload_libraries: ''
      ssl_max_protocol_version: TLSv1.3
      ssl_min_protocol_version: TLSv1.3
      wal_keep_size: 512MB
      wal_level: logical
      wal_log_hints: 'on'
      wal_receiver_timeout: 5s
      wal_sender_timeout: 5s
    syncReplicaElectionConstraint:
      enabled: false
  primaryUpdateMethod: restart
  primaryUpdateStrategy: unsupervised
  replicationSlots:
    highAvailability:
      enabled: true
      slotPrefix: _cnpg_
    synchronizeReplicas:
      enabled: true
    updateInterval: 30
  resources: {}
  smartShutdownTimeout: 180
  startDelay: 3600
  stopDelay: 1800
  storage:
    resizeInUseVolumes: true
    size: 10Gi
    storageClass: csi-sc-viettelplugin-hdd
  switchoverDelay: 3600
status:
  availableArchitectures:
    - goArch: amd64
      hash: e43340cd2ccfa2a8120cf5de6035fe4b18d799bb2feabf99a91434dd9ba92e4c
    - goArch: arm64
      hash: f453a8cb50a418ff9cac24c818b9e155b80487b4152444e48d187161ecdfc0eb
  certificates:
    clientCASecret: demo-pg-4-ca
    expirations:
      demo-pg-4-ca: 2026-03-29 07:44:38 +0000 UTC
      demo-pg-4-replication: 2026-03-29 07:44:38 +0000 UTC
      demo-pg-4-server: 2026-03-29 07:44:38 +0000 UTC
    replicationTLSSecret: demo-pg-4-replication
    serverAltDNSNames:
      - demo-pg-4-rw
      - demo-pg-4-rw.default
      - demo-pg-4-rw.default.svc
      - demo-pg-4-r
      - demo-pg-4-r.default
      - demo-pg-4-r.default.svc
      - demo-pg-4-ro
      - demo-pg-4-ro.default
      - demo-pg-4-ro.default.svc
    serverCASecret: demo-pg-4-ca
    serverTLSSecret: demo-pg-4-server
  cloudNativePGCommitHash: 4bef8412
  cloudNativePGOperatorHash: e43340cd2ccfa2a8120cf5de6035fe4b18d799bb2feabf99a91434dd9ba92e4c
  conditions:
    - lastTransitionTime: '2025-12-29T09:22:19Z'
      message: Cluster is Ready
      reason: ClusterIsReady
      status: 'True'
      type: Ready
    - lastTransitionTime: '2025-12-29T07:50:23Z'
      message: Continuous archiving is working
      reason: ContinuousArchivingSuccess
      status: 'True'
      type: ContinuousArchiving
  configMapResourceVersion:
    metrics:
      cnpg-default-monitoring: '75315375'
  currentPrimary: demo-pg-4-1
  currentPrimaryTimestamp: '2025-12-29T07:50:22.794380Z'
  healthyPVC:
    - demo-pg-4-1
    - demo-pg-4-2
    - demo-pg-4-3
    - demo-pg-4-4
  image: ghcr.io/cloudnative-pg/postgresql:16.1
  instanceNames:
    - demo-pg-4-1
    - demo-pg-4-2
    - demo-pg-4-3
    - demo-pg-4-4
  instances: 4
  instancesReportedState:
    demo-pg-4-1:
      isPrimary: true
      timeLineID: 1
    demo-pg-4-2:
      isPrimary: false
      timeLineID: 1
    demo-pg-4-3:
      isPrimary: false
      timeLineID: 1
    demo-pg-4-4:
      isPrimary: false
      timeLineID: 1
  instancesStatus:
    healthy:
      - demo-pg-4-1
      - demo-pg-4-2
      - demo-pg-4-3
      - demo-pg-4-4
  latestGeneratedNode: 4
  managedRolesStatus:
    byStatus:
      not-managed:
        - app
      reconciled:
        - app_user
        - readonly_user
      reserved:
        - postgres
        - streaming_replica
    passwordStatus:
      app_user:
        resourceVersion: '75314114'
        transactionID: 733
      readonly_user:
        resourceVersion: '75314226'
        transactionID: 734
  phase: Cluster in healthy state
  poolerIntegrations:
    pgBouncerIntegration: {}
  pvcCount: 4
  readService: demo-pg-4-r
  readyInstances: 4
  secretsResourceVersion:
    applicationSecretVersion: '75315345'
    clientCaSecretVersion: '75315340'
    managedRoleSecretVersion:
      app-user-secret-cnpg: '75314114'
      readonly-user-secret-cnpg: '75314226'
    replicationSecretVersion: '75315344'
    serverCaSecretVersion: '75315340'
    serverSecretVersion: '75315342'
  switchReplicaClusterStatus: {}
  targetPrimary: demo-pg-4-1
  targetPrimaryTimestamp: '2025-12-29T07:49:39.166506Z'
  timelineID: 1
  topology:
    instances:
      demo-pg-4-1: {}
      demo-pg-4-2: {}
      demo-pg-4-3: {}
      demo-pg-4-4: {}
    nodesUsed: 3
    successfullyExtracted: true
  writeService: demo-pg-4-rw

```

## Ops Request
```yaml
apiVersion: ops.example.com/v1alpha1
kind: OpsRequest
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: >
      {"apiVersion":"ops.example.com/v1alpha1","kind":"OpsRequest","metadata":{"annotations":{},"name":"demo-cluster-4-scale-postgres-1","namespace":"default"},"spec":{"clusterRef":{"name":"demo-cluster-4"},"engineRef":{"name":"demo-pg-4"},"params":{"replicas":"4"},"type":"HorizontalScaling"}}
  creationTimestamp: '2025-12-29T09:23:15Z'
  finalizers:
    - ops.example.com/finalizer
  generation: 1
  name: demo-cluster-4-scale-postgres-1
  namespace: default
  resourceVersion: '75377576'
  selfLink: >-
    /apis/ops.example.com/v1alpha1/namespaces/default/opsrequests/demo-cluster-4-scale-postgres-1
  uid: 01e191e6-c83c-4a03-be2d-dd823507d293
spec:
  clusterRef:
    name: demo-cluster-4
  engineRef:
    name: demo-pg-4
  params:
    replicas: '4'
  timeout: 30m
  type: HorizontalScaling
status:
  completionTime: '2025-12-29T09:23:15Z'
  conditions:
    - lastTransitionTime: '2025-12-29T09:23:15Z'
      message: Started HorizontalScaling operation
      observedGeneration: 1
      reason: OperationStarted
      status: 'True'
      type: Processing
    - lastTransitionTime: '2025-12-29T09:23:15Z'
      message: Operation completed successfully
      observedGeneration: 1
      reason: OperationSucceeded
      status: 'True'
      type: Completed
  message: Scaled from 4 to 4 replicas successfully
  observedGeneration: 1
  output:
    currentReplicas: '4'
    previousReplicas: '4'
  phase: Succeeded
  progress: 100
  startTime: '2025-12-29T09:23:15Z'
```