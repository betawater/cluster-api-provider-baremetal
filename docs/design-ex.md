# CAPBM (Cluster API Provider Bare Metal) - ClusterClass 方案设计

## 一、架构总览
```
用户输入 (机器池清单 + 拓扑配置)
    │
    ├── 机器池定义 (BareMetalHostInventory)
    │   ├── node-01, 192.168.1.101, root, password123, control-plane
    │   ├── node-02, 192.168.1.102, root, password123, control-plane
    │   ├── node-03, 192.168.1.103, root, password123, control-plane
    │   ├── node-04, 192.168.1.104, root, password123, worker
    │   └── node-05, 192.168.1.105, root, password123, worker
    │
    └── Cluster 拓扑定义 (replicas, version, variables)
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Management Cluster                       │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  CAPI Core (内置)                                     │  │
│  │  ClusterTopology Controller ──→ 管理 ClusterClass     │  │
│  │  Machine Controller ──→ 关联 Machine 与 Node          │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  CAPBM Provider (自研)                                │  │
│  │  ┌─────────────────────────────────────────────────┐  │  │
│  │  │ BareMetalCluster Controller                     │  │  │
│  │  │ - 管理集群级别基础设施状态                        │  │  │
│  │  └─────────────────────────────────────────────────┘  │  │
│  │  ┌─────────────────────────────────────────────────┐  │  │
│  │  │ BareMetalMachine Controller                     │  │  │
│  │  │ - 从机器池分配机器                                │  │  │
│  │  │ - SSH 连接管理                                  │  │  │
│  │  │ - 机器预检 (OS/网络/内核)                        │  │  │
│  │  │ - ProviderID 生成与状态上报                      │  │  │
│  │  └─────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  Kubeadm Bootstrap Provider (内置)                    │  │
│  │  - 生成 kubeadm init/join 配置                        │  │
│  │  - 执行 cloud-init/Ignition 脚本                      │  │
│  └───────────────────────────────────────────────────────┘ │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  ClusterClass 定义                                    │  │
│  │  - BareMetalClusterClass                              │  │
│  │  - BareMetalMachineTemplate (CP/Worker)               │  │
│  │  - KubeadmControlPlaneTemplate                        │  │
│  │  - KubeadmConfigTemplate                              │  │
│  │  - Variables & Patches                                │  │
│  └───────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────┘
         │
         ▼ (SSH + cloud-init)
┌────────────────────────────────────────────────────────────┐
│                    Workload Nodes (裸金属)                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │ node-01  │  │ node-02  │  │ node-03  │                  │
│  │ OS已安装 │  │ OS 已安装 │  │ OS 已安装 │                 │
│  │ SSH可达  │  │ SSH 可达  │  │ SSH 可达 │                  │
│  └──────────┘  └──────────┘  └──────────┘                  │
└────────────────────────────────────────────────────────────┘
```

## 二、核心概念

### 2.1 什么是 ClusterClass

ClusterClass 是 Cluster API 的高级抽象，通过声明式模板定义集群拓扑结构，实现：
- **减少样板代码**：无需手动创建每个 Machine/Template 资源
- **灵活定制**：通过 Variables 和 Patches 实现集群间差异化配置
- **生命周期管理**：支持升级、扩缩容、健康检查等自动化操作

### 2.2 机器信息管理机制

在裸金属环境中，机器是预先存在的物理服务器。ClusterClass 通过 `replicas` 指定数量，但需要有一个地方定义可用的机器列表。CAPBM 采用 **BareMetalHostInventory** 资源来管理可用机器池：

```
┌─────────────────────────────────────────────────────────┐
│              BareMetalHostInventory                      │
│  ┌───────────────────────────────────────────────────┐  │
│  │  hosts:                                            │  │
│  │  - name: node-01                                   │  │
│  │    ip: 192.168.1.101                               │  │
│  │    credentialsRef: node-01-creds                   │  │
│  │    role: control-plane                             │  │
│  │    status: Available                               │  │
│  │  - name: node-02                                   │  │
│  │    ip: 192.168.1.102                               │  │
│  │    credentialsRef: node-02-creds                   │  │
│  │    role: control-plane                             │  │
│  │    status: Allocated (cluster-a)                   │  │
│  │  ...                                               │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

**工作流程**：
1. 用户创建 `BareMetalHostInventory` 定义所有可用裸金属机器
2. 创建 Cluster 时指定 `hostInventoryRef` 引用机器池
3. BareMetalMachine Controller 根据 `replicas` 从机器池中分配可用机器
4. 分配后更新机器状态为 `Allocated`，删除时释放回 `Available`

### 2.3 与原方案的对比

| 维度 | 原方案 (手动资源) | ClusterClass 方案 |
|------|------------------|-------------------|
| 资源数量 | 每个节点需独立 BareMetalMachine | 通过 Template + replicas 自动管理 |
| 机器信息 | 每个 BareMetalMachine 单独指定 IP/主机名 | 统一在 BareMetalHostInventory 中管理 |
| 配置复用 | 需手动复制 Template | ClusterClass 定义一次，多处使用 |
| 差异化配置 | 需修改多个资源 | 通过 Variables 和 Overrides 实现 |
| 升级管理 | 手动逐个升级 | 拓扑控制器自动编排升级流程 |
| 扩展性 | 添加节点需创建新资源 | 修改 replicas 即可，自动从机器池分配 |

## 三、CRD 设计

### 3.1 BareMetalCluster (保持不变)

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalCluster
metadata:
  name: my-cluster
  namespace: default
spec:
  controlPlaneEndpoint:
    host: "lb.example.com"
    port: 6443
  network:
    podCIDR: "10.244.0.0/16"
    serviceCIDR: "10.96.0.0/12"
    dnsDomain: "cluster.local"
status:
  ready: true
  conditions:
    - type: Ready
      status: "True"
      reason: ClusterReady
      message: "Cluster infrastructure is ready"
```

**Go 类型定义** (保持不变):
```go
type BareMetalClusterSpec struct {
    ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
    Network              NetworkConfig         `json:"network,omitempty"`
}

type NetworkConfig struct {
    PodCIDR     string `json:"podCIDR,omitempty"`
    ServiceCIDR string `json:"serviceCIDR,omitempty"`
    DNSDomain   string `json:"dnsDomain,omitempty"`
}

type BareMetalClusterStatus struct {
    Ready      bool               `json:"ready,omitempty"`
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

### 3.2 BareMetalClusterTemplate (新增)

用于 ClusterClass 引用的集群模板：

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalClusterTemplate
metadata:
  name: baremetal-clusterclass-v0.1.0
  namespace: default
spec:
  template:
    spec:
      controlPlaneEndpoint:
        host: "${CONTROL_PLANE_ENDPOINT}"
        port: 6443
```

### 3.3 BareMetalMachineTemplate (优化)

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachineTemplate
metadata:
  name: baremetal-clusterclass-v0.1.0-control-plane
  namespace: default
spec:
  template:
    spec:
      sshPort: 22
      credentialsRef:
        name: "${CREDENTIALS_SECRET}"
      role: "control-plane"
      preFlightChecks:
        enabled: true
        osVersions:
          - "ubuntu:20.04"
          - "ubuntu:22.04"
          - "centos:7"
          - "rocky:8"
          - "rocky:9"
        minDiskGB: 20
        minMemoryGB: 2
        kernelVersion: ">=3.10"
```

### 3.4 BareMetalHostInventory (新增核心 CRD)

这是管理裸金属机器池的核心资源：

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: datacenter-a-hosts
  namespace: default
spec:
  hosts:
  - name: node-01
    hostName: "node-01"
    ipAddress: "192.168.1.101"
    sshPort: 22
    credentialsRef:
      name: node-01-credentials
    role: "control-plane"
    labels:
      rack: "rack-1"
      zone: "zone-a"
  - name: node-02
    hostName: "node-02"
    ipAddress: "192.168.1.102"
    sshPort: 22
    credentialsRef:
      name: node-02-credentials
    role: "control-plane"
    labels:
      rack: "rack-1"
      zone: "zone-a"
  - name: node-03
    hostName: "node-03"
    ipAddress: "192.168.1.103"
    sshPort: 22
    credentialsRef:
      name: node-03-credentials
    role: "control-plane"
    labels:
      rack: "rack-2"
      zone: "zone-b"
  - name: node-04
    hostName: "node-04"
    ipAddress: "192.168.1.104"
    sshPort: 22
    credentialsRef:
      name: node-04-credentials
    role: "worker"
    labels:
      rack: "rack-2"
      zone: "zone-b"
  - name: node-05
    hostName: "node-05"
    ipAddress: "192.168.1.105"
    sshPort: 22
    credentialsRef:
      name: node-05-credentials
    role: "worker"
    labels:
      rack: "rack-3"
      zone: "zone-a"
status:
  totalHosts: 5
  availableHosts: 2
  allocatedHosts: 3
  hostsStatus:
  - name: node-01
    state: Allocated
    clusterRef:
      name: cluster-a
      namespace: default
  - name: node-02
    state: Allocated
    clusterRef:
      name: cluster-a
      namespace: default
  - name: node-03
    state: Available
  - name: node-04
    state: Available
  - name: node-05
    state: Allocated
    clusterRef:
      name: cluster-b
      namespace: default
```

**Go 类型定义**:
```go
type BareMetalHostInventorySpec struct {
    Hosts []HostEntry `json:"hosts"`
}

type HostEntry struct {
    Name           string                        `json:"name"`
    HostName       string                        `json:"hostName"`
    IPAddress      string                        `json:"ipAddress"`
    SSHPort        int                           `json:"sshPort,omitempty"`
    CredentialsRef corev1.LocalObjectReference   `json:"credentialsRef"`
    Role           string                        `json:"role,omitempty"`
    Labels         map[string]string             `json:"labels,omitempty"`
}

type BareMetalHostInventoryStatus struct {
    TotalHosts     int              `json:"totalHosts"`
    AvailableHosts int              `json:"availableHosts"`
    AllocatedHosts int              `json:"allocatedHosts"`
    HostsStatus    []HostStatusEntry `json:"hostsStatus,omitempty"`
}

type HostStatusEntry struct {
    Name       string                  `json:"name"`
    State      HostState               `json:"state"`
    ClusterRef *corev1.ObjectReference `json:"clusterRef,omitempty"`
}

type HostState string

const (
    HostStateAvailable  HostState = "Available"
    HostStateAllocated  HostState = "Allocated"
    HostStateMaintenance HostState = "Maintenance"
)
```

### 3.5 ClusterClass 定义

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: ClusterClass
metadata:
  name: baremetal-clusterclass-v0.1.0
  namespace: default
spec:
  controlPlane:
    templateRef:
      apiVersion: controlplane.cluster.x-k8s.io/v1beta2
      kind: KubeadmControlPlaneTemplate
      name: baremetal-clusterclass-v0.1.0-control-plane
    machineInfrastructure:
      templateRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        name: baremetal-clusterclass-v0.1.0-control-plane
    healthCheck:
      checks:
        nodeStartupTimeoutSeconds: 900
        unhealthyNodeConditions:
        - type: Ready
          status: Unknown
          timeoutSeconds: 300
        - type: Ready
          status: "False"
          timeoutSeconds: 300
  infrastructure:
    templateRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: BareMetalClusterTemplate
      name: baremetal-clusterclass-v0.1.0
  workers:
    machineDeployments:
    - class: default-worker
      bootstrap:
        templateRef:
          apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
          kind: KubeadmConfigTemplate
          name: baremetal-clusterclass-v0.1.0-default-worker
      infrastructure:
        templateRef:
          apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
          kind: BareMetalMachineTemplate
          name: baremetal-clusterclass-v0.1.0-default-worker
      healthCheck:
        checks:
          nodeStartupTimeoutSeconds: 600
          unhealthyNodeConditions:
          - type: Ready
            status: Unknown
            timeoutSeconds: 300
          - type: Ready
            status: "False"
            timeoutSeconds: 300
  variables:
  - name: controlPlaneEndpoint
    required: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          host:
            type: string
            description: "控制面负载均衡地址"
          port:
            type: integer
            description: "API Server 端口"
            default: 6443
  - name: credentialsSecret
    required: true
    schema:
      openAPIV3Schema:
        type: string
        description: "SSH 凭据 Secret 名称"
  - name: hostInventoryRef
    required: true
    schema:
      openAPIV3Schema:
        type: string
        description: "BareMetalHostInventory 资源名称"
  - name: kubernetesVersion
    required: true
    schema:
      openAPIV3Schema:
        type: string
        description: "Kubernetes 版本"
        pattern: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
  - name: podCIDR
    schema:
      openAPIV3Schema:
        type: string
        default: "10.244.0.0/16"
        description: "Pod CIDR"
  - name: serviceCIDR
    schema:
      openAPIV3Schema:
        type: string
        default: "10.96.0.0/12"
        description: "Service CIDR"
  - name: preFlightChecks
    schema:
      openAPIV3Schema:
        type: object
        properties:
          enabled:
            type: boolean
            default: true
          minDiskGB:
            type: integer
            default: 20
          minMemoryGB:
            type: integer
            default: 2
  patches:
  - name: controlPlaneEndpoint
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalClusterTemplate
        matchResources:
          infrastructureCluster: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/controlPlaneEndpoint/host
        valueFrom:
          variable: controlPlaneEndpoint.host
      - op: add
        path: /spec/template/spec/controlPlaneEndpoint/port
        valueFrom:
          variable: controlPlaneEndpoint.port
  - name: credentialsSecret
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/credentialsRef/name
        valueFrom:
          variable: credentialsSecret
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        matchResources:
          machineDeploymentClass:
            names:
            - default-worker
      jsonPatches:
      - op: add
        path: /spec/template/spec/credentialsRef/name
        valueFrom:
          variable: credentialsSecret
  - name: kubernetesVersion
    definitions:
    - selector:
        apiVersion: controlplane.cluster.x-k8s.io/v1beta2
        kind: KubeadmControlPlaneTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/version
        valueFrom:
          variable: kubernetesVersion
    - selector:
        apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
        kind: KubeadmConfigTemplate
        matchResources:
          machineDeploymentClass:
            names:
            - default-worker
      jsonPatches:
      - op: add
        path: /spec/template/spec/clusterConfiguration/kubernetesVersion
        valueFrom:
          variable: kubernetesVersion
  - name: networkCIDRs
    definitions:
    - selector:
        apiVersion: controlplane.cluster.x-k8s.io/v1beta2
        kind: KubeadmControlPlaneTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/kubeadmConfigSpec/clusterConfiguration/networking/podSubnet
        valueFrom:
          variable: podCIDR
      - op: add
        path: /spec/template/spec/kubeadmConfigSpec/clusterConfiguration/networking/serviceSubnet
        valueFrom:
          variable: serviceCIDR
```

### 3.6 KubeadmControlPlaneTemplate

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: KubeadmControlPlaneTemplate
metadata:
  name: baremetal-clusterclass-v0.1.0-control-plane
  namespace: default
spec:
  template:
    spec:
      kubeadmConfigSpec:
        clusterConfiguration:
          apiServer:
            extraArgs:
              authorization-mode: "Node,RBAC"
          etcd:
            local:
              dataDir: /var/lib/etcd
          networking:
            podSubnet: ""
            serviceSubnet: ""
        initConfiguration:
          nodeRegistration:
            kubeletExtraArgs:
              max-pods: "250"
```

### 3.7 KubeadmConfigTemplate (Worker)

```yaml
apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
kind: KubeadmConfigTemplate
metadata:
  name: baremetal-clusterclass-v0.1.0-default-worker
  namespace: default
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          kubeletExtraArgs:
            max-pods: "250"
```

## 四、Cluster 使用示例

### 4.1 通过拓扑创建集群

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-baremetal-cluster
  namespace: default
spec:
  topology:
    classRef:
      name: baremetal-clusterclass-v0.1.0
    version: v1.31.0
    controlPlane:
      replicas: 3
      metadata:
        labels:
          role: control-plane
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
        replicas: 2
        metadata:
          labels:
            role: worker
    variables:
    - name: controlPlaneEndpoint
      value:
        host: "lb.example.com"
        port: 6443
    - name: credentialsSecret
      value: "baremetal-ssh-credentials"
    - name: hostInventoryRef
      value: "datacenter-a-hosts"
    - name: kubernetesVersion
      value: "v1.31.0"
    - name: podCIDR
      value: "10.244.0.0/16"
    - name: serviceCIDR
      value: "10.96.0.0/12"
```

### 4.2 凭据 Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: baremetal-ssh-credentials
  namespace: default
stringData:
  username: "root"
  password: "password123"
  # 或使用 SSH Key
  # ssh-privatekey: |
  #   -----BEGIN OPENSSH PRIVATE KEY-----
  #   ...
  #   -----END OPENSSH PRIVATE KEY-----
```

### 4.3 多 Worker 池示例 (差异化配置)

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: multi-pool-cluster
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
        name: general-purpose
        replicas: 3
      - class: default-worker
        name: high-memory
        replicas: 2
        variables:
          overrides:
          - name: preFlightChecks
            value:
              enabled: true
              minMemoryGB: 8
              minDiskGB: 50
    variables:
    - name: controlPlaneEndpoint
      value:
        host: "lb.example.com"
        port: 6443
    - name: credentialsSecret
      value: "baremetal-ssh-credentials"
    - name: hostInventoryRef
      value: "datacenter-a-hosts"
    - name: kubernetesVersion
      value: "v1.31.0"
```

## 五、控制器设计

### 5.1 BareMetalCluster Controller (保持不变)

**职责**:
- 验证集群配置
- 设置 `status.ready = true`
- 处理删除逻辑

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

### 5.2 BareMetalMachine Controller (优化)

**职责**:
- 从 BareMetalHostInventory 分配可用机器
- SSH 连接到目标机器
- 执行预检 (OS 版本、网络、内核参数)
- 生成并设置 ProviderID
- 更新状态和 Conditions
- 释放机器回机器池（删除时）

**调和流程**:
```
Reconcile
    │
    ├── 获取 BareMetalMachine
    │
    ├── 获取关联的 Machine 对象
    │
    ├── 获取 BareMetalHostInventory
    │
    ├── 分配可用机器 (如果未分配)
    │   ├── 查找 Available 状态的机器
    │   ├── 过滤匹配 role 的机器
    │   ├── 更新机器状态为 Allocated
    │   └── 记录 clusterRef
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

**核心代码结构**:
```go
func (r *BareMetalMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    bmMachine := &infrav1.BareMetalMachine{}
    if err := r.Get(ctx, req.NamespacedName, bmMachine); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    machine, err := util.GetOwnerMachine(ctx, r.Client, bmMachine.ObjectMeta)
    if err != nil {
        return ctrl.Result{}, err
    }
    if machine == nil {
        log.Info("Waiting for Machine Controller to set OwnerRef")
        return ctrl.Result{}, nil
    }

    if !bmMachine.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, bmMachine)
    }

    return r.reconcileNormal(ctx, bmMachine, machine)
}

func (r *BareMetalMachineReconciler) reconcileNormal(ctx context.Context, bmMachine *infrav1.BareMetalMachine, machine *clusterv1.Machine) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // 1. 获取 HostInventory
    hostInventory, err := r.getHostInventory(ctx, bmMachine)
    if err != nil {
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    // 2. 分配机器 (如果尚未分配)
    if bmMachine.Spec.HostName == "" {
        host, err := r.allocateHost(ctx, hostInventory, bmMachine)
        if err != nil {
            return ctrl.Result{RequeueAfter: 30 * time.Second}, err
        }
        bmMachine.Spec.HostName = host.HostName
        bmMachine.Spec.IPAddress = host.IPAddress
        bmMachine.Spec.CredentialsRef = host.CredentialsRef
        if err := r.Update(ctx, bmMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 3. 获取凭据
    creds, err := r.getCredentials(ctx, bmMachine)
    if err != nil {
        conditions.MarkFalse(bmMachine, infrav1.MachineReadyCondition, infrav1.CredentialsNotFoundReason, clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, bmMachine)
    }

    // 4. 建立 SSH 连接
    sshClient, err := r.sshManager.Connect(bmMachine.Spec.IPAddress, bmMachine.Spec.SSHPort, creds)
    if err != nil {
        conditions.MarkFalse(bmMachine, infrav1.SSHConnectedCondition, infrav1.SSHConnectionFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
        return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, bmMachine)
    }
    defer sshClient.Close()

    // 5. 执行预检
    if err := r.runPreFlightChecks(ctx, sshClient, bmMachine); err != nil {
        conditions.MarkFalse(bmMachine, infrav1.PreFlightChecksPassedCondition, infrav1.PreFlightChecksFailedReason, clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
    }
    conditions.MarkTrue(bmMachine, infrav1.PreFlightChecksPassedCondition)

    // 6. 设置 ProviderID
    providerID := fmt.Sprintf("baremetal://%s", bmMachine.Spec.HostName)
    if bmMachine.Spec.ProviderID == nil || *bmMachine.Spec.ProviderID != providerID {
        bmMachine.Spec.ProviderID = ptr.To(providerID)
        if err := r.Update(ctx, bmMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 7. 更新状态
    bmMachine.Status.Ready = true
    bmMachine.Status.ProviderID = providerID
    bmMachine.Status.Addresses = []clusterv1.MachineAddress{
        {Type: clusterv1.MachineInternalIP, Address: bmMachine.Spec.IPAddress},
        {Type: clusterv1.MachineHostName, Address: bmMachine.Spec.HostName},
    }
    conditions.MarkTrue(bmMachine, infrav1.MachineReadyCondition)

    return ctrl.Result{}, r.Status().Update(ctx, bmMachine)
}

func (r *BareMetalMachineReconciler) allocateHost(ctx context.Context, inventory *infrav1.BareMetalHostInventory, bmMachine *infrav1.BareMetalMachine) (*infrav1.HostEntry, error) {
    for i, host := range inventory.Spec.Hosts {
        if inventory.Status.HostsStatus[i].State != infrav1.HostStateAvailable {
            continue
        }
        if bmMachine.Spec.Role != "" && host.Role != bmMachine.Spec.Role {
            continue
        }
        // 分配此主机
        inventory.Status.HostsStatus[i].State = infrav1.HostStateAllocated
        inventory.Status.HostsStatus[i].ClusterRef = &corev1.ObjectReference{
            Name:      bmMachine.Labels[clusterv1.ClusterNameLabel],
            Namespace: bmMachine.Namespace,
        }
        inventory.Status.AllocatedHosts++
        inventory.Status.AvailableHosts--
        
        if err := r.Status().Update(ctx, inventory); err != nil {
            return nil, err
        }
        return &host, nil
    }
    return nil, fmt.Errorf("no available hosts in inventory %s", inventory.Name)
}
```

