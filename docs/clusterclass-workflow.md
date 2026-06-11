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
│  │ (modules/capbm)     │  │ - 创建控制面 Machine            │  │
│  │ - 验证端点          │  │ - 生成 kubeadm 配置             │  │
│  │ - 设置 Ready        │  └──────────────┬──────────────────┘  │
│  └─────────┬───────────┘                 │                      │
│            │                             ▼                      │
│            ▼              ┌─────────────────────────────────┐  │
│  ┌─────────────────────┐  │ BareMetalMachine Controller     │  │
│  │ Machine Controller  │◄─┤ (modules/capbm)                 │  │
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
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ ClusterVersion Controller (CVO)                         │   │
│  │ (modules/cvo)                                           │   │
│  │ - 监控 DesiredUpdate 变更                               │   │
│  │ - 验证升级路径                                          │   │
│  │ - 执行 K8S + Addon 升级                                 │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## 二、模块职责

| 模块 | API Group | 控制器 | 职责 |
|------|-----------|--------|------|
| **CAPBM** | `infrastructure.cluster.x-k8s.io` | BareMetalCluster, BareMetalMachine | 基础设施管理 |
| **CVO** | `cvo.capbm.io` | ClusterVersion | 版本管理与升级 |
| **CAPI Core** | `cluster.x-k8s.io` | Cluster, Machine | 集群生命周期 |
| **Kubeadm** | `controlplane.cluster.x-k8s.io` | KubeadmControlPlane | 控制面管理 |
| **Kubeadm Bootstrap** | `bootstrap.cluster.x-k8s.io` | KubeadmConfig | 引导配置 |

## 三、详细流程分解

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
      name: baremetal-clusterclass
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

**控制器**: `cluster.x-k8s.io` 内置的 ClusterTopology Controller

**输入**:
- `Cluster.spec.topology` - 用户定义的集群拓扑
- `ClusterClass` - 集群类定义
- 所有模板资源 (BareMetalMachineTemplate, KubeadmConfigTemplate 等)

**输出**:
```yaml
# 1. BareMetalCluster (基础设施)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalCluster
metadata:
  name: my-cluster
  namespace: default
spec:
  controlPlaneEndpoint:
    host: "lb.example.com"
    port: 6443

---
# 2. KubeadmControlPlane (控制面)
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: KubeadmControlPlane
metadata:
  name: my-cluster-control-plane
  namespace: default
spec:
  replicas: 3
  version: v1.31.0
  machineTemplate:
    infrastructureRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: BareMetalMachineTemplate
      name: my-cluster-control-plane-template
  kubeadmConfigSpec:
    clusterConfiguration:
      apiServer:
        extraArgs:
          authorization-mode: "Node,RBAC"

---
# 3. BareMetalMachineTemplate (控制面模板)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachineTemplate
metadata:
  name: my-cluster-control-plane-template
  namespace: default
spec:
  template:
    spec:
      hostName: ""
      ipAddress: ""
      sshPort: 22
      credentialsRef:
        name: baremetal-ssh-credentials
      role: control-plane

---
# 4. MachineDeployment (Worker)
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineDeployment
metadata:
  name: my-cluster-md-0
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      cluster.x-k8s.io/cluster-name: my-cluster
  template:
    spec:
      clusterName: my-cluster
      version: v1.31.0
      bootstrap:
        configRef:
          apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
          kind: KubeadmConfigTemplate
          name: my-cluster-md-0-template
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        name: my-cluster-md-0-template

---
# 5. BareMetalMachineTemplate (Worker 模板)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachineTemplate
metadata:
  name: my-cluster-md-0-template
  namespace: default
spec:
  template:
    spec:
      hostName: ""
      ipAddress: ""
      sshPort: 22
      credentialsRef:
        name: baremetal-ssh-credentials
      role: worker

---
# 6. KubeadmConfigTemplate
apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
kind: KubeadmConfigTemplate
metadata:
  name: my-cluster-md-0-template
  namespace: default
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          kubeletExtraArgs:
            max-pods: "250"
```

---

### 阶段 3：CAPBM 控制器调谐

#### 3.1 BareMetalCluster Controller

**代码位置**: `modules/capbm/internal/controllers/baremetalcluster_controller.go`

