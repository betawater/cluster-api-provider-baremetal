# CAPBM (Cluster API Provider Bare Metal) 详细设计

## 一、架构总览

CAPBM 采用多模块架构，包含两个独立的管理器：

| 模块 | API Group | 用途 |
|------|-----------|------|
| **CVO** (Cluster Version Operator) | `cvo.capbm.io` | 集群版本管理和升级协调 |
| **CAPBM** | `infrastructure.cluster.x-k8s.io` | 裸金属基础设施管理 |

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
│  │  CAPBM Provider (modules/capbm)                       │  │
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
│  │  CVO (Cluster Version Operator) (modules/cvo)         │  │
│  │  ┌─────────────────────────────────────────────────┐  │  │
│  │  │ ClusterVersion Controller                       │  │  │
│  │  │ - 集群版本管理                                  │  │  │
│  │  │ - 升级路径验证                                  │  │  │
│  │  │ - Addon 生命周期管理                            │  │  │
│  │  └─────────────────────────────────────────────────┘  │  │
│  │  ┌─────────────────────────────────────────────────┐  │  │
│  │  │ Addon Upgrader                                  │  │  │
│  │  │ - CNI/CSI 安装与升级                            │  │  │
│  │  │ - 版本对比与依赖排序                            │  │  │
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

## 二、CRD 设计

### 2.1 CAPBM 模块 CRD (`infrastructure.cluster.x-k8s.io`)

#### BareMetalCluster
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
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
// modules/capbm/api/v1beta1/baremetalcluster_types.go
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

#### BareMetalMachine
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
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
// modules/capbm/api/v1beta1/baremetalmachine_types.go
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

### 2.2 CVO 模块 CRD (`cvo.capbm.io`)

#### ClusterVersion
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
    version: v1.31.1
    image: registry.example.com/capbm/release:v1.31.1
status:
  actualVersion: v1.31.0
  desired:
    version: v1.31.1
    image: registry.example.com/capbm/release:v1.31.1
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
      phase: Upgrading
```

**Go 类型定义**:
```go
// modules/cvo/api/v1beta1/clusterversion_types.go
type ClusterVersionSpec struct {
    ClusterRef    corev1.ObjectReference `json:"clusterRef"`
    Channel       string                 `json:"channel,omitempty"`
    DesiredUpdate *Update                `json:"desiredUpdate,omitempty"`
}

type Update struct {
    Version string `json:"version,omitempty"`
    Image   string `json:"image,omitempty"`
    Force   bool   `json:"force,omitempty"`
}

type ClusterVersionStatus struct {
    ObservedGeneration int64                    `json:"observedGeneration"`
    Desired            Release                  `json:"desired"`
    ActualVersion      string                   `json:"actualVersion"`
    History            []UpdateHistory          `json:"history,omitempty"`
    Conditions         []metav1.Condition       `json:"conditions,omitempty"`
    AvailableUpdates   []Release                `json:"availableUpdates,omitempty"`
    ComponentStatus    []ComponentStatus        `json:"componentStatus,omitempty"`
    AddonStatus        []AddonVersionStatus     `json:"addonStatus,omitempty"`
}
```

#### ReleaseImage
```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
spec:
  version: v1.31.1
  image: registry.example.com/capbm/release:v1.31.1
  
  # 组件定义 (高内聚)
  components:
    kubernetes:
      version: v1.31.1
      type: binary
      installStrategy:
        timeout: 600s
        retryCount: 3
        method: package
        serviceName: kubelet
      upgradeStrategy:
        type: Rolling
        maxConcurrent: 1
        timeout: 900s
        retryCount: 3
        drain: true
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/kubernetes
              type: directory
          etcdSnapshot: true
        rollback:
          script: scripts/rollback-kubernetes.sh
          timeout: 600s
        healthCheck:
          command: kubectl get nodes
          timeout: 60s
          retries: 3
    
    containerd:
      version: 1.7.24
      type: binary
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/containerd/config.toml
              type: file
        rollback:
          script: scripts/rollback-containerd.sh
          timeout: 300s
        healthCheck:
          command: systemctl is-active containerd
          timeout: 30s
          retries: 3
  
  # Addon 定义
  addons:
    - name: calico
      type: helm
      version: v3.28.0
      contentPath: charts/calico-v3.28.0.tgz
      namespace: kube-system
      dependencies: []
      installStrategy:
        timeout: 300s
        retryCount: 3
        createNamespace: true
        wait: true
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/cni/net.d
              type: directory
        rollback:
          script: scripts/rollback-calico.sh
          timeout: 300s
        healthCheck:
          command: kubectl get pods -n kube-system -l k8s-app=calico-node
          timeout: 60s
          retries: 3
  
  # 升级图
  upgradeGraph:
    - name: phase-1-runtime
      order: 1
      blocking: true
      components: [containerd]
    - name: phase-2-kubernetes
      order: 2
      blocking: true
      components: [kubernetes]
    - name: phase-3-addons
      order: 3
      blocking: false
      components: [calico, ceph-csi]