### 5.3 BareMetalHostInventory Controller (新增)

**职责**:
- 管理机器池状态
- 跟踪机器分配和释放
- 提供可用机器查询

**调和流程**:
```
Reconcile
    │
    ├── 获取 BareMetalHostInventory
    │
    ├── 统计机器状态
    │   ├── 遍历所有 hosts
    │   ├── 检查是否有关联的 BareMetalMachine
    │   ├── 更新 Available/Allocated 计数
    │   └── 更新 hostsStatus
    │
    └── 更新 status
```

## 六、组件安装设计

### 6.1 问题背景

裸金属机器通常只安装了基础操作系统，缺少 Kubernetes 运行所需的组件：
- **containerd**: 容器运行时
- **kubeadm**: 集群初始化工具
- **kubelet**: 节点代理
- **kubectl**: 命令行工具（可选）

CAPBM 需要在预检通过后、kubeadm 执行前，通过 SSH 远程安装这些组件。

### 6.2 架构设计

#### 6.2.1 组件安装流程

```
┌─────────────────────────────────────────────────────────────────┐
│                    组件安装流程                                  │
│                                                                 │
│  预检通过                                                        │
│      │                                                          │
│      ▼                                                          │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 1. 检测 OS 类型和包管理器                                 │  │
│  │    - Ubuntu/Debian → apt                                 │  │
│  │    - CentOS/RHEL/Rocky → yum/dnf                         │  │
│  └────────────────────────┬─────────────────────────────────┘  │
│                           │                                    │
│                           ▼                                    │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ 2. 检查组件是否已安装及版本                               │  │
│  │    - containerd --version                                │  │
│  │    - kubeadm version                                     │  │
│  │    - kubelet --version                                   │  │
│  └────────────────────────┬─────────────────────────────────┘  │
│                           │                                    │
│                    已安装且版本匹配？                           │
│                    ╱              ╲                            │
│                   是              否                           │
│                   │               │                            │
│                   ▼               ▼                            │
│  ┌─────────────────────┐ ┌──────────────────────────────────┐ │
│  │ 跳过安装            │ │ 3. 执行安装脚本                   │ │
│  │ 继续预检            │ │    - 配置包仓库                   │ │
│  │                     │ │    - 安装 containerd              │ │
│  │                     │ │    - 安装 kubeadm/kubelet/kubectl │ │
│  │                     │ │    - 生成组件配置                 │ │
│  └─────────────────────┘ └──────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

#### 6.2.2 配置生成流程（与安装耦合）

**核心原则**：配置是组件安装的一部分，不是独立的管理流程。每次安装/升级时，配置完整重新生成。

| 组件 | 配置方式 | CAPBM 管理范围 |
|------|---------|---------------|
| **containerd** | `/etc/containerd/config.toml` | 完整生成，升级时重新生成 |
| **kubelet** | systemd drop-in + `KUBELET_EXTRA_ARGS` | 生成 drop-in 文件，升级时重新生成 |
| **kubeadm** | 无持久配置（通过 `kubeadm init/join` 传递） | 仅安装二进制，配置由 KubeadmConfigTemplate 管理 |
| **kubectl** | 无持久配置 | 仅安装二进制 |

**containerd 配置生成流程**：

```
安装/升级 containerd
    │
    ▼
┌─────────────────────────────────────────────────┐
│ 1. 安装/升级 containerd 包                       │
│    └── apt/dnf install/upgrade containerd       │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 2. 生成配置                                      │
│    ├── containerd config default (当前版本)      │
│    ├── 应用 config.systemdCgroup                │
│    ├── 应用 config.sandboxImage                 │
│    ├── 应用 config.registryMirrors              │
│    ├── 应用 config.maxConcurrentDownloads       │
│    └── 追加 config.rawConfig                    │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 3. 验证并重启                                    │
│    ├── containerd config dump 验证               │
│    └── systemctl restart containerd             │
└─────────────────────────────────────────────────┘
```

**kubelet 配置生成流程**：

```
安装/升级 kubelet
    │
    ▼
┌─────────────────────────────────────────────────┐
│ 1. 安装/升级 kubelet 包                          │
│    └── apt/dnf install/upgrade kubelet          │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 2. 生成 systemd drop-in 配置                     │
│    └── /etc/systemd/system/kubelet.service.d/   │
│        └── 10-capbm.conf                        │
│            ├── Environment="KUBELET_CGROUP_ARGS=  │
│            │   --cgroup-driver=systemd"          │
│            ├── Environment="KUBELET_MAX_PODS=     │
│            │   --max-pods=250"                   │
│            └── Environment="KUBELET_EXTRA_ARGS=   │
│                --feature-gates=..."              │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 3. 重新加载 systemd                              │
│    └── systemctl daemon-reload                  │
└─────────────────────────────────────────────────┘
```

### 6.3 CRD 设计扩展

在 `BareMetalMachineSpec` 中新增组件安装配置。配置与组件耦合，符合高内聚原则：

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachine
spec:
  # ... 现有字段 ...
  
  componentInstall:
    # 是否启用自动安装（默认 true）
    enabled: true
    
    # 安装策略
    strategy: "InstallIfMissing"  # InstallIfMissing | AlwaysInstall | Skip
    
    # 容器运行时配置（配置与 containerd 耦合）
    containerRuntime:
      type: "containerd"          # containerd | docker | cri-o
      version: "1.7.0"            # 可选，不指定则安装最新版
      config:                     # containerd 配置（安装时生成）
        systemdCgroup: true
        sandboxImage: "registry.k8s.io/pause:3.9"
        registryMirrors:
          - host: "docker.io"
            endpoints: ["https://mirror.example.com"]
        maxConcurrentDownloads: 3
        rawConfig: |
          [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
            runtime_type = "io.containerd.runc.v2"
      
    # Kubernetes 组件配置（配置与 kubernetes 耦合）
    kubernetes:
      version: "1.31.0"           # 与 Cluster.spec.topology.version 一致
      repository:                 # 自定义仓库配置
        baseUrl: "https://pkgs.k8s.io"
        gpgKey: "https://pkgs.k8s.io/core:/stable:/v1.31/deb/Release.key"
      config:                     # kubernetes 配置（安装时生成）
        kubelet:
          cgroupDriver: "systemd"
          maxPods: 250
          extraArgs:
            feature-gates: "RotateKubeletServerCertificate=true"
            kube-reserved: "cpu=250m,memory=512Mi"
      
    # 安装超时
    timeout: "300s"
```

**Go 类型定义**:

```go
type ComponentInstallConfig struct {
    // Enabled indicates whether automatic component installation is enabled.
    // +optional
    // +kubebuilder:default=true
    Enabled bool `json:"enabled,omitempty"`
    
    // Strategy defines the installation strategy.
    // +optional
    // +kubebuilder:default=InstallIfMissing
    Strategy InstallStrategy `json:"strategy,omitempty"`
    
    // ContainerRuntime specifies the container runtime configuration.
    // +optional
    ContainerRuntime ContainerRuntimeConfig `json:"containerRuntime,omitempty"`
    
    // Kubernetes specifies the Kubernetes components configuration.
    // +optional
    Kubernetes KubernetesComponentsConfig `json:"kubernetes,omitempty"`
    
    // Timeout is the maximum time to wait for installation to complete.
    // +optional
    Timeout *metav1.Duration `json:"timeout,omitempty"`
    
    // AirGap defines configuration for offline/air-gapped installations.
    // +optional
    AirGap *AirGapConfig `json:"airGap,omitempty"`
    
    // RollbackOnError indicates whether to rollback on installation failure.
    // +optional
    RollbackOnError bool `json:"rollbackOnError,omitempty"`
    
    // MaxRetries is the maximum number of retries for installation.
    // +optional
    // +kubebuilder:default=3
    MaxRetries int `json:"maxRetries,omitempty"`
}

type InstallStrategy string

const (
    // InstallIfMissing installs only if components are not present or version mismatch.
    InstallIfMissing InstallStrategy = "InstallIfMissing"
    // AlwaysInstall always reinstalls components.
    AlwaysInstall InstallStrategy = "AlwaysInstall"
    // Skip skips installation (assumes components are pre-installed).
    Skip InstallStrategy = "Skip"
)

type ContainerRuntimeConfig struct {
    // Type is the container runtime type (containerd, docker, cri-o).
    // +optional
    // +kubebuilder:default=containerd
    Type string `json:"type,omitempty"`
    
    // Version is the desired version of the container runtime.
    // +optional
    Version string `json:"version,omitempty"`
    
    // RegistryMirrors is a list of registry mirror URLs (legacy, use Config instead).
    // +optional
    RegistryMirrors []string `json:"registryMirrors,omitempty"`
    
    // Config holds runtime-specific configuration.
    // Applied during installation/upgrade.
    // +optional
    Config *RuntimeConfig `json:"config,omitempty"`
}

type RuntimeConfig struct {
    // SystemdCgroup enables systemd cgroup driver.
    // +optional
    SystemdCgroup *bool `json:"systemdCgroup,omitempty"`
    
    // SandboxImage is the pause/sandbox image.
    // +optional
    SandboxImage string `json:"sandboxImage,omitempty"`
    
    // RegistryMirrors declares registry mirror configuration.
    // +optional
    RegistryMirrors []RegistryMirrorEntry `json:"registryMirrors,omitempty"`
    
    // MaxConcurrentDownloads sets max concurrent downloads.
    // +optional
    MaxConcurrentDownloads *int `json:"maxConcurrentDownloads,omitempty"`
    
    // RawConfig is raw TOML configuration appended to the final config.
    // +optional
    RawConfig string `json:"rawConfig,omitempty"`
}

type RegistryMirrorEntry struct {
    // Host is the registry host to mirror.
    Host string `json:"host"`
    // Endpoints is the list of mirror endpoints.
    Endpoints []string `json:"endpoints"`
}

type KubernetesComponentsConfig struct {
    // Version is the desired Kubernetes version.
    // +optional
    Version string `json:"version,omitempty"`
    
    // Repository is the custom package repository configuration.
    // +optional
    Repository *PackageRepository `json:"repository,omitempty"`
    
    // Config holds kubernetes component configuration.
    // Applied during installation/upgrade.
    // +optional
    Config *KubernetesConfig `json:"config,omitempty"`
}

type KubernetesConfig struct {
    // Kubelet holds kubelet-specific configuration.
    // +optional
    Kubelet *KubeletConfig `json:"kubelet,omitempty"`
}

type KubeletConfig struct {
    // CgroupDriver sets the cgroup driver (cgroupfs or systemd).
    // +optional
    // +kubebuilder:default=systemd
    CgroupDriver string `json:"cgroupDriver,omitempty"`
    
    // MaxPods sets the maximum number of pods.
    // +optional
    MaxPods *int `json:"maxPods,omitempty"`
    
    // ExtraArgs declares additional kubelet command-line arguments.
    // +optional
    ExtraArgs map[string]string `json:"extraArgs,omitempty"`
    
    // RawConfig is raw kubelet configuration (YAML format).
    // +optional
    RawConfig string `json:"rawConfig,omitempty"`
}

type PackageRepository struct {
    // BaseURL is the base URL of the package repository.
    // +optional
    BaseURL string `json:"baseUrl,omitempty"`
    
    // GPGKey is the URL to the GPG key for the repository.
    // +optional
    GPGKey string `json:"gpgKey,omitempty"`
}

// AirGapConfig defines configuration for offline/air-gapped installations.
type AirGapConfig struct {
    // Enabled indicates whether air-gapped installation mode is used.
    // +optional
    Enabled bool `json:"enabled,omitempty"`
    
    // BinarySource specifies how binaries are delivered in air-gapped mode.
    // Options: HTTPServer | ConfigMap | Secret | LocalPath
    // +optional
    // +kubebuilder:default=HTTPServer
    BinarySource string `json:"binarySource,omitempty"`
    
    // HTTPServerConfig is the configuration for HTTP binary source.
    // +optional
    HTTPServerConfig *HTTPServerConfig `json:"httpServerConfig,omitempty"`
    
    // LocalPath is the path on target machine where binaries are pre-placed.
    // +optional
    LocalPath string `json:"localPath,omitempty"`
    
    // PreloadImages is a list of container images to preload into containerd.
    // +optional
    PreloadImages []string `json:"preloadImages,omitempty"`
}

type HTTPServerConfig struct {
    // BaseURL is the HTTP server URL serving binary packages.
    BaseURL string `json:"baseUrl"`
    
    // TLSSecretRef references a Secret containing TLS client certificate.
    // +optional
    TLSSecretRef *corev1.LocalObjectReference `json:"tlsSecretRef,omitempty"`
    
    // InsecureSkipVerify skips TLS verification (for internal CAs).
    // +optional
    InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}
```

### 6.4 ClusterClass 变量和 Patches

在 ClusterClass 中新增组件安装变量：

```yaml
  variables:
  - name: componentInstall
    schema:
      openAPIV3Schema:
        type: object
        properties:
          enabled:
            type: boolean
            default: true
          strategy:
            type: string
            default: "InstallIfMissing"
            enum:
              - "InstallIfMissing"
              - "AlwaysInstall"
              - "Skip"
          containerRuntime:
            type: object
            properties:
              type:
                type: string
                default: "containerd"
                enum:
                  - "containerd"
                  - "cri-o"
                  - "docker"
              version:
                type: string
              config:
                type: object
                properties:
                  systemdCgroup:
                    type: boolean
                    default: true
                  sandboxImage:
                    type: string
                    default: "registry.k8s.io/pause:3.9"
                  registryMirrors:
                    type: array
                    items:
                      type: object
                      properties:
                        host:
                          type: string
                        endpoints:
                          type: array
                          items:
                            type: string
                  maxConcurrentDownloads:
                    type: integer
                  rawConfig:
                    type: string
                    description: "Raw TOML configuration"
          kubernetes:
            type: object
            properties:
              version:
                type: string
              repository:
                type: object
                properties:
                  baseUrl:
                    type: string
                  gpgKey:
                    type: string
              config:
                type: object
                properties:
                  kubelet:
                    type: object
                    properties:
                      cgroupDriver:
                        type: string
                        default: "systemd"
                      maxPods:
                        type: integer
                        default: 250
                      extraArgs:
                        type: object
                        additionalProperties:
                          type: string
                      rawConfig:
                        type: string
          airGap:
            type: object
            properties:
              enabled:
                type: boolean
                default: false
              binarySource:
                type: string
                default: "HTTPServer"
                enum:
                  - "HTTPServer"
                  - "ConfigMap"
                  - "LocalPath"
              httpServerConfig:
                type: object
                properties:
                  baseUrl:
                    type: string
                  insecureSkipVerify:
                    type: boolean
                    default: false
              preloadImages:
                type: array
                items:
                  type: string
          firewall:
            type: object
            properties:
              configure:
                type: boolean
                default: true
              autoDetect:
                type: boolean
                default: true
          selinux:
            type: object
            properties:
              configure:
                type: boolean
                default: true
          timeout:
            type: string
            default: "300s"
          rollbackOnError:
            type: boolean
            default: false
          maxRetries:
            type: integer
            default: 3
```

Patch 定义：

```yaml
  patches:
  - name: componentInstall
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/componentInstall
        valueFrom:
          variable: componentInstall
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        matchResources:
          machineDeploymentClass:
            names:
            - default-worker
      jsonPatches:
      - op: add
        path: /spec/template/spec/componentInstall
        valueFrom:
          variable: componentInstall
```

### 6.5 安装脚本设计

#### 6.5.1 安装流程

```
SSH 连接成功
    │
    ▼
检测 OS 类型
    │
    ├── Ubuntu/Debian
    │   ├── 检测 apt
    │   ├── 配置 Kubernetes apt 仓库
    │   └── 使用 apt 安装
    │
    ├── CentOS/RHEL/Rocky/AlmaLinux
    │   ├── 检测 yum/dnf
    │   ├── 配置 Kubernetes yum 仓库
    │   └── 使用 yum/dnf 安装
    │
    └── 不支持的 OS → 报错退出
    
    ▼
安装 containerd
    │
    ├── 检查是否已安装
    ├── 安装 containerd 包
    ├── 生成配置（与安装耦合）
    │   ├── containerd config default
    │   ├── 应用 systemdCgroup
    │   ├── 应用 sandboxImage
    │   ├── 应用 registryMirrors
    │   └── 追加 rawConfig
    └── 验证并重启服务
    
    ▼
安装 Kubernetes 组件
    │
    ├── 安装 kubeadm（仅二进制，无持久配置）
    ├── 安装 kubelet
    │   ├── 安装 kubelet 包
    │   └── 生成 systemd drop-in 配置
    │       └── /etc/systemd/system/kubelet.service.d/10-capbm.conf
    ├── 安装 kubectl（仅二进制，无持久配置）
    └── 启用 kubelet 服务
    
    ▼
验证安装
    │
    ├── containerd --version
    ├── containerd config dump
    ├── kubeadm version
    ├── kubelet --version
    └── systemctl is-active kubelet
```

#### 6.5.2 containerd 配置生成脚本（Ubuntu/Debian）

