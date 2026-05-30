# ClusterClass 资源创建与控制器调谐全流程详解

## 一、整体架构图

```
用户创建 Cluster 资源
         │
         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    ClusterTopology Controller                    │
│  (CAPI 核心控制器，负责解析 ClusterClass 并生成所有子资源)        │
│                                                                 │
│  输入:                                                          │
│  - Cluster.spec.topology.classRef                               │
│  - Cluster.spec.topology.variables                              │
│  - Cluster.spec.topology.controlPlane.replicas                  │
│  - Cluster.spec.topology.workers.machineDeployments             │
│                                                                 │
│  读取:                                                          │
│  - ClusterClass 定义                                            │
│  - 所有 templateRef 指向的模板资源                               │
│                                                                 │
│  输出:                                                          │
│  - BareMetalCluster                                             │
│  - KubeadmControlPlane                                          │
│  - BareMetalMachineTemplate (CP)                                │
│  - MachineDeployment (Worker)                                   │
│  - BareMetalMachineTemplate (Worker)                            │
│  - KubeadmConfigTemplate                                        │
│  - MachineHealthCheck                                           │
└──────────────────────────┬──────────────────────────────────────┘
                           │ 创建/更新资源
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    各 Provider 控制器                            │
│                                                                 │
│  ┌─────────────────────┐  ┌─────────────────────────────────┐  │
│  │ BareMetalCluster    │  │ KubeadmControlPlane Controller  │  │
│  │ Controller          │  │                                 │  │
│  │ - 验证端点          │  │ - 创建控制面 Machine            │  │
│  │ - 设置 Ready        │  │ - 生成 kubeadm 配置             │  │
│  └─────────┬───────────┘  └──────────────┬──────────────────┘  │
│            │                             │                      │
│            ▼                             ▼                      │
│  ┌─────────────────────┐  ┌─────────────────────────────────┐  │
│  │ Machine Controller  │◄─┤ BareMetalMachine Controller     │  │
│  │ - 关联 InfraMachine │  │ - 分配主机                      │  │
│  │ - 设置 ProviderRef  │  │ - SSH 连接                      │  │
│  └─────────┬───────────┘  │ - 预检                          │  │
│            │              │ - 设置 ProviderID               │  │
│            ▼              └─────────────────────────────────┘  │
│  ┌─────────────────────┐                                       │
│  │ KubeadmConfig       │                                       │
│  │ Controller          │                                       │
│  │ - 生成 cloud-init   │                                       │
│  │ - 设置 bootstrap    │                                       │
│  │   data secret       │                                       │
│  └─────────────────────┘                                       │
└─────────────────────────────────────────────────────────────────┘
```

## 二、详细流程分解

### 阶段 1：用户创建 Cluster 资源

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
  namespace: default
spec:
  topology:
    classRef:
      name: baremetal-clusterclass-v0.1.0
    version: v1.31.0
    controlPlane:
      replicas: 3
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
        replicas: 2
    variables:
    - name: controlPlaneEndpoint
      value:
        host: "lb.example.com"
        port: 6443
    - name: credentialsSecret
      value: "baremetal-ssh-credentials"
    - name: hostInventoryRef
      value: "datacenter-a-hosts"
```

**触发**：Kubernetes API Server 创建 Cluster 资源，ClusterTopology Controller watch 到事件。

---

### 阶段 2：ClusterTopology Controller 调谐

#### 2.1 读取 ClusterClass

```
ClusterTopology Controller
    │
    ├── 读取 Cluster.spec.topology.classRef
    │   └── 获取 ClusterClass: baremetal-clusterclass-v0.1.0
    │
    ├── 解析 ClusterClass.spec
    │   ├── infrastructure.templateRef → BareMetalClusterTemplate
    │   ├── controlPlane.templateRef → KubeadmControlPlaneTemplate
    │   ├── controlPlane.machineInfrastructure.templateRef → BareMetalMachineTemplate (CP)
    │   └── workers.machineDeployments[0]
    │       ├── bootstrap.templateRef → KubeadmConfigTemplate
    │       └── infrastructure.templateRef → BareMetalMachineTemplate (Worker)
    │
    └── 解析 ClusterClass.spec.variables
        └── 定义可配置变量及 schema
