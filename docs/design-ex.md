# CAPBM (Cluster API Provider Bare Metal) - ClusterClass 方案设计

## 一、架构总览
```
用户输入 (机器列表 + 拓扑配置)
    │
    ├── node-01, 192.168.1.101, root, password123, control-plane
    ├── node-02, 192.168.1.102, root, password123, control-plane
    └── node-03, 192.168.1.103, root, password123, worker
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
│  │  │ - 管理集群级别基础设施状态                      │  │  │
│  │  └─────────────────────────────────────────────────┘  │  │
│  │  ┌─────────────────────────────────────────────────┐  │  │
│  │  │ BareMetalMachine Controller                     │  │  │
│  │  │ - SSH 连接管理                                  │  │  │
│  │  │ - 机器预检 (OS/网络/内核)                       │  │  │
│  │  │ - ProviderID 生成与状态上报                     │  │  │
│  │  └─────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  Kubeadm Bootstrap Provider (内置)                    │  │
│  │  - 生成 kubeadm init/join 配置                        │  │
│  │  - 执行 cloud-init/Ignition 脚本                      │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  ClusterClass 定义                                    │  │
│  │  - BareMetalClusterClass                              │  │
│  │  - BareMetalMachineTemplate (CP/Worker)               │  │
│  │  - KubeadmControlPlaneTemplate                        │  │
│  │  - KubeadmConfigTemplate                              │  │
│  │  - Variables & Patches                                │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
         │
         ▼ (SSH + cloud-init)
┌────────────────────────────────────────────────────────────┐
│                    Workload Nodes (裸金属)                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │ node-01  │  │ node-02  │  │ node-03  │                  │
│  │ OS已安装 │  │ OS已安装 │  │ OS已安装 │                  │
│  │ SSH可达  │  │ SSH可达  │  │ SSH 可达 │                  │
│  └──────────┘  └──────────┘  └──────────┘                  │
└────────────────────────────────────────────────────────────┘
```

## 二、核心概念

### 2.1 什么是 ClusterClass
ClusterClass 是 Cluster API 的高级抽象，通过声明式模板定义集群拓扑结构，实现：
- **减少样板代码**：无需手动创建每个 Machine/Template 资源
- **灵活定制**：通过 Variables 和 Patches 实现集群间差异化配置
- **生命周期管理**：支持升级、扩缩容、健康检查等自动化操作

### 2.2 与原方案的对比
| 维度 | 原方案 (手动资源) | ClusterClass 方案 |
|------|------------------|-------------------|
| 资源数量 | 每个节点需独立 BareMetalMachine | 通过 Template + replicas 自动管理 |
| 配置复用 | 需手动复制 Template | ClusterClass 定义一次，多处使用 |
| 差异化配置 | 需修改多个资源 | 通过 Variables 和 Overrides 实现 |
| 升级管理 | 手动逐个升级 | 拓扑控制器自动编排升级流程 |
| 扩展性 | 添加节点需创建新资源 | 修改 replicas 即可 |

## 三、CRD 设计

### 3.1 BareMetalCluster (保持不变)
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
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
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
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
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
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

### 3.4 ClusterClass 定义
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
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
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
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
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
          apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
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
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
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
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
        kind: BareMetalMachineTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/credentialsRef/name
        valueFrom:
          variable: credentialsSecret
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
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

### 3.5 KubeadmControlPlaneTemplate
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
        initConfiguration:
          nodeRegistration:
            kubeletExtraArgs:
              max-pods: "250"