```bash
#!/bin/bash
set -euo pipefail

CONFIG_FILE="/etc/containerd/config.toml"
SYSTEMD_CGROUP="${SYSTEMD_CGROUP:-true}"
SANDBOX_IMAGE="${SANDBOX_IMAGE:-registry.k8s.io/pause:3.9}"
REGISTRY_MIRRORS="${REGISTRY_MIRRORS:-}"
MAX_CONCURRENT_DOWNLOADS="${MAX_CONCURRENT_DOWNLOADS:-}"
RAW_CONFIG="${RAW_CONFIG:-}"

echo "=== 安装并配置 containerd (Ubuntu/Debian) ==="

# 1. 安装 containerd
install_containerd() {
    if command -v containerd &>/dev/null; then
        current_version=$(containerd --version | awk '{print $3}')
        if [ -n "$CONTAINERD_VERSION" ] && [ "$current_version" != "$CONTAINERD_VERSION" ]; then
            echo "Upgrading containerd: $current_version -> $CONTAINERD_VERSION"
            apt-get remove -y containerd || true
            apt-get install -y containerd
        else
            echo "containerd already installed: $current_version"
        fi
    else
        apt-get update
        apt-get install -y containerd
    fi
}

# 2. 生成配置（安装的一部分）
generate_containerd_config() {
    local temp_config=$(mktemp)
    
    # 生成当前版本默认配置
    containerd config default > "$temp_config"
    
    # 应用基础配置
    if [ "$SYSTEMD_CGROUP" = "true" ]; then
        sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' "$temp_config"
    fi
    
    if [ -n "$SANDBOX_IMAGE" ]; then
        sed -i "s|sandbox_image = .*|sandbox_image = \"${SANDBOX_IMAGE}\"|g" "$temp_config"
    fi
    
    if [ -n "$MAX_CONCURRENT_DOWNLOADS" ]; then
        sed -i "s/max_concurrent_downloads = .*/max_concurrent_downloads = ${MAX_CONCURRENT_DOWNLOADS}/g" "$temp_config"
    fi
    
    # 应用 Registry Mirrors
    if [ -n "$REGISTRY_MIRRORS" ]; then
        IFS=';' read -ra MIRROR_ENTRIES <<< "$REGISTRY_MIRRORS"
        for entry in "${MIRROR_ENTRIES[@]}"; do
            host="${entry%%=*}"
            endpoints="${entry#*=}"
            IFS=',' read -ra ENDPOINT_LIST <<< "$endpoints"
            endpoint_config=""
            for ep in "${ENDPOINT_LIST[@]}"; do
                endpoint_config="${endpoint_config}    endpoint = [\"${ep}\"]\n"
            done
            cat >> "$temp_config" << EOF

[plugins."io.containerd.grpc.v1.cri".registry.mirrors."${host}"]
$(echo -e "$endpoint_config")
EOF
        done
    fi
    
    # 应用 RawConfig
    if [ -n "$RAW_CONFIG" ]; then
        echo "$RAW_CONFIG" >> "$temp_config"
    fi
    
    # 验证并替换
    if containerd --config "$temp_config" config dump > /dev/null 2>&1; then
        mv "$temp_config" "$CONFIG_FILE"
    else
        rm -f "$temp_config"
        echo "ERROR: Invalid containerd configuration"
        return 1
    fi
}

install_containerd
generate_containerd_config
systemctl restart containerd

echo "=== containerd 安装和配置完成 ==="
```

#### 6.5.3 kubelet 配置生成脚本（Ubuntu/Debian）

```bash
#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-1.31.0}"
DROP_IN_DIR="/etc/systemd/system/kubelet.service.d"
DROP_IN_FILE="${DROP_IN_DIR}/10-capbm.conf"
CGROUP_DRIVER="${CGROUP_DRIVER:-systemd}"
MAX_PODS="${MAX_PODS:-250}"
EXTRA_ARGS="${EXTRA_ARGS:-}"
RAW_CONFIG="${RAW_CONFIG:-}"

echo "=== 安装并配置 kubelet (Ubuntu/Debian) ==="

# 1. 安装 kubelet
install_kubelet() {
    if command -v kubelet &>/dev/null; then
        current_version=$(kubelet --version | awk '{print $2}')
        if [ "$current_version" != "v${K8S_VERSION}" ]; then
            echo "Upgrading kubelet: $current_version -> v${K8S_VERSION}"
            apt-get update
            apt-get install -y apt-transport-https ca-certificates curl gpg
            
            minor_version=$(echo "$K8S_VERSION" | cut -d'.' -f1,2)
            curl -fsSL "https://pkgs.k8s.io/core:/stable:/v${minor_version}/deb/Release.key" | \
                gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
            echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v${minor_version}/deb/ /" > \
                /etc/apt/sources.list.d/kubernetes.list
            
            apt-get update
            apt-get install -y "kubelet=${K8S_VERSION}-*"
            apt-mark hold kubelet
        else
            echo "kubelet already installed: v${K8S_VERSION}"
        fi
    else
        apt-get update
        apt-get install -y apt-transport-https ca-certificates curl gpg
        
        minor_version=$(echo "$K8S_VERSION" | cut -d'.' -f1,2)
        curl -fsSL "https://pkgs.k8s.io/core:/stable:/v${minor_version}/deb/Release.key" | \
            gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
        echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v${minor_version}/deb/ /" > \
            /etc/apt/sources.list.d/kubernetes.list
        
        apt-get update
        apt-get install -y "kubelet=${K8S_VERSION}-*"
        apt-mark hold kubelet
    fi
}

# 2. 生成 kubelet 配置（安装的一部分）
generate_kubelet_config() {
    mkdir -p "$DROP_IN_DIR"
    
    local extra_args=""
    
    # cgroup driver
    if [ -n "$CGROUP_DRIVER" ]; then
        extra_args="${extra_args} --cgroup-driver=${CGROUP_DRIVER}"
    fi
    
    # max pods
    if [ -n "$MAX_PODS" ]; then
        extra_args="${extra_args} --max-pods=${MAX_PODS}"
    fi
    
    # extra args
    if [ -n "$EXTRA_ARGS" ]; then
        extra_args="${extra_args} ${EXTRA_ARGS}"
    fi
    
    # 生成 drop-in 文件
    cat > "$DROP_IN_FILE" << EOF
[Service]
Environment="KUBELET_EXTRA_ARGS=${extra_args}"
EOF

    # 追加 RawConfig 到 /var/lib/kubelet/config.yaml (如果存在)
    if [ -n "$RAW_CONFIG" ]; then
        local kubelet_config="/var/lib/kubelet/config.yaml"
        if [ -f "$kubelet_config" ]; then
            echo "$RAW_CONFIG" >> "$kubelet_config"
        else
            echo "$RAW_CONFIG" > "$kubelet_config"
        fi
    fi
    
    systemctl daemon-reload
    systemctl enable kubelet
}

install_kubelet
generate_kubelet_config

echo "=== kubelet 安装和配置完成 ==="
```

#### 6.5.4 CentOS/RHEL/Rocky 安装脚本

```bash
#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-1.31.0}"
CONTAINERD_VERSION="${CONTAINERD_VERSION:-}"
SYSTEMD_CGROUP="${SYSTEMD_CGROUP:-true}"
SANDBOX_IMAGE="${SANDBOX_IMAGE:-registry.k8s.io/pause:3.9}"
REGISTRY_MIRRORS="${REGISTRY_MIRRORS:-}"
CGROUP_DRIVER="${CGROUP_DRIVER:-systemd}"
MAX_PODS="${MAX_PODS:-250}"
EXTRA_ARGS="${EXTRA_ARGS:-}"

echo "=== 开始安装 Kubernetes 组件 (RHEL/CentOS/Rocky) ==="

# 检测包管理器
if command -v dnf &>/dev/null; then
    PKG_MANAGER="dnf"
elif command -v yum &>/dev/null; then
    PKG_MANAGER="yum"
else
    echo "ERROR: 不支持的包管理器"
    exit 1
fi

# 1. 安装 containerd
install_containerd() {
    echo "--- 安装 containerd ---"
    
    if command -v containerd &>/dev/null; then
        current_version=$(containerd --version | awk '{print $3}')
        if [ -n "$CONTAINERD_VERSION" ] && [ "$current_version" != "$CONTAINERD_VERSION" ]; then
            echo "Upgrading containerd: $current_version -> $CONTAINERD_VERSION"
            $PKG_MANAGER remove -y containerd || true
            $PKG_MANAGER install -y containerd
        else
            echo "containerd already installed: $current_version"
        fi
    else
        $PKG_MANAGER install -y containerd
    fi
    
    # 生成配置
    generate_containerd_config
    systemctl restart containerd
}

generate_containerd_config() {
    local CONFIG_FILE="/etc/containerd/config.toml"
    local temp_config=$(mktemp)
    
    containerd config default > "$temp_config"
    
    if [ "$SYSTEMD_CGROUP" = "true" ]; then
        sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' "$temp_config"
    fi
    
    if [ -n "$SANDBOX_IMAGE" ]; then
        sed -i "s|sandbox_image = .*|sandbox_image = \"${SANDBOX_IMAGE}\"|g" "$temp_config"
    fi
    
    if [ -n "$REGISTRY_MIRRORS" ]; then
        IFS=';' read -ra MIRROR_ENTRIES <<< "$REGISTRY_MIRRORS"
        for entry in "${MIRROR_ENTRIES[@]}"; do
            host="${entry%%=*}"
            endpoints="${entry#*=}"
            IFS=',' read -ra ENDPOINT_LIST <<< "$endpoints"
            endpoint_config=""
            for ep in "${ENDPOINT_LIST[@]}"; do
                endpoint_config="${endpoint_config}    endpoint = [\"${ep}\"]\n"
            done
            cat >> "$temp_config" << EOF

[plugins."io.containerd.grpc.v1.cri".registry.mirrors."${host}"]
$(echo -e "$endpoint_config")
EOF
        done
    fi
    
    if containerd --config "$temp_config" config dump > /dev/null 2>&1; then
        mv "$temp_config" "$CONFIG_FILE"
    else
        rm -f "$temp_config"
        echo "ERROR: Invalid containerd configuration"
        return 1
    fi
}

# 2. 安装 Kubernetes 组件
install_kubernetes() {
    echo "--- 安装 Kubernetes 组件 ---"
    
    # 添加 Kubernetes yum 仓库
    minor_version=$(echo "$K8S_VERSION" | cut -d'.' -f1,2)
    cat > /etc/yum.repos.d/kubernetes.repo << EOF
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/v${minor_version}/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/v${minor_version}/rpm/repodata/repomd.xml.key
EOF
    
    # 安装组件
    $PKG_MANAGER install -y kubelet-${K8S_VERSION} kubeadm-${K8S_VERSION} kubectl-${K8S_VERSION}
    
    # 生成 kubelet 配置
    generate_kubelet_config
}

generate_kubelet_config() {
    local DROP_IN_DIR="/etc/systemd/system/kubelet.service.d"
    local DROP_IN_FILE="${DROP_IN_DIR}/10-capbm.conf"
    
    mkdir -p "$DROP_IN_DIR"
    
    local extra_args=""
    if [ -n "$CGROUP_DRIVER" ]; then
        extra_args="${extra_args} --cgroup-driver=${CGROUP_DRIVER}"
    fi
    if [ -n "$MAX_PODS" ]; then
        extra_args="${extra_args} --max-pods=${MAX_PODS}"
    fi
    if [ -n "$EXTRA_ARGS" ]; then
        extra_args="${extra_args} ${EXTRA_ARGS}"
    fi
    
    cat > "$DROP_IN_FILE" << EOF
[Service]
Environment="KUBELET_EXTRA_ARGS=${extra_args}"
EOF

    systemctl daemon-reload
    systemctl enable kubelet
}

# 执行安装
install_containerd
install_kubernetes

echo "=== 组件安装完成 ==="
echo "  containerd: $(containerd --version)"
echo "  kubeadm: $(kubeadm version -o short)"
echo "  kubelet: $(kubelet --version)"
echo "  kubectl: $(kubectl version --client --short 2>/dev/null || kubectl version --client)"
```

### 6.6 Controller 集成

在 `BareMetalMachine Controller` 的调谐流程中集成组件安装和配置生成：

```go
func (r *BareMetalMachineReconciler) reconcileNormal(ctx context.Context, bmMachine *infrav1.BareMetalMachine, machine *clusterv1.Machine) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // 1. 获取 HostInventory
    hostInventory, err := r.getHostInventory(ctx, bmMachine)
    if err != nil {
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    // 2. 分配机器 (如果尚未分配)
    if bmMachine.Spec.HostName == "" {
        host, err := r.allocateHost(ctx, hostInventory, bmMachine)
        if err != nil {
            return ctrl.Result{RequeueAfter: 30 * time.Second}, err
        }
        bmMachine.Spec.HostName = host.HostName
        bmMachine.Spec.IPAddress = host.IPAddress
        bmMachine.Spec.CredentialsRef = &host.CredentialsRef
        if err := r.Update(ctx, bmMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 3. 获取凭据
    creds, err := r.getCredentials(ctx, bmMachine)
    if err != nil {
        markConditionFalse(bmMachine, infrav1.MachineReadyCondition, infrav1.CredentialsNotFoundReason, clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, bmMachine)
    }

    // 4. 建立 SSH 连接
    sshConn, err := r.SSHManager.Connect(bmMachine.Spec.IPAddress, bmMachine.Spec.SSHPort, *creds)
    if err != nil {
        markConditionFalse(bmMachine, infrav1.SSHConnectedCondition, infrav1.SSHConnectionFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
        return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, bmMachine)
    }
    defer sshConn.Close()

    markConditionTrue(bmMachine, infrav1.SSHConnectedCondition)

    // 5. 执行预检
    preflightConfig := ssh.DefaultPreflightConfig()
    preflightResult, err := ssh.RunPreflightChecks(ctx, sshConn, preflightConfig)
    if err != nil || !preflightResult.Passed {
        markConditionFalse(bmMachine, infrav1.PreFlightChecksPassedCondition, infrav1.PreFlightChecksFailedReason, clusterv1.ConditionSeverityError, fmt.Sprintf("pre-flight checks failed: %v", preflightResult.Errors))
        return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
    }
    markConditionTrue(bmMachine, infrav1.PreFlightChecksPassedCondition)

    // 6. 安装组件并生成配置 (新增)
    if bmMachine.Spec.ComponentInstall != nil && bmMachine.Spec.ComponentInstall.Enabled {
        installResult, err := r.installComponents(ctx, sshConn, bmMachine)
        if err != nil {
            markConditionFalse(bmMachine, infrav1.ComponentsInstalledCondition, infrav1.ComponentInstallFailedReason, clusterv1.ConditionSeverityError, err.Error())
            return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
        }
        if !installResult.Completed {
            log.Info("Component installation in progress", "progress", installResult.Progress)
            return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
        }
        if !installResult.Success {
            markConditionFalse(bmMachine, infrav1.ComponentsInstalledCondition, infrav1.ComponentInstallFailedReason, clusterv1.ConditionSeverityError, installResult.Error)
            return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
        }
        markConditionTrue(bmMachine, infrav1.ComponentsInstalledCondition)
        
        // 更新已安装组件版本
        bmMachine.Status.InstalledComponents = infrav1.ComponentVersions{
            ContainerRuntime: installResult.ComponentVersions.ContainerRuntime,
            Kubeadm:          installResult.ComponentVersions.Kubeadm,
            Kubelet:          installResult.ComponentVersions.Kubelet,
            Kubectl:          installResult.ComponentVersions.Kubectl,
        }
    }

    // 7. 设置 ProviderID
    providerID := fmt.Sprintf("baremetal://%s", bmMachine.Spec.HostName)
    if bmMachine.Spec.ProviderID == nil || *bmMachine.Spec.ProviderID != providerID {
        bmMachine.Spec.ProviderID = &providerID
        if err := r.Update(ctx, bmMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 8. 更新状态
    bmMachine.Status.Ready = true
    bmMachine.Status.ProviderID = providerID
    bmMachine.Status.Addresses = []clusterv1.MachineAddress{
        {Type: clusterv1.MachineInternalIP, Address: bmMachine.Spec.IPAddress},
        {Type: clusterv1.MachineHostName, Address: bmMachine.Spec.HostName},
    }
    markConditionTrue(bmMachine, infrav1.MachineReadyCondition)

    return ctrl.Result{}, r.Status().Update(ctx, bmMachine)
}
```

#### 6.6.1 installComponents 实现

```go
func (r *BareMetalMachineReconciler) installComponents(ctx context.Context, sshConn *ssh.SSHConnection, bmMachine *infrav1.BareMetalMachine) (*installer.InstallResult, error) {
    log := ctrl.LoggerFrom(ctx)
    
    config := bmMachine.Spec.ComponentInstall
    if config == nil {
        config = &infrav1.ComponentInstallConfig{
            Enabled:  true,
            Strategy: infrav1.InstallIfMissing,
        }
    }
    
    if !config.Enabled || config.Strategy == infrav1.Skip {
        log.Info("Component installation disabled or skipped")
        return &installer.InstallResult{Completed: true, Success: true, Progress: "Installation disabled or skipped"}, nil
    }
    
    // 获取 Kubernetes 版本
    k8sVersion := extractK8sVersion(machine)
    
    // 构建安装参数
    installParams := installer.InstallParams{
        K8SVersion: k8sVersion,
        Role:       bmMachine.Spec.Role,
        ContainerRuntime: installer.ContainerRuntimeParams{
            Type:    config.ContainerRuntime.Type,
            Version: config.ContainerRuntime.Version,
            Config:  config.ContainerRuntime.Config,
        },
        Kubernetes: installer.KubernetesParams{
            Version: k8sVersion,
            Config:  config.Kubernetes.Config,
        },
        Timeout:    config.Timeout,
        MaxRetries: config.MaxRetries,
    }
    
    // 执行安装
    inst := installer.New(sshConn, installParams)
    return inst.Install(ctx)
}

func extractK8sVersion(machine *clusterv1.Machine) string {
    if machine == nil || machine.Spec.Version == "" {
        return ""
    }
    v := machine.Spec.Version
    if len(v) > 0 && v[0] == 'v' {
        return v[1:]
    }
    return v
}
```

### 6.7 安装状态 Condition

新增 `ComponentsInstalled` Condition 跟踪组件安装状态：

```go
const (
    // ComponentsInstalledCondition reports whether required components are installed.
    ComponentsInstalledCondition clusterv1.ConditionType = "ComponentsInstalled"
    
    // ComponentInstallFailedReason indicates component installation failed.
    ComponentInstallFailedReason = "ComponentInstallFailed"
    
    // ComponentsInstalledReason indicates components are installed.
    ComponentsInstalledReason = "ComponentsInstalled"
)
```

### 6.8 升级场景处理

当 Kubernetes 版本升级时，组件安装逻辑需要处理版本更新和配置重新生成。

#### 6.8.1 升级流程

```
Machine 版本升级触发
    │
    ▼
┌─────────────────────────────────────────────────┐
│ 1. 检测当前组件版本                              │
│    ├── containerd --version                      │
│    ├── kubeadm version                           │
│    └── kubelet --version                         │
└─────────────────┬───────────────────────────────┘
                  │
           版本是否匹配？
           ╱              ╲
          是               否
          │                │
          ▼                ▼
┌──────────────────┐ ┌──────────────────────────┐
│ 跳过安装         │ │ 2. 执行升级               │
│ 保留现有配置     │ │    ├── 升级 containerd    │
│                  │ │    ├── 重新生成配置        │
│                  │ │    ├── 升级 kubeadm       │
│                  │ │    ├── 升级 kubelet       │
│                  │ │    └── 重新生成 drop-in   │
└──────────────────┘ └──────────┬───────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────┐
│ 3. 验证升级                                      │
│    ├── containerd config dump 验证配置           │
│    ├── systemctl restart containerd              │
│    └── systemctl daemon-reload                   │
└─────────────────────────────────────────────────┘
```

#### 6.8.2 配置重新生成策略

| 组件 | 升级时配置处理 | 说明 |
|------|---------------|------|
| **containerd** | 完整重新生成 | 使用新版本 `containerd config default` + 应用声明的配置 |
| **kubelet** | 重新生成 drop-in | 重新写入 `/etc/systemd/system/kubelet.service.d/10-capbm.conf` |
| **kubeadm** | 无配置 | 仅升级二进制 |
| **kubectl** | 无配置 | 仅升级二进制 |

#### 6.8.3 升级示例

```yaml
# 升级前
spec:
  componentInstall:
    containerRuntime:
      type: "containerd"
      version: "1.7.0"
      config:
        systemdCgroup: true
        sandboxImage: "registry.k8s.io/pause:3.9"
        registryMirrors:
          - host: "docker.io"
            endpoints: ["https://mirror.example.com"]
    kubernetes:
      version: "1.30.0"
      config:
        kubelet:
          cgroupDriver: "systemd"
          maxPods: 250

# 升级后（修改版本即可，配置自动重新生成）
spec:
  componentInstall:
    containerRuntime:
      type: "containerd"
      version: "1.7.0"          # 可保持不变或升级
      config:
        systemdCgroup: true
        sandboxImage: "registry.k8s.io/pause:3.9"
        registryMirrors:
          - host: "docker.io"
            endpoints: ["https://mirror.example.com"]
    kubernetes:
      version: "1.31.0"         # 升级版本
      config:
        kubelet:
          cgroupDriver: "systemd"
          maxPods: 250
```

在 `InstallIfMissing` 策略下，版本不匹配会触发自动升级和配置重新生成。

### 6.9 用户使用示例

