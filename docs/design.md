# CAPBM (Cluster API Provider Bare Metal) 详细设计

## 一、架构总览
```
用户输入 (机器列表)
    │
    ├── node-01, 192.168.1.101, root, password123
    ├── node-02, 192.168.1.102, root, password123
    └── node-03, 192.168.1.103, root, password123
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Management Cluster                       │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  CAPBM Provider (自研)                                │  │
│  │  ┌─────────────────────────────────────────────────┐  │  │
│  │  │ BareMetalCluster Controller                     │  │  │
│  │  │ - 管理集群级别基础设施状态                        │  │  │
│  │  └─────────────────────────────────────────────────┘  │  │
│  │  ┌─────────────────────────────────────────────────┐  │  │
│  │  │ BareMetalMachine Controller                     │  │  │
│  │  │ - SSH 连接管理                                  │  │  │
│  │  │ - 机器预检 (OS/网络/内核)                        │  │  │
│  │  │ - ProviderID 生成与状态上报                      │  │  │
│  │  └─────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  CAPI Core (内置)                                     │  │
│  │  Machine Controller ──→ 关联 Machine 与 Node          │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  Kubeadm Bootstrap Provider (内置)                    │  │
│  │  - 生成 kubeadm init/join 配置                        │  │
│  │  - 执行 cloud-init/Ignition 脚本                      │  │
│  └───────────────────────────────────────────────────────┘ │
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

## 二、CRD 设计

### 2.1 BareMetalCluster
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalCluster
metadata:
  name: my-cluster
  namespace: default
spec:
  # 控制面端点 (用户提供的外部 LB 地址)
  controlPlaneEndpoint:
    host: "lb.example.com"
    port: 6443
  
  # 可选：集群级别的网络配置
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

**Go 类型定义**:
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

### 2.2 BareMetalMachine
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalMachine
metadata:
  name: node-01
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
    role: control-plane
spec:
  # 机器标识
  hostName: "node-01"
  ipAddress: "192.168.1.101"
  sshPort: 22
  
  # 凭据引用
  credentialsRef:
    name: node-01-credentials
    namespace: default
  
  # 可选：电源管理 (IPMI/Redfish)
  powerManagement:
    type: "ipmi"
    address: "192.168.1.101:623"
    credentialsRef:
      name: node-01-bmc-credentials
  
  # 可选：机器角色标签
  role: "control-plane"
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
      message: "Machine is connected via SSH"
    - type: PreFlightChecksPassed
      status: "True"
      reason: ChecksPassed
      message: "All pre-flight checks passed"
```

**Go 类型定义**:
```go
type BareMetalMachineSpec struct {
    HostName        string                    `json:"hostName"`
    IPAddress       string                    `json:"ipAddress"`
    SSHPort         int                       `json:"sshPort,omitempty"`
    CredentialsRef  corev1.LocalObjectReference `json:"credentialsRef"`
    PowerManagement *PowerManagementConfig    `json:"powerManagement,omitempty"`
    Role            string                    `json:"role,omitempty"`
}

type PowerManagementConfig struct {
    Type           string                    `json:"type"`
    Address        string                    `json:"address"`
    CredentialsRef corev1.LocalObjectReference `json:"credentialsRef"`
}

type BareMetalMachineStatus struct {
    Ready      bool               `json:"ready,omitempty"`
    ProviderID string             `json:"providerID,omitempty"`
    Addresses  []clusterv1.MachineAddress `json:"addresses,omitempty"`
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

### 2.3 BareMetalMachineTemplate
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalMachineTemplate
metadata:
  name: my-cluster-cp-template
  namespace: default
spec:
  template:
    spec:
      sshPort: 22
      credentialsRef:
        name: default-credentials
      role: "control-plane"
```

## 三、控制器设计

### 3.1 BareMetalCluster Controller
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

**核心代码结构**:
```go
func (r *BareMetalClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    cluster := &infrav1.BareMetalCluster{}
    if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 处理删除
    if !cluster.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, cluster)
    }

    // 正常调和
    return r.reconcileNormal(ctx, cluster)
}

func (r *BareMetalClusterReconciler) reconcileNormal(ctx context.Context, cluster *infrav1.BareMetalCluster) (ctrl.Result, error) {
    // 验证配置
    if cluster.Spec.ControlPlaneEndpoint.Host == "" {
        conditions.MarkFalse(cluster, infrav1.ReadyCondition, infrav1.EndpointNotSetReason, clusterv1.ConditionSeverityError, "controlPlaneEndpoint is required")
        return ctrl.Result{}, nil
    }

    // 设置就绪状态
    cluster.Status.Ready = true
    conditions.MarkTrue(cluster, infrav1.ReadyCondition)

    return ctrl.Result{}, r.Status().Update(ctx, cluster)
}
```

