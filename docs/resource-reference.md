# CAPBM 资源属性完整参考

## 一、资源概览

| 资源 | API Group | 用途 |
|------|-----------|------|
| **BareMetalCluster** | infrastructure.cluster.x-k8s.io/v1beta1 | 集群级别基础设施状态 |
| **BareMetalClusterTemplate** | infrastructure.cluster.x-k8s.io/v1beta1 | ClusterClass 用的集群模板 |
| **BareMetalMachine** | infrastructure.cluster.x-k8s.io/v1beta1 | 单台裸金属机器实例 |
| **BareMetalMachineTemplate** | infrastructure.cluster.x-k8s.io/v1beta1 | ClusterClass 用的机器模板 |
| **BareMetalHostInventory** | infrastructure.cluster.x-k8s.io/v1beta1 | 裸金属机器池清单 |
| **KubeadmControlPlane** | controlplane.cluster.x-k8s.io/v1beta2 | 控制面管理 (kubeadm) |
| **KubeadmControlPlaneTemplate** | controlplane.cluster.x-k8s.io/v1beta2 | ClusterClass 用的控制面模板 |
| **KubeadmConfig** | bootstrap.cluster.x-k8s.io/v1beta2 | 节点引导配置 |
| **KubeadmConfigTemplate** | bootstrap.cluster.x-k8s.io/v1beta2 | ClusterClass 用的引导配置模板 |
| **ClusterClass** | cluster.x-k8s.io/v1beta2 | 集群拓扑模板定义 |

---

## 二、资源详细属性

### 1. BareMetalCluster

**用途**: 表示一个裸金属集群的基础设施状态

| 路径 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `spec.controlPlaneEndpoint.host` | string | 否 | - | API Server 主机地址 |
| `spec.controlPlaneEndpoint.port` | int32 | 否 | - | API Server 端口 |
| `spec.network.podCIDR` | string | 否 | - | Pod 网络 CIDR |
| `spec.network.serviceCIDR` | string | 否 | - | Service 网络 CIDR |
| `spec.network.dnsDomain` | string | 否 | cluster.local | DNS 域名 |
| `status.ready` | bool | - | false | 基础设施是否就绪 |
| `status.initialization.provisioned` | *bool | - | - | 基础设施是否已配置完成 |
| `status.conditions` | []Condition | - | - | 条件列表 |

**Annotations**:
- `baremetal.cluster.x-k8s.io/endpoint-source`: 端点来源 ("cluster" 或 "infrastructure")

### 2. BareMetalClusterTemplate

**用途**: ClusterClass 中引用的集群模板

| 路径 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `spec.template.metadata.labels` | map[string]string | 否 | 标签 |
| `spec.template.metadata.annotations` | map[string]string | 否 | 注解 |
| `spec.template.spec` | BareMetalClusterSpec | 是 | 嵌入式 BareMetalClusterSpec |

### 3. BareMetalMachine

**用途**: 表示单台裸金属机器实例

| 路径 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `spec.providerID` | *string | 否 | - | 格式: baremetal://\<hostname\> |
| `spec.hostInventoryRef.name` | string | 否 | - | 引用的机器池名称 |
| `spec.hostName` | string | 否 | - | 主机名（可直接指定或从机器池分配） |
| `spec.ipAddress` | string | 否 | - | IP 地址（可直接指定或从机器池分配） |
| `spec.sshPort` | int | 否 | 22 | SSH 端口 |
| `spec.credentialsRef.name` | string | 否 | - | SSH 凭据 Secret 名称 |
| `spec.powerManagement.type` | string | 否 | - | 电源管理类型 (ipmi/redfish) |
| `spec.powerManagement.address` | string | 否 | - | BMC 地址 |
| `spec.powerManagement.credentialsRef.name` | string | 否 | - | BMC 凭据 Secret 名称 |
| `spec.role` | string | 否 | - | 角色 (control-plane/worker) |
| `status.ready` | bool | - | false | 机器是否就绪 |
| `status.providerID` | string | - | - | 机器唯一标识 |
| `status.addresses` | []MachineAddress | - | - | 地址列表 (InternalIP, HostName) |
| `status.conditions` | []Condition | - | - | 条件列表 |

**Conditions 类型**:
- `Ready`: 机器整体就绪状态
- `SSHConnected`: SSH 连接状态
- `PreFlightChecksPassed`: 预检通过状态

### 4. BareMetalMachineTemplate

**用途**: ClusterClass 中引用的机器模板

| 路径 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `spec.template.metadata.labels` | map[string]string | 否 | 标签 |
| `spec.template.metadata.annotations` | map[string]string | 否 | 注解 |
| `spec.template.spec.sshPort` | int | 否 | SSH 端口 |
| `spec.template.spec.hostInventoryRef.name` | string | 否 | 机器池名称 |
| `spec.template.spec.credentialsRef.name` | string | 否 | SSH 凭据 Secret 名称 |
| `spec.template.spec.powerManagement` | PowerManagementConfig | 否 | 电源管理配置 |
| `spec.template.spec.role` | string | 否 | 角色 |

### 5. BareMetalHostInventory

**用途**: 管理可用裸金属机器池