#### 6.9.1 标准集群（containerd + kubelet 配置）

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
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
    - name: componentInstall
      value:
        enabled: true
        strategy: "InstallIfMissing"
        containerRuntime:
          type: "containerd"
          config:
            systemdCgroup: true
            sandboxImage: "registry.k8s.io/pause:3.9"
            registryMirrors:
              - host: "docker.io"
                endpoints:
                  - "https://mirror.example.com"
              - host: "gcr.io"
                endpoints:
                  - "https://gcr-mirror.example.com"
            maxConcurrentDownloads: 3
            rawConfig: |
              [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
                runtime_type = "io.containerd.runc.v2"
                options:
                  BinaryName = "/usr/bin/nvidia-container-runtime"
        kubernetes:
          version: "1.31.0"
          config:
            kubelet:
              cgroupDriver: "systemd"
              maxPods: 250
              extraArgs:
                feature-gates: "RotateKubeletServerCertificate=true"
                kube-reserved: "cpu=250m,memory=512Mi"
```

#### 6.9.2 离线环境集群

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
spec:
  topology:
    variables:
    - name: componentInstall
      value:
        enabled: true
        strategy: "InstallIfMissing"
        airGap:
          enabled: true
          binarySource: "HTTPServer"
          httpServerConfig:
            baseUrl: "https://internal-pkg.example.com/k8s/v1.31.0"
            insecureSkipVerify: false
        containerRuntime:
          type: "containerd"
          config:
            systemdCgroup: true
            sandboxImage: "registry.k8s.io/pause:3.9"
            registryMirrors:
              - host: "docker.io"
                endpoints:
                  - "https://internal-registry.example.com"
        kubernetes:
          version: "1.31.0"
          repository:
            baseUrl: "https://internal-pkg.example.com/kubernetes"
            gpgKey: "https://internal-pkg.example.com/kubernetes/gpg.key"
          config:
            kubelet:
              cgroupDriver: "systemd"
              maxPods: 250
```

#### 6.9.3 多 Worker 池（差异化配置）

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
spec:
  topology:
    workers:
      machineDeployments:
      - class: default-worker
        name: general-purpose
        replicas: 3
        variables:
          overrides:
          - name: componentInstall
            value:
              containerRuntime:
                config:
                  maxConcurrentDownloads: 3
              kubernetes:
                config:
                  kubelet:
                    maxPods: 110
      - class: default-worker
        name: gpu-nodes
        replicas: 2
        variables:
          overrides:
          - name: componentInstall
            value:
              containerRuntime:
                config:
                  rawConfig: |
                    [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
                      runtime_type = "io.containerd.runc.v2"
                      options:
                        BinaryName = "/usr/bin/nvidia-container-runtime"
              kubernetes:
                config:
                  kubelet:
                    maxPods: 50
                    extraArgs:
                      feature-gates: "DevicePlugins=true"
```

### 6.10 用户自定义仓库

对于内网环境，用户可能需要配置自定义包仓库：

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
spec:
  topology:
    variables:
    - name: componentInstall
      value:
        enabled: true
        strategy: "InstallIfMissing"
        kubernetes:
          repository:
            baseUrl: "https://internal-mirror.example.com/kubernetes"
            gpgKey: "https://internal-mirror.example.com/kubernetes/gpg.key"
        containerRuntime:
          registryMirrors:
            - "https://internal-registry-mirror.example.com"
```

### 6.11 超时和重试

安装脚本通过 context timeout 控制超时：

```go
func (r *BareMetalMachineReconciler) installComponents(ctx context.Context, sshConn *ssh.SSHConnection, bmMachine *infrav1.BareMetalMachine) (*InstallResult, error) {
    timeout := 5 * time.Minute
    if bmMachine.Spec.ComponentInstall.Timeout != nil {
        timeout = bmMachine.Spec.ComponentInstall.Timeout.Duration
    }
    
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    // 执行安装脚本...
}
```

### 6.12 离线/Air-Gap 安装支持

#### 6.11.1 问题背景

在内网或隔离环境中，目标机器无法访问外部包仓库。CAPBM 需要支持离线安装模式，通过以下方式分发组件：

| 模式 | 适用场景 | 分发方式 |
|------|---------|---------|
| HTTP Server | 内网有 HTTP 服务 | 从内网 HTTP 服务器下载 RPM/DEB/二进制 |
| ConfigMap/Secret | 小体积二进制 | 通过 Kubernetes 资源下发 |
| Local Path | 镜像已预烧录 | 从本地指定路径读取 |
| Preload Images | 容器镜像预加载 | tar 包导入 containerd |

#### 6.11.2 CRD 扩展

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachine
spec:
  componentInstall:
    enabled: true
    airGap:
      enabled: true
      binarySource: "HTTPServer"  # HTTPServer | ConfigMap | LocalPath
      
      # HTTP Server 模式
      httpServerConfig:
        baseUrl: "https://internal-pkg.example.com/k8s/v1.31.0"
        insecureSkipVerify: false
        tlsSecretRef:
          name: "internal-ca-cert"
      
      # LocalPath 模式（当 binarySource=LocalPath 时）
      # localPath: "/opt/k8s-binaries"
      
      # 预加载容器镜像列表
      preloadImages:
        - "registry.k8s.io/kube-apiserver:v1.31.0"
        - "registry.k8s.io/kube-controller-manager:v1.31.0"
        - "registry.k8s.io/kube-scheduler:v1.31.0"
        - "registry.k8s.io/kube-proxy:v1.31.0"
        - "registry.k8s.io/pause:3.9"
        - "registry.k8s.io/etcd:3.5.15-0"
        - "registry.k8s.io/coredns/coredns:v1.11.1"
```

#### 6.11.3 二进制包准备流程

```
管理端准备
    │
    ├── 1. 下载所需二进制包
    │   ├── kubeadm/kubelet/kubectl (指定版本)
    │   ├── containerd 静态二进制
    │   └── 依赖库 (libseccomp 等)
    │
    ├── 2. 打包
    │   ├── 按 OS 类型分类 (deb/rpm/tar.gz)
    │   ├── 生成 checksum 文件
    │   └── 打包为 versioned archive
    │
    └── 3. 分发到内网
        ├── 上传到内网 HTTP 服务器
        ├── 或导入 ConfigMap/Secret
        └── 或预置到目标机器本地路径
```

#### 6.11.4 离线安装脚本 (HTTP Server 模式)

```bash
#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-1.31.0}"
BASE_URL="${BASE_URL:-https://internal-pkg.example.com/k8s}"
CHECKSUM_FILE="checksums.sha256"
INSTALL_DIR="/tmp/k8s-install"

echo "=== 离线安装开始 ==="

# 1. 创建安装目录
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

# 2. 下载包并校验
download_and_verify() {
    local file="$1"
    local url="${BASE_URL}/${file}"
    
    echo "下载: $file"
    curl -fsSL --retry 3 --retry-delay 5 "$url" -o "$file"
    
    echo "校验: $file"
    local expected_checksum
    expected_checksum=$(grep "$file" "$CHECKSUM_FILE" | awk '{print $1}')
    local actual_checksum
    actual_checksum=$(sha256sum "$file" | awk '{print $1}')
    
    if [ "$expected_checksum" != "$actual_checksum" ]; then
        echo "ERROR: 校验失败 $file"
        echo "  期望: $expected_checksum"
        echo "  实际: $actual_checksum"
        return 1
    fi
    echo "校验通过: $file"
}

# 3. 安装 containerd
install_containerd_offline() {
    echo "--- 离线安装 containerd ---"
    
    if command -v containerd &>/dev/null; then
        current_version=$(containerd --version | awk '{print $3}')
        if [ "$current_version" = "${CONTAINERD_VERSION:-}" ]; then
            echo "containerd 已安装: $current_version"
            return 0
        fi
    fi
    
    download_and_verify "containerd-${CONTAINERD_VERSION:-latest}.linux-amd64.tar.gz"
    
    tar -C /usr/local -xzf "containerd-*.linux-amd64.tar.gz"
    
    # 安装 systemd 服务文件
    cp /usr/local/bin/containerd /usr/bin/
    containerd config default > /etc/containerd/config.toml
    sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
    
    # 创建 systemd service
    cat > /etc/systemd/system/containerd.service << 'EOF'
[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target local-fs.target

[Service]
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/bin/containerd
Type=notify
Delegate=yes
KillMode=process
Restart=always
RestartSec=5
LimitNPROC=infinity
LimitCORE=infinity
LimitNOFILE=infinity
TasksMax=infinity
OOMScoreAdjust=-999

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable --now containerd
    
    echo "containerd 离线安装完成"
}

# 4. 安装 Kubernetes 组件
install_k8s_offline() {
    echo "--- 离线安装 Kubernetes 组件 ---"
    
    if command -v kubeadm &>/dev/null; then
        current_version=$(kubeadm version -o short 2>/dev/null || echo "")
        if [ "$current_version" = "v${K8S_VERSION}" ]; then
            echo "Kubernetes 组件已安装且版本匹配"
            return 0
        fi
    fi
    
    download_and_verify "kubeadm-v${K8S_VERSION}-linux-amd64"
    download_and_verify "kubelet-v${K8S_VERSION}-linux-amd64"
    download_and_verify "kubectl-v${K8S_VERSION}-linux-amd64"
    
    install -m 0755 "kubeadm-v${K8S_VERSION}-linux-amd64" /usr/bin/kubeadm
    install -m 0755 "kubelet-v${K8S_VERSION}-linux-amd64" /usr/bin/kubelet
    install -m 0755 "kubectl-v${K8S_VERSION}-linux-amd64" /usr/bin/kubectl
    
    # 创建 kubelet systemd service
    cat > /etc/systemd/system/kubelet.service << 'EOF'
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/home/
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/usr/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable kubelet
    
    echo "Kubernetes 组件离线安装完成"
}

# 5. 预加载容器镜像 (如果有)
preload_images() {
    local images_file="${1:-}"
    if [ -z "$images_file" ] || [ ! -f "$images_file" ]; then
        echo "无预加载镜像列表"
        return 0
    fi
    
    echo "--- 预加载容器镜像 ---"
    while IFS= read -r image_tar; do
        if [ -f "$image_tar" ]; then
            echo "导入: $image_tar"
            ctr -n k8s.io images import "$image_tar" || true
        fi
    done < "$images_file"
    echo "镜像预加载完成"
}

# 执行安装
install_containerd_offline
install_k8s_offline
preload_images "${PRELOAD_IMAGES_LIST:-}"

# 清理
rm -rf "$INSTALL_DIR"

echo "=== 离线安装完成 ==="
```

#### 6.11.5 镜像预加载设计

```yaml
# 预加载配置示例
spec:
  componentInstall:
    airGap:
      enabled: true
      preloadImages:
        - "registry.k8s.io/kube-apiserver:v1.31.0"
        - "registry.k8s.io/kube-controller-manager:v1.31.0"
        - "registry.k8s.io/kube-scheduler:v1.31.0"
        - "registry.k8s.io/kube-proxy:v1.31.0"
        - "registry.k8s.io/pause:3.9"
        - "registry.k8s.io/etcd:3.5.15-0"
        - "registry.k8s.io/coredns/coredns:v1.11.1"
```

预加载流程：
```
管理端准备镜像 tar 包
    │
    ├── 1. 拉取并打包
    │   ├── skopeo copy docker://registry.k8s.io/kube-apiserver:v1.31.0 oci-archive:kube-apiserver.tar
    │   └── 对所有必需镜像执行
    │
    ├── 2. 分发到目标机器
    │   ├── 通过 HTTP Server 下载
    │   └── 或预置到本地路径
    │
    └── 3. 导入 containerd
        ├── ctr -n k8s.io images import kube-apiserver.tar
        └── 验证镜像已加载
```

### 6.13 安装脚本健壮性设计

#### 6.12.1 幂等性保证

所有安装脚本必须满足幂等性，即多次执行结果一致：

```bash
#!/bin/bash
set -euo pipefail

# 幂等性安装函数模板
install_with_idempotency() {
    local component="$1"
    local desired_version="$2"
    local version_cmd="$3"
    local install_cmd="$4"
    
    # 检查当前状态
    if command -v "$component" &>/dev/null; then
        local current_version
        current_version=$(eval "$version_cmd" 2>/dev/null || echo "unknown")
        
        if [ "$current_version" = "$desired_version" ]; then
            echo "[SKIP] $component 已安装且版本匹配: $current_version"
            return 0
        else
            echo "[UPGRADE] $component 版本不匹配: $current_version -> $desired_version"
            # 记录当前状态用于可能的回滚
            echo "$current_version" > "/tmp/.capbm_rollback_${component}.version"
        fi
    else
        echo "[INSTALL] $component 未安装，开始安装"
    fi
    
    # 执行安装
    eval "$install_cmd"
    
    # 验证安装结果
    if command -v "$component" &>/dev/null; then
        local new_version
        new_version=$(eval "$version_cmd" 2>/dev/null || echo "unknown")
        if [ "$new_version" = "$desired_version" ]; then
            echo "[SUCCESS] $component 安装成功: $new_version"
            return 0
        else
            echo "[ERROR] $component 安装后版本不正确: $new_version"
            return 1
        fi
    else
        echo "[ERROR] $component 安装后仍不可用"
        return 1
    fi
}

# 使用示例
install_with_idempotency \
    "kubeadm" \
    "v1.31.0" \
    "kubeadm version -o short" \
    "apt-get install -y kubeadm=1.31.0-*"
```

#### 6.12.2 错误处理和回滚机制

```bash
#!/bin/bash
set -euo pipefail

# 全局错误处理
ROLLBACK_STACK=()
ERROR_OCCURRED=false

# 注册回滚操作
register_rollback() {
    ROLLBACK_STACK=("$1" "${ROLLBACK_STACK[@]}")
}

# 执行回滚
perform_rollback() {
    echo "=== 开始回滚 ==="
    ERROR_OCCURRED=true
    
    for action in "${ROLLBACK_STACK[@]}"; do
        echo "执行回滚: $action"
        eval "$action" || echo "WARNING: 回滚操作失败: $action"
    done
    
    echo "=== 回滚完成 ==="
}

# 陷阱捕获错误
trap 'perform_rollback' ERR

# 安全安装函数
safe_install() {
    local component="$1"
    local install_fn="$2"
    local rollback_fn="$3"
    
    echo "安装: $component"
    register_rollback "$rollback_fn"
    
    # 执行安装
    eval "$install_fn"
    
    # 安装成功，移除此回滚注册
    ROLLBACK_STACK=("${ROLLBACK_STACK[@]/$rollback_fn/}")
    echo "安装成功: $component"
}

# 回滚操作示例
rollback_containerd() {
    systemctl stop containerd 2>/dev/null || true
    apt-get remove -y containerd 2>/dev/null || yum remove -y containerd 2>/dev/null || true
}

rollback_kubelet() {
    systemctl stop kubelet 2>/dev/null || true
    apt-get remove -y kubelet 2>/dev/null || yum remove -y kubelet 2>/dev/null || true
}

# 使用安全安装
safe_install "containerd" "install_containerd" "rollback_containerd"
safe_install "kubelet" "install_kubelet" "rollback_kubelet"

if [ "$ERROR_OCCURRED" = false ]; then
    echo "=== 安装成功，无需回滚 ==="
fi
```

#### 6.12.3 安装进度追踪

```bash
#!/bin/bash

# 进度状态文件
PROGRESS_FILE="/tmp/.capbm_install_progress"

# 更新进度
update_progress() {
    local step="$1"
    local status="$2"  # started | completed | failed
    local message="${3:-}"
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    cat > "$PROGRESS_FILE" << EOF
{
    "step": "$step",
    "status": "$status",
    "message": "$message",
    "timestamp": "$timestamp"
}
EOF
}

# 安装步骤包装器
with_progress() {
    local step_name="$1"
    shift
    
    update_progress "$step_name" "started" "开始执行"
    
    if "$@"; then
        update_progress "$step_name" "completed" "执行成功"
        return 0
    else
        update_progress "$step_name" "failed" "执行失败"
        return 1
    fi
}

# 使用示例
with_progress "detect_os" detect_os_type
with_progress "install_containerd" install_containerd
with_progress "install_kubeadm" install_kubeadm
with_progress "install_kubelet" install_kubelet
with_progress "verify_installation" verify_all
```

Controller 读取进度状态：
```go
type InstallProgress struct {
    Step      string    `json:"step"`
    Status    string    `json:"status"`    // started | completed | failed
    Message   string    `json:"message"`
    Timestamp time.Time `json:"timestamp"`
}

func (r *BareMetalMachineReconciler) getInstallProgress(ctx context.Context, sshConn *ssh.SSHConnection) (*InstallProgress, error) {
    result, err := sshConn.RunCommand(ctx, "cat /tmp/.capbm_install_progress 2>/dev/null || echo '{}'")
    if err != nil {
        return nil, err
    }
    
    var progress InstallProgress
    if err := json.Unmarshal([]byte(result.Stdout), &progress); err != nil {
        return nil, err
    }
    
    return &progress, nil
}
```

#### 6.12.4 健康检查验证

安装完成后必须执行健康检查：

```bash
#!/bin/bash

verify_installation() {
    local errors=()
    
    echo "=== 安装验证 ==="
    
    # 1. containerd 验证
    echo "--- containerd ---"
    if ! command -v containerd &>/dev/null; then
        errors+=("containerd 未找到")
    else
        echo "版本: $(containerd --version)"
    fi
    
    if ! systemctl is-active --quiet containerd; then
        errors+=("containerd 服务未运行")
    else
        echo "服务状态: running"
    fi
    
    # 验证 crictl 连接
    if command -v crictl &>/dev/null; then
        if crictl info &>/dev/null; then
            echo "CRI 连接: OK"
        else
            errors+=("crictl 无法连接 containerd")
        fi
    fi
    
    # 2. kubeadm 验证
    echo "--- kubeadm ---"
    if ! command -v kubeadm &>/dev/null; then
        errors+=("kubeadm 未找到")
    else
        echo "版本: $(kubeadm version -o short)"
    fi
    
    # 3. kubelet 验证
    echo "--- kubelet ---"
    if ! command -v kubelet &>/dev/null; then
        errors+=("kubelet 未找到")
    else
        echo "版本: $(kubelet --version)"
    fi
    
    if ! systemctl is-active --quiet kubelet; then
        echo "WARNING: kubelet 服务未运行 (正常，等待 kubeadm 初始化)"
    fi
    
    # 4. 网络端口验证
    echo "--- 网络端口 ---"
    check_port 10250 "kubelet API"
    check_port 6443 "kube-apiserver (control-plane only)"
    
    # 5. 文件系统验证
    echo "--- 文件系统 ---"
    check_dir "/etc/kubernetes" "Kubernetes 配置目录"
    check_dir "/var/lib/kubelet" "kubelet 数据目录"
    check_dir "/etc/containerd" "containerd 配置目录"
    
    # 输出结果
    if [ ${#errors[@]} -gt 0 ]; then
        echo ""
        echo "=== 验证失败 ==="
        for err in "${errors[@]}"; do
            echo "  ERROR: $err"
        done
        return 1
    fi
    
    echo "=== 验证通过 ==="
    return 0
}

check_port() {
    local port="$1"
    local desc="$2"
    if ss -tlnp | grep -q ":${port} "; then
        echo "端口 $port ($desc): 监听中"
    else
        echo "端口 $port ($desc): 未监听 (可能正常)"
    fi
}

check_dir() {
    local dir="$1"
    local desc="$2"
    if [ -d "$dir" ]; then
        echo "目录 $dir ($desc): 存在"
    else
        echo "目录 $dir ($desc): 不存在 (可能正常)"
    fi
}
```

### 6.14 更多 OS 和运行时支持

#### 6.13.1 支持的 OS 矩阵

| OS | 版本 | 包管理器 | 支持状态 |
|----|------|---------|---------|
| Ubuntu | 20.04, 22.04, 24.04 | apt | 完全支持 |
| Debian | 11, 12 | apt | 完全支持 |
| CentOS | 7, 8, 9 | yum/dnf | 完全支持 |
| RHEL | 8, 9 | dnf | 完全支持 |
| Rocky Linux | 8, 9 | dnf | 完全支持 |
| AlmaLinux | 8, 9 | dnf | 完全支持 |
| openSUSE | 15.x | zypper | 实验性支持 |
| SLES | 15.x | zypper | 实验性支持 |
| Amazon Linux | 2, 2023 | yum/dnf | 实验性支持 |
| Flatcar | 最新版 | 二进制/ignition | 特殊支持 (见 6.13.3) |
| Talos | 最新版 | 不可变 OS | 不支持 (自带组件) |

#### 6.13.2 SUSE/zypper 安装脚本

```bash
#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-1.31.0}"

echo "=== SUSE 安装开始 ==="

# 安装 containerd
install_containerd_suse() {
    echo "--- 安装 containerd ---"
    
    if command -v containerd &>/dev/null; then
        echo "containerd 已安装: $(containerd --version)"
        return 0
    fi
    
    zypper refresh
    zypper install -y containerd
    
    mkdir -p /etc/containerd
    containerd config default > /etc/containerd/config.toml
    sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
    
    systemctl enable --now containerd
    echo "containerd 安装完成"
}

# 安装 Kubernetes 组件
install_k8s_suse() {
    echo "--- 安装 Kubernetes 组件 ---"
    
    if command -v kubeadm &>/dev/null; then
        current_version=$(kubeadm version -o short 2>/dev/null || echo "")
        if [ "$current_version" = "v${K8S_VERSION}" ]; then
            echo "Kubernetes 组件已安装且版本匹配"
            return 0
        fi
    fi
    
    # 添加 Kubernetes 仓库
    cat > /etc/zypp/repos.d/kubernetes.repo << EOF
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/v${K8S_VERSION%.*}/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/v${K8S_VERSION%.*}/rpm/repodata/repomd.xml.key
EOF

    zypper refresh
    zypper install -y "kubelet-${K8S_VERSION}" "kubeadm-${K8S_VERSION}" "kubectl-${K8S_VERSION}"
    
    systemctl enable kubelet
    
    echo "Kubernetes 组件安装完成"
}

install_containerd_suse
install_k8s_suse

echo "=== SUSE 安装完成 ==="
```

#### 6.13.3 Flatcar Container Linux 支持

Flatcar 是只读文件系统 OS，需要特殊处理：

```yaml
# Flatcar 安装配置
spec:
  componentInstall:
    strategy: "InstallIfMissing"
    osType: "flatcar"
    flatcar:
      # 使用 Ignition 配置
      useIgnition: true
      # 写入 /opt 目录 (Flatcar 可写路径)
      installPath: "/opt/kubernetes"
      # binfmt_misc 配置
      enableBinfmtMisc: true
```

Flatcar 安装脚本：
```bash
#!/bin/bash
set -euo pipefail

# Flatcar 使用 /opt/bin 作为自定义二进制安装路径
INSTALL_PREFIX="/opt/bin"
K8S_VERSION="${K8S_VERSION:-1.31.0}"

echo "=== Flatcar 安装开始 ==="

# Flatcar 已预装 containerd，只需验证
verify_containerd() {
    if command -v containerd &>/dev/null; then
        echo "containerd 已预装: $(containerd --version)"
        
        # 确保配置正确
        if [ ! -f /etc/containerd/config.toml ]; then
            mkdir -p /etc/containerd
            containerd config default > /etc/containerd/config.toml
            sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
            systemctl restart containerd
        fi
        return 0
    fi
    echo "ERROR: containerd 未找到"
    return 1
}

# 安装 Kubernetes 二进制到 /opt/bin
install_k8s_binaries() {
    echo "--- 安装 Kubernetes 二进制 ---"
    
    mkdir -p "$INSTALL_PREFIX"
    
    # 下载二进制 (从官方或内网源)
    local base_url="https://dl.k8s.io/v${K8S_VERSION}/bin/linux/amd64"
    
    for binary in kubeadm kubelet kubectl; do
        echo "下载 $binary"
        curl -fsSL "${base_url}/${binary}" -o "${INSTALL_PREFIX}/${binary}"
        chmod +x "${INSTALL_PREFIX}/${binary}"
    done
    
    # 创建符号链接到 PATH
    for binary in kubeadm kubelet kubectl; do
        ln -sf "${INSTALL_PREFIX}/${binary}" "/usr/local/bin/${binary}" 2>/dev/null || true
    done
    
    echo "Kubernetes 二进制安装完成"
}

# 配置 kubelet systemd 服务 (使用 drop-in)
configure_kubelet_service() {
    echo "--- 配置 kubelet 服务 ---"
    
    mkdir -p /etc/systemd/system/kubelet.service.d
    
    cat > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf << 'EOF'
[Service]
Environment="KUBELET_EXTRA_ARGS=--container-runtime-endpoint=unix:///run/containerd/containerd.sock"
EOF

    systemctl daemon-reload
    systemctl enable kubelet
    
    echo "kubelet 服务配置完成"
}

verify_containerd
install_k8s_binaries
configure_kubelet_service

echo "=== Flatcar 安装完成 ==="
```

#### 6.13.4 CRI-O 运行时支持

```yaml
spec:
  componentInstall:
    containerRuntime:
      type: "cri-o"
      version: "1.31"
```

CRI-O 安装脚本：
```bash
#!/bin/bash
set -euo pipefail

CRIO_VERSION="${CRIO_VERSION:-1.31}"
OS_ID=$(. /etc/os-release && echo "$ID")

echo "=== CRI-O 安装开始 ==="

install_crio_debian() {
    echo "--- Debian/Ubuntu 安装 CRI-O ---"
    
    if command -v crio &>/dev/null; then
        echo "CRI-O 已安装: $(crio --version | head -1)"
        return 0
    fi
    
    export OS="xUbuntu_22.04"  # 根据实际版本调整
    
    echo "deb https://pkgs.k8s.io/addons:/cri-o:/stable:/${CRIO_VERSION}/deb/${OS}/ /" > \
        /etc/apt/sources.list.d/cri-o.list
    
    curl -fsSL "https://pkgs.k8s.io/addons:/cri-o:/stable:/${CRIO_VERSION}/deb/${OS}/Release.key" | \
        gpg --dearmor -o /etc/apt/keyrings/cri-o-apt-keyring.gpg
    
    apt-get update
    apt-get install -y "cri-o-${CRIO_VERSION}"
    
    systemctl enable --now crio
    echo "CRI-O 安装完成"
}

install_crio_rhel() {
    echo "--- RHEL/CentOS 安装 CRI-O ---"
    
    if command -v crio &>/dev/null; then
        echo "CRI-O 已安装: $(crio --version | head -1)"
        return 0
    fi
    
    cat > /etc/yum.repos.d/cri-o.repo << EOF
[cri-o]
name=CRI-O
baseurl=https://pkgs.k8s.io/addons:/cri-o:/stable:/${CRIO_VERSION}/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/addons:/cri-o:/stable:/${CRIO_VERSION}/rpm/repodata/repomd.xml.key
EOF

    dnf install -y "cri-o-${CRIO_VERSION}"
    
    systemctl enable --now crio
    echo "CRI-O 安装完成"
}

# 根据 OS 选择安装方法
case "$OS_ID" in
    ubuntu|debian)
        install_crio_debian
        ;;
    centos|rhel|rocky|almalinux|fedora)
        install_crio_rhel
        ;;
    *)
        echo "ERROR: 不支持的 OS: $OS_ID"
        exit 1
        ;;
esac

echo "=== CRI-O 安装完成 ==="
```

#### 6.13.5 Docker 运行时支持 (仅用于兼容)

```yaml
spec:
  componentInstall:
    containerRuntime:
      type: "docker"
      version: "24.0"
      # 需要安装 cri-dockerd 作为 shim
      criDockerd:
        enabled: true
        version: "0.3.12"
```

Docker + cri-dockerd 安装脚本：
```bash
#!/bin/bash
set -euo pipefail

DOCKER_VERSION="${DOCKER_VERSION:-24.0}"
CRI_DOCKERD_VERSION="${CRI_DOCKERD_VERSION:-0.3.12}"

echo "=== Docker + cri-dockerd 安装开始 ==="

# 安装 Docker
install_docker() {
    echo "--- 安装 Docker ---"
    
    if command -v docker &>/dev/null; then
        echo "Docker 已安装: $(docker --version)"
        return 0
    fi
    
    # 使用官方安装脚本
    curl -fsSL https://get.docker.com | sh -s -- --version "$DOCKER_VERSION"
    
    systemctl enable --now docker
    echo "Docker 安装完成"
}

# 安装 cri-dockerd
install_cri_dockerd() {
    echo "--- 安装 cri-dockerd ---"
    
    if command -v cri-dockerd &>/dev/null; then
        echo "cri-dockerd 已安装"
        return 0
    fi
    
    local arch="amd64"
    curl -fsSL "https://github.com/Mirantis/cri-dockerd/releases/download/v${CRI_DOCKERD_VERSION}/cri-dockerd-${CRI_DOCKERD_VERSION}.${arch}.tgz" | \
        tar -xz -C /tmp
    
    install /tmp/cri-dockerd/cri-dockerd /usr/local/bin/
    
    # 创建 systemd service
    cat > /etc/systemd/system/cri-dockerd.service << 'EOF'
[Unit]
Description=CRI Interface for Docker Application Container Engine
Documentation=https://docs.mirantis.com
After=network-online.target firewalld.service docker.service
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/cri-dockerd --network-plugin=cni --pod-infra-container-image=registry.k8s.io/pause:3.9
ExecReload=/bin/kill -s HUP $MAINPID
TimeoutSec=0
RestartSec=2
Restart=always

[Install]
WantedBy=multi-user.target
EOF

    cat > /etc/systemd/system/cri-dockerd.socket << 'EOF'
[Unit]
Description=CRI Docker Socket for the API
PartOf=cri-dockerd.service

[Socket]
ListenStream=/run/cri-dockerd.sock

[Install]
WantedBy=sockets.target
EOF

    systemctl daemon-reload
    systemctl enable --now cri-dockerd.socket cri-dockerd
    
    echo "cri-dockerd 安装完成"
}

install_docker
install_cri_dockerd

echo "=== Docker + cri-dockerd 安装完成 ==="
```

### 6.15 网络配置和防火墙

#### 6.14.1 端口需求

| 组件 | 端口 | 协议 | 方向 | 说明 |
|------|------|------|------|------|
| kube-apiserver | 6443 | TCP | 入站 | Kubernetes API server |
| etcd | 2379-2380 | TCP | 入站 | etcd 客户端和 peer |
| kubelet | 10250 | TCP | 入站 | kubelet API |
| kubelet | 10259 | TCP | 入站 | kube-scheduler (control-plane) |
| kubelet | 10257 | TCP | 入站 | kube-controller-manager (control-plane) |
| containerd | 未开放 | - | - | 使用 Unix socket |
| CNI |  varies | TCP/UDP | 双向 | 取决于 CNI 插件 |

#### 6.14.2 防火墙配置脚本

```bash
#!/bin/bash
set -euo pipefail

ROLE="${ROLE:-worker}"  # control-plane | worker
FIREWALL_CMD="firewall-cmd"
USE_FIREWALLD=false
USE_UFW=false
USE_IPTABLES=false

# 检测防火墙工具
detect_firewall() {
    if command -v firewall-cmd &>/dev/null && systemctl is-active --quiet firewalld; then
        USE_FIREWALLD=true
        FIREWALL_CMD="firewall-cmd"
        echo "检测到 firewalld"
    elif command -v ufw &>/dev/null && ufw status | grep -q "Status: active"; then
        USE_UFW=true
        FIREWALL_CMD="ufw"
        echo "检测到 ufw"
    else
        USE_IPTABLES=true
        echo "使用 iptables (默认)"
    fi
}

# 开放端口
open_port() {
    local port="$1"
    local proto="${2:-tcp}"
    local desc="$3"
    
    echo "开放端口: $port/$proto ($desc)"
    
    if $USE_FIREWALLD; then
        firewall-cmd --permanent --add-port="${port}/${proto}"
    elif $USE_UFW; then
        ufw allow "${port}/${proto}" comment "$desc"
    else
        iptables -A INPUT -p "$proto" --dport "$port" -j ACCEPT
    fi
}

# 配置防火墙规则
configure_firewall() {
    echo "=== 配置防火墙 ==="
    
    # 通用规则 (所有节点)
    open_port 10250 tcp "kubelet API"
    
    # Control-plane 额外规则
    if [ "$ROLE" = "control-plane" ]; then
        open_port 6443 tcp "kube-apiserver"
        open_port 2379 tcp "etcd client"
        open_port 2380 tcp "etcd peer"
        open_port 10257 tcp "kube-controller-manager"
        open_port 10259 tcp "kube-scheduler"
    fi
    
    # 应用更改
    if $USE_FIREWALLD; then
        firewall-cmd --reload
    elif $USE_UFW; then
        ufw reload
    fi
    
    echo "防火墙配置完成"
}

detect_firewall
configure_firewall
```

#### 6.14.3 SELinux 配置

```bash
#!/bin/bash
set -euo pipefail

echo "=== SELinux 配置 ==="

if ! command -v getenforce &>/dev/null; then
    echo "SELinux 未安装，跳过"
    exit 0
fi

SELINUX_STATUS=$(getenforce)
echo "当前 SELinux 状态: $SELINUX_STATUS"

if [ "$SELINUX_STATUS" = "Disabled" ]; then
    echo "SELinux 已禁用，跳过配置"
    exit 0
fi

# 安装 containerd SELinux 策略 (如果可用)
install_containerd_selinux() {
    echo "--- 配置 containerd SELinux ---"
    
    if command -v semodule &>/dev/null; then
        # 检查是否有 containerd 策略
        if semodule -l 2>/dev/null | grep -q containerd; then
            echo "containerd SELinux 策略已安装"
        else
            echo "安装 containerd SELinux 策略"
            # 通常由 containerd-selinux 包提供
            if command -v dnf &>/dev/null; then
                dnf install -y container-selinux 2>/dev/null || true
            elif command -v yum &>/dev/null; then
                yum install -y container-selinux 2>/dev/null || true
            fi
        fi
    fi
}

# 配置 kubelet SELinux 上下文
configure_kubelet_selinux() {
    echo "--- 配置 kubelet SELinux 上下文 ---"
    
    # 确保 kubelet 数据目录有正确上下文
    if command -v semanage &>/dev/null; then
        semanage fcontext -a -t var_lib_t "/var/lib/kubelet(/.*)?" 2>/dev/null || true
        restorecon -Rv /var/lib/kubelet 2>/dev/null || true
    fi
}

# 配置 CNI SELinux
configure_cni_selinux() {
    echo "--- 配置 CNI SELinux ---"
    
    if [ -d /etc/cni ]; then
        restorecon -Rv /etc/cni 2>/dev/null || true
    fi
    
    if [ -d /opt/cni ]; then
        restorecon -Rv /opt/cni 2>/dev/null || true
    fi
}

install_containerd_selinux
configure_kubelet_selinux
configure_cni_selinux

echo "=== SELinux 配置完成 ==="
```

#### 6.14.4 网络预检脚本

```bash
#!/bin/bash
set -euo pipefail

echo "=== 网络预检 ==="

ERRORS=()

# 检查必需端口是否可用
check_port_available() {
    local port="$1"
    local proto="${2:-tcp}"
    local desc="$3"
    
    if ss -tlnp | grep -q ":${port} "; then
        echo "WARNING: 端口 $port/$proto ($desc) 已被占用"
    else
        echo "OK: 端口 $port/$proto ($desc) 可用"
    fi
}

# 检查网络连通性
check_connectivity() {
    echo "--- 网络连通性 ---"
    
    # 检查到 API server 的连通性 (worker 节点)
    if [ -n "${API_SERVER_ENDPOINT:-}" ]; then
        if curl -fsSk --connect-timeout 5 "https://${API_SERVER_ENDPOINT}:6443/healthz" &>/dev/null; then
            echo "OK: API server 可达"
        else
            ERRORS+=("无法连接到 API server: ${API_SERVER_ENDPOINT}")
        fi
    fi
    
    # 检查 DNS 解析
    if command -v nslookup &>/dev/null; then
        if nslookup kubernetes.default.svc.cluster.local &>/dev/null; then
            echo "OK: DNS 解析正常"
        else
            echo "WARNING: DNS 解析可能有问题"
        fi
    fi
    
    # 检查到 etcd peer 的连通性 (control-plane)
    if [ "$ROLE" = "control-plane" ] && [ -n "${ETCD_PEERS:-}" ]; then
        for peer in $ETCD_PEERS; do
            if ping -c 1 -W 2 "$peer" &>/dev/null; then
                echo "OK: etcd peer $peer 可达"
            else
                ERRORS+=("无法到达 etcd peer: $peer")
            fi
        done
    fi
}

# 检查 MTU
check_mtu() {
    echo "--- MTU 检查 ---"
    
    local mtu
    mtu=$(ip link show | grep "mtu" | head -1 | awk '{print $5}')
    
    if [ -n "$mtu" ] && [ "$mtu" -lt 1280 ]; then
        ERRORS+=("MTU 过小: $mtu (最小 1280)")
    else
        echo "OK: MTU = $mtu"
    fi
}

# 检查桥接流量 iptables
check_bridge_nf() {
    echo "--- 桥接流量检查 ---"
    
    local br_nf
    br_nf=$(sysctl -n net.bridge.bridge-nf-call-iptables 2>/dev/null || echo "0")
    
    if [ "$br_nf" = "1" ]; then
        echo "OK: bridge-nf-call-iptables 已启用"
    else
        echo "WARNING: bridge-nf-call-iptables 未启用，可能导致网络问题"
        echo "执行: sysctl net.bridge.bridge-nf-call-iptables=1"
    fi
}

# 执行检查
check_port_available 6443 tcp "kube-apiserver"
check_port_available 10250 tcp "kubelet"
check_port_available 2379 tcp "etcd"
check_connectivity
check_mtu
check_bridge_nf

# 输出结果
if [ ${#ERRORS[@]} -gt 0 ]; then
    echo ""
    echo "=== 网络预检发现以下问题 ==="
    for err in "${ERRORS[@]}"; do
        echo "  ERROR: $err"
    done
    exit 1
fi

echo "=== 网络预检通过 ==="
```

#### 6.14.5 完整安装流程集成

更新后的完整安装流程：

```
SSH 连接成功
    │
    ▼
┌─────────────────────────────────────────┐
│ 1. 环境准备                              │
│    ├── 检测 OS 类型和版本                │
│    ├── 检测包管理器                      │
│    ├── 检测防火墙工具                    │
│    └── 检测 SELinux 状态                │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 2. 网络预检                              │
│    ├── 端口可用性检查                    │
│    ├── 网络连通性检查                    │
│    ├── MTU 检查                         │
│    └── 桥接流量检查                      │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 3. 前置配置                              │
│    ├── 配置防火墙规则                    │
│    ├── 配置 SELinux 策略                │
│    ├── 禁用 swap                        │
│    └── 配置内核参数                      │
│        ├── net.bridge.bridge-nf-call-iptables │
│        └── fs.inotify.max-user-watches  │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 4. 组件安装                              │
│    ├── 选择安装模式 (在线/离线)          │
│    ├── 安装容器运行时                    │
│    │   ├── containerd (默认)            │
│    │   ├── CRI-O (可选)                 │
│    │   └ Docker + cri-dockerd (兼容)    │
│    └── 安装 Kubernetes 组件              │
│        ├── kubeadm                      │
│        ├── kubelet                      │
│        └── kubectl                      │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 5. 安装验证                              │
│    ├── 组件版本验证                      │
│    ├── 服务状态检查                      │
│    ├── CRI 连接检查                      │
│    └── 文件系统检查                      │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 6. 清理                                  │
│    ├── 删除临时文件                      │
│    ├── 清理包缓存                        │
│    └── 记录安装结果                      │
└─────────────────────────────────────────┘
```

---


### 6.16 CNI/CSI 安装与升级设计

#### 6.16.1 问题背景

裸金属集群在 kubeadm init/join 完成后，还需要安装以下核心插件才能正常运行：
- **CNI (Container Network Interface)**: 负责 Pod 网络通信，如 Calico, Cilium, Flannel
- **CSI (Container Storage Interface)**: 负责持久化存储供给，如 Ceph-CSI, Cinder-CSI, Local-CSI

CAPBM 需要在组件安装流程中集成 CNI/CSI 的安装和升级能力，支持：
- 多种 CNI/CSI 插件选择
- 在线和离线 (air-gap) 两种安装模式
- 版本管理和滚动升级
- 配置管理和回滚

#### 6.16.2 架构设计

```
┌─────────────────────────────────────────────────────────────────┐
│                    CNI/CSI 安装架构                              │
│                                                                 │
│  组件安装完成 (containerd + kubelet)                              │
│      │                                                          │
│      ▼                                                          │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ kubeadm init (control-plane) / kubeadm join (worker)     │  │
│  └────────────────────────┬─────────────────────────────────┘  │
│                           │                                    │
│                    kube-apiserver 就绪                          │
│                    ╱              ╲                            │
│                   是               │                           │
│                   │                │                           │
│                   ▼                ▼                           │
│  ┌─────────────────────┐ ┌──────────────────────────────────┐ │
│  │ CNI 安装            │ │ CSI 安装 (可选，可延后)           │ │
│  │ - 安装 CNI 二进制    │ │ - 部署 CSI Controller            │ │
│  │ - 部署 CNI 插件      │ │ - 部署 CSI Node DaemonSet        │ │
│  │ - 验证网络连通性     │ │ - 创建 StorageClass              │ │
│  └─────────────────────┘ └──────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**安装时机**:
| 组件 | 安装时机 | 说明 |
|------|---------|------|
| CNI | kubeadm init 后，CoreDNS 启动前 | kubelet 需要 CNI 才能将 Node 标记为 Ready |
| CSI | 集群初始化完成后（可异步） | 不影响集群基本功能，可按需安装 |

**管理方式**:
| 方式 | 适用场景 | 优缺点 |
|------|---------|--------|
| Manifest (kubectl apply) | 简单部署，无需额外依赖 | 升级需要手动管理版本 |
| Helm Chart | 复杂配置，支持 values 覆盖 | 需要预装 Helm 或使用 Helm SDK |

CAPBM 采用 **Manifest + Helm 双模式**，用户可通过 `installMode` 选择。

#### 6.16.3 CRD 设计扩展

在 `ComponentInstallConfig` 中新增 CNI 和 CSI 配置段：

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachine
spec:
  componentInstall:
    # ... 现有配置 ...
    
    # CNI 网络插件配置
    cni:
      enabled: true
      type: "calico"           # calico | cilium | flannel
      version: "3.26.1"
      installMode: "Manifest"  # Manifest | Helm
      upgradeStrategy: "RollingUpdate"
      config:
        podCIDR: "10.244.0.0/16"
        calico:
          ipam: "CalicoIPAM"
          mtu: 0
          bgp:
            enabled: true
            peerIPs: []
          typha:
            enabled: false
            replicas: 1
      airGap:
        enabled: false
        manifestSource: "HTTPServer"
        httpServerConfig:
          baseUrl: "https://internal-pkg.example.com/cni"
        cniPluginsArchive: "/opt/cni-plugins/cni-plugins-linux-amd64-v1.3.0.tgz"
        
    # CSI 存储插件配置
    csi:
      enabled: false
      driver: "ceph-csi"       # ceph-csi | cinder-csi | local-csi | nfs-csi
      version: "3.9.0"
      installMode: "Helm"
      config:
        cephCsi:
          clusterID: "my-ceph-cluster"
          monitors:
            - "10.0.0.10:6789"
            - "10.0.0.11:6789"
          rbd:
            enabled: true
            pool: "kubernetes"
          storageClass:
            name: "ceph-rbd"
            reclaimPolicy: "Delete"
            fsType: "ext4"
      airGap:
        enabled: false
        manifestSource: "HTTPServer"
        httpServerConfig:
          baseUrl: "https://internal-pkg.example.com/csi"
        chartArchive: "/opt/charts/ceph-csi.tgz"
```

**Go 类型定义**:

```go
// CNIConfig defines the CNI plugin installation configuration.
type CNIConfig struct {
	Enabled         bool               `json:"enabled,omitempty"`
	Type            string             `json:"type,omitempty"`
	Version         string             `json:"version,omitempty"`
	InstallMode     string             `json:"installMode,omitempty"`
	UpgradeStrategy string             `json:"upgradeStrategy,omitempty"`
	Config          *CNIPluginConfig   `json:"config,omitempty"`
	AirGap          *CNIAirGapConfig   `json:"airGap,omitempty"`
}

type CNIPluginConfig struct {
	PodCIDR string         `json:"podCIDR,omitempty"`
	Calico  *CalicoConfig  `json:"calico,omitempty"`
	Cilium  *CiliumConfig  `json:"cilium,omitempty"`
	Flannel *FlannelConfig `json:"flannel,omitempty"`
}

type CalicoConfig struct {
	IPAM  string             `json:"ipam,omitempty"`
	MTU   int                `json:"mtu,omitempty"`
	BGP   *CalicoBGPConfig   `json:"bgp,omitempty"`
	Typha *CalicoTyphaConfig `json:"typha,omitempty"`
}

type CalicoBGPConfig struct {
	Enabled bool     `json:"enabled,omitempty"`
	PeerIPs []string `json:"peerIPs,omitempty"`
}

type CalicoTyphaConfig struct {
	Enabled  bool `json:"enabled,omitempty"`
	Replicas int  `json:"replicas,omitempty"`
}

type CiliumConfig struct {
	KubeProxyReplacement  string              `json:"kubeProxyReplacement,omitempty"`
	RoutingMode           string              `json:"routingMode,omitempty"`
	IPv4NativeRoutingCIDR string              `json:"ipv4NativeRoutingCIDR,omitempty"`
	Hubble                *CiliumHubbleConfig `json:"hubble,omitempty"`
}

type CiliumHubbleConfig struct {
	Enabled bool `json:"enabled,omitempty"`
	Relay   bool `json:"relay,omitempty"`
	UI      bool `json:"ui,omitempty"`
}

type FlannelConfig struct {
	Backend string `json:"backend,omitempty"`
	MTU     int    `json:"mtu,omitempty"`
}

type CNIAirGapConfig struct {
	Enabled           bool              `json:"enabled,omitempty"`
	ManifestSource    string            `json:"manifestSource,omitempty"`
	HTTPServerConfig  *HTTPServerConfig `json:"httpServerConfig,omitempty"`
	LocalPath         string            `json:"localPath,omitempty"`
	ChartArchive      string            `json:"chartArchive,omitempty"`
	CNIPluginsArchive string            `json:"cniPluginsArchive,omitempty"`
}

// CSIConfig defines the CSI driver installation configuration.
type CSIConfig struct {
	Enabled     bool             `json:"enabled,omitempty"`
	Driver      string           `json:"driver,omitempty"`
	Version     string           `json:"version,omitempty"`
	InstallMode string           `json:"installMode,omitempty"`
	Config      *CSIDriverConfig `json:"config,omitempty"`
	AirGap      *CSIAirGapConfig `json:"airGap,omitempty"`
}

type CSIDriverConfig struct {
	CephCsi   *CephCsiConfig   `json:"cephCsi,omitempty"`
	CinderCsi *CinderCsiConfig `json:"cinderCsi,omitempty"`
	LocalCsi  *LocalCsiConfig  `json:"localCsi,omitempty"`
	NfsCsi    *NfsCsiConfig    `json:"nfsCsi,omitempty"`
}

type CephCsiConfig struct {
	ClusterID    string           `json:"clusterID"`
	Monitors     []string         `json:"monitors"`
	CephFS       *CephFSConfig    `json:"cephfs,omitempty"`
	RBD          *RBDConfig       `json:"rbd,omitempty"`
	StorageClass *CSIStorageClass `json:"storageClass,omitempty"`
}

type CephFSConfig struct {
	Enabled     bool `json:"enabled,omitempty"`
	KernelMount bool `json:"kernelMount,omitempty"`
	FuseMount   bool `json:"fuseMount,omitempty"`
}

type RBDConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Pool    string `json:"pool,omitempty"`
}

type CinderCsiConfig struct {
	OpenstackCloudConfigSecret string           `json:"openstackCloudConfigSecret"`
	StorageClass               *CSIStorageClass `json:"storageClass,omitempty"`
}

type LocalCsiConfig struct {
	StorageClass *CSIStorageClass `json:"storageClass,omitempty"`
}

type NfsCsiConfig struct {
	Server       string           `json:"server"`
	Share        string           `json:"share"`
	StorageClass *CSIStorageClass `json:"storageClass,omitempty"`
}

type CSIStorageClass struct {
	Name              string            `json:"name"`
	ReclaimPolicy     string            `json:"reclaimPolicy,omitempty"`
	FSType            string            `json:"fsType,omitempty"`
	VolumeBindingMode string            `json:"volumeBindingMode,omitempty"`
	MountOptions      []string          `json:"mountOptions,omitempty"`
	Parameters        map[string]string `json:"parameters,omitempty"`
}

type CSIAirGapConfig struct {
	Enabled          bool              `json:"enabled,omitempty"`
	ManifestSource   string            `json:"manifestSource,omitempty"`
	HTTPServerConfig *HTTPServerConfig `json:"httpServerConfig,omitempty"`
	LocalPath        string            `json:"localPath,omitempty"`
	ChartArchive     string            `json:"chartArchive,omitempty"`
}
```

更新 `ComponentInstallConfig`:

```go
type ComponentInstallConfig struct {
	// ... 现有字段 ...
	CNI CNIConfig `json:"cni,omitempty"`
	CSI CSIConfig `json:"csi,omitempty"`
}
```

#### 6.16.4 CNI 安装流程

##### 6.16.4.1 在线安装流程

```
kubeadm init 执行完成
    │
    ▼
┌─────────────────────────────────────────────────┐
│ 1. 检测 CNI 状态                                  │
│    ├── 检查 /opt/cni/bin 目录                    │
│    ├── 检查 /etc/cni/net.d 目录                  │
│    └── 检查 CNI DaemonSet/Deployment 状态        │
└─────────────────┬───────────────────────────────┘
                  │
           是否已安装且版本匹配？
           ╱              ╲
          是               否
          │                │
          ▼                ▼
┌──────────────────┐ ┌──────────────────────────┐
│ 跳过安装         │ │ 2. 安装 CNI 二进制插件    │
│                  │ │    ├── 下载 CNI plugins   │
│                  │ │    └── 解压到 /opt/cni/bin│
└──────────────────┘ └──────────┬───────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────┐
│ 3. 部署 CNI 插件                                  │
│    ├── 渲染 Manifest (替换 PodCIDR 等参数)        │
│    ├── kubectl apply -f                          │
│    └── 等待 DaemonSet/Deployment Ready           │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 4. 验证 CNI 状态                                  │
│    ├── CNI Pod 全部 Running                      │
│    ├── Node 状态变为 Ready                       │
│    └── 跨节点网络连通性测试                       │
└─────────────────────────────────────────────────┘
```

##### 6.16.4.2 CNI 在线安装脚本 (Calico 示例)

```bash
#!/bin/bash
set -euo pipefail

CNI_TYPE="${CNI_TYPE:-calico}"
CNI_VERSION="${CNI_VERSION:-3.26.1}"
POD_CIDR="${POD_CIDR:-10.244.0.0/16}"
CNI_PLUGINS_VERSION="${CNI_PLUGINS_VERSION:-1.3.0}"

echo "=== CNI 安装开始 (type=$CNI_TYPE, version=$CNI_VERSION) ==="

install_cni_plugins() {
    if [ -d "/opt/cni/bin" ] && [ "$(ls -A /opt/cni/bin 2>/dev/null)" ]; then
        echo "CNI 二进制插件已安装"
        return 0
    fi
    mkdir -p /opt/cni/bin
    curl -fsSL "https://github.com/containernetworking/plugins/releases/download/v${CNI_PLUGINS_VERSION}/cni-plugins-linux-amd64-v${CNI_PLUGINS_VERSION}.tgz" | tar -C /opt/cni/bin -xz
    echo "CNI 二进制插件安装完成"
}

install_calico() {
    local manifest_url="https://raw.githubusercontent.com/projectcalico/calico/v${CNI_VERSION}/manifests/calico.yaml"
    local temp_manifest=$(mktemp)
    curl -fsSL "$manifest_url" -o "$temp_manifest"
    sed -i "s|\"192.168.0.0/16\"|\"${POD_CIDR}\"|g" "$temp_manifest"
    kubectl apply -f "$temp_manifest"
    rm -f "$temp_manifest"
    kubectl rollout status daemonset/calico-node -n kube-system --timeout=300s
    echo "Calico 部署完成"
}

install_cilium_helm() {
    helm repo add cilium https://helm.cilium.io/
    helm repo update
    helm upgrade --install cilium cilium/cilium \
        --namespace kube-system --version "v${CNI_VERSION}" \
        --set ipam.mode=kubernetes \
        --set kubeProxyReplacement="${CILIUM_KUBE_PROXY_REPLACEMENT:-partial}" \
        --wait --timeout=300s
    echo "Cilium 部署完成"
}

install_flannel() {
    local manifest_url="https://github.com/flannel-io/flannel/releases/download/v${CNI_VERSION}/kube-flannel.yml"
    local temp_manifest=$(mktemp)
    curl -fsSL "$manifest_url" -o "$temp_manifest"
    sed -i "s|\"10.244.0.0/16\"|\"${POD_CIDR}\"|g" "$temp_manifest"
    kubectl apply -f "$temp_manifest"
    rm -f "$temp_manifest"
    kubectl rollout status daemonset/kube-flannel-ds -n kube-flannel --timeout=300s
    echo "Flannel 部署完成"
}

verify_cni() {
    [ -d "/opt/cni/bin" ] && [ -n "$(ls -A /opt/cni/bin 2>/dev/null)" ] && echo "CNI 二进制: OK" || { echo "ERROR: /opt/cni/bin 为空"; return 1; }
    [ -d "/etc/cni/net.d" ] && [ -n "$(ls -A /etc/cni/net.d 2>/dev/null)" ] && echo "CNI 配置: OK" || { echo "ERROR: /etc/cni/net.d 为空"; return 1; }
    local status=$(kubectl get node $(hostname) -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")
    [ "$status" = "True" ] && echo "Node Ready: OK" || echo "WARNING: Node 尚未 Ready"
    echo "CNI 验证完成"
}

install_cni_plugins
case "$CNI_TYPE" in
    calico)  install_calico ;;
    cilium)  install_cilium_helm ;;
    flannel) install_flannel ;;
    *) echo "ERROR: 不支持的 CNI 类型: $CNI_TYPE"; exit 1 ;;