```

#### 2.2 应用 Patches（JSON Patch）

```
ClusterClass.spec.patches:
    │
    ├── patch: controlPlaneEndpoint
    │   ├── selector: BareMetalClusterTemplate
    │   └── jsonPatches:
    │       ├── path: /spec/template/spec/controlPlaneEndpoint/host
    │       │   └── valueFrom: variable: controlPlaneEndpoint.host → "lb.example.com"
    │       └── path: /spec/template/spec/controlPlaneEndpoint/port
    │           └── valueFrom: variable: controlPlaneEndpoint.port → 6443
    │
    ├── patch: credentialsSecret
    │   ├── selector: BareMetalMachineTemplate (controlPlane: true)
    │   │   └── jsonPatches:
    │   │       └── path: /spec/template/spec/credentialsRef/name
    │   │           └── valueFrom: variable: credentialsSecret → "baremetal-ssh-credentials"
    │   └── selector: BareMetalMachineTemplate (machineDeploymentClass: default-worker)
    │       └── jsonPatches:
    │           └── path: /spec/template/spec/credentialsRef/name
    │               └── valueFrom: variable: credentialsSecret → "baremetal-ssh-credentials"
    │
    └── patch: hostInventoryRef
        ├── selector: BareMetalMachineTemplate (controlPlane: true)
        │   └── jsonPatches:
        │       └── path: /spec/template/spec/hostInventoryRef/name
        │           └── valueFrom: variable: hostInventoryRef → "datacenter-a-hosts"
        └── selector: BareMetalMachineTemplate (machineDeploymentClass: default-worker)
            └── jsonPatches:
                └── path: /spec/template/spec/hostInventoryRef/name
                    └── valueFrom: variable: hostInventoryRef → "datacenter-a-hosts"
```

#### 2.3 生成实际资源

ClusterTopology Controller 根据模板和 patch 结果创建以下资源：

```
生成的资源树:
    │
    ├── Cluster (已存在)
    │   └── ownerReferences → 无
    │
    ├── BareMetalCluster (新建)
    │   ├── metadata.name: my-cluster-xxxxx
    │   ├── metadata.ownerReferences: [Cluster]
    │   ├── metadata.labels:
    │   │   ├── cluster.x-k8s.io/cluster-name: my-cluster
    │   │   └── cluster.x-k8s.io/topology-owned: "true"
    │   └── spec:
    │       └── controlPlaneEndpoint:
    │           ├── host: "lb.example.com"
    │           └── port: 6443
    │
    ├── KubeadmControlPlane (新建)
    │   ├── metadata.name: my-cluster-xxxxx
    │   ├── metadata.ownerReferences: [Cluster]
    │   ├── spec.replicas: 3
    │   ├── spec.version: v1.31.0
    │   ├── spec.machineTemplate.infrastructureRef:
    │   │   └── BareMetalMachineTemplate (my-cluster-xxxxx-cp)
    │   └── spec.kubeadmConfigSpec: (从模板 + patch 生成)
    │
    ├── BareMetalMachineTemplate (CP) (新建)
    │   ├── metadata.name: my-cluster-xxxxx-cp
    │   ├── metadata.ownerReferences: [KubeadmControlPlane]
    │   └── spec.template.spec:
    │       ├── sshPort: 22
    │       ├── credentialsRef.name: "baremetal-ssh-credentials"
    │       ├── hostInventoryRef.name: "datacenter-a-hosts"
    │       └── role: "control-plane"
    │
    ├── MachineDeployment (新建)
    │   ├── metadata.name: my-cluster-md-0-xxxxx
    │   ├── metadata.ownerReferences: [Cluster]
    │   ├── spec.replicas: 2
    │   ├── spec.template.spec:
    │   │   ├── clusterName: my-cluster
    │   │   ├── version: v1.31.0
    │   │   ├── bootstrap.configRef: KubeadmConfigTemplate
    │   │   └── infrastructureRef: BareMetalMachineTemplate
    │
    ├── BareMetalMachineTemplate (Worker) (新建)
    │   ├── metadata.name: my-cluster-md-0-xxxxx
    │   ├── metadata.ownerReferences: [MachineDeployment]
    │   └── spec.template.spec:
    │       ├── sshPort: 22
    │       ├── credentialsRef.name: "baremetal-ssh-credentials"
    │       ├── hostInventoryRef.name: "datacenter-a-hosts"
    │       └── role: "worker"
    │
    ├── KubeadmConfigTemplate (新建)
    │   ├── metadata.name: my-cluster-md-0-xxxxx
    │   └── metadata.ownerReferences: [MachineDeployment]
    │
    └── MachineHealthCheck (新建，如果 ClusterClass 定义了)
        ├── metadata.ownerReferences: [Cluster]
        └── spec: (从 ClusterClass 生成)