| 路径 | 类型 | 必填 | 说明 |
|------|------|------|------|
| **Spec** | | | |
| `spec.hosts[].name` | string | 是 | 机器条目唯一标识 |
| `spec.hosts[].hostName` | string | 是 | 主机名 |
| `spec.hosts[].ipAddress` | string | 是 | IP 地址 |
| `spec.hosts[].sshPort` | int | 否 | SSH 端口 (默认 22) |
| `spec.hosts[].credentialsRef.name` | string | 是 | SSH 凭据 Secret 名称 |
| `spec.hosts[].role` | string | 否 | 角色 (control-plane/worker) |
| `spec.hosts[].labels` | map[string]string | 否 | 用户自定义标签 |
| **Status** | | | |
| `status.totalHosts` | int | - | 总机器数 |
| `status.availableHosts` | int | - | 可用机器数 |
| `status.allocatedHosts` | int | - | 已分配机器数 |
| `status.hostsStatus[].name` | string | - | 机器名称 |
| `status.hostsStatus[].state` | HostState | - | 状态 (Available/Allocated/Maintenance) |
| `status.hostsStatus[].clusterRef` | ObjectReference | - | 分配的集群引用 |

### 6. KubeadmControlPlane

**用途**: 管理控制面节点的生命周期，使用 kubeadm 初始化和配置

| 路径 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `spec.replicas` | int | 是 | - | 控制面节点数量 |
| `spec.version` | string | 是 | - | Kubernetes 版本 (如 v1.31.0) |
| `spec.machineTemplate.infrastructureRef` | ObjectReference | 是 | - | 指向 BareMetalMachineTemplate |
| `spec.machineTemplate.nodeDrainTimeout` | Duration | 否 | 0s | 节点排空超时时间 |
| `spec.kubeadmConfigSpec.clusterConfiguration` | ClusterConfiguration | 否 | - | kubeadm 集群配置 |
| `spec.kubeadmConfigSpec.clusterConfiguration.apiServer` | APIServer | 否 | - | API Server 配置 |
| `spec.kubeadmConfigSpec.clusterConfiguration.controllerManager` | ControlPlaneComponent | 否 | - | Controller Manager 配置 |
| `spec.kubeadmConfigSpec.clusterConfiguration.scheduler` | ControlPlaneComponent | 否 | - | Scheduler 配置 |
| `spec.kubeadmConfigSpec.clusterConfiguration.etcd` | Etcd | 否 | local | etcd 配置 |
| `spec.kubeadmConfigSpec.clusterConfiguration.networking` | Networking | 否 | - | 网络配置 |
| `spec.kubeadmConfigSpec.initConfiguration` | InitConfiguration | 否 | - | kubeadm init 配置 |
| `spec.kubeadmConfigSpec.joinConfiguration` | JoinConfiguration | 否 | - | kubeadm join 配置 |
| `spec.kubeadmConfigSpec.files` | []File | 否 | - | 要部署的文件 |
| `spec.kubeadmConfigSpec.preKubeadmCommands` | []string | 否 | - | kubeadm 执行前命令 |
| `spec.kubeadmConfigSpec.postKubeadmCommands` | []string | 否 | - | kubeadm 执行后命令 |
| `spec.kubeadmConfigSpec.users` | []User | 否 | - | 要创建的用户 |
| `spec.kubeadmConfigSpec.ntp` | NTP | 否 | - | NTP 配置 |
| `spec.kubeadmConfigSpec.diskSetup` | DiskSetup | 否 | - | 磁盘配置 |
| `spec.kubeadmConfigSpec.mounts` | []Mount | 否 | - | 挂载点配置 |
| `spec.kubeadmConfigSpec.format` | string | 否 | - | 引导数据格式 (cloud-config/ignition) |
| `spec.rolloutStrategy` | RolloutStrategy | 否 | RollingUpdate | 滚动更新策略 |
| `spec.rolloutStrategy.type` | string | 否 | RollingUpdate | 策略类型 |
| `spec.rolloutStrategy.rollingUpdate.maxSurge` | intstr | 否 | 1 | 最大超额数量 |
| `status.replicas` | int | - | - | 当前副本数 |
| `status.readyReplicas` | int | - | - | 就绪副本数 |
| `status.updatedReplicas` | int | - | - | 已更新副本数 |
| `status.unavailableReplicas` | int | - | - | 不可用副本数 |
| `status.version` | string | - | - | 当前 Kubernetes 版本 |
| `status.selector` | string | - | - | Machine 选择器 |
| `status.conditions` | []Condition | - | - | 条件列表 |

**Conditions 类型**:
- `Available`: 控制面是否可用
- `Ready`: 控制面是否就绪
- `MachinesCreated`: Machine 是否已创建
- `MachinesReady`: Machine 是否就绪
- `Resized`: 副本数是否符合预期
- `ControlPlaneComponentsHealthy`: 控制面组件是否健康

### 7. KubeadmControlPlaneTemplate

**用途**: ClusterClass 中引用的控制面模板

| 路径 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `spec.template.spec.replicas` | int | 否 | 控制面副本数 (通常由 Cluster 覆盖) |
| `spec.template.spec.version` | string | 否 | Kubernetes 版本 (通常由 Cluster 覆盖) |
| `spec.template.spec.machineTemplate` | KCPMachineTemplate | 是 | Machine 模板配置 |
| `spec.template.spec.kubeadmConfigSpec` | KubeadmConfigSpec | 是 | kubeadm 配置 |

### 8. KubeadmConfig

**用途**: 为单个节点生成引导数据 (cloud-init/Ignition)