esac
verify_cni
echo "=== CNI 安装完成 ==="
```

##### 6.16.4.3 CNI 离线安装流程

```
管理端准备
    │
    ├── 1. 下载 CNI plugins 二进制包
    │   └── cni-plugins-linux-amd64-v1.3.0.tgz
    │
    ├── 2. 下载 CNI Manifest/Helm Chart
    │   ├── calico.yaml (渲染后的，含正确 PodCIDR)
    │   └── 或 calico-3.26.1.tgz (Helm Chart)
    │
    └── 3. 分发到内网
        ├── 上传到内网 HTTP 服务器
        └── 或预置到目标机器 /opt/capbm/cni/
        
目标机器安装
    │
    ├── 1. 安装 CNI 二进制插件
    │   └── tar -C /opt/cni/bin -xz /opt/capbm/cni/cni-plugins-*.tgz
    │
    ├── 2. 部署 CNI 插件
    │   ├── Manifest: kubectl apply -f /opt/capbm/cni/calico.yaml
    │   └── Helm: helm install /opt/capbm/cni/calico-3.26.1.tgz
    │
    └── 3. 验证
```

##### 6.16.4.4 CNI 离线安装脚本

```bash
#!/bin/bash
set -euo pipefail

CNI_TYPE="${CNI_TYPE:-calico}"
CNI_VERSION="${CNI_VERSION:-3.26.1}"
CNI_PLUGINS_ARCHIVE="${CNI_PLUGINS_ARCHIVE:-/opt/capbm/cni/cni-plugins-linux-amd64-v1.3.0.tgz}"
CNI_MANIFEST_PATH="${CNI_MANIFEST_PATH:-/opt/capbm/cni/calico.yaml}"
CNI_CHART_ARCHIVE="${CNI_CHART_ARCHIVE:-/opt/capbm/cni/calico-${CNI_VERSION}.tgz}"
INSTALL_MODE="${INSTALL_MODE:-Manifest}"