**调和流程**:
```
Reconcile
    │
    ├── 获取 BareMetalCluster
    │
    ├── 验证 controlPlaneEndpoint 是否配置
    │
    ├── 设置 status.ready = true
    │
    └── 更新 Conditions
```

**状态更新**:
```yaml
status:
  ready: true
  conditions:
    - type: Ready
      status: "True"
      reason: ClusterReady
      message: "Cluster infrastructure is ready"
```

#### 3.2 BareMetalMachine Controller

**代码位置**: `modules/capbm/internal/controllers/baremetalmachine_controller.go`

**调和流程**:
```
Reconcile
    │
    ├── 获取 BareMetalMachine
    │
    ├── 获取关联的 Machine 对象
    │
    ├── 获取凭据 Secret
    │
    ├── 建立 SSH 连接
    │   ├── 连接失败 → 设置条件失败 → 重试
    │   └── 连接成功 → 继续
    │
    ├── 执行预检
    │   ├── OS 版本检查
    │   ├── 网络连通性检查
    │   ├── 内核参数检查
    │   └── 磁盘空间检查
    │
    ├── 生成 ProviderID (baremetal://<hostname>)
    │
    ├── 更新 status.providerID
    │
    ├── 更新 status.addresses
    │
    └── 设置 status.ready = true
```

**状态更新**:
```yaml
status:
  ready: true
  providerID: "baremetal://node-01"
  addresses:
    - type: InternalIP
      address: "192.168.1.101"
    - type: HostName
      address: "node-01"
  conditions:
    - type: Ready
      status: "True"
      reason: SSHConnected
    - type: PreFlightChecksPassed
      status: "True"
      reason: ChecksPassed
```

---

### 阶段 4：CVO 控制器调谐

#### 4.1 ClusterVersion Controller

**代码位置**: `modules/cvo/internal/controllers/clusterversion_controller.go`

**用户创建 ClusterVersion**:
```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
  namespace: default
spec:
  clusterRef:
    name: my-cluster
    namespace: default
  desiredUpdate:
    version: v1.31.0
    image: registry.example.com/capbm/release:v1.31.0
```

**调和流程**:
```
Reconcile
    │
    ├── 获取 ClusterVersion
    │
    ├── 同步 UpgradePath 和 ReleaseCatalog
    │
    ├── 计算可用更新
    │
    ├── 判断是否需要升级
    │   ├── K8S 升级: cv.Status.ActualVersion != cv.Spec.DesiredUpdate.Version
    │   └── Addon 升级: 遍历 ReleaseImage.Addons 比较版本
    │
    ├── 获取目标 ReleaseImage
    │
    ├── Phase 1: K8S 升级 (仅当 K8S 版本变更时)
    │   └── GraphExecutor.ExecuteUpgradeGraph()
    │
    ├── Phase 2: Addon 升级 (总是执行)
    │   └── executeAddonUpgrades()
    │
    └── 更新状态
        ├── cv.Status.ActualVersion = cv.Spec.DesiredUpdate.Version
        └── cv.Status.AddonStatus = ...
```

**状态更新**:
```yaml
status:
  actualVersion: v1.31.0
  desired:
    version: v1.31.0
    image: registry.example.com/capbm/release:v1.31.0
  conditions:
    - type: Available
      status: "True"
      reason: AsExpected
    - type: Progressing
      status: "False"
      reason: AsExpected
  addonStatus:
    - name: calico
      version: v3.28.0
      phase: Installed
    - name: ceph-csi
      version: v3.11.0
      phase: Installed
```

---

### 阶段 5：升级触发流程

#### 5.1 K8S + Addon 同时升级

```yaml
# 用户修改 ClusterVersion
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
spec:
  desiredUpdate:
    version: v1.31.1  # 从 v1.31.0 升级到 v1.31.1
```

**触发流程**:
```
Reconcile
    │
    ├── 判断 K8S 升级: cv.Status.ActualVersion != cv.Spec.DesiredUpdate.Version ✓
    │
    ├── 判断 Addon 升级: 遍历 ReleaseImage.Addons 比较版本 ✓
    │
    ├── Phase 1: K8S 升级
    │   └── GraphExecutor.ExecuteUpgradeGraph()
    │
    └── Phase 2: Addon 升级
        └── executeAddonUpgrades()
```

#### 5.2 仅 Addon 升级 (K8S 版本不变)