```

### 3.6 KubeadmConfigTemplate (Worker)
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

### 5.2 BareMetalMachine Controller (保持不变)
**职责**:
- SSH 连接到目标机器
- 执行预检 (OS 版本、网络、内核参数)
- 生成并设置 ProviderID
- 更新状态和 Conditions

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

## 六、SSH 连接管理 (保持不变)

### 6.1 SSH Manager 设计
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
            if _, _, err := conn.Client.Conn.SendRequest("keepalive", true, nil); err == nil {
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

### 6.2 预检脚本
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

## 七、关键设计决策
| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **ClusterClass 命名** | 包含版本 vs 不包含 | 包含版本 (v0.1.0) | 支持平滑升级和回滚 |
| **Template 命名** | 独立命名 vs 前缀统一 | 前缀统一 (clusterclass-name-*) | 避免冲突，便于管理 |
| **变量作用域** | 全局 vs 局部覆盖 | 两者结合 | 全局默认，局部覆盖提供灵活性 |
| **Machine 命名** | 自动生成 vs 用户指定 | 自动生成 + Custom Naming | 符合 RFC 1123，用户可控 |
| **预检配置** | 硬编码 vs 变量化 | 变量化 | 不同场景可自定义预检阈值 |
| **凭据管理** | 每机器独立 vs 集群共享 | 集群共享 + 可选覆盖 | 简化配置，支持特殊场景 |

## 八、项目结构
```
cluster-api-provider-baremetal/
├── api/
│   └── v1beta2/
│       ├── baremetalcluster_types.go
│       ├── baremetalclustertemplate_types.go    # 新增
│       ├── baremetalmachine_types.go
│       ├── baremetalmachinetemplate_types.go
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
├── cmd/
│   └── main.go
├── internal/
│   ├── controllers/
│   │   ├── baremetalcluster_controller.go
│   │   ├── baremetalmachine_controller.go
│   │   └── suite_test.go
│   └── ssh/
│       ├── manager.go
│       ├── client.go
│       └── preflight.go
├── config/
│   ├── crd/
│   │   ├── bases/
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalclusters.yaml
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalclustertemplates.yaml  # 新增
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalmachines.yaml
│   │   │   └── infrastructure.cluster.x-k8s.io_baremetalmachinetemplates.yaml
│   │   └── kustomization.yaml
│   ├── clusterclass/
│   │   ├── baremetal-clusterclass.yaml           # 新增: ClusterClass 定义
│   │   ├── baremetal-cluster-template.yaml       # 新增
│   │   ├── baremetal-machine-template-cp.yaml    # 新增
│   │   ├── baremetal-machine-template-worker.yaml # 新增
│   │   ├── kubeadm-controlplane-template.yaml    # 新增
│   │   └── kubeadm-config-template.yaml          # 新增
│   ├── rbac/
│   └── manager/
├── templates/
│   └── clusterclass/
│       └── baremetal-clusterclass-v0.1.0.yaml    # 新增: clusterctl 可用模板
├── go.mod
└── go.sum
```

## 九、部署与使用流程

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

### 9.3 创建集群
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

### 9.4 扩缩容
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
```

### 9.5 升级集群
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

## 十、开发路线图
| 阶段 | 内容 | 工作量 |
|------|------|--------|
| **Phase 1** | CRD 定义 (含 Template 类型) + 基础 Controller 框架 | 1 周 |
| **Phase 2** | ClusterClass YAML 模板 + Patches 定义 | 1 周 |
| **Phase 3** | SSH 连接管理 + 凭据处理 | 1 周 |
| **Phase 4** | 预检逻辑 + ProviderID 生成 | 1 周 |
| **Phase 5** | 状态上报 + Conditions 管理 | 1 周 |
| **Phase 6** | 删除逻辑 + 清理 | 1 周 |
| **Phase 7** | ClusterClass 集成测试 + 变量覆盖测试 | 1 周 |
| **Phase 8** | 升级/扩缩容 E2E 测试 + 文档 | 2 周 |
| **总计** | | **9 周** |

## 十一、优势总结

### 11.1 相比原方案的核心优势
1. **声明式拓扑管理**
   - 用户只需定义 Cluster 拓扑，无需手动管理每个 Machine 资源
   - ClusterTopology Controller 自动处理资源创建和生命周期

2. **配置复用与标准化**
   - ClusterClass 定义一次，可创建多个集群实例
   - 通过 Variables 实现集群间差异化配置

3. **灵活的变量覆盖**
   - 支持全局变量默认值
   - 支持 MachineDeployment 级别变量覆盖
   - 支持复杂类型 (object, array, map)

4. **自动化运维能力**
   - 内置健康检查 (MachineHealthCheck)
   - 自动升级编排 (Control Plane → Workers)
   - 滚动更新策略

5. **可扩展性**
   - 支持多 Worker 池 (不同配置/规格)
   - 支持 Runtime SDK 扩展 (未来)
   - 支持 Patches 条件启用 (enabledIf)

### 11.2 适用场景
- **标准化裸金属集群部署**：多套相同配置的集群
- **混合规格集群**：不同 Worker 池使用不同硬件配置
- **需要频繁升级的环境**：利用 ClusterClass 升级能力
- **多租户场景**：通过 ClusterClass 隔离不同租户的集群模板