echo "=== CNI 离线安装开始 ==="

install_cni_plugins_offline() {
    [ -d "/opt/cni/bin" ] && [ -n "$(ls -A /opt/cni/bin 2>/dev/null)" ] && { echo "CNI 二进制已安装"; return 0; }
    [ ! -f "$CNI_PLUGINS_ARCHIVE" ] && { echo "ERROR: CNI 二进制插件包不存在"; return 1; }
    mkdir -p /opt/cni/bin && tar -C /opt/cni/bin -xzf "$CNI_PLUGINS_ARCHIVE"
    echo "CNI 二进制离线安装完成"
}

install_cni_manifest_offline() {
    [ ! -f "$CNI_MANIFEST_PATH" ] && { echo "ERROR: Manifest 不存在"; return 1; }
    kubectl apply -f "$CNI_MANIFEST_PATH"
    case "$CNI_TYPE" in
        calico)  kubectl rollout status daemonset/calico-node -n kube-system --timeout=300s ;;
        flannel) kubectl rollout status daemonset/kube-flannel-ds -n kube-flannel --timeout=300s ;;
    esac
    echo "CNI 离线部署完成 (Manifest)"
}

install_cni_helm_offline() {
    [ ! -f "$CNI_CHART_ARCHIVE" ] && { echo "ERROR: Helm Chart 不存在"; return 1; }
    helm upgrade --install "$CNI_TYPE" "$CNI_CHART_ARCHIVE" --namespace kube-system --wait --timeout=300s
    echo "CNI 离线部署完成 (Helm)"
}

install_cni_plugins_offline
case "$INSTALL_MODE" in
    Manifest) install_cni_manifest_offline ;;
    Helm)     install_cni_helm_offline ;;
esac
echo "=== CNI 离线安装完成 ==="
```

#### 6.16.5 CSI 安装流程

##### 6.16.5.1 在线安装流程

```
集群初始化完成 (kubeadm init + CNI 就绪)
    │
    ▼
┌─────────────────────────────────────────────────┐
│ 1. 检测 CSI 状态                                  │
│    ├── 检查 CSI Controller Deployment            │
│    ├── 检查 CSI Node DaemonSet                   │
│    └── 检查 StorageClass                         │
└─────────────────┬───────────────────────────────┘
                  │
           是否已安装且版本匹配？
           ╱              ╲
          是               否
          │                │
          ▼                ▼
┌──────────────────┐ ┌──────────────────────────┐
│ 跳过安装         │ │ 2. 部署 CSI Controller    │
│                  │ │    ├── 渲染 Manifest/Chart │
│                  │ │    └── 等待 Deployment Ready│
└──────────────────┘ └──────────┬───────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────┐
│ 3. 部署 CSI Node DaemonSet                       │
│    └── 等待 DaemonSet Ready                      │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 4. 创建 StorageClass                             │
│    └── kubectl apply -f                          │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 5. 验证 CSI 状态                                 │
│    ├── CSIDriver 资源存在                        │
│    ├── Controller/Node Pod Running               │
│    ├── StorageClass 存在                         │
│    └── PVC 创建/绑定测试 (可选)                  │
└─────────────────────────────────────────────────┘
```

##### 6.16.5.2 CSI 在线安装脚本 (Ceph-CSI 示例)

```bash
#!/bin/bash
set -euo pipefail

CSI_DRIVER="${CSI_DRIVER:-ceph-csi}"
CSI_VERSION="${CSI_VERSION:-3.9.0}"
INSTALL_MODE="${INSTALL_MODE:-Helm}"
CEPH_CLUSTER_ID="${CEPH_CLUSTER_ID:-my-ceph-cluster}"
CEPH_MONITORS="${CEPH_MONITORS:-10.0.0.10:6789,10.0.0.11:6789}"
CEPH_RBD_POOL="${CEPH_RBD_POOL:-kubernetes}"
SC_NAME="${SC_NAME:-ceph-rbd}"

echo "=== CSI 安装开始 (driver=$CSI_DRIVER, version=$CSI_VERSION) ==="

install_ceph_csi_helm() {
    helm repo add ceph-csi https://ceph.github.io/csi-charts
    helm repo update
    local monitors_json="["
    local first=true
    IFS=',' read -ra MON_ARRAY <<< "$CEPH_MONITORS"
    for mon in "${MON_ARRAY[@]}"; do
        $first && { monitors_json="${monitors_json}\"${mon}\""; first=false; } || monitors_json="${monitors_json},\"${mon}\""
    done
    monitors_json="${monitors_json}]"
    helm upgrade --install ceph-csi ceph-csi/ceph-csi \
        --namespace ceph-csi --create-namespace --version "v${CSI_VERSION}" \
        --set "csiConfig[0].clusterID=${CEPH_CLUSTER_ID}" \
        --set "csiConfig[0].monitors=${monitors_json}" \
        --set "storageClass.create=true" \
        --set "storageClass.name=${SC_NAME}" \
        --set "storageClass.pool=${CEPH_RBD_POOL}" \
        --wait --timeout=300s
    echo "Ceph-CSI 部署完成"
}