| 路径 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `spec.clusterConfiguration` | ClusterConfiguration | 否 | 集群配置 (仅控制面) |
| `spec.initConfiguration` | InitConfiguration | 否 | kubeadm init 配置 (仅第一个控制面) |
| `spec.joinConfiguration` | JoinConfiguration | 否 | kubeadm join 配置 (其他节点) |
| `spec.files` | []File | 否 | 要部署的文件 |
| `spec.preKubeadmCommands` | []string | 否 | kubeadm 执行前命令 |
| `spec.postKubeadmCommands` | []string | 否 | kubeadm 执行后命令 |
| `spec.users` | []User | 否 | 要创建的用户 |
| `spec.ntp` | NTP | 否 | NTP 配置 |
| `spec.diskSetup` | DiskSetup | 否 | 磁盘配置 |
| `spec.mounts` | []Mount | 否 | 挂载点配置 |
| `spec.format` | string | 否 | 引导数据格式 |
| `spec.verbosity` | string | 否 | kubeadm 日志级别 |
| `status.ready` | bool | - | 引导数据是否已生成 |
| `status.dataSecretName` | string | - | 引导数据 Secret 名称 |
| `status.observedGeneration` | int | - | 观察到的 generation |

### 9. KubeadmConfigTemplate

**用途**: ClusterClass 中引用的引导配置模板 (用于 Worker 节点)

| 路径 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `spec.template.spec.joinConfiguration` | JoinConfiguration | 否 | kubeadm join 配置 |
| `spec.template.spec.files` | []File | 否 | 要部署的文件 |
| `spec.template.spec.preKubeadmCommands` | []string | 否 | kubeadm 执行前命令 |
| `spec.template.spec.postKubeadmCommands` | []string | 否 | kubeadm 执行后命令 |
| `spec.template.spec.users` | []User | 否 | 要创建的用户 |
| `spec.template.spec.ntp` | NTP | 否 | NTP 配置 |
| `spec.template.spec.diskSetup` | DiskSetup | 否 | 磁盘配置 |
| `spec.template.spec.mounts` | []Mount | 否 | 挂载点配置 |
| `spec.template.spec.format` | string | 否 | 引导数据格式 |

### 10. ClusterClass

**用途**: 定义集群拓扑模板，通过 variables 和 patches 实现灵活的集群配置

| 路径 | 类型 | 必填 | 说明 |
|------|------|------|------|
| **Infrastructure** | | | |
| `spec.infrastructure.templateRef` | ObjectReference | 是 | 指向 BareMetalClusterTemplate |
| **ControlPlane** | | | |
| `spec.controlPlane.templateRef` | ObjectReference | 是 | 指向 KubeadmControlPlaneTemplate |
| `spec.controlPlane.machineInfrastructure.templateRef` | ObjectReference | 是 | 指向 BareMetalMachineTemplate (控制面) |
| `spec.controlPlane.naming.template` | string | 否 | 控制面命名模板 |
| `spec.controlPlane.healthCheck` | HealthCheck | 否 | 控制面健康检查配置 |
| `spec.controlPlane.healthCheck.checks.nodeStartupTimeoutSeconds` | int | 否 | 节点启动超时 (秒) |
| `spec.controlPlane.healthCheck.checks.unhealthyNodeConditions[]` | []Condition | 否 | 不健康节点条件 |
| `spec.controlPlane.healthCheck.remediation.triggerIf` | Trigger | 否 | 触发修复条件 |
| **Workers** | | | |
| `spec.workers.machineDeployments[]` | []MachineDeploymentClass | 是 | Worker 部署类列表 |
| `spec.workers.machineDeployments[].class` | string | 是 | 部署类名称 (如 default-worker) |
| `spec.workers.machineDeployments[].bootstrap.templateRef` | ObjectReference | 是 | 指向 KubeadmConfigTemplate |
| `spec.workers.machineDeployments[].infrastructure.templateRef` | ObjectReference | 是 | 指向 BareMetalMachineTemplate |
| `spec.workers.machineDeployments[].naming.template` | string | 否 | 命名模板 |
| `spec.workers.machineDeployments[].healthCheck` | HealthCheck | 否 | 健康检查配置 |
| `spec.workers.machineDeployments[].variables` | []VariableOverride | 否 | 变量覆盖 |
| `spec.workers.machinePools[]` | []MachinePoolClass | 否 | MachinePool 类列表 |
| **Variables** | | | |
| `spec.variables[]` | []Variable | 是 | 可配置变量列表 |
| `spec.variables[].name` | string | 是 | 变量名称 |
| `spec.variables[].required` | bool | 否 | 是否必填 |
| `spec.variables[].schema.openAPIV3Schema` | JSONSchemaProps | 是 | 变量 schema 定义 |
| `spec.variables[].schema.openAPIV3Schema.type` | string | 是 | 类型 (string/integer/object/array/boolean) |
| `spec.variables[].schema.openAPIV3Schema.default` | any | 否 | 默认值 |
| `spec.variables[].schema.openAPIV3Schema.description` | string | 否 | 描述 |
| `spec.variables[].schema.openAPIV3Schema.properties` | map | 否 | 对象属性 (type=object 时) |
| `spec.variables[].schema.openAPIV3Schema.items` | JSONSchemaProps | 否 | 数组项 schema (type=array 时) |
| **Patches** | | | |
| `spec.patches[]` | []Patch | 是 | Patch 列表 |
| `spec.patches[].name` | string | 是 | Patch 名称 |
| `spec.patches[].description` | string | 否 | Patch 描述 |
| `spec.patches[].enabledIf` | string | 否 | 启用条件 (Go template) |
| `spec.patches[].definitions[]` | []PatchDefinition | 是 | Patch 定义列表 |
| `spec.patches[].definitions[].selector.apiVersion` | string | 是 | 目标资源 API 版本 |
| `spec.patches[].definitions[].selector.kind` | string | 是 | 目标资源类型 |
| `spec.patches[].definitions[].selector.matchResources` | MatchResources | 是 | 匹配规则 |
| `spec.patches[].definitions[].selector.matchResources.controlPlane` | bool | 否 | 是否匹配控制面 |
| `spec.patches[].definitions[].selector.matchResources.infrastructureCluster` | bool | 否 | 是否匹配基础设施集群 |
| `spec.patches[].definitions[].selector.matchResources.machineDeploymentClass.names` | []string | 否 | 匹配的部署类名称 |
| `spec.patches[].definitions[].jsonPatches[]` | []JSONPatch | 是 | JSON Patch 列表 |
| `spec.patches[].definitions[].jsonPatches[].op` | string | 是 | 操作 (add/remove/replace) |
| `spec.patches[].definitions[].jsonPatches[].path` | string | 是 | JSON 路径 |
| `spec.patches[].definitions[].jsonPatches[].value` | any | 否 | 固定值 |
| `spec.patches[].definitions[].jsonPatches[].valueFrom.variable` | string | 否 | 变量引用 |
| `spec.patches[].definitions[].jsonPatches[].valueFrom.template` | string | 否 | Go template |
| **Status** | | | |
| `status.conditions` | []Condition | - | 条件列表 |
| `status.variables` | []VariableStatus | - | 变量状态 |