```

#### 其他 CVO CRD
- `UpgradePath` - 升级路径和兼容性规则
- `ReleaseCatalog` - 可用发布版本目录
- `ClusterAddon` - 集群插件生命周期管理

## 三、控制器设计

### 3.1 CAPBM 模块控制器

#### BareMetalCluster Controller
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
// modules/capbm/internal/controllers/baremetalcluster_controller.go
func (r *BareMetalClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    cluster := &capbmv1.BareMetalCluster{}
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

func (r *BareMetalClusterReconciler) reconcileNormal(ctx context.Context, cluster *capbmv1.BareMetalCluster) (ctrl.Result, error) {
    // 验证配置
    if cluster.Spec.ControlPlaneEndpoint.Host == "" {
        markConditionFalse(&cluster.Status.Conditions, capbmv1.ReadyCondition, capbmv1.EndpointNotSetReason, clusterv1.ConditionSeverityError, "controlPlaneEndpoint is required")
        return ctrl.Result{}, nil
    }

    // 设置就绪状态
    cluster.Status.Ready = true
    markConditionTrue(&cluster.Status.Conditions, capbmv1.ReadyCondition)

    return ctrl.Result{}, r.Status().Update(ctx, cluster)
}
```

#### BareMetalMachine Controller
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

### 3.2 CVO 模块控制器

#### ClusterVersion Controller
**职责**:
- 监控 `DesiredUpdate` 变更
- 验证升级路径
- 执行 K8S 升级 (通过 UpgradeGraph)
- 执行 Addon 升级
- 更新升级状态

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

**核心代码结构**:
```go
// modules/cvo/internal/controllers/clusterversion_controller.go
func (r *ClusterVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    cv := &cfov1.ClusterVersion{}
    if err := r.Get(ctx, req.NamespacedName, cv); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 同步 UpgradePath 和 ReleaseCatalog
    if err := r.syncUpgradePath(ctx, cv); err != nil {
        return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
    }
    if err := r.syncReleaseCatalog(ctx, cv); err != nil {
        return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
    }

    // 判断是否需要升级
    needsK8SUpgrade := cv.Status.ActualVersion != cv.Spec.DesiredUpdate.Version
    releaseImage, _ := r.fetchReleaseImage(ctx, cv)
    needsAddonUpgrade := r.needsAddonUpgrade(ctx, cv, releaseImage)

    if !needsK8SUpgrade && !needsAddonUpgrade {
        return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
    }

    // 执行升级
    if err := r.executeUpgrade(ctx, cv, releaseImage, needsK8SUpgrade); err != nil {
        return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
    }

    // 更新状态
    cv.Status.ActualVersion = cv.Spec.DesiredUpdate.Version
    r.updateAddonStatus(cv, releaseImage)

    return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ClusterVersionReconciler) executeUpgrade(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage, needsK8SUpgrade bool) error {
    // Phase 1: K8S 升级 (仅当 K8S 版本变更时)
    if needsK8SUpgrade {
        executor := upgrader.NewGraphExecutor(r.Client, r.Puller, nil)
        if err := executor.ExecuteUpgradeGraph(ctx, cv, releaseImage); err != nil {
            return fmt.Errorf("k8s upgrade failed: %w", err)
        }
    }

    // Phase 2: Addon 升级 (总是执行)
    if err := r.executeAddonUpgrades(ctx, cv, releaseImage); err != nil {
        return fmt.Errorf("addon upgrades failed: %w", err)
    }

    // 更新 Addon 状态
    r.updateAddonStatus(cv, releaseImage)

    return nil
}
```

## 四、SSH 连接管理

### 4.1 SSH Manager 设计
```go
// modules/capbm/internal/ssh/manager.go
type SSHManager struct {
    connections map[string]*SSHConnection
    mu          sync.RWMutex
    idleTimeout time.Duration
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
        if time.Since(conn.LastUsed) < m.idleTimeout {
            // 检查连接是否存活
            if _, _, err := conn.Client.SendRequest("keepalive@google.com", true, nil); err == nil {
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

## 五、升级流程设计

### 5.1 升级触发机制

```yaml
# 方式一：更新 DesiredUpdate.Version 触发 K8S + Addon 升级
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
spec:
  desiredUpdate:
    version: v1.31.1   # 从 v1.31.0 升级到 v1.31.1