install_ceph_csi_manifest() {
    kubectl create namespace ceph-csi --dry-run=client -o yaml | kubectl apply -f -
    local base="https://raw.githubusercontent.com/ceph/ceph-csi/v${CSI_VERSION}/deploy/cephcsi/kubernetes"
    for f in csi-config-map.yaml csi-rbdplugin.yaml csi-rbdplugin-provisioner.yaml; do
        curl -fsSL "${base}/${f}" | kubectl apply -n ceph-csi -f -
    done
    cat <<EOF | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${SC_NAME}
provisioner: rbd.csi.ceph.com
parameters:
  clusterID: ${CEPH_CLUSTER_ID}
  pool: ${CEPH_RBD_POOL}
  imageFeatures: layering
  csi.storage.k8s.io/provisioner-secret-name: csi-rbd-secret
  csi.storage.k8s.io/provisioner-secret-namespace: ceph-csi
  csi.storage.k8s.io/controller-expand-secret-name: csi-rbd-secret
  csi.storage.k8s.io/controller-expand-secret-namespace: ceph-csi
  csi.storage.k8s.io/node-stage-secret-name: csi-rbd-secret
  csi.storage.k8s.io/node-stage-secret-namespace: ceph-csi
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
EOF
    kubectl rollout status deployment/ceph-csi-rbdplugin-provisioner -n ceph-csi --timeout=300s
    echo "Ceph-CSI 部署完成"
}

install_local_csi() {
    kubectl apply -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/v${CSI_VERSION}/deploy/local-path-storage.yaml"
    kubectl rollout status deployment/local-path-provisioner -n local-path-storage --timeout=300s
    echo "Local-CSI 部署完成"
}

verify_csi() {
    kubectl get storageclass "$SC_NAME" &>/dev/null && echo "StorageClass ($SC_NAME): OK" || { echo "ERROR: StorageClass 不存在"; return 1; }
    kubectl get csidriver 2>/dev/null | grep -q "$CSI_DRIVER" && echo "CSIDriver: OK" || echo "WARNING: CSIDriver 尚未注册"
    echo "CSI 验证完成"
}

case "$CSI_DRIVER" in
    ceph-csi)
        [ "$INSTALL_MODE" = "Helm" ] && install_ceph_csi_helm || install_ceph_csi_manifest ;;
    local-csi) install_local_csi ;;
    *) echo "ERROR: 不支持的 CSI 类型: $CSI_DRIVER"; exit 1 ;;
esac
verify_csi
echo "=== CSI 安装完成 ==="
```

##### 6.16.5.3 CSI 离线安装流程

```
管理端准备
    │
    ├── 1. 下载 CSI Helm Chart / Manifest
    │   └── ceph-csi-3.9.0.tgz
    │
    ├── 2. 准备 CSI 镜像列表并打包
    │   ├── quay.io/cephcsi/cephcsi:v3.9.0
    │   ├── registry.k8s.io/sig-storage/csi-provisioner:v3.6.0
    │   ├── registry.k8s.io/sig-storage/csi-attacher:v4.4.0
    │   └── 等...
    │
    └── 3. 分发到内网
        
目标机器安装
    │
    ├── 1. 加载 CSI 镜像到 containerd
    │   └── ctr -n k8s.io images import csi-images.tar
    │
    ├── 2. 部署 CSI (Helm/Manifest 从本地读取)
    │
    └── 3. 验证
```

##### 6.16.5.4 CSI 离线安装脚本

```bash
#!/bin/bash
set -euo pipefail

CSI_DRIVER="${CSI_DRIVER:-ceph-csi}"
CSI_VERSION="${CSI_VERSION:-3.9.0}"
INSTALL_MODE="${INSTALL_MODE:-Helm}"
CSI_CHART_ARCHIVE="${CSI_CHART_ARCHIVE:-/opt/capbm/csi/ceph-csi-${CSI_VERSION}.tgz}"
CSI_IMAGES_ARCHIVE="${CSI_IMAGES_ARCHIVE:-/opt/capbm/csi/ceph-csi-images.tar}"

echo "=== CSI 离线安装开始 ==="

load_csi_images() {
    [ ! -f "$CSI_IMAGES_ARCHIVE" ] && { echo "ERROR: CSI 镜像包不存在"; return 1; }
    ctr -n k8s.io images import "$CSI_IMAGES_ARCHIVE"
    echo "CSI 镜像加载完成"
}

install_csi_helm_offline() {
    [ ! -f "$CSI_CHART_ARCHIVE" ] && { echo "ERROR: Helm Chart 不存在"; return 1; }
    helm upgrade --install "$CSI_DRIVER" "$CSI_CHART_ARCHIVE" \
        --namespace "${CSI_DRIVER}" --create-namespace \
        --set "csiConfig[0].clusterID=${CEPH_CLUSTER_ID}" \
        --set "csiConfig[0].monitors=${CEPH_MONITORS}" \
        --set "storageClass.create=true" \
        --set "storageClass.name=${SC_NAME}" \
        --wait --timeout=300s
    echo "CSI 离线部署完成 (Helm)"
}

install_csi_manifest_offline() {
    local manifest_path="${CSI_MANIFEST_PATH:-/opt/capbm/csi/${CSI_DRIVER}.yaml}"
    [ ! -f "$manifest_path" ] && { echo "ERROR: Manifest 不存在"; return 1; }
    kubectl create namespace "${CSI_DRIVER}" --dry-run=client -o yaml | kubectl apply -f -
    kubectl apply -n "${CSI_DRIVER}" -f "$manifest_path"
    echo "CSI 离线部署完成 (Manifest)"
}

verify_csi() {
    kubectl get storageclass "$SC_NAME" &>/dev/null && echo "StorageClass: OK" || { echo "ERROR: StorageClass 不存在"; return 1; }
    echo "CSI 验证完成"
}

load_csi_images
case "$INSTALL_MODE" in
    Helm)     install_csi_helm_offline ;;
    Manifest) install_csi_manifest_offline ;;
esac
verify_csi
echo "=== CSI 离线安装完成 ==="
```

#### 6.16.6 CNI/CSI 升级流程

##### 6.16.6.1 升级触发条件

| 触发方式 | 说明 |
|---------|------|
| 版本变更 | `cni.version` 或 `csi.version` 字段变化 |
| 配置变更 | `cni.config` 或 `csi.config` 字段变化 |
| 手动触发 | 通过 Annotation 或 Condition 手动触发 |

##### 6.16.6.2 CNI 升级流程

```
检测到 CNI 版本/配置变更
    │
    ▼
┌─────────────────────────────────────────────────┐
│ 1. 备份当前配置                                   │
│    ├── 导出当前 Manifest/Helm values             │
│    └── 保存到 /tmp/.capbm_cni_backup/           │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 2. 执行升级                                       │
│    ├── Manifest: 下载新 Manifest → kubectl apply │
│    └── Helm: helm upgrade (自动滚动更新)          │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 3. 验证升级                                       │
│    ├── CNI Pod 全部 Running (新版本镜像)          │
│    ├── Node 保持 Ready                           │
│    └── 跨节点网络连通性测试                       │
└─────────────────┬───────────────────────────────┘
                  │
            验证通过？
            ╱              ╲
           是               否
           │                │
           ▼                ▼
┌──────────────────┐ ┌──────────────────────────┐
│ 升级成功         │ │ 4. 回滚                   │
│                  │ │    ├── 恢复备份的 Manifest │
│                  │ │    └── 或 helm rollback   │
└──────────────────┘ └──────────────────────────┘
```

##### 6.16.6.3 CSI 升级流程

```
检测到 CSI 版本/配置变更
    │
    ▼
┌─────────────────────────────────────────────────┐
│ 1. 备份当前配置 + PVC/PV 状态快照                │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 2. 执行升级                                       │
│    ├── 升级 CSI Controller (滚动更新)            │
│    ├── 升级 CSI Node DaemonSet (逐节点)          │
│    └── 更新 StorageClass (如有变更)              │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│ 3. 验证升级                                       │
│    ├── Controller/Node Pod Running               │
│    ├── 已有 PVC/PV 状态正常                      │
│    └── 创建测试 PVC 验证供给能力                  │
└─────────────────────────────────────────────────┘
```

##### 6.16.6.4 升级脚本 (CNI 示例)

```bash
#!/bin/bash
set -euo pipefail

CNI_TYPE="${CNI_TYPE:-calico}"
CURRENT_VERSION="${CURRENT_VERSION:-}"
TARGET_VERSION="${TARGET_VERSION:-3.26.1}"
POD_CIDR="${POD_CIDR:-10.244.0.0/16}"
BACKUP_DIR="/tmp/.capbm_cni_backup/$(date +%Y%m%d%H%M%S)"

echo "=== CNI 升级开始 ($CURRENT_VERSION -> $TARGET_VERSION) ==="

backup_current_config() {
    mkdir -p "$BACKUP_DIR"
    kubectl get daemonset/calico-node -n kube-system -o yaml > "$BACKUP_DIR/calico-node.yaml" 2>/dev/null || true
    kubectl get deployment/calico-kube-controllers -n kube-system -o yaml > "$BACKUP_DIR/calico-kube-controllers.yaml" 2>/dev/null || true
    helm get values calico -n kube-system > "$BACKUP_DIR/calico-values.yaml" 2>/dev/null || true
    echo "配置已备份到: $BACKUP_DIR"
}

rollback_cni() {
    echo "=== 开始回滚 CNI ==="
    [ -f "$BACKUP_DIR/calico-node.yaml" ] && kubectl apply -f "$BACKUP_DIR/calico-node.yaml"
    [ -f "$BACKUP_DIR/calico-kube-controllers.yaml" ] && kubectl apply -f "$BACKUP_DIR/calico-kube-controllers.yaml"
    helm rollback calico -n kube-system 2>/dev/null || true
    echo "回滚完成"
}

upgrade_cni() {
    case "$CNI_TYPE" in
        calico)
            local url="https://raw.githubusercontent.com/projectcalico/calico/v${TARGET_VERSION}/manifests/calico.yaml"
            local tmp=$(mktemp)
            curl -fsSL "$url" -o "$tmp"
            sed -i "s|\"192.168.0.0/16\"|\"${POD_CIDR}\"|g" "$tmp"
            kubectl apply -f "$tmp" && rm -f "$tmp"
            kubectl rollout status daemonset/calico-node -n kube-system --timeout=300s
            ;;
        cilium)
            helm upgrade cilium cilium/cilium --namespace kube-system --version "v${TARGET_VERSION}" --reuse-values --wait --timeout=300s
            ;;
        flannel)
            local url="https://github.com/flannel-io/flannel/releases/download/v${TARGET_VERSION}/kube-flannel.yml"
            local tmp=$(mktemp)
            curl -fsSL "$url" -o "$tmp"
            sed -i "s|\"10.244.0.0/16\"|\"${POD_CIDR}\"|g" "$tmp"
            kubectl apply -f "$tmp" && rm -f "$tmp"
            kubectl rollout status daemonset/kube-flannel-ds -n kube-flannel --timeout=300s
            ;;
    esac
    echo "CNI 升级完成"
}

verify_upgrade() {
    local not_ready=$(kubectl get pods -n kube-system -l k8s-app=calico-node --field-selector=status.phase!=Running 2>/dev/null | wc -l)
    [ "$not_ready" -gt 0 ] && { echo "ERROR: $not_ready 个 CNI Pod 未 Running"; return 1; }
    not_ready=$(kubectl get nodes --field-selector=status.conditions[?(@.type=="Ready")].status!=True 2>/dev/null | wc -l)
    [ "$not_ready" -gt 0 ] && { echo "ERROR: $not_ready 个 Node 未 Ready"; return 1; }
    echo "CNI 升级验证通过"
}

backup_current_config
if upgrade_cni; then
    if verify_upgrade; then
        echo "=== CNI 升级成功 ==="
    else
        echo "ERROR: 验证失败，执行回滚"; rollback_cni; exit 1
    fi
else
    echo "ERROR: 升级失败，执行回滚"; rollback_cni; exit 1
fi
```

#### 6.16.7 支持的 CNI/CSI 矩阵

| 类型 | 插件 | 最低版本 | 安装模式 | 离线支持 | 说明 |
|------|------|---------|---------|---------|------|
| CNI | Calico | 3.26+ | Manifest | 是 | 默认推荐，BGP 模式适合裸金属 |
| CNI | Cilium | 1.14+ | Helm | 是 | eBPF 高性能网络，支持 kube-proxy 替换 |
| CNI | Flannel | 0.23+ | Manifest | 是 | 简单轻量，适合小规模集群 |
| CSI | Ceph-CSI | 3.9+ | Helm/Manifest | 是 | 企业级分布式存储 |
| CSI | Cinder-CSI | 1.28+ | Helm/Manifest | 是 | OpenStack 环境 |
| CSI | Local-CSI | - | Manifest | 是 | hostPath 本地存储，开发测试 |
| CSI | NFS-CSI | 4.5+ | Manifest | 是 | 实验性支持 |

#### 6.16.8 ClusterClass 变量和 Patches

在 ClusterClass 中新增 CNI/CSI 变量：

```yaml
  variables:
  - name: cni
    schema:
      openAPIV3Schema:
        type: object
        properties:
          enabled:
            type: boolean
            default: true
          type:
            type: string
            default: "calico"
            enum: ["calico", "cilium", "flannel"]
          version:
            type: string
          installMode:
            type: string
            default: "Manifest"
            enum: ["Manifest", "Helm"]
          config:
            type: object
            properties:
              podCIDR:
                type: string
              calico:
                type: object
                properties:
                  ipam:
                    type: string
                    default: "CalicoIPAM"
                  mtu:
                    type: integer
                    default: 0
                  bgp:
                    type: object
                    properties:
                      enabled:
                        type: boolean
                        default: true
                      peerIPs:
                        type: array
                        items:
                          type: string
                  typha:
                    type: object
                    properties:
                      enabled:
                        type: boolean
                        default: false
                      replicas:
                        type: integer
                        default: 1
              cilium:
                type: object
                properties:
                  kubeProxyReplacement:
                    type: string
                    default: "partial"
                  routingMode:
                    type: string
                    default: "tunnel"
                  hubble:
                    type: object
                    properties:
                      enabled:
                        type: boolean
                        default: false
              flannel:
                type: object
                properties:
                  backend:
                    type: string
                    default: "vxlan"
                  mtu:
                    type: integer
                    default: 0
          airGap:
            type: object
            properties:
              enabled:
                type: boolean
                default: false
              manifestSource:
                type: string
                default: "HTTPServer"
              httpServerConfig:
                type: object
                properties:
                  baseUrl:
                    type: string
              cniPluginsArchive:
                type: string

  - name: csi
    schema:
      openAPIV3Schema:
        type: object
        properties:
          enabled:
            type: boolean
            default: false
          driver:
            type: string
            enum: ["ceph-csi", "cinder-csi", "local-csi", "nfs-csi"]
          version:
            type: string
          installMode:
            type: string
            default: "Helm"
            enum: ["Manifest", "Helm"]
          config:
            type: object
            properties:
              cephCsi:
                type: object
                properties:
                  clusterID:
                    type: string
                  monitors:
                    type: array
                    items:
                      type: string
                  rbd:
                    type: object
                    properties:
                      enabled:
                        type: boolean
                      pool:
                        type: string
                  storageClass:
                    type: object
                    properties:
                      name:
                        type: string
                        default: "ceph-rbd"
                      reclaimPolicy:
                        type: string
                        default: "Delete"
                      fsType:
                        type: string
                        default: "ext4"
              localCsi:
                type: object
                properties:
                  storageClass:
                    type: object
                    properties:
                      name:
                        type: string
                        default: "local-path"
              nfsCsi:
                type: object
                properties:
                  server:
                    type: string
                  share:
                    type: string
                  storageClass:
                    type: object
                    properties:
                      name:
                        type: string
          airGap:
            type: object
            properties:
              enabled:
                type: boolean
                default: false
              manifestSource:
                type: string
                default: "HTTPServer"
              httpServerConfig:
                type: object
                properties:
                  baseUrl:
                    type: string
              chartArchive:
                type: string
```

Patch 定义：

```yaml
  patches:
  - name: cni
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/componentInstall/cni
        valueFrom:
          variable: cni
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        matchResources:
          machineDeploymentClass:
            names:
            - default-worker
      jsonPatches:
      - op: add
        path: /spec/template/spec/componentInstall/cni
        valueFrom:
          variable: cni

  - name: csi
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/componentInstall/csi
        valueFrom:
          variable: csi