```yaml
# 用户修改 ClusterVersion (K8S 版本不变，但指向新 ReleaseImage)
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
spec:
  desiredUpdate:
    version: v1.31.0     # K8S 版本不变
    image: registry.example.com/capbm/release:v1.31.0-patch1  # 新 ReleaseImage
```

**触发流程**:
```
Reconcile
    │
    ├── 判断 K8S 升级: cv.Status.ActualVersion == cv.Spec.DesiredUpdate.Version ✗
    │   └── 跳过 K8S 升级
    │
    ├── 判断 Addon 升级: 遍历 ReleaseImage.Addons 比较版本 ✓
    │
    └── Phase 2: Addon 升级
        └── executeAddonUpgrades()
```

---

## 四、完整时序图

```
用户    ClusterTopology    Cluster    BareMetalCluster    KCP    Machine    BMM    ClusterVersion
 │           │               │              │              │        │         │          │
 │──创建 Cluster────────────►│              │              │        │         │          │
 │           │               │              │              │        │         │          │
 │           │─解析 ClusterClass────────────►│              │        │         │          │
 │           │               │              │              │        │         │          │
 │           │─创建 BareMetalCluster───────►│              │        │         │          │
 │           │               │              │              │        │         │          │
 │           │─创建 KCP─────────────────────┼─────────────►│        │         │          │
 │           │               │              │              │        │         │          │
 │           │─创建 MachineDeployment───────┼──────────────┼───────►│         │          │
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │         │          │
 │           │               │              │◄─调和────────│        │         │          │
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │◄─调和────│          │
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │◄─分配主机│          │
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │◄─SSH 连接│          │
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │◄─预检────│          │
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │◄─设置 ProviderID    │
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │         │          │
 │──创建 ClusterVersion─────────────────────────────────────────────────────────►│
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │         │◄─调和    │
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │         │◄─同步 UpgradePath
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │         │◄─同步 ReleaseCatalog
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │         │◄─获取 ReleaseImage
 │           │               │              │              │        │         │          │
 │           │               │              │              │        │         │◄─执行升级
 │           │               │              │              │        │         │          │
```

---

## 五、关键控制器代码位置

| 控制器 | 模块 | 代码路径 |
|--------|------|---------|
| BareMetalCluster Controller | CAPBM | `modules/capbm/internal/controllers/baremetalcluster_controller.go` |
| BareMetalMachine Controller | CAPBM | `modules/capbm/internal/controllers/baremetalmachine_controller.go` |
| ClusterVersion Controller | CVO | `modules/cvo/internal/controllers/clusterversion_controller.go` |
| Graph Executor | CVO | `modules/cvo/internal/upgrader/graph_executor.go` |
| Addon Upgrader | CVO | `modules/cvo/internal/addon/upgrader.go` |

---

## 六、API Groups 参考

### CVO 模块 (`cvo.capbm.io`)

| CRD | 描述 |
|-----|------|
| `ClusterVersion` | 集群版本状态和升级目标 |
| `ReleaseImage` | 发布版本镜像和组件定义 |
| `UpgradePath` | 升级路径和兼容性规则 |
| `ReleaseCatalog` | 可用发布版本目录 |
| `ClusterAddon` | 集群插件生命周期管理 |

### CAPBM 模块 (`infrastructure.cluster.x-k8s.io`)

| CRD | 描述 |
|-----|------|
| `BareMetalCluster` | 裸金属集群基础设施 |
| `BareMetalMachine` | 裸金属机器实例 |
| `BareMetalHostInventory` | 主机池管理 |
| `BareMetalClusterTemplate` | 集群模板 |
| `BareMetalMachineTemplate` | 机器模板 |

---

## 七、关键设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **架构模式** | 单模块 vs 多模块 | 多模块 | CVO 和 CAPBM 职责分离，独立演进 |
| **升级触发** | 独立字段 vs DesiredUpdate | DesiredUpdate | 统一入口，简化使用 |
| **升级顺序** | Addon 先 vs K8S 先 | K8S 先 | Addon 通常兼容多个 K8S 版本 |
| **版本来源** | ReleaseImage vs 直接指定 | ReleaseImage | 保持单一数据源 |
| **并发控制** | 串行 vs 并行 | 串行 (依赖顺序) | 保证依赖关系 |