### 3.2 BareMetalMachine Controller
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

**核心代码结构**:
```go
func (r *BareMetalMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    bmMachine := &infrav1.BareMetalMachine{}
    if err := r.Get(ctx, req.NamespacedName, bmMachine); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 获取关联的 Machine
    machine, err := util.GetOwnerMachine(ctx, r.Client, bmMachine.ObjectMeta)
    if err != nil {
        return ctrl.Result{}, err
    }
    if machine == nil {
        log.Info("Waiting for Machine Controller to set OwnerRef")
        return ctrl.Result{}, nil
    }

    // 处理删除
    if !bmMachine.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, bmMachine)
    }

    // 正常调和
    return r.reconcileNormal(ctx, bmMachine, machine)
}

func (r *BareMetalMachineReconciler) reconcileNormal(ctx context.Context, bmMachine *infrav1.BareMetalMachine, machine *clusterv1.Machine) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // 1. 获取凭据
    creds, err := r.getCredentials(ctx, bmMachine.Spec.CredentialsRef, bmMachine.Namespace)
    if err != nil {
        conditions.MarkFalse(bmMachine, infrav1.ReadyCondition, infrav1.CredentialsNotFoundReason, clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, bmMachine)
    }

    // 2. 建立 SSH 连接
    sshClient, err := r.sshManager.Connect(bmMachine.Spec.IPAddress, bmMachine.Spec.SSHPort, creds)
    if err != nil {
        conditions.MarkFalse(bmMachine, infrav1.ReadyCondition, infrav1.SSHConnectionFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
        return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, bmMachine)
    }
    defer sshClient.Close()

    // 3. 执行预检
    if err := r.runPreFlightChecks(ctx, sshClient, bmMachine); err != nil {
        conditions.MarkFalse(bmMachine, infrav1.PreFlightChecksPassedCondition, infrav1.PreFlightChecksFailedReason, clusterv1.ConditionSeverityError, err.Error())
        return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
    }
    conditions.MarkTrue(bmMachine, infrav1.PreFlightChecksPassedCondition)

    // 4. 设置 ProviderID
    providerID := fmt.Sprintf("baremetal://%s", bmMachine.Spec.HostName)
    if bmMachine.Spec.ProviderID == nil || *bmMachine.Spec.ProviderID != providerID {
        bmMachine.Spec.ProviderID = ptr.To(providerID)
        if err := r.Update(ctx, bmMachine); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 5. 更新状态
    bmMachine.Status.Ready = true
    bmMachine.Status.ProviderID = providerID
    bmMachine.Status.Addresses = []clusterv1.MachineAddress{
        {Type: clusterv1.MachineInternalIP, Address: bmMachine.Spec.IPAddress},
        {Type: clusterv1.MachineHostName, Address: bmMachine.Spec.HostName},
    }
    conditions.MarkTrue(bmMachine, infrav1.ReadyCondition)

    return ctrl.Result{}, r.Status().Update(ctx, bmMachine)
}
```

## 四、SSH 连接管理