```

---

### 阶段 3：各控制器调谐流程

#### 3.1 Cluster Controller 调谐

```
Cluster Controller
    │
    ├── watch: Cluster 资源
    │
    ├── 检查 spec.infrastructureRef
    │   └── 指向 BareMetalCluster
    │
    ├── 等待 BareMetalCluster.status.ready = true
    │   └── 如果未就绪，requeue
    │
    ├── 设置 Cluster.status.infrastructureReady = true
    │
    ├── 设置 Cluster.spec.controlPlaneEndpoint
    │   └── 从 BareMetalCluster.spec.controlPlaneEndpoint 复制
    │
    └── 检查 spec.controlPlaneRef
        └── 指向 KubeadmControlPlane
```

#### 3.2 BareMetalCluster Controller 调谐

```
BareMetalCluster Controller
    │
    ├── watch: BareMetalCluster 资源
    │
    ├── 获取 owner Cluster 资源
    │
    ├── 检查 Cluster.spec.controlPlaneEndpoint 是否有效
    │   └── 如果无效，等待 (用户/ClusterClass 提供)
    │
    ├── reconcileControlPlaneEndpoint()
    │   ├── 复制 Cluster.spec.controlPlaneEndpoint 到 BareMetalCluster.spec
    │   └── 复制网络配置 (PodCIDR, ServiceCIDR)
    │
    ├── 设置 BareMetalCluster.status.initialization.provisioned = true
    │
    ├── 设置 BareMetalCluster.status.ready = true
    │
    └── 设置 Conditions
        └── type: Ready, status: True
```

#### 3.3 KubeadmControlPlane Controller 调谐

```
KubeadmControlPlane Controller
    │
    ├── watch: KubeadmControlPlane 资源
    │
    ├── 检查 Cluster.status.infrastructureReady
    │   └── 如果未就绪，等待
    │
    ├── 根据 spec.replicas 创建 Machine 资源
    │   └── 创建 3 个 Machine (control-plane)
    │
    ├── 为每个 Machine 设置:
    │   ├── bootstrap.configRef → KubeadmConfig
    │   └── infrastructureRef → BareMetalMachine
    │
    ├── 生成 kubeadm init 配置
    │   └── 使用 Cluster.spec.controlPlaneEndpoint
    │
    └── 等待控制面 Machine 就绪
        └── 逐个初始化 etcd 和 API Server
```

#### 3.4 Machine Controller 调谐 (控制面)

```
Machine Controller
    │
    ├── watch: Machine 资源
    │
    ├── 检查 bootstrap.dataSecretName
    │   └── 等待 KubeadmConfig Controller 生成
    │
    ├── 检查 infrastructureRef
    │   └── 指向 BareMetalMachine
    │
    ├── 设置 Machine.status.infrastructureReady
    │   └── 等待 BareMetalMachine.status.ready = true
    │
    ├── 设置 Machine.status.bootstrapReady
    │   └── 等待 KubeadmConfig.status.ready = true
    │
    └── 关联 Node 资源
        └── 通过 ProviderID 匹配