**Patch valueFrom 类型**:
- `variable`: 引用 ClusterClass 变量 (如 `controlPlaneEndpoint.host`)
- `template`: Go template 表达式 (如 `{{ .builtin.cluster.name }}-control-plane`)
- `value`: 硬编码值 (直接在 jsonPatches 中使用)

**Builtin Variables** (可在 template 中使用):
- `builtin.cluster.name`: 集群名称
- `builtin.cluster.namespace`: 集群命名空间
- `builtin.cluster.topology.version`: 集群 Kubernetes 版本
- `builtin.cluster.network.pods`: Pod CIDR
- `builtin.cluster.network.services`: Service CIDR
- `builtin.controlPlane.replicas`: 控制面副本数
- `builtin.controlPlane.version`: 控制面版本
- `builtin.machineDeployment.replicas`: MachineDeployment 副本数
- `builtin.machineDeployment.version`: MachineDeployment 版本
- `builtin.machineDeployment.class`: MachineDeployment 类名称

**Conditions 类型**:
- `Ready`: ClusterClass 是否就绪
- `VariablesValidationSucceeded`: 变量验证是否成功

---

## 三、CAPI 核心资源关联属性

### Cluster (cluster.x-k8s.io/v1beta2)

| 路径 | 说明 |
|------|------|
| `spec.infrastructureRef` | 指向 BareMetalCluster |
| `spec.controlPlaneRef` | 指向 KubeadmControlPlane |
| `spec.topology.classRef.name` | ClusterClass 名称 |
| `spec.topology.version` | Kubernetes 版本 |
| `spec.topology.controlPlane.replicas` | 控制面副本数 |
| `spec.topology.controlPlane.metadata` | 控制面元数据 (labels/annotations) |
| `spec.topology.workers.machineDeployments[]` | Worker 部署列表 |
| `spec.topology.variables[]` | ClusterClass 变量值 |
| `spec.controlPlaneEndpoint` | API Server 端点（由 InfraCluster 同步） |
| `spec.clusterNetwork` | 集群网络配置 |
| `spec.clusterNetwork.pods.cidrBlocks` | Pod CIDR 列表 |
| `spec.clusterNetwork.services.cidrBlocks` | Service CIDR 列表 |
| `spec.clusterNetwork.serviceDomain` | Service DNS 域名 |
| `status.infrastructureReady` | 基础设施就绪状态 |
| `status.controlPlaneReady` | 控制面就绪状态 |
| `status.failureDomains` | 可用故障域 |

### Machine (cluster.x-k8s.io/v1beta2)