# 方式二：更新 DesiredUpdate.Image 仅触发 Addon 升级 (K8S 版本不变)
spec:
  desiredUpdate:
    version: v1.31.0     # K8S 版本不变
    image: registry.example.com/capbm/release:v1.31.0-patch1  # 新 ReleaseImage
```

### 5.2 升级执行流程

```
executeUpgrade(ctx, cv, releaseImage):
    │
    ├── Phase 1: K8S 升级 (仅当 K8S 版本变更时)
    │   ├── 1.1 备份 etcd
    │   ├── 1.2 升级 Control Plane 节点 (滚动)
    │   │   ├── containerd
    │   │   ├── CNI/CSI (节点级二进制)
    │   │   └── kubeadm upgrade node (通过 KCP)
    │   └── 1.3 升级 Worker 节点 (滚动)
    │
    └── Phase 2: Addon 升级 (总是执行)
        ├── 2.1 按依赖顺序排序 Addon
        ├── 2.2 逐个升级 Addon
        │   ├── 备份当前配置
        │   ├── 执行升级 (Helm/Manifest)
        │   ├── 健康检查
        │   └── 更新 ClusterAddon.Status
        └── 2.3 更新 ClusterVersion.Status.AddonStatus
```

### 5.3 Addon 升级触发判断

```go
func (r *ClusterVersionReconciler) needsAddonUpgrade(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) bool {
    for _, addonDef := range releaseImage.Spec.Addons {
        if addonDef.Version == "" {
            continue
        }

        clusterAddon := &cfov1.ClusterAddon{}
        err := r.Get(ctx, types.NamespacedName{Name: addonDef.Name, Namespace: cv.Namespace}, clusterAddon)

        if apierrors.IsNotFound(err) {
            return true  // 新 Addon，需要安装
        }

        if err != nil {
            continue
        }

        // 版本不同，需要升级
        if clusterAddon.Status.Version != addonDef.Version {
            return true
        }
    }
    return false
}
```

## 六、部署与使用流程

### 6.1 安装

```bash
# 安装 CAPI core components
clusterctl init --core cluster-api --bootstrap kubeadm --control-plane kubeadm

# 安装 CAPBM provider
kubectl apply -k modules/capbm/config/default/

# 安装 CVO (version operator)
kubectl apply -k modules/cvo/config/default/

# 部署 ClusterClass templates
kubectl apply -k modules/capbm/config/clusterclass/
```

### 6.2 用户输入转化

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
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: BareMetalCluster
    name: my-cluster
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1beta2
    kind: KubeadmControlPlane
    name: my-cluster-cp

---
# 2. BareMetalCluster
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalCluster
metadata:
  name: my-cluster
spec:
  controlPlaneEndpoint:
    host: "lb.example.com"
    port: 6443

---
# 3. ClusterVersion (CVO)
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
spec:
  clusterRef:
    name: my-cluster
    namespace: default
  desiredUpdate:
    version: v1.31.0
    image: registry.example.com/capbm/release:v1.31.0

---
# 4. 凭据 Secrets
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
# 5. KubeadmControlPlane
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: KubeadmControlPlane
metadata:
  name: my-cluster-cp
spec:
  replicas: 3
  version: v1.31.0
  machineTemplate:
    infrastructureRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
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
# 6. BareMetalMachineTemplate (Control Plane)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
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
# 7. BareMetalMachine (每台控制面节点)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
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
# ... node-02, node-03

---
# 8. MachineDeployment (Worker)
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
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalMachineTemplate
        name: my-cluster-md-template

---
# 9. BareMetalMachineTemplate (Worker)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
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
# 10. BareMetalMachine (每台 Worker 节点)
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
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
# ... node-05
```

## 七、关键设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **架构模式** | 单模块 vs 多模块 | 多模块 | CVO 和 CAPBM 职责分离，独立演进 |
| **ProviderID 格式** | `baremetal://<hostname>` vs `baremetal://<ip>` | hostname | 更稳定，IP 可能变化 |
| **SSH 连接管理** | 每次创建 vs 连接池 | 连接池 | 减少连接开销，提高性能 |
| **预检时机** | Provider 内部 vs preKubeadmCommands | 两者结合 | Provider 检查基础设施，preKubeadmCommands 检查 K8s 依赖 |
| **凭据存储** | 明文 vs Secret | Secret | 安全性，支持 RBAC 控制访问 |
| **电源管理** | 可选 vs 必需 | 可选 | 不是所有环境都有 IPMI/Redfish |
| **升级触发** | 独立字段 vs DesiredUpdate | DesiredUpdate | 统一入口，简化使用 |
| **升级顺序** | Addon 先 vs K8S 先 | K8S 先 | Addon 通常兼容多个 K8S 版本 |
| **版本来源** | ReleaseImage vs 直接指定 | ReleaseImage | 保持单一数据源 |