```

#### 3.5 BareMetalMachine Controller 调谐 (控制面)

```
BareMetalMachine Controller
    │
    ├── watch: BareMetalMachine 资源
    │
    ├── 获取 owner Machine 资源
    │
    ├── 检查 spec.hostInventoryRef
    │   └── 指向 BareMetalHostInventory
    │
    ├── allocateHostFromInventory()
    │   ├── 获取 BareMetalHostInventory
    │   ├── 查找 Available 状态的机器
    │   ├── 过滤 role = "control-plane"
    │   ├── 更新机器状态为 Allocated
    │   └── 设置 BareMetalMachine.spec:
    │       ├── hostName: "node-01"
    │       ├── ipAddress: "192.168.1.101"
    │       └── credentialsRef: {name: "node-01-credentials"}
    │
    ├── 获取凭据 Secret
    │
    ├── SSHManager.Connect()
    │   └── 建立 SSH 连接到 192.168.1.101:22
    │
    ├── RunPreflightChecks()
    │   ├── OS 版本检查
    │   ├── 内核版本检查
    │   ├── 磁盘空间检查
    │   └── 内存检查
    │
    ├── 设置 ProviderID
    │   └── baremetal://node-01
    │
    ├── 设置 BareMetalMachine.status:
    │   ├── ready: true
    │   ├── providerID: "baremetal://node-01"
    │   └── addresses:
    │       ├── type: InternalIP, address: "192.168.1.101"
    │       └── type: HostName, address: "node-01"
    │
    └── 设置 Conditions
        ├── SSHConnected: True
        ├── PreFlightChecksPassed: True
        └── Ready: True
```

#### 3.6 KubeadmConfig Controller 调谐

```
KubeadmConfig Controller
    │
    ├── watch: KubeadmConfig 资源
    │
    ├── 生成 cloud-init 脚本
    │   ├── 如果是第一个控制面节点:
    │   │   └── kubeadm init 配置
    │   └── 如果是其他节点:
    │       └── kubeadm join 配置
    │
    ├── 创建 bootstrap data Secret
    │   └── 包含 cloud-init 脚本
    │
    └── 设置 KubeadmConfig.status:
        ├── ready: true
        └── dataSecretName: "my-cluster-xxxxx-bootstrap"
```

#### 3.7 MachineDeployment Controller 调谐 (Worker)

```
MachineDeployment Controller
    │
    ├── watch: MachineDeployment 资源
    │
    ├── 等待 Cluster.status.controlPlaneInitialized
    │   └── 控制面就绪后才创建 Worker
    │
    ├── 根据 spec.replicas 创建 MachineSet
    │   └── 创建 1 个 MachineSet
    │
    └── 设置 MachineDeployment.status:
        └── replicas: 2 (期望)
```

#### 3.8 MachineSet Controller 调谐

```
MachineSet Controller
    │
    ├── watch: MachineSet 资源
    │
    ├── 根据 spec.replicas 创建 Machine 资源
    │   └── 创建 2 个 Machine (worker)
    │
    └── 设置 MachineSet.status:
        ├── replicas: 2
        └── readyReplicas: 0 (等待 Machine 就绪)
```

#### 3.9 BareMetalMachine Controller 调谐 (Worker)

```
BareMetalMachine Controller (Worker)
    │
    ├── watch: BareMetalMachine 资源
    │
    ├── allocateHostFromInventory()
    │   ├── 查找 Available 状态的机器
    │   ├── 过滤 role = "worker"
    │   └── 分配 node-04, node-05
    │
    ├── SSH 连接 + 预检 (同控制面)
    │
    └── 设置 ProviderID
        ├── baremetal://node-04
        └── baremetal://node-05
```

---

## 三、完整时序图

```
时间轴 ──────────────────────────────────────────────────────────────────────►