## 代码目录结构规划
```
cluster-api-provider-baremetal/
├── api/
│   └── v1beta2/
│       ├── baremetalcluster_types.go          # BareMetalCluster CRD 类型
│       ├── baremetalclustertemplate_types.go  # BareMetalClusterTemplate CRD 类型 (新增)
│       ├── baremetalmachine_types.go          # BareMetalMachine CRD 类型
│       ├── baremetalmachinetemplate_types.go  # BareMetalMachineTemplate CRD 类型
│       ├── groupversion_info.go               # API 组版本信息
│       ├── conditions.go                      # Condition 类型定义
│       └── zz_generated.deepcopy.go           # 自动生成的 DeepCopy 方法
│
├── cmd/
│   └── main.go                                # 入口文件，启动 manager
│
├── internal/
│   ├── controllers/
│   │   ├── baremetalcluster_controller.go     # BareMetalCluster 控制器
│   │   ├── baremetalmachine_controller.go     # BareMetalMachine 控制器
│   │   └── suite_test.go                     # 控制器单元测试
│   │
│   └── ssh/
│       ├── manager.go                         # SSH 连接池管理
│       ├── client.go                          # SSH 客户端封装
│       └── preflight.go                       # 预检逻辑实现
│
├── config/
│   ├── crd/
│   │   ├── bases/
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalclusters.yaml
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalclustertemplates.yaml
│   │   │   ├── infrastructure.cluster.x-k8s.io_baremetalmachines.yaml
│   │   │   └── infrastructure.cluster.x-k8s.io_baremetalmachinetemplates.yaml
│   │   ├── kustomization.yaml
│   │   └── kustomizeconfig.yaml
│   │
│   ├── clusterclass/                          # ClusterClass 相关配置 (新增)
│   │   ├── baremetal-clusterclass.yaml        # ClusterClass 主定义
│   │   ├── baremetal-cluster-template.yaml    # BareMetalClusterTemplate
│   │   ├── baremetal-machine-template-cp.yaml # ControlPlane MachineTemplate
│   │   ├── baremetal-machine-template-worker.yaml # Worker MachineTemplate
│   │   ├── kubeadm-controlplane-template.yaml # KubeadmControlPlaneTemplate
│   │   ├── kubeadm-config-template.yaml       # KubeadmConfigTemplate
│   │   └── kustomization.yaml
│   │
│   ├── rbac/
│   │   ├── role.yaml                          # ClusterRole 定义
│   │   ├── role_binding.yaml                  # ClusterRoleBinding
│   │   ├── service_account.yaml               # ServiceAccount
│   │   └── kustomization.yaml
│   │
│   ├── manager/
│   │   ├── manager.yaml                       # Deployment 定义
│   │   └── kustomization.yaml
│   │
│   └── default/
│       ├── kustomization.yaml                 # 默认 kustomize 入口
│       └── manager_auth_proxy_patch.yaml
│
├── templates/
│   └── clusterclass/
│       └── baremetal-clusterclass-v0.1.0.yaml # clusterctl 可用的完整模板
│
├── test/
│   ├── e2e/
│   │   ├── clusterclass_test.go              # ClusterClass E2E 测试
│   │   └── helpers.go
│   └── utils/
│       └── ssh_mock.go
│
├── hack/
│   ├── boilerplate.go.txt                     # Go 文件头部模板
│   └── verify-shellcheck.sh                   # 脚本检查
│
├── Makefile                                   # 构建/测试/生成命令
├── Dockerfile                                 # Controller 镜像
├── PROJECT                                    # kubebuilder 项目配置
├── go.mod                                     # Go 模块定义
├── go.sum                                     # 依赖校验
└── README.md                                  # 项目说明
```

### 各目录/文件说明
| 目录/文件 | 说明 |
|-----------|------|
| `api/v1beta2/` | CRD Go 类型定义，包含 Spec/Status 结构体和 DeepCopy 方法 |
| `cmd/main.go` | 程序入口，初始化 manager、注册控制器和 Webhook |
| `internal/controllers/` | 控制器实现，包含 BareMetalCluster 和 BareMetalMachine 的 Reconcile 逻辑 |
| `internal/ssh/` | SSH 连接池、客户端封装和预检脚本执行逻辑 |
| `config/crd/` | CRD YAML 定义，由 controller-gen 自动生成 |
| `config/clusterclass/` | ClusterClass 相关模板定义（核心新增部分） |
| `config/rbac/` | RBAC 权限配置 |
| `config/manager/` | Controller Deployment 配置 |
| `templates/clusterclass/` | 供 `clusterctl generate cluster` 使用的完整模板 |
| `test/` | 单元测试和 E2E 测试 |
| `Makefile` | 构建、测试、生成 CRD/DeepCopy/manifests 的快捷命令 |

### 关键新增内容 (相比原 design.md)
1. **`api/v1beta2/baremetalclustertemplate_types.go`** - 新增 ClusterClass 所需的 Cluster Template 类型
2. **`config/clusterclass/`** - 整个目录为新增，包含 6 个 ClusterClass 相关 YAML 文件
3. **`templates/clusterclass/`** - 新增 clusterctl 可识别的模板目录
4. **`test/e2e/clusterclass_test.go`** - 新增 ClusterClass 专项 E2E 测试