| 路径 | 说明 |
|------|------|
| `spec.clusterName` | 所属集群名称 |
| `spec.bootstrap.configRef` | 指向 KubeadmConfig |
| `spec.infrastructureRef` | 指向 BareMetalMachine |
| `spec.version` | Kubernetes 版本 |
| `spec.providerID` | 云提供商 ID (baremetal://hostname) |
| `spec.failureDomain` | 故障域 |
| `status.infrastructureReady` | 基础设施就绪状态 |
| `status.bootstrapReady` | Bootstrap 就绪状态 |
| `status.nodeRef` | 关联的 Node 资源引用 |
| `status.addresses` | 节点地址列表 |
| `status.phase` | 机器生命周期阶段 |

### MachineDeployment (cluster.x-k8s.io/v1beta2)

| 路径 | 说明 |
|------|------|
| `spec.clusterName` | 所属集群名称 |
| `spec.replicas` | 期望副本数 |
| `spec.selector.matchLabels` | Machine 选择器 |
| `spec.template.spec.clusterName` | 所属集群名称 |
| `spec.template.spec.version` | Kubernetes 版本 |
| `spec.template.spec.bootstrap.configRef` | 指向 KubeadmConfigTemplate |
| `spec.template.spec.infrastructureRef` | 指向 BareMetalMachineTemplate |
| `spec.strategy` | 滚动更新策略 |
| `spec.strategy.type` | 策略类型 (RollingUpdate/OnDelete) |
| `spec.strategy.rollingUpdate.maxSurge` | 最大超额数量 |
| `spec.strategy.rollingUpdate.maxUnavailable` | 最大不可用数量 |
| `spec.minReadySeconds` | 最小就绪秒数 |
| `spec.revisionHistoryLimit` | 保留的历史版本数 |
| `spec.paused` | 是否暂停 |
| `status.replicas` | 当前副本数 |
| `status.readyReplicas` | 就绪副本数 |
| `status.updatedReplicas` | 已更新副本数 |
| `status.unavailableReplicas` | 不可用副本数 |
| `status.phase` | 部署阶段 |
| `status.conditions` | 条件列表 |

### MachineHealthCheck (cluster.x-k8s.io/v1beta2)

| 路径 | 说明 |
|------|------|
| `spec.clusterName` | 所属集群名称 |
| `spec.selector.matchLabels` | Machine 选择器 |
| `spec.unhealthyConditions[]` | 不健康条件列表 |
| `spec.unhealthyConditions[].type` | 条件类型 |
| `spec.unhealthyConditions[].status` | 条件状态 |
| `spec.unhealthyConditions[].timeout` | 超时时间 |
| `spec.maxUnhealthy` | 最大不健康数量/百分比 |
| `spec.nodeStartupTimeout` | 节点启动超时时间 |
| `spec.remediationTemplate` | 修复模板引用 |
| `status.expectedMachines` | 期望的机器数量 |
| `status.currentHealthy` | 当前健康机器数量 |
| `status.targets` | 被标记为不健康的机器列表 |

### ClusterClass (cluster.x-k8s.io/v1beta2)

| 路径 | 说明 |
|------|------|
| `spec.infrastructure.templateRef` | 指向 BareMetalClusterTemplate |
| `spec.controlPlane.templateRef` | 指向 KubeadmControlPlaneTemplate |
| `spec.controlPlane.machineInfrastructure.templateRef` | 指向 BareMetalMachineTemplate (CP) |
| `spec.workers.machineDeployments[].class` | Worker 部署类名称 |
| `spec.workers.machineDeployments[].bootstrap.templateRef` | 指向 KubeadmConfigTemplate |
| `spec.workers.machineDeployments[].infrastructure.templateRef` | 指向 BareMetalMachineTemplate (Worker) |
| `spec.variables[]` | 可配置变量定义列表 |
| `spec.patches[]` | JSON Patch 定义列表 |
| `status.conditions` | ClusterClass 状态条件 |

---

## 四、资源关联关系图

### 4.1 ClusterClass 模式完整架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Cluster (CAPI Core)                          │
│  spec:                                                             │
│    topology.classRef ──────────────────┐                           │
│    infrastructureRef ────────────┐     │                           │
│    controlPlaneRef ──────────┐   │     │                           │
│    controlPlaneEndpoint ◄────┼───┼─────┤ (从 InfraCluster 同步)     │
│    clusterNetwork ───────────┼───┼─────┤                           │
│      pods.cidrBlocks ────────┼───┘     │                           │
│      services.cidrBlocks ────┼─────────┤                           │
│      serviceDomain ──────────┼─────────┤                           │
│  status:                                                             │
│    infrastructureReady ◄─────┼─────────┤                           │
│    controlPlaneReady ◄───────┼─────────┤                           │
└─────────────────────────────┼─────────┼───────────────────────────┘
                              │         │
          ┌───────────────────┼─────────┼───────────────────┐
          ▼                   ▼         ▼                   ▼
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────────┐
│ BareMetalCluster │  │ KubeadmControlPlane│  │    ClusterClass      │
│                  │  │                  │  │                      │
│ spec:            │  │ spec:            │  │ spec:                │
│   controlPlane   │  │   replicas       │  │   variables[]        │
│     Endpoint     │  │   version        │  │   patches[]          │
│   network:       │  │   machineTemplate│  │   infrastructure:    │
│     podCIDR      │  │     infraRef ────┼──┼─► templateRef        │
│     serviceCIDR  │  │   kubeadmConfig  │  │   controlPlane:      │
│ status:          │  │ status:          │  │     templateRef      │
│   ready          │  │   replicas       │  │     machineInfra     │
│   initialized    │  │   readyReplicas  │  │   workers:           │
│   conditions     │  │   version        │  │     machineDeploy    │
└────────┬─────────┘  │   conditions     │  │       []             │
         │            └────────┬─────────┘  │       bootstrap      │
         │                     │            │       infra          │
         │                     │            └──────────┬───────────┘
         │                     │                       │
         ▼                     ▼                       ▼
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────────┐
│ BareMetalMachine │  │ KubeadmConfig    │  │ BareMetalMachineTpl  │
│ Template         │  │ (控制面)         │  │                      │
│ spec:            │  │ spec:            │  │ spec:                │
│   template.spec: │  │   clusterCon     │  │   template.spec:     │
│     sshPort      │  │     fig          │  │     sshPort          │
│     hostInvent   │  │   initConfig     │  │     hostInventoryRef │
│       oryRef     │  │   preKubeadm     │  │     credentialsRef   │
│     credentials  │  │     Commands     │  │     role             │
│     role         │  │   files          │  │                      │
└────────┬─────────┘  │   users          │  └──────────┬───────────┘
         │            └────────┬─────────┘             │
         │                     │                       │
         │              ┌──────┴──────┐                │
         │              ▼             ▼                │
         │     ┌────────────┐ ┌────────────┐          │
         │     │KubeadmCon  │ │KubeadmCon  │          │
         │     │figTemplate │ │figTemplate │          │
         │     │(Worker)    │ │(Worker)    │          │
         │     └────────────┘ └────────────┘          │
         │                     │                       │
         ▼                     ▼                       ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        BareMetalMachine                             │
│  spec:                                                             │
│    providerID ◄──────────── 设置后关联到 Node                        │
│    hostInventoryRef ──────┐                                        │
│    hostName ◄─────────────┼── 从机器池分配或直接指定                 │
│    ipAddress ◄────────────┤                                        │
│    credentialsRef ◄───────┤                                        │
│    role                   │                                        │
│  status:                                                           │
│    ready                   │                                        │
│    providerID ────────────► 用于关联 Node 资源                       │
│    addresses               │                                        │
│    conditions              │                                        │
└───────────────────────────┼────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     BareMetalHostInventory                          │
│  spec:                                                             │
│    hosts[]:                                                        │
│      - name                                                        │
│      - hostName                                                    │
│      - ipAddress                                                   │
│      - sshPort                                                     │
│      - credentialsRef                                              │
│      - role                                                        │
│      - labels                                                      │
│  status:                                                           │
│    totalHosts                                                      │
│    availableHosts                                                  │
│    allocatedHosts                                                  │
│    hostsStatus[]:                                                  │
│      - name                                                        │
│      - state (Available/Allocated/Maintenance)                     │
│      - clusterRef                                                  │
└─────────────────────────────────────────────────────────────────────┘
```

### 4.2 资源引用关系详细图

```
Cluster.spec.topology
    │
    ├── classRef ──────────────────────────────────────────┐
    │                                                      │
    │    ClusterClass                                      │
    │    ├── infrastructure.templateRef ───────────────────┼──► BareMetalClusterTemplate
    │    ├── controlPlane.templateRef ─────────────────────┼──► KubeadmControlPlaneTemplate
    │    ├── controlPlane.machineInfrastructure.templateRef┼──► BareMetalMachineTemplate (CP)
    │    ├── workers.machineDeployments[].bootstrap        │
    │    │   .templateRef ─────────────────────────────────┼──► KubeadmConfigTemplate
    │    └── workers.machineDeployments[].infrastructure   │
    │        .templateRef ─────────────────────────────────┼──► BareMetalMachineTemplate (Worker)
    │                                                      │
    └── variables[] ───────────────────────────────────────┘
         │
         ▼ (通过 patches 注入到模板)
    生成的资源:
    ├── BareMetalCluster.spec.controlPlaneEndpoint
    ├── BareMetalMachineTemplate.spec.hostInventoryRef
    ├── BareMetalMachineTemplate.spec.credentialsRef
    └── KubeadmControlPlaneTemplate.spec.version

Cluster (自动生成)
    ├── infrastructureRef ────────────────────────────────► BareMetalCluster
    ├── controlPlaneRef ──────────────────────────────────► KubeadmControlPlane
    │
    KubeadmControlPlane
    │   ├── machineTemplate.infrastructureRef ────────────► BareMetalMachineTemplate (CP)
    │   └── (创建 Machine)
    │       ├── bootstrap.configRef ──────────────────────► KubeadmConfig
    │       └── infrastructureRef ────────────────────────► BareMetalMachine
    │
    MachineDeployment
    │   ├── template.spec.bootstrap.configRef ────────────► KubeadmConfigTemplate
    │   └── template.spec.infrastructureRef ──────────────► BareMetalMachineTemplate (Worker)
    │
    MachineSet (由 MachineDeployment 创建)
    │   └── template.spec.infrastructureRef ──────────────► BareMetalMachineTemplate
    │
    Machine (由 MachineSet 创建)
        ├── bootstrap.configRef ──────────────────────────► KubeadmConfig
        └── infrastructureRef ────────────────────────────► BareMetalMachine

BareMetalMachine
    └── hostInventoryRef ─────────────────────────────────► BareMetalHostInventory
```

### 4.3 OwnerReferences 层级关系

```
Cluster (my-cluster)
├── ownerReferences: []
│
├── BareMetalCluster (my-cluster-xxxxx)
│   └── ownerReferences: [Cluster]
│
├── KubeadmControlPlane (my-cluster-xxxxx)
│   └── ownerReferences: [Cluster]
│   │
│   └── BareMetalMachineTemplate (my-cluster-xxxxx-cp)
│       └── ownerReferences: [KubeadmControlPlane]
│       │
│       └── Machine (my-cluster-xxxxx-abc12)
│           ├── ownerReferences: [KubeadmControlPlane]
│           │
│           ├── BareMetalMachine (my-cluster-xxxxx-abc12)
│           │   └── ownerReferences: [Machine]
│           │
│           └── KubeadmConfig (my-cluster-xxxxx-abc12)
│               └── ownerReferences: [Machine]
│
└── MachineDeployment (my-cluster-md-0-xxxxx)
    └── ownerReferences: [Cluster]
    │
    └── MachineSet (my-cluster-md-0-xxxxx-abc12)
        └── ownerReferences: [MachineDeployment]
        │
        ├── Machine (my-cluster-md-0-xxxxx-def34)
        │   ├── ownerReferences: [MachineSet]
        │   │
        │   ├── BareMetalMachine (my-cluster-md-0-xxxxx-def34)
        │   │   └── ownerReferences: [Machine]
        │   │
        │   └── KubeadmConfig (my-cluster-md-0-xxxxx-def34)
        │       └── ownerReferences: [Machine]
        │
        └── Machine (my-cluster-md-0-xxxxx-ghi56)
            └── ...
```

### 4.4 ClusterClass Patch 注入关系

```
Cluster.spec.topology.variables
    │
    ├── controlPlaneEndpoint.host ────────────────────────────────┐
    ├── controlPlaneEndpoint.port ────────────────────────────────┤
    ├── credentialsSecret ────────────────────────────────────────┤
    ├── hostInventoryRef ─────────────────────────────────────────┤
    ├── kubernetesVersion ────────────────────────────────────────┤
    ├── podCIDR ──────────────────────────────────────────────────┤
    ├── serviceCIDR ──────────────────────────────────────────────┤
    └── preFlightChecks ──────────────────────────────────────────┤
                                                                  │
    ClusterClass.spec.patches                                     │
    ├── name: controlPlaneEndpoint                                │
    │   └── definitions:                                          │
    │       └── selector: BareMetalClusterTemplate                │
    │           └── jsonPatches:                                  │
    │               ├── path: /spec/template/spec/controlPlane    │
    │               │   Endpoint/host ◄───────────────────────────┘
    │               └── path: /spec/template/spec/controlPlane    │
    │                   Endpoint/port ◄───────────────────────────┘
    │                                                             │
    ├── name: credentialsSecret                                   │
    │   └── definitions:                                          │
    │       ├── selector: BareMetalMachineTemplate (CP)           │
    │       │   └── jsonPatches:                                  │
    │       │       └── path: /spec/template/spec/credentialsRef  │
    │       │           /name ◄───────────────────────────────────┘
    │       └── selector: BareMetalMachineTemplate (Worker)       │
    │           └── jsonPatches:                                  │
    │               └── path: /spec/template/spec/credentialsRef  │
    │                   /name ◄───────────────────────────────────┘
    │                                                             │
    ├── name: hostInventoryRef                                    │
    │   └── definitions:                                          │
    │       ├── selector: BareMetalMachineTemplate (CP)           │
    │       │   └── jsonPatches:                                  │
    │       │       └── path: /spec/template/spec/hostInventoryRef│
    │       │           /name ◄───────────────────────────────────┘
    │       └── selector: BareMetalMachineTemplate (Worker)       │
    │           └── jsonPatches:                                  │
    │               └── path: /spec/template/spec/hostInventoryRef│
    │                   /name ◄───────────────────────────────────┘
    │                                                             │
    └── name: kubernetesVersion                                   │
        └── definitions:                                          │
            └── selector: KubeadmControlPlaneTemplate             │
                └── jsonPatches:                                  │
                    └── path: /spec/template/spec/version ◄───────┘
```

---

## 五、关键字段流转

### ControlPlaneEndpoint 流转

```
用户/ClusterClass variables
         │
         ▼
Cluster.spec.topology.variables[].value.controlPlaneEndpoint
         │
         ▼ (ClusterTopology Controller patch)
BareMetalCluster.spec.controlPlaneEndpoint
         │
         ▼ (Cluster Controller 同步)
Cluster.spec.controlPlaneEndpoint
```

### 网络配置流转

```
Cluster.spec.clusterNetwork
         │
         ├── pods.cidrBlocks ──────────────────────────────┐
         ├── services.cidrBlocks ──────────────────────────┤
         └── serviceDomain ────────────────────────────────┤
                                                           ▼
                                              BareMetalCluster.spec.network
                                                           │
                                                           ▼ (KubeadmControlPlane 读取)
                                        KubeadmConfigSpec.clusterConfiguration.networking
                                                           │
                                                           ▼ (kubeadm 使用)
                                        kube-apiserver --service-cluster-ip-range
                                        kube-controller-manager --cluster-cidr
                                        kubelet --cluster-dns
```

### KubeadmConfig 生成流转

```
KubeadmControlPlane.spec.kubeadmConfigSpec
         │
         ├── clusterConfiguration ─────────────────────────┐
         ├── initConfiguration (第一个控制面节点) ─────────┤
         ├── joinConfiguration (其他节点) ─────────────────┤
         ├── preKubeadmCommands ───────────────────────────┤
         ├── postKubeadmCommands ──────────────────────────┤
         ├── files ────────────────────────────────────────┤
         └── users ────────────────────────────────────────┤
                                                           ▼
                                              KubeadmConfig Controller
                                                           │
                                                           ▼ (生成 cloud-init/Ignition)
                                        Secret (bootstrap-data)
                                                           │
                                                           ▼ (Machine Controller 挂载)
                                        Machine.spec.bootstrap.dataSecretName
                                                           │
                                                           ▼ (kubelet 使用)
                                        /etc/kubernetes/ 配置 + 证书
```

### 控制面滚动更新流转

```
KubeadmControlPlane.spec.version 变更
         │
         ▼ (KubeadmControlPlane Controller 检测版本变化)
    1. 选择要升级的 Machine
         │
         ▼
    2. 删除旧 Machine
         │
         ▼
    3. 创建新 Machine (使用新版本 KubeadmConfig)
         │
         ▼
    4. 等待新 Machine Ready
         │
         ├── 基础设施就绪 (BareMetalMachine.status.ready)
         ├── Bootstrap 就绪 (KubeadmConfig.status.ready)
         └── Node 健康 (Node.status.conditions)
         │
         ▼
    5. 继续下一个 Machine (maxSurge 控制并发)
         │
         ▼
    6. 所有 Machine 升级完成
```

### 机器分配流转

```
BareMetalHostInventory.spec.hosts[]
         │
         ▼ (BareMetalMachine Controller allocate)
BareMetalMachine.spec {hostName, ipAddress, credentialsRef}
         │
         ▼ (SSH 连接 + 预检)
BareMetalMachine.status {providerID, ready}
         │
         ▼ (CAPI Cluster Controller)
更新 BareMetalHostInventory.status.hostsStatus[].state = "Allocated"
```

### ProviderID 关联 Node

```
BareMetalMachine.status.providerID = "baremetal://node-01"
         │
         ▼ (kubelet 启动时报告相同的 providerID)
Node.spec.providerID = "baremetal://node-01"
         │
         ▼ (CAPI Machine Controller 匹配)
Machine.status.nodeRef → 关联到具体的 Node 资源
```

### Worker 节点创建流转

```
MachineDeployment.spec.replicas
         │
         ▼ (MachineDeployment Controller)
    创建 MachineSet
         │
         ▼ (MachineSet Controller)
    创建 Machine
         │
         ├── bootstrap.configRef → KubeadmConfigTemplate
         │         │
         │         ▼ (KubeadmConfigTemplate Controller)
         │    创建 KubeadmConfig
         │         │
         │         ▼ (生成 join 配置)
         │    Secret (bootstrap-data)
         │
         └── infrastructureRef → BareMetalMachineTemplate
                   │
                   ▼ (BareMetalMachine Controller)
              创建 BareMetalMachine
                   │
                   ├── 从 BareMetalHostInventory 分配主机
                   ├── SSH 连接 + 预检
                   └── 设置 providerID
```

---

## 六、快速查询表

### 常用 kubectl 命令

```bash
# 查看所有 CAPBM 资源
kubectl get baremetalcluster,baremetalmachine,baremetalhostinventory

# 查看所有 Kubeadm 相关资源
kubectl get kubeadmcontrolplane,kubeadmconfig

# 查看机器池状态
kubectl get baremetalhostinventory <name> -o jsonpath='{.status}'

# 查看机器的 ProviderID
kubectl get baremetalmachine <name> -o jsonpath='{.status.providerID}'

# 查看集群端点来源
kubectl get baremetalcluster <name> -o jsonpath='{.metadata.annotations.baremetal\.cluster\.x-k8s\.io/endpoint-source}'

# 查看 Machine 关联的 Node
kubectl get machine <name> -o jsonpath='{.status.nodeRef.name}'

# 查看 KubeadmControlPlane 状态
kubectl get kubeadmcontrolplane <name> -o jsonpath='{.status}'

# 查看 KubeadmConfig 生成的 Secret
kubectl get kubeadmconfig <name> -o jsonpath='{.status.dataSecretName}'

# 查看 MachineDeployment 状态
kubectl get machinedeployment <name> -o jsonpath='{.status}'

# 查看 MachineHealthCheck 状态
kubectl get machinehealthcheck <name> -o jsonpath='{.status}'
```

### 字段必填规则

| 场景 | 必填字段 |
|------|----------|
| 手动创建 BareMetalCluster | controlPlaneEndpoint |
| 手动创建 BareMetalMachine | hostName, ipAddress, credentialsRef |
| ClusterClass 模式 | Cluster.spec.topology 中的 variables |
| 机器池模式 | hostInventoryRef (在 Machine 中) |
| 统一凭据模式 | credentialsRef 可省略，从机器池继承 |
| KubeadmControlPlane | replicas, version, machineTemplate.infrastructureRef |
| KubeadmConfig (控制面) | clusterConfiguration 或 initConfiguration |
| KubeadmConfig (Worker) | joinConfiguration |

### Kubeadm 配置常用字段

| 配置项 | 路径 | 说明 |
|--------|------|------|
| API Server 额外参数 | `clusterConfiguration.apiServer.extraArgs` | 自定义 API Server 参数 |
| etcd 数据目录 | `clusterConfiguration.etcd.local.dataDir` | etcd 数据存储路径 |
| Pod CIDR | `clusterConfiguration.networking.podSubnet` | Pod 网络范围 |
| Service CIDR | `clusterConfiguration.networking.serviceSubnet` | Service 网络范围 |
| DNS 服务 IP | `clusterConfiguration.dns.imageTag` | CoreDNS 镜像版本 |
| kubelet 额外参数 | `initConfiguration.nodeRegistration.kubeletExtraArgs` | 自定义 kubelet 参数 |
| 部署前命令 | `preKubeadmCommands` | kubeadm 执行前运行的命令 |
| 部署后命令 | `postKubeadmCommands` | kubeadm 执行后运行的命令 |
| 自定义文件 | `files` | 要部署到节点的文件 |

### 资源创建顺序

```
1. BareMetalHostInventory (机器池)
2. BareMetalClusterTemplate (ClusterClass 用)
3. BareMetalMachineTemplate (ClusterClass 用)
4. KubeadmControlPlaneTemplate (ClusterClass 用)
5. KubeadmConfigTemplate (ClusterClass 用)
6. ClusterClass
7. Cluster (引用 ClusterClass)
   │
   └── 自动生成:
       ├── BareMetalCluster
       ├── KubeadmControlPlane
       ├── BareMetalMachine (控制面)
       ├── KubeadmConfig (控制面)
       ├── MachineDeployment
       ├── BareMetalMachineTemplate (Worker)
       ├── KubeadmConfigTemplate (Worker)
       ├── BareMetalMachine (Worker)
       └── KubeadmConfig (Worker)
```