用户    ClusterTopology    Cluster    BareMetalCluster    KCP    Machine    BMM    KubeadmConfig
 │           │               │              │              │        │        │         │
 │─创建 Cluster─►              │              │              │        │        │         │
 │           │               │              │              │        │        │         │
 │           │─读取 ClusterClass─►          │              │        │        │         │
 │           │─应用 Patches─►              │              │        │        │         │
 │           │               │              │              │        │        │         │
 │           │─创建 BareMetalCluster─────────────────────►│        │        │         │
 │           │─创建 KCP─────────────────────►              │        │        │         │
 │           │─创建 MD─────────────────────►              │        │        │         │
 │           │               │              │              │        │        │         │
 │           │               │◄──watch────────────────────│        │        │         │
 │           │               │              │              │        │        │         │
 │           │               │──验证端点────►│              │        │        │         │
 │           │               │◄──Ready=true─│              │        │        │         │
 │           │               │              │              │        │        │         │
 │           │               │──设置 infraReady────────────│        │        │         │
 │           │               │              │              │        │        │         │
 │           │◄──watch──────────────────────│              │        │        │         │
 │           │               │              │              │        │        │         │
 │           │──创建 3 个 Machine─────────────────────────►│        │        │         │
 │           │               │              │              │        │        │         │
 │           │               │              │              │        │◄─watch──│         │
 │           │               │              │              │        │        │         │
 │           │               │              │              │        │──创建 BMM─────────►│
 │           │               │              │              │        │        │         │
 │           │               │              │              │        │        │──分配主机│
 │           │               │              │              │        │        │──SSH 连接│
 │           │               │              │              │        │        │──预检    │
 │           │               │              │              │        │        │──ProviderID
 │           │               │              │              │        │        │◄─Ready──│
 │           │               │              │              │        │        │         │
 │           │               │              │              │        │◄─infraReady──────│
 │           │               │              │              │        │        │         │
 │           │               │              │              │        │──生成 bootstrap──►│
 │           │               │              │              │        │◄─dataSecret─────│
 │           │               │              │              │        │        │         │
 │           │               │              │              │        │──关联 Node───────│
 │           │               │              │              │        │◄─Node Ready─────│
 │           │               │              │              │        │        │         │
 │           │◄─控制面就绪──────────────────│              │        │        │         │
 │           │               │              │              │        │        │         │
 │    MD Controller    MachineSet Controller              │        │        │         │
 │           │               │                            │        │        │         │
 │           │◄─watch────────│                            │        │        │         │
 │           │               │                            │        │        │         │
 │           │──创建 MachineSet───────────────────────────►│        │        │         │
 │           │               │                            │        │        │         │
 │           │               │──创建 2 个 Machine─────────►│        │        │         │
 │           │               │                            │        │        │         │
 │           │               │                            │        │──创建 BMM─────────►│
 │           │               │                            │        │        │         │
 │           │               │                            │        │        │──分配主机│
 │           │               │                            │        │        │──SSH 连接│
 │           │               │                            │        │        │──预检    │
 │           │               │                            │        │        │──ProviderID
 │           │               │                            │        │        │◄─Ready──│
 │           │               │                            │        │        │         │
 │           │               │                            │        │──关联 Node───────│
 │           │               │                            │        │◄─Node Ready─────│
 │           │               │                            │        │        │         │
 │           │◄─Worker 就绪───────────────────────────────│        │        │         │
 │           │                                            │        │        │         │
 │    Component Installer (Helm/Manifest Jobs)            │        │        │         │
 │           │                                            │        │        │         │
 │           │──创建 Job: install-calico                  │        │        │         │
 │           │──创建 Job: install-ceph-csi                │        │        │         │
 │           │──创建 Job: install-gateway-api             │        │        │         │
 │           │──创建 Job: install-metallb                 │        │        │         │
 │           │                                            │        │        │         │
 │           │◄─Job 调度到可用节点────────────────────────│        │        │         │
 │           │──下载 chart/manifest                       │        │        │         │
 │           │──加载容器镜像                              │        │        │         │
 │           │──执行 helm install / kubectl apply         │        │        │         │
 │           │──更新 ConfigMap 状态                       │        │        │         │
 │           │                                            │        │        │         │
 │           │◄─所有组件 installed────────────────────────│        │        │         │
 │           │                                            │        │        │         │
 │    LoadBalancer Controller                             │        │        │         │
 │           │                                            │        │        │         │
 │           │──注册 control-plane 节点到 LB 后端          │        │        │         │
 │           │──注册 worker 节点到 Ingress LB 后端         │        │        │         │
 │           │──更新 LB 状态 ConfigMap                    │        │        │         │
 │           │                                            │        │        │         │
 │           │◄─负载均衡器 Ready──────────────────────────│        │        │         │
 │           │                                            │        │        │         │
 ▼           ▼                    ▼                      ▼        ▼        ▼         ▼