## 八、项目结构

```
cluster-api-provider-baremetal/
├── modules/
│   ├── cvo/                    # Cluster Version Operator
│   │   ├── go.mod
│   │   ├── api/v1beta1/        # CVO API types
│   │   │   ├── clusterversion_types.go
│   │   │   ├── releaseimage_types.go
│   │   │   ├── upgradepath_types.go
│   │   │   ├── releasecatalog_types.go
│   │   │   ├── clusteraddon_types.go
│   │   │   ├── component_types.go
│   │   │   ├── addon_types.go
│   │   │   ├── upgrade_types.go
│   │   │   └── groupversion_info.go
│   │   ├── cmd/manager/        # CVO entry point
│   │   ├── internal/           # CVO controllers & logic
│   │   │   ├── controllers/
│   │   │   │   ├── clusterversion_controller.go
│   │   │   │   └── suite_test.go
│   │   │   ├── upgrader/
│   │   │   │   ├── graph_executor.go
│   │   │   │   ├── control_plane_upgrader.go
│   │   │   │   ├── backup_rollback.go
│   │   │   │   ├── health_checker.go
│   │   │   │   ├── diff_components.go
│   │   │   │   ├── encryption/
│   │   │   │   ├── metrics/
│   │   │   │   └── retry/
│   │   │   ├── addon/
│   │   │   │   ├── upgrader.go
│   │   │   │   ├── helm_installer.go
│   │   │   │   └── manifest_installer.go
│   │   │   └── registry/
│   │   ├── pkg/ssh/            # Public SSH package
│   │   └── config/             # CVO deployment configs
│   │       ├── crd/bases/      # Generated CRD YAMLs
│   │       ├── rbac/
│   │       └── manager/
│   │
│   └── capbm/                  # CAPBM Infrastructure Provider
│       ├── go.mod
│       ├── api/v1beta1/        # CAPBM API types
│       │   ├── baremetalcluster_types.go
│       │   ├── baremetalmachine_types.go
│       │   ├── baremetalhostinventory_types.go
│       │   ├── baremetalclustertemplate_types.go
│       │   ├── baremetalmachinetemplate_types.go
│       │   ├── conditions.go
│       │   └── groupversion_info.go
│       ├── cmd/manager/        # CAPBM entry point
│       ├── internal/           # Controllers, SSH, LB, etc.
│       │   ├── controllers/
│       │   │   ├── baremetalcluster_controller.go
│       │   │   ├── baremetalmachine_controller.go
│       │   │   ├── baremetalhostinventory_controller.go
│       │   │   └── suite_test.go
│       │   ├── ssh/
│       │   ├── installer/
│       │   ├── lb/
│       │   ├── cni/
│       │   ├── csi/
│       │   ├── network/
│       │   ├── health/
│       │   ├── gateway/
│       │   ├── component/
│       │   ├── helm/
│       │   ├── images/
│       │   └── node/
│       └── config/             # CAPBM deployment configs
│           ├── crd/bases/      # Generated CRD YAMLs
│           ├── rbac/
│           ├── manager/
│           └── clusterclass/   # ClusterClass templates
│
├── docs/                       # Design documentation
├── hack/                       # Helper scripts
├── templates/                  # Templates
├── test/                       # E2E tests
├── go.work                     # Go workspace definition
├── Makefile
├── Dockerfile.cvo              # CVO Docker build
└── Dockerfile.capbm            # CAPBM Docker build
```

## 九、开发路线图

| 阶段 | 内容 | 工作量 |
|------|------|--------|
| **Phase 1** | CRD 定义 + 基础 Controller 框架 | 1 周 |
| **Phase 2** | SSH 连接管理 + 凭据处理 | 1 周 |
| **Phase 3** | 预检逻辑 + ProviderID 生成 | 1 周 |
| **Phase 4** | 状态上报 + Conditions 管理 | 1 周 |
| **Phase 5** | 删除逻辑 + 清理 | 1 周 |
| **Phase 6** | CVO 模块开发 (ClusterVersion Controller) | 2 周 |
| **Phase 7** | 升级流程实现 (UpgradeGraph + Addon Upgrader) | 2 周 |
| **Phase 8** | 集成测试 + 文档 | 2 周 |
| **总计** | | **11 周** |

## 十、API Groups 参考

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