```

#### 6.16.9 Controller 集成

在 `BareMetalMachine Controller` 的调谐流程中新增 CNI/CSI 安装步骤：

```go
func (r *BareMetalMachineReconciler) reconcileNormal(ctx context.Context, bmMachine *infrav1.BareMetalMachine, machine *clusterv1.Machine) (ctrl.Result, error) {
	// ... 现有步骤 1-6 (分配机器、SSH、预检、组件安装) ...

	// 7. 安装 CNI (仅 control-plane 首个节点执行)
	if bmMachine.Spec.ComponentInstall != nil && bmMachine.Spec.ComponentInstall.CNI.Enabled {
		if r.isFirstControlPlaneNode(ctx, bmMachine) {
			cniResult, err := r.installCNI(ctx, sshConn, bmMachine, machine)
			if err != nil {
				markConditionFalse(bmMachine, infrav1.CNIInstalledCondition, infrav1.CNIInstallFailedReason, clusterv1.ConditionSeverityError, err.Error())
				return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
			}
			if !cniResult.Completed {
				log.Info("CNI installation in progress", "progress", cniResult.Progress)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			if !cniResult.Success {
				markConditionFalse(bmMachine, infrav1.CNIInstalledCondition, infrav1.CNIInstallFailedReason, clusterv1.ConditionSeverityError, cniResult.Error)
				return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
			}
			markConditionTrue(bmMachine, infrav1.CNIInstalledCondition)
		}
	}

	// 8. 安装 CSI (仅 control-plane 首个节点执行，可选)
	if bmMachine.Spec.ComponentInstall != nil && bmMachine.Spec.ComponentInstall.CSI.Enabled {
		if r.isFirstControlPlaneNode(ctx, bmMachine) {
			csiResult, err := r.installCSI(ctx, sshConn, bmMachine, machine)
			if err != nil {
				markConditionFalse(bmMachine, infrav1.CSIInstalledCondition, infrav1.CSIInstallFailedReason, clusterv1.ConditionSeverityError, err.Error())
				return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
			}
			if !csiResult.Completed {
				log.Info("CSI installation in progress", "progress", csiResult.Progress)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			if !csiResult.Success {
				markConditionFalse(bmMachine, infrav1.CSIInstalledCondition, infrav1.CSIInstallFailedReason, clusterv1.ConditionSeverityError, csiResult.Error)
				return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
			}
			markConditionTrue(bmMachine, infrav1.CSIInstalledCondition)
		}
	}

	// ... 后续步骤 (设置 ProviderID、更新状态) ...
}

// isFirstControlPlaneNode 判断是否为首个 control-plane 节点
func (r *BareMetalMachineReconciler) isFirstControlPlaneNode(ctx context.Context, bmMachine *infrav1.BareMetalMachine) bool {
	if bmMachine.Spec.Role != "control-plane" {
		return false
	}
	clusterName := bmMachine.Labels[clusterv1.ClusterNameLabel]
	var bmMachineList infrav1.BareMetalMachineList
	if err := r.List(ctx, &bmMachineList, client.InNamespace(bmMachine.Namespace), client.MatchingLabels{clusterv1.ClusterNameLabel: clusterName}); err != nil {
		return false
	}
	for _, m := range bmMachineList.Items {
		if m.Spec.Role == "control-plane" && m.Status.Ready && m.Name != bmMachine.Name {
			return false
		}
	}
	return true
}

// installCNI 安装 CNI 插件
func (r *BareMetalMachineReconciler) installCNI(ctx context.Context, sshConn *ssh.SSHConnection, bmMachine *infrav1.BareMetalMachine, machine *clusterv1.Machine) (*installer.CNIInstallResult, error) {
	cniConfig := bmMachine.Spec.ComponentInstall.CNI
	params := installer.CNIParams{
		Type:        cniConfig.Type,
		Version:     cniConfig.Version,
		InstallMode: cniConfig.InstallMode,
		PodCIDR:     cniConfig.Config.PodCIDR,
		AirGap:      cniConfig.AirGap != nil && cniConfig.AirGap.Enabled,
	}
	if cniConfig.Config.Calico != nil {
		params.Calico = &installer.CalicoParams{
			IPAM: cniConfig.Config.Calico.IPAM,
			MTU:  cniConfig.Config.Calico.MTU,
		}
	}
	inst := installer.NewCNI(sshConn, params)
	return inst.Install(ctx)
}

// installCSI 安装 CSI 驱动
func (r *BareMetalMachineReconciler) installCSI(ctx context.Context, sshConn *ssh.SSHConnection, bmMachine *infrav1.BareMetalMachine, machine *clusterv1.Machine) (*installer.CSIInstallResult, error) {
	csiConfig := bmMachine.Spec.ComponentInstall.CSI
	params := installer.CSIParams{
		Driver:      csiConfig.Driver,
		Version:     csiConfig.Version,
		InstallMode: csiConfig.InstallMode,
		AirGap:      csiConfig.AirGap != nil && csiConfig.AirGap.Enabled,
	}
	if csiConfig.Config.CephCsi != nil {
		params.CephCsi = &installer.CephCsiParams{
			ClusterID: csiConfig.Config.CephCsi.ClusterID,
			Monitors:  csiConfig.Config.CephCsi.Monitors,
			Pool:      csiConfig.Config.CephCsi.RBD.Pool,
		}
	}
	inst := installer.NewCSI(sshConn, params)
	return inst.Install(ctx)
}
```

新增 Conditions:

```go
const (
	CNIInstalledCondition clusterv1.ConditionType = "CNIInstalled"
	CNIInstallFailedReason                       = "CNIInstallFailed"
	CSIInstalledCondition clusterv1.ConditionType = "CSIInstalled"
	CSIInstallFailedReason                       = "CSIInstallFailed"
)
```

#### 6.16.10 用户使用示例

##### 6.16.10.1 标准集群 (Calico CNI + Local-CSI)

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
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
    - name: cni
      value:
        enabled: true
        type: "calico"
        version: "3.26.1"
        config:
          calico:
            ipam: "CalicoIPAM"
            bgp:
              enabled: true
            typha:
              enabled: true
              replicas: 1
    - name: csi
      value:
        enabled: true
        driver: "local-csi"
        config:
          localCsi:
            storageClass:
              name: "local-path"
              reclaimPolicy: "Delete"
              volumeBindingMode: "WaitForFirstConsumer"
```

##### 6.16.10.2 高性能集群 (Cilium CNI + Ceph-CSI)

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
spec:
  topology:
    variables:
    - name: cni
      value:
        enabled: true
        type: "cilium"
        version: "1.15.0"
        installMode: "Helm"
        config:
          cilium:
            kubeProxyReplacement: "strict"
            routingMode: "native"
            ipv4NativeRoutingCIDR: "10.0.0.0/8"
            hubble:
              enabled: true
              relay: true
              ui: true
    - name: csi
      value:
        enabled: true
        driver: "ceph-csi"
        version: "3.9.0"
        installMode: "Helm"
        config:
          cephCsi:
            clusterID: "prod-ceph"
            monitors:
              - "10.0.0.10:6789"
              - "10.0.0.11:6789"
              - "10.0.0.12:6789"
            rbd:
              enabled: true
              pool: "kubernetes-ssd"
            cephfs:
              enabled: true
            storageClass:
              name: "ceph-rbd-ssd"
              reclaimPolicy: "Delete"
              fsType: "ext4"
```

##### 6.16.10.3 离线环境集群

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
spec:
  topology:
    variables:
    - name: cni
      value:
        enabled: true
        type: "calico"
        version: "3.26.1"
        airGap:
          enabled: true
          manifestSource: "HTTPServer"
          httpServerConfig:
            baseUrl: "https://internal-pkg.example.com/cni"
          cniPluginsArchive: "/opt/capbm/cni/cni-plugins-linux-amd64-v1.3.0.tgz"
    - name: csi
      value:
        enabled: true
        driver: "ceph-csi"
        version: "3.9.0"
        airGap:
          enabled: true
          manifestSource: "HTTPServer"
          httpServerConfig:
            baseUrl: "https://internal-pkg.example.com/csi"
          chartArchive: "/opt/capbm/csi/ceph-csi-3.9.0.tgz"
```

##### 6.16.10.4 CNI 升级示例

```yaml
# 升级前
- name: cni
  value:
    type: "calico"
    version: "3.26.1"

# 升级后 (修改 version 即可，自动触发滚动升级)
- name: cni
  value:
    type: "calico"
    version: "3.27.0"
```

#### 6.16.11 CNI/CSI 安装与完整安装流程集成

更新后的完整安装流程（包含 CNI/CSI）:

```
SSH 连接成功
    │
    ▼
┌─────────────────────────────────────────┐
│ 1. 环境准备                              │
│    ├── 检测 OS 类型和版本                │
│    ├── 检测包管理器                      │
│    ├── 检测防火墙工具                    │
│    └── 检测 SELinux 状态                │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 2. 网络预检                              │
│    ├── 端口可用性检查                    │
│    ├── 网络连通性检查                    │
│    ├── MTU 检查                         │
│    └── 桥接流量检查                      │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 3. 前置配置                              │
│    ├── 配置防火墙规则                    │
│    ├── 配置 SELinux 策略                │
│    ├── 禁用 swap                        │
│    └── 配置内核参数                      │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 4. 组件安装                              │
│    ├── 安装容器运行时 (containerd 等)    │
│    └── 安装 Kubernetes 组件              │
│        ├── kubeadm                      │
│        ├── kubelet                      │
│        └── kubectl                      │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 5. kubeadm init/join                     │
│    └── 由 Kubeadm Bootstrap Provider 执行│
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 6. CNI 安装 (仅首个 control-plane)       │
│    ├── 安装 CNI 二进制插件               │
│    ├── 部署 CNI 插件 (Calico/Cilium 等)  │
│    └── 验证网络连通性                    │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 7. CSI 安装 (可选，仅首个 control-plane)  │
│    ├── 部署 CSI Controller              │
│    ├── 部署 CSI Node DaemonSet          │
│    ├── 创建 StorageClass                │
│    └── 验证存储供给能力                  │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 8. 安装验证                              │
│    ├── 组件版本验证                      │
│    ├── CNI/CSI 状态检查                  │
│    └── Node Ready 确认                   │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 9. 清理                                  │
│    ├── 删除临时文件                      │
│    └── 记录安装结果                      │
└─────────────────────────────────────────┘
```


## 七、SSH 连接管理 (保持不变)

### 7.1 SSH Manager 设计

```go
type SSHManager struct {
    connections map[string]*SSHConnection
    mu          sync.RWMutex
}

type SSHConnection struct {
    Client    *ssh.Client
    Host      string
    Port      int
    LastUsed  time.Time
}

func (m *SSHManager) Connect(host string, port int, creds Credentials) (*SSHConnection, error) {
    key := fmt.Sprintf("%s:%d", host, port)
    
    m.mu.RLock()
    if conn, exists := m.connections[key]; exists {
        if time.Since(conn.LastUsed) < 5*time.Minute {
            if _, _, err := conn.Client.Conn.SendRequest("keepalive@google.com", true, nil); err == nil {
                conn.LastUsed = time.Now()
                m.mu.RUnlock()
                return conn, nil
            }
        }
    }
    m.mu.RUnlock()

    config := &ssh.ClientConfig{
        User: creds.Username,
        Auth: []ssh.AuthMethod{
            ssh.Password(creds.Password),
        },
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
        Timeout:         10 * time.Second,
    }

    client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to %s:%d: %w", host, port, err)
    }

    conn := &SSHConnection{
        Client:   client,
        Host:     host,
        Port:     port,
        LastUsed: time.Now(),
    }

    m.mu.Lock()
    m.connections[key] = conn
    m.mu.Unlock()

    return conn, nil
}
```

### 7.2 预检脚本

```bash
#!/bin/bash
set -euo pipefail

echo "=== 预检开始 ==="

# 1. OS 版本检查
if [ -f /etc/os-release ]; then
    . /etc/os-release
    echo "OS: $NAME $VERSION_ID"
    case "$ID" in
        centos|rhel|almalinux|rocky|ubuntu|debian)
            echo "Supported OS detected"
            ;;
        *)
            echo "Unsupported OS: $ID"
            exit 1
            ;;
    esac
else
    echo "Cannot detect OS"
    exit 1
fi

# 2. 内核版本检查
KERNEL_VERSION=$(uname -r | cut -d'-' -f1)
REQUIRED_VERSION="3.10"
if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$KERNEL_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
    echo "Kernel version $KERNEL_VERSION is too old, need >= $REQUIRED_VERSION"
    exit 1
fi

# 3. 磁盘空间检查 (至少 20GB 可用)
AVAILABLE_GB=$(df -BG / | awk 'NR==2 {print $4}' | tr -d 'G')
if [ "$AVAILABLE_GB" -lt 20 ]; then
    echo "Insufficient disk space: ${AVAILABLE_GB}GB available, need 20GB"
    exit 1
fi

# 4. 内存检查 (至少 2GB)
TOTAL_MEM_GB=$(free -g | awk '/^Mem:/{print $2}')
if [ "$TOTAL_MEM_GB" -lt 2 ]; then
    echo "Insufficient memory: ${TOTAL_MEM_GB}GB, need 2GB"
    exit 1
fi

# 5. 网络连通性检查
if ! ping -c 1 -W 2 8.8.8.8 &>/dev/null; then
    echo "WARNING: Cannot reach external network"
fi

# 6. Swap 检查
if swapon --show | grep -q .; then
    echo "WARNING: Swap is enabled, should be disabled for K8s"
fi

echo "=== 预检完成 ==="
```

## 八、关键设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **机器信息管理** | 每 Machine 独立配置 vs 统一机器池 | 统一机器池 (BareMetalHostInventory) | 集中管理，支持多集群共享，自动分配 |
| **ClusterClass 命名** | 包含版本 vs 不包含 | 包含版本 (v0.1.0) | 支持平滑升级和回滚 |
| **Template 命名** | 独立命名 vs 前缀统一 | 前缀统一 (clusterclass-name-*) | 避免冲突，便于管理 |
| **变量作用域** | 全局 vs 局部覆盖 | 两者结合 | 全局默认，局部覆盖提供灵活性 |
| **Machine 命名** | 自动生成 vs 用户指定 | 自动生成 + Custom Naming | 符合 RFC 1123，用户可控 |
| **预检配置** | 硬编码 vs 变量化 | 变量化 | 不同场景可自定义预检阈值 |
| **凭据管理** | 每机器独立 vs 集群共享 | 每机器独立 + 集群共享可选 | 安全性更高，支持不同机器不同凭据 |

## 九、项目结构

```
cluster-api-provider-baremetal/
├── api/
│   └── v1beta1/
│       ├── baremetalcluster_types.go
│       ├── baremetalclustertemplate_types.go    # 新增
│       ├── baremetalmachine_types.go
│       ├── baremetalmachinetemplate_types.go
│       ├── baremetalhostinventory_types.go      # 新增核心 CRD
│       ├── groupversion_info.go
│       ├── conditions.go
│       └── zz_generated.deepcopy.go
├── cmd/
│   └── main.go
├── internal/
│   ├── controllers/
│   │   ├── baremetalcluster_controller.go
│   │   ├── baremetalmachine_controller.go       # 优化：增加机器分配逻辑
│   │   ├── baremetalhostinventory_controller.go # 新增
│   │   └── suite_test.go
│   ├── ssh/
│   │   ├── manager.go
│   │   ├── client.go
│   │   └── preflight.go
│   ├── installer/                                # 新增：组件安装模块
│   │   ├── installer.go                         # 安装入口和调度
│   │   ├── detector.go                          # OS/包管理器检测
│   │   ├── progress.go                          # 进度追踪
│   │   ├── rollback.go                          # 回滚管理
│   │   ├── scripts/
│   │   │   ├── install_containerd_ubuntu.sh     # containerd Ubuntu
│   │   │   ├── install_containerd_rhel.sh       # containerd RHEL
│   │   │   ├── install_k8s_ubuntu.sh            # K8s Ubuntu
│   │   │   ├── install_k8s_rhel.sh              # K8s RHEL
│   │   │   ├── install_crio.sh                  # CRI-O 安装
│   │   │   ├── install_docker.sh                # Docker + cri-dockerd
│   │   │   ├── install_offline.sh               # 离线安装
│   │   │   ├── install_flatcar.sh               # Flatcar 特殊处理
│   │   │   └── install_suse.sh                  # SUSE 安装
│   │   └── templates/
│   │       ├── containerd.service.tmpl          # systemd 模板
│   │       └── kubelet.service.tmpl
│   ├── cni/                                      # 新增：CNI 安装模块
│   │   ├── cni.go                               # CNI 安装入口
│   │   ├── calico.go                            # Calico 安装逻辑
│   │   ├── cilium.go                            # Cilium 安装逻辑
│   │   ├── flannel.go                           # Flannel 安装逻辑
│   │   ├── verify.go                            # CNI 状态验证
│   │   └── scripts/
│   │       ├── install_calico.sh                # Calico 在线安装
│   │       ├── install_cilium.sh                # Cilium 在线安装
│   │       ├── install_flannel.sh               # Flannel 在线安装
│   │       ├── install_cni_offline.sh           # CNI 离线安装
│   │       └── upgrade_cni.sh                   # CNI 升级/回滚
│   ├── csi/                                      # 新增：CSI 安装模块
│   │   ├── csi.go                               # CSI 安装入口
│   │   ├── ceph_csi.go                          # Ceph-CSI 安装逻辑
│   │   ├── cinder_csi.go                        # Cinder-CSI 安装逻辑
│   │   ├── local_csi.go                         # Local-CSI 安装逻辑
│   │   ├── nfs_csi.go                           # NFS-CSI 安装逻辑
│   │   ├── verify.go                            # CSI 状态验证
│   │   └── scripts/
│   │       ├── install_ceph_csi.sh              # Ceph-CSI 在线安装
│   │       ├── install_local_csi.sh             # Local-CSI 在线安装
│   │       ├── install_csi_offline.sh           # CSI 离线安装
│   │       └── upgrade_csi.sh                   # CSI 升级/回滚
│   ├── network/                                  # 新增：网络配置模块
│   │   ├── firewall.go                          # 防火墙配置
│   │   ├── selinux.go                           # SELinux 配置
│   │   ├── sysctl.go                            # 内核参数
│   │   └── scripts/
│   │       ├── configure_firewall.sh
│   │       ├── configure_selinux.sh
│   │       └── network_preflight.sh
│   └── health/
│       └── verify_installation.go               # 安装验证
├── config/
│   ├── crd/
│   │   ├── bases/
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalclusters.yaml
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalclustertemplates.yaml
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalmachines.yaml
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalmachinetemplates.yaml
│   │   │   └── infrastructure.cluster.x-k8s.io_baremetalhostinventories.yaml  # 新增
│   │   └── kustomization.yaml
│   ├── clusterclass/
│   │   ├── baremetal-clusterclass.yaml
│   │   ├── baremetal-cluster-template.yaml
│   │   ├── baremetal-machine-template-cp.yaml
│   │   ├── baremetal-machine-template-worker.yaml
│   │   ├── kubeadm-controlplane-template.yaml
│   │   └── kubeadm-config-template.yaml
│   ├── rbac/
│   └── manager/
├── templates/
│   └── clusterclass/
│       └── baremetal-clusterclass-v0.1.0.yaml
├── hack/
│   ├── prepare-offline-packages.sh              # 离线包准备脚本
│   ├── preload-images.sh                        # 镜像预加载脚本
│   ├── prepare-cni-offline.sh                   # CNI 离线包准备
│   └── prepare-csi-offline.sh                   # CSI 离线包准备
├── go.mod
└── go.sum
```

## 十、部署与使用流程

### 9.1 安装 Provider

```bash
# 安装 CAPI 核心组件
clusterctl init --core cluster-api --bootstrap kubeadm --control-plane kubeadm

# 安装 CAPBM Provider
clusterctl init --infrastructure baremetal
```

### 9.2 部署 ClusterClass

```bash
# 应用 ClusterClass 及相关模板
kubectl apply -f config/clusterclass/
```

### 9.3 创建机器池

```bash
# 创建 BareMetalHostInventory
kubectl apply -f baremetalhostinventory.yaml
```

### 9.4 创建集群

```bash
# 1. 创建凭据 Secret
kubectl apply -f credentials.yaml

# 2. 创建 Cluster (引用 ClusterClass)
kubectl apply -f cluster-topology.yaml

# 3. 查看集群状态
clusterctl describe cluster my-baremetal-cluster

# 4. 获取 kubeconfig
clusterctl get kubeconfig my-baremetal-cluster > workload-kubeconfig
```

### 9.5 扩缩容

```bash
# 扩容 Worker 节点到 5 个
kubectl patch cluster my-baremetal-cluster --type='merge' -p '{
  "spec": {
    "topology": {
      "workers": {
        "machineDeployments": [
          {
            "class": "default-worker",
            "name": "md-0",
            "replicas": 5
          }
        ]
      }
    }
  }
}'

# 扩缩容由 ClusterTopology Controller 自动处理
# BareMetalMachine Controller 会自动从机器池分配/释放机器
```

### 9.6 升级集群

```bash
# 升级 Kubernetes 版本
kubectl patch cluster my-baremetal-cluster --type='merge' -p '{
  "spec": {
    "topology": {
      "version": "v1.32.0"
    }
  }
}'

# 升级流程:
# 1. Control Plane 逐个升级 (先 etcd, 再 API Server)
# 2. Worker 节点逐个滚动升级
# 3. 每个节点升级前执行预检
```

## 十一、开发路线图

| 阶段 | 内容 | 工作量 |
|------|------|--------|
| **Phase 1** | CRD 定义 (含 Template 和 HostInventory 类型) + 基础 Controller 框架 | 1.5 周 |
| **Phase 2** | ClusterClass YAML 模板 + Patches 定义 | 1 周 |
| **Phase 3** | BareMetalHostInventory Controller + 机器分配逻辑 | 1.5 周 |
| **Phase 4** | SSH 连接管理 + 凭据处理 | 1 周 |
| **Phase 5** | 预检逻辑 + ProviderID 生成 | 1 周 |
| **Phase 6** | 状态上报 + Conditions 管理 | 1 周 |
| **Phase 7** | 删除逻辑 + 机器释放 | 1 周 |
| **Phase 8** | ClusterClass 集成测试 + 变量覆盖测试 | 1 周 |
| **Phase 9** | 升级/扩缩容 E2E 测试 + 文档 | 2 周 |
| **Phase 10** | 组件安装模块 (在线模式: Ubuntu/CentOS + containerd) | 1.5 周 |
| **Phase 11** | 组件安装模块 (离线模式 + 多 OS 支持) | 1.5 周 |
| **Phase 12** | 组件安装模块 (CRI-O/Docker + 回滚机制) | 1 周 |
| **Phase 13** | 网络配置模块 (防火墙/SELinux/内核参数) | 1 周 |
| **Phase 14** | 组件安装 E2E 测试 + 健壮性优化 | 1.5 周 |
| **总计** | | **15.5 周** |

## 十二、优势总结

### 11.1 相比原方案的核心优势

1. **声明式拓扑管理**
   - 用户只需定义 Cluster 拓扑，无需手动管理每个 Machine 资源
   - ClusterTopology Controller 自动处理资源创建和生命周期

2. **机器池集中管理**
   - BareMetalHostInventory 统一管理所有裸金属机器
   - 支持多集群共享机器池
   - 自动分配和释放机器

3. **配置复用与标准化**
   - ClusterClass 定义一次，可创建多个集群实例
   - 通过 Variables 实现集群间差异化配置

4. **灵活的变量覆盖**
   - 支持全局变量默认值
   - 支持 MachineDeployment 级别变量覆盖
   - 支持复杂类型 (object, array, map)

5. **自动化运维能力**
   - 内置健康检查 (MachineHealthCheck)
   - 自动升级编排 (Control Plane → Workers)
   - 滚动更新策略

6. **可扩展性**
   - 支持多 Worker 池 (不同配置/规格)
   - 支持 Runtime SDK 扩展 (未来)
   - 支持 Patches 条件启用 (enabledIf)

7. **完整的组件安装能力**
   - 自动检测 OS 类型并选择对应安装方式
   - 支持在线和离线 (air-gap) 两种安装模式
   - 支持多种容器运行时 (containerd/CRI-O/Docker)
   - 幂等性保证，支持安全重试
   - 失败自动回滚，避免机器处于不一致状态
   - 安装进度实时追踪

8. **企业级网络和安全支持**
   - 自动配置防火墙规则 (firewalld/ufw/iptables)
   - SELinux 策略自动配置
   - 内核参数自动优化
   - 网络预检和端口检查

### 11.2 适用场景

- **标准化裸金属集群部署**：多套相同配置的集群
- **混合规格集群**：不同 Worker 池使用不同硬件配置
- **需要频繁升级的环境**：利用 ClusterClass 升级能力
- **多租户场景**：通过 ClusterClass 隔离不同租户的集群模板
- **共享基础设施**：多个集群共享同一批裸金属机器
- **隔离网络环境**：支持 air-gap 离线安装，适用于内网/安全隔离环境
- **混合 OS 环境**：同时管理 Ubuntu/CentOS/SUSE/Flatcar 等多种 OS
- **合规要求严格的环境**：自动配置 SELinux、防火墙等安全策略