集群创建完成 (所有组件安装完成，负载均衡器就绪)
```

---

## 四、关键 OwnerReferences 关系

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

---

## 五、Controller 调谐触发条件

| 控制器 | Watch 资源 | 触发条件 | Requeue 条件 |
|--------|-----------|----------|-------------|
| **ClusterTopology** | Cluster | topology 变化 | 子资源未就绪 |
| **Cluster** | Cluster | infrastructureRef 变化 | InfraCluster 未就绪 |
| **BareMetalCluster** | BareMetalCluster | 创建/更新 | 端点未设置 |
| **KubeadmControlPlane** | KubeadmControlPlane | 创建/更新 | Cluster 未就绪 |
| **Machine** | Machine | 创建/更新 | Bootstrap/Infra 未就绪 |
| **BareMetalMachine** | BareMetalMachine | 创建/更新 | 主机分配失败/SSH 失败 |
| **KubeadmConfig** | KubeadmConfig | 创建/更新 | 等待集群信息 |
| **MachineDeployment** | MachineDeployment | 创建/更新 | 控制面未初始化 |
| **MachineSet** | MachineSet | 创建/更新 | Machine 未就绪 |
| **BareMetalHostInventory** | BareMetalHostInventory | 创建/更新 | 机器状态变化 |

---

## 六、错误处理与重试

### 典型错误场景

| 场景 | 处理方式 | 重试间隔 |
|------|----------|----------|
| 机器池无可用主机 | 设置 Condition False | 30s |
| SSH 连接失败 | 设置 Condition Warning | 30s |
| 预检失败 | 设置 Condition Error | 60s |
| 凭据 Secret 不存在 | 设置 Condition Error | 10s |
| Cluster 端点未设置 | 等待 | 10s |
| 控制面未初始化 | 等待 | 10s |

### 重试机制

```go
// 典型的重试模式
func (r *BareMetalMachineReconciler) reconcileNormal(...) (ctrl.Result, error) {
    // 1. 分配主机
    if err := r.allocateHost(ctx, bmMachine); err != nil {
        markConditionFalse(...)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, bmMachine)
    }

    // 2. SSH 连接
    if err := r.sshManager.Connect(...); err != nil {
        markConditionFalse(...)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, bmMachine)
    }

    // 3. 预检
    if !preflightResult.Passed {
        markConditionFalse(...)
        return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
    }

    // 成功
    markConditionTrue(...)
    return ctrl.Result{}, r.Status().Update(ctx, bmMachine)
}
```

---

## 七、总结

整个流程可以概括为：

1. **用户创建 Cluster** → 触发 ClusterTopology Controller
2. **ClusterTopology** → 读取 ClusterClass，应用 Patches，生成所有子资源
3. **各 Provider Controller** → 并行调谐各自负责的资源
4. **级联依赖** → 上层资源等待下层资源就绪后才继续
5. **最终状态** → 所有资源 Ready，集群可用

关键设计原则：
- **声明式**：用户只定义期望状态，Controller 负责实现
- **最终一致性**：通过 Requeue 和 Watch 机制逐步收敛
- **OwnerReferences**：自动级联删除和状态传播
- **Conditions**：标准化的状态报告机制