### 4.1 SSH Manager 设计
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
            // 检查连接是否存活
            if _, _, err := conn.Client.Conn.SendRequest("keepalive", true, nil); err == nil {
                conn.LastUsed = time.Now()
                m.mu.RUnlock()
                return conn, nil
            }
        }
    }
    m.mu.RUnlock()

    // 创建新连接
    config := &ssh.ClientConfig{
        User: creds.Username,
        Auth: []ssh.AuthMethod{
            ssh.Password(creds.Password),
        },
        HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 生产环境应使用固定 HostKey
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

### 4.2 预检脚本
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

## 五、部署与使用流程

### 5.1 用户输入转化
用户提供机器列表：
```
node-01, 192.168.1.101, root, password123, control-plane
node-02, 192.168.1.102, root, password123, control-plane
node-03, 192.168.1.103, root, password123, control-plane
node-04, 192.168.1.104, root, password123, worker
node-05, 192.168.1.105, root, password123, worker
```

自动化脚本生成 CAPI 资源：
```yaml
# 1. Cluster
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  clusterNetwork:
    pods:
      cidrBlocks: ["10.244.0.0/16"]
    services:
      cidrBlocks: ["10.96.0.0/12"]
  controlPlaneEndpoint:
    host: "lb.example.com"
    port: 6443
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
    kind: BareMetalCluster
    name: my-cluster
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1beta2
    kind: KubeadmControlPlane
    name: my-cluster-cp

---
# 2. BareMetalCluster
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalCluster
metadata:
  name: my-cluster
spec:
  controlPlaneEndpoint:
    host: "lb.example.com"
    port: 6443

---
# 3. 凭据 Secrets
apiVersion: v1
kind: Secret
metadata:
  name: node-01-credentials
stringData:
  username: "root"
  password: "password123"
---
# ... 为每台机器创建 Secret

---
# 4. KubeadmControlPlane
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: KubeadmControlPlane
metadata:
  name: my-cluster-cp
spec:
  replicas: 3
  version: v1.31.0
  machineTemplate:
    infrastructureRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
      kind: BareMetalMachineTemplate
      name: my-cluster-cp-template
  kubeadmConfigSpec:
    clusterConfiguration:
      apiServer:
        extraArgs:
          - name: "authorization-mode"
            value: "Node,RBAC"
      etcd:
        local:
          dataDir: /var/lib/etcd
    initConfiguration:
      nodeRegistration:
        kubeletExtraArgs:
          - name: "max-pods"
            value: "250"

---
# 5. BareMetalMachineTemplate (Control Plane)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalMachineTemplate
metadata:
  name: my-cluster-cp-template
spec:
  template:
    spec:
      sshPort: 22
      credentialsRef:
        name: default-cp-credentials
      role: "control-plane"

---
# 6. BareMetalMachine (每台控制面节点)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalMachine
metadata:
  name: node-01
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
spec:
  hostName: "node-01"
  ipAddress: "192.168.1.101"
  sshPort: 22
  credentialsRef:
    name: node-01-credentials
  role: "control-plane"
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalMachine
metadata:
  name: node-02
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
spec:
  hostName: "node-02"
  ipAddress: "192.168.1.102"
  sshPort: 22
  credentialsRef:
    name: node-02-credentials
  role: "control-plane"
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalMachine
metadata:
  name: node-03
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
spec:
  hostName: "node-03"
  ipAddress: "192.168.1.103"
  sshPort: 22
  credentialsRef:
    name: node-03-credentials
  role: "control-plane"

---
# 7. MachineDeployment (Worker)
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineDeployment
metadata:
  name: my-cluster-md-0
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
          name: my-cluster-md-template
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
        kind: BareMetalMachineTemplate
        name: my-cluster-md-template

---
# 8. BareMetalMachineTemplate (Worker)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalMachineTemplate
metadata:
  name: my-cluster-md-template
spec:
  template:
    spec:
      sshPort: 22
      credentialsRef:
        name: default-worker-credentials
      role: "worker"

---
# 9. BareMetalMachine (每台 Worker 节点)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalMachine
metadata:
  name: node-04
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
spec:
  hostName: "node-04"
  ipAddress: "192.168.1.104"
  sshPort: 22
  credentialsRef:
    name: node-04-credentials
  role: "worker"
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: BareMetalMachine
metadata:
  name: node-05
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
spec:
  hostName: "node-05"
  ipAddress: "192.168.1.105"
  sshPort: 22
  credentialsRef:
    name: node-05-credentials
  role: "worker"
```

## 六、关键设计决策
| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **ProviderID 格式** | `baremetal://<hostname>` vs `baremetal://<ip>` | hostname | 更稳定，IP 可能变化 |
| **SSH 连接管理** | 每次创建 vs 连接池 | 连接池 | 减少连接开销，提高性能 |
| **预检时机** | Provider 内部 vs preKubeadmCommands | 两者结合 | Provider 检查基础设施，preKubeadmCommands 检查 K8s 依赖 |
| **凭据存储** | 明文 vs Secret | Secret | 安全性，支持 RBAC 控制访问 |
| **电源管理** | 可选 vs 必需 | 可选 | 不是所有环境都有 IPMI/Redfish |

## 七、项目结构
```
cluster-api-provider-baremetal/
├── api/
│   └── v1beta2/
│       ├── baremetalcluster_types.go
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
│   ├── rbac/
│   └── manager/
├── go.mod
└── go.sum
```

## 八、开发路线图
| 阶段 | 内容 | 工作量 |
|------|------|--------|
| **Phase 1** | CRD 定义 + 基础 Controller 框架 | 1 周 |
| **Phase 2** | SSH 连接管理 + 凭据处理 | 1 周 |
| **Phase 3** | 预检逻辑 + ProviderID 生成 | 1 周 |
| **Phase 4** | 状态上报 + Conditions 管理 | 1 周 |
| **Phase 5** | 删除逻辑 + 清理 | 1 周 |
| **Phase 6** | 集成测试 + 文档 | 2 周 |
| **总计** | | **7 周** |
