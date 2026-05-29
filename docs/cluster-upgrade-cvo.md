## 七、集群升级设计 (CVO 机制)

参考 OpenShift Cluster Version Operator (CVO) 机制，设计裸金属集群的升级管理能力。

### 7.1 核心架构

采用四个 CRD 资源，职责清晰、不耦合：

| 资源 | 作用域 | 数量 | 核心职责 | 数据来源 |
|------|--------|------|---------|---------|
| **ClusterVersion** | 每集群 | 每集群 1 个 | 升级目标、策略、状态、历史 | 用户配置 + 其他资源查询 |
| **ReleaseImage** | 每版本 | 每版本 1 个 | 组件版本映射、升级依赖图、Manifest | OCI 镜像拉取 |
| **UpgradePath** | 全局 | 全局 1 个 | 升级图 (edges)、兼容性规则 | OCI 镜像拉取 |
| **ReleaseCatalog** | 全局 | 全局 1 个 | 所有已发布版本列表、通道索引 | OCI 镜像拉取 |

```
┌─────────────────────────────────────────────────────────────────────┐
│                        全局资源 (Global)                              │
│                                                                     │
│  ┌──────────────────┐              ┌──────────────────┐            │
│  │  ReleaseCatalog  │              │   UpgradePath    │            │
│  │  spec.image      │              │  spec.image      │            │
│  │  所有已发布版本   │              │  升级图 (edges)   │            │
│  │  通道索引         │              │  兼容性规则       │            │
│  └────────┬─────────┘              └────────┬─────────┘            │
│           │                                 │                      │
│           │ 查询版本信息                     │ 查询升级路径          │
│           ▼                                 ▼                      │
│  ┌──────────────────────────────────────────────────┐             │
│  │              ClusterVersion Controller            │             │
│  │                                                   │             │
│  │  1. 从 ReleaseCatalog 查询目标版本信息             │             │
│  │  2. 从 UpgradePath 验证升级路径合法性              │             │
│  │  3. 从 ReleaseImage 获取升级依赖图和 Manifest      │             │
│  │  4. 执行升级                                       │             │
│  │  5. 更新状态                                       │             │
│  └──────────────────────┬───────────────────────────┘             │
│                         │                                          │
└─────────────────────────┼──────────────────────────────────────────┘
                          │ 每集群
                          ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        每集群资源 (Per-Cluster)                       │
│                                                                     │
│  ┌──────────────────┐              ┌──────────────────┐            │
│  │ ClusterVersion   │─────────────▶│  ReleaseImage    │            │
│  │ (my-cluster)     │  拉取 OCI    │  (v1.32.0)       │            │
│  │                  │              │  组件版本映射      │            │
│  │ spec:            │              │  升级依赖图        │            │
│  │   desiredUpdate  │              │  Manifest 文件     │            │
│  │   channel        │              └──────────────────┘            │
│  │ status:          │                                              │
│  │   actualVersion  │              ┌──────────────────┐            │
│  │   history        │              │  ReleaseImage    │            │
│  │   conditions     │              │  (v1.31.0)       │            │
│  └──────────────────┘              └──────────────────┘            │
└─────────────────────────────────────────────────────────────────────┘
```

### 7.2 image 字段使用

#### 7.2.1 image 字段在各资源中的含义

| 资源 | image 字段位置 | 含义 | 数据来源 | 使用方式 |
|------|---------------|------|---------|---------|
| **ReleaseCatalog** | `spec.image` | 全局版本目录的 OCI 镜像地址 | 用户配置或内置默认值 | Controller 定期拉取，解析所有已发布版本列表 |
| **ReleaseCatalog** | `status.releases[].image` | 每个版本的 ReleaseImage OCI 地址 | 从 spec.image 的 OCI 镜像中解析 | ClusterVersion 查询目标版本的 image 地址 |
| **UpgradePath** | `spec.image` | 升级路径图的 OCI 镜像地址 | 用户配置或内置默认值 | Controller 定期拉取，解析升级图和兼容性规则 |
| **ReleaseImage** | `spec.image` | 该版本完整内容的 OCI 镜像地址 | 从 ReleaseCatalog 查询得到 | Controller 拉取，解析组件版本、升级图、Manifest |
| **ClusterVersion** | `spec.desiredUpdate.image` | 用户手动指定的 ReleaseImage 地址 (可选) | 用户配置 | 优先级高于 ReleaseCatalog 自动查询 |

#### 7.2.2 image 字段流转图

```
ReleaseCatalog.spec.image (用户配置或默认值)
  = "registry.example.com/capbm/release-catalog:latest"
       │
       │ crane.Pull()
       ▼
  ┌─────────────────────────┐
  │ /catalog.json           │
  │ { "releases": [         │
  │     { "version": "v1.32.0",                                     │
  │       "image": "registry.example.com/capbm/release:v1.32.0" }   │
  │   ]                     │
  │ }                       │
  └───────────┬─────────────┘
              │
              │ status.releases[].image
              ▼
  ClusterVersion.spec.desiredUpdate.version = "v1.32.0"
              │
              │ 如果 desiredUpdate.image 为空:
              │ → 从 ReleaseCatalog.status.releases 查询
              │ → 获取 image = "registry.example.com/capbm/release:v1.32.0"
              ▼
  crane.Pull("registry.example.com/capbm/release:v1.32.0")
       │
       ▼
  ┌─────────────────────────┐
  │ /release.json           │ → 创建/更新 ReleaseImage CR
  │ /upgrade-graph.json     │ → 解析升级依赖图
  │ /manifests/             │ → kubectl apply
  │ /scripts/               │ → SSH 执行
  │ /signatures/            │ → 验证签名
  └─────────────────────────┘
```

#### 7.2.3 默认值兜底机制

ReleaseCatalog 和 UpgradePath 不存在时，Controller 使用内置默认值：

```go
const (
    ReleaseCatalogName      = "global"
    DefaultCatalogImage     = "registry.example.com/capbm/release-catalog:latest"
    UpgradePathName         = "global"
    DefaultUpgradePathImage = "registry.example.com/capbm/upgrade-path:latest"
)

func (r *ClusterVersionReconciler) syncReleaseCatalog(ctx context.Context, cv *infrav1.ClusterVersion) error {
    catalog := &infrav1.ReleaseCatalog{}
    err := r.Get(ctx, types.NamespacedName{Name: ReleaseCatalogName}, catalog)
    
    var catalogImage string
    if err == nil {
        catalogImage = catalog.Spec.Image
    } else if apierrors.IsNotFound(err) {
        catalogImage = DefaultCatalogImage
        catalog = &infrav1.ReleaseCatalog{
            ObjectMeta: metav1.ObjectMeta{Name: ReleaseCatalogName},
            Spec: infrav1.ReleaseCatalogSpec{Image: DefaultCatalogImage},
        }
        r.Create(ctx, catalog)
    } else {
        return err
    }
    return r.pullAndSyncCatalog(ctx, catalogImage)
}
```

**开箱即用**: 在线环境无需配置，Controller 自动使用默认值。

**离线覆盖**: 用户创建自定义 CR 指向内网 registry：
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ReleaseCatalog
metadata:
  name: global
spec:
  image: "internal-registry.example.com/capbm/release-catalog:latest"
```

### 7.3 CRD 详细设计

#### 7.3.1 ClusterVersion (每集群)

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
spec:
  clusterRef:
    name: my-cluster
    namespace: default
  channel: "stable-v1.31"
  desiredUpdate:
    version: "v1.32.0"
    image: ""                    # 可选，不填则从 ReleaseCatalog 查询
    force: false
status:
  observedGeneration: 1
  desired:
    version: "v1.32.0"
    image: "registry.example.com/capbm/release:v1.32.0"
  actualVersion: "v1.31.0"
  history:
    - state: "Completed"
      version: "v1.31.0"
      image: "registry.example.com/capbm/release:v1.31.0"
      verified: true
      startedTime: "2024-01-15T10:00:00Z"
      completionTime: "2024-01-15T10:30:00Z"
  conditions:
    - type: "Available"
      status: "True"
      reason: "AsExpected"
    - type: "Progressing"
      status: "False"
      reason: "AsExpected"
    - type: "Failing"
      status: "False"
      reason: "AsExpected"
    - type: "Upgradeable"
      status: "True"
      reason: "PreconditionsPassed"
    - type: "RetrievedUpdates"
      status: "True"
      reason: "AsExpected"
  availableUpdates:
    - version: "v1.32.0"
      image: "registry.example.com/capbm/release:v1.32.0"
  componentStatus:
    - name: "etcd"
      version: "v1.31.0"
      targetVersion: "v1.32.0"
      phase: "Completed"
    - name: "kube-apiserver"
      version: "v1.31.0"
      targetVersion: "v1.32.0"
      phase: "Upgrading"
```

#### 7.3.2 ReleaseImage (每版本)

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-32-0
  labels:
    infrastructure.cluster.x-k8s.io/release-version: "v1.32.0"
spec:
  version: "v1.32.0"
  image: "registry.example.com/capbm/release:v1.32.0"
  channels: ["stable-v1.31", "stable-v1.32"]
  previousVersions: ["v1.31.0", "v1.31.1", "v1.31.2"]
  components:
    kubernetes:
      kubeApiserver: "v1.32.0"
      kubeControllerManager: "v1.32.0"
      kubeScheduler: "v1.32.0"
      kubeProxy: "v1.32.0"
      kubelet: "v1.32.0"
      kubeadm: "v1.32.0"
      etcd: "3.5.12"
      coredns: "1.11.1"
    containerd: "1.7.13"
    calico: "3.27.0"
    cephCsi: "3.10.0"
  upgradeGraph:
    - phase: "CRDs"
      order: 100
      blocking: true
      components:
        - name: "kubernetes-crds"
          manifests: ["crds/core-crds.yaml"]
          blocking: true
          healthCheck:
            type: "CRDEstablished"
            timeout: "5m"
    - phase: "ControlPlane"
      order: 200
      blocking: true
      rollingUpdate:
        maxUnavailable: 1
      components:
        - name: "etcd"
          manifests: ["control-plane/etcd.yaml"]
          blocking: true
          healthCheck:
            type: "PodReady"
            namespace: "kube-system"
            labelSelector: "component=etcd"
            timeout: "10m"
        - name: "kube-apiserver"
          manifests: ["control-plane/apiserver.yaml"]
          blocking: true
          dependsOn: ["etcd"]
          healthCheck:
            type: "EndpointHealthy"
            endpoint: "https://localhost:6443/healthz"
            timeout: "10m"
        - name: "kube-controller-manager"
          manifests: ["control-plane/controller-manager.yaml"]
          dependsOn: ["kube-apiserver"]
        - name: "kube-scheduler"
          manifests: ["control-plane/scheduler.yaml"]
          dependsOn: ["kube-apiserver"]
    - phase: "Infrastructure"
      order: 300
      blocking: true
      components:
        - name: "coredns"
          manifests: ["infrastructure/coredns.yaml"]
          dependsOn: ["kube-apiserver"]
        - name: "calico"
          manifests: ["infrastructure/calico.yaml"]
          dependsOn: ["kube-apiserver"]
          healthCheck:
            type: "DaemonSetReady"
            namespace: "kube-system"
            name: "calico-node"
            timeout: "10m"
        - name: "ceph-csi"
          manifests: ["infrastructure/ceph-csi.yaml"]
          dependsOn: ["kube-apiserver"]
          healthCheck:
            type: "DeploymentReady"
            namespace: "ceph-csi"
            name: "ceph-csi-rbdplugin-provisioner"
            timeout: "10m"
    - phase: "NodeComponents"
      order: 400
      blocking: true
      rollingUpdate:
        maxUnavailable: 1
      components:
        - name: "containerd"
          scripts: ["scripts/upgrade_containerd.sh"]
        - name: "kubelet"
          scripts: ["scripts/upgrade_kubelet.sh"]
          dependsOn: ["containerd"]
  contentHash: "sha256:abc123..."
status:
  verified: true
  manifestCount: 12
```

#### 7.3.3 UpgradePath (全局)

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: UpgradePath
metadata:
  name: global
spec:
  image: "registry.example.com/capbm/upgrade-path:latest"
  graph:
    edges:
      - from: "v1.31.0"
        to: "v1.31.1"
        recommended: true
      - from: "v1.31.0"
        to: "v1.31.2"
        recommended: true
      - from: "v1.31.0"
        to: "v1.32.0"
        recommended: true
      - from: "v1.31.1"
        to: "v1.31.2"
        recommended: true
      - from: "v1.31.1"
        to: "v1.32.0"
        recommended: true
      - from: "v1.31.2"
        to: "v1.32.0"
        recommended: true
      - from: "v1.32.0"
        to: "v1.33.0"
        recommended: false
  rules:
    maxVersionSkip: 2
    blockedUpgrades:
      - from: "v1.30.*"
        to: "v1.33.*"
        reason: "跨越超过 2 个 minor 版本"
status:
  lastSyncTime: "2024-02-20T13:55:00Z"
  syncSucceeded: true
  imageDigest: "sha256:def456..."
```

#### 7.3.4 ReleaseCatalog (全局)

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ReleaseCatalog
metadata:
  name: global
spec:
  image: "registry.example.com/capbm/release-catalog:latest"
  syncInterval: "1h"
status:
  lastSyncTime: "2024-02-20T13:55:00Z"
  syncSucceeded: true
  imageDigest: "sha256:xyz789..."
  releases:
    - version: "v1.31.0"
      image: "registry.example.com/capbm/release:v1.31.0"
      channels: ["stable-v1.30", "stable-v1.31"]
      releaseDate: "2024-01-15T00:00:00Z"
    - version: "v1.31.1"
      image: "registry.example.com/capbm/release:v1.31.1"
      channels: ["stable-v1.31"]
      releaseDate: "2024-02-01T00:00:00Z"
    - version: "v1.31.2"
      image: "registry.example.com/capbm/release:v1.31.2"
      channels: ["stable-v1.31"]
      releaseDate: "2024-02-15T00:00:00Z"
    - version: "v1.32.0"
      image: "registry.example.com/capbm/release:v1.32.0"
      channels: ["stable-v1.31", "stable-v1.32"]
      releaseDate: "2024-03-01T00:00:00Z"
  channels:
    stable-v1.31:
      - version: "v1.31.0"
      - version: "v1.31.1"
      - version: "v1.31.2"
      - version: "v1.32.0"
    stable-v1.32:
      - version: "v1.32.0"
```

### 7.4 Go 类型定义

```go
// ==================== ClusterVersion ====================

type ClusterVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   ClusterVersionSpec   `json:"spec,omitempty"`
	Status ClusterVersionStatus `json:"status,omitempty"`
}

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
	ObservedGeneration int64              `json:"observedGeneration"`
	Desired            Release            `json:"desired"`
	ActualVersion      string             `json:"actualVersion"`
	History            []UpdateHistory    `json:"history,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	AvailableUpdates   []Release          `json:"availableUpdates,omitempty"`
	ComponentStatus    []ComponentStatus  `json:"componentStatus,omitempty"`
}

type Release struct {
	Version string `json:"version"`
	Image   string `json:"image"`
}

type UpdateState string

const (
	CompletedUpdate UpdateState = "Completed"
	PartialUpdate   UpdateState = "Partial"
)

type UpdateHistory struct {
	State          UpdateState  `json:"state"`
	Version        string       `json:"version"`
	Image          string       `json:"image"`
	Verified       bool         `json:"verified"`
	StartedTime    metav1.Time  `json:"startedTime"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

type ComponentStatus struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	TargetVersion string `json:"targetVersion"`
	Phase         string `json:"phase"`
}

const (
	UpgradeAvailable   = "Available"
	UpgradeProgressing = "Progressing"
	UpgradeFailing     = "Failing"
	UpgradeUpgradeable = "Upgradeable"
	UpgradeRetrieved   = "RetrievedUpdates"
)

// ==================== ReleaseImage ====================

type ReleaseImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   ReleaseImageSpec   `json:"spec,omitempty"`
	Status ReleaseImageStatus `json:"status,omitempty"`
}

type ReleaseImageSpec struct {
	Version          string            `json:"version"`
	Image            string            `json:"image"`
	Channels         []string          `json:"channels,omitempty"`
	PreviousVersions []string          `json:"previousVersions,omitempty"`
	Components       ComponentVersions `json:"components"`
	UpgradeGraph     []UpgradePhase    `json:"upgradeGraph"`
	ContentHash      string            `json:"contentHash,omitempty"`
}

type ComponentVersions struct {
	Kubernetes map[string]string `json:"kubernetes"`
	Containerd string            `json:"containerd,omitempty"`
	Calico     string            `json:"calico,omitempty"`
	Cilium     string            `json:"cilium,omitempty"`
	CephCsi    string            `json:"cephCsi,omitempty"`
}

type UpgradePhase struct {
	Name          string             `json:"name"`
	Order         int                `json:"order"`
	Blocking      bool               `json:"blocking"`
	RollingUpdate *RollingUpdate     `json:"rollingUpdate,omitempty"`
	Components    []UpgradeComponent `json:"components"`
}

type RollingUpdate struct {
	MaxUnavailable int `json:"maxUnavailable,omitempty"`
}

type UpgradeComponent struct {
	Name        string       `json:"name"`
	Manifests   []string     `json:"manifests,omitempty"`
	Scripts     []string     `json:"scripts,omitempty"`
	Blocking    bool         `json:"blocking"`
	DependsOn   []string     `json:"dependsOn,omitempty"`
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`
}

type HealthCheck struct {
	Type          string        `json:"type"`
	Namespace     string        `json:"namespace,omitempty"`
	Name          string        `json:"name,omitempty"`
	LabelSelector string        `json:"labelSelector,omitempty"`
	Endpoint      string        `json:"endpoint,omitempty"`
	Timeout       metav1.Duration `json:"timeout,omitempty"`
}

type ReleaseImageStatus struct {
	Verified      bool `json:"verified"`
	ManifestCount int  `json:"manifestCount"`
}

// ==================== UpgradePath ====================

type UpgradePath struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   UpgradePathSpec   `json:"spec,omitempty"`
	Status UpgradePathStatus `json:"status,omitempty"`
}

type UpgradePathSpec struct {
	Image string           `json:"image"`
	Graph UpgradeGraphData `json:"graph"`
	Rules CompatibilityRules `json:"rules,omitempty"`
}

type UpgradeGraphData struct {
	Edges []GraphEdge `json:"edges"`
}

type GraphEdge struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Recommended bool   `json:"recommended"`
}

type CompatibilityRules struct {
	MaxVersionSkip  int              `json:"maxVersionSkip,omitempty"`
	BlockedUpgrades []BlockedUpgrade `json:"blockedUpgrades,omitempty"`
}

type BlockedUpgrade struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

type UpgradePathStatus struct {
	LastSyncTime  metav1.Time `json:"lastSyncTime,omitempty"`
	SyncSucceeded bool        `json:"syncSucceeded"`
	ImageDigest   string      `json:"imageDigest,omitempty"`
}

// ==================== ReleaseCatalog ====================

type ReleaseCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   ReleaseCatalogSpec   `json:"spec,omitempty"`
	Status ReleaseCatalogStatus `json:"status,omitempty"`
}

type ReleaseCatalogSpec struct {
	Image        string          `json:"image"`
	SyncInterval metav1.Duration `json:"syncInterval,omitempty"`
}

type ReleaseCatalogStatus struct {
	LastSyncTime  metav1.Time         `json:"lastSyncTime,omitempty"`
	SyncSucceeded bool                `json:"syncSucceeded"`
	ImageDigest   string              `json:"imageDigest,omitempty"`
	Releases      []ReleaseEntry      `json:"releases,omitempty"`
	Channels      map[string][]ChannelVersion `json:"channels,omitempty"`
}

type ReleaseEntry struct {
	Version     string   `json:"version"`
	Image       string   `json:"image"`
	Channels    []string `json:"channels,omitempty"`
	ReleaseDate string   `json:"releaseDate,omitempty"`
}

type ChannelVersion struct {
	Version string `json:"version"`
}
```

### 7.5 Channel 设计

#### 7.5.1 Channel 命名规则

格式: `{stability}-{major}.{minor}`

| 前缀 | 含义 | 适用场景 |
|------|------|---------|
| `stable-*` | 稳定通道 | 生产环境 |
| `fast-*` | 快速通道 | 测试/预发环境 |
| `eus-*` | 扩展支持通道 | 长期支持需求 |

示例:
- `stable-v1.31`: v1.31 系列的稳定版本
- `fast-v1.32`: v1.32 系列的快速版本
- `eus-v1.30`: v1.30 扩展支持版本

#### 7.5.2 Channel 存储位置

Channel 信息内嵌在 `ReleaseCatalog.status.channels` 中：

```yaml
status:
  channels:
    stable-v1.31:
      - version: "v1.31.0"
      - version: "v1.31.1"
      - version: "v1.31.2"
      - version: "v1.32.0"
    stable-v1.32:
      - version: "v1.32.0"
```

#### 7.5.3 Channel 升级规则

内嵌在 `UpgradePath.spec.rules` 中：

```yaml
rules:
  maxVersionSkip: 2
  blockedUpgrades:
    - from: "v1.30.*"
      to: "v1.33.*"
      reason: "跨越超过 2 个 minor 版本"
```

### 7.6 升级依赖图设计

#### 7.6.1 升级阶段

| Phase | Order | 内容 | Blocking |
|-------|-------|------|----------|
| CRDs | 100 | CRD 升级 | 是 |
| ControlPlane | 200 | etcd → apiserver → ccm → scheduler | 是 |
| Infrastructure | 300 | coredns, kube-proxy, CNI, CSI | 是 |
| NodeComponents | 400 | containerd, kubelet (滚动升级) | 是 |

#### 7.6.2 组件类型

通过字段存在性自动推断：
- 有 `manifests` → Manifest 类型 (`kubectl apply -f`)
- 有 `scripts` → Script 类型 (SSH 执行)

#### 7.6.3 HealthCheck 类型 (最小集)

| 类型 | 适用 | 验证内容 |
|------|------|---------|
| `PodReady` | Pod 组件 | 指定 labelSelector 的 Pod 全部 Running |
| `DaemonSetReady` | DaemonSet | 所有节点上的 Pod 都已调度且 Ready |
| `DeploymentReady` | Deployment | 期望的 Pod 副本数都已 Ready |
| `EndpointHealthy` | API 服务 | HTTP 端点返回 200 |
| `CRDEstablished` | CRD | CRD 状态为 Established |
| `ServiceRunning` | systemd 服务 | 服务状态为 active |

#### 7.6.4 滚动升级

```yaml
rollingUpdate:
  maxUnavailable: 1
```

#### 7.6.5 升级图 DAG 示例

```
Phase 100: CRDs
    │
    └── kubernetes-crds
            │
            ▼
Phase 200: ControlPlane
    │
    ├── etcd ──── dependsOn ──── kube-apiserver
    │                              │
    │                    ┌─────────┴─────────┐
    │                    ▼                   ▼
    │              kube-controller-manager  kube-scheduler
    │                                        │
    │                                        ▼
    │                               Phase 300: Infrastructure
    │                                        │
    │                    ┌───────────────────┼───────────────────┐
    │                    ▼                   ▼                   ▼
    │                coredns              kube-proxy            calico
    │                                                             │
    │                                                             ▼
    │                                                    Phase 400: NodeComponents
    │                                                             │
    │                                                  ┌──────────┴──────────┐
    │                                                  ▼                     ▼
    │                                              containerd ──dependsOn── kubelet
```

### 7.7 OCI 镜像结构

#### 7.7.1 ReleaseCatalog OCI

```
registry.example.com/capbm/release-catalog:latest
└── /catalog.json          # releases 列表 + channels 索引
```

#### 7.7.2 UpgradePath OCI

```
registry.example.com/capbm/upgrade-path:latest
└── /upgrade-path.json     # graph.edges + rules
```

#### 7.7.3 ReleaseImage OCI (每版本一个)

```
registry.example.com/capbm/release:v1.32.0
├── /release.json          # 组件版本映射
├── /upgrade-graph.json    # 升级依赖图
├── /manifests/            # k8s manifest 文件
│   ├── crds/
│   ├── control-plane/
│   └── infrastructure/
├── /scripts/              # 节点升级脚本
│   ├── upgrade_containerd.sh
│   └── upgrade_kubelet.sh
└── /signatures/           # 签名文件
```

### 7.8 完整升级流程

```
用户设置 ClusterVersion.spec.desiredUpdate.version = "v1.32.0"
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 1: 查询 ReleaseCatalog 获取目标版本信息                   │
│                                                              │
│  1. 读取 ReleaseCatalog (name=global)                        │
│     → 不存在则使用默认值并自动创建 CR                         │
│  2. 在 status.releases 中查找 version="v1.32.0"              │
│  3. 获取 image = "registry.example.com/capbm/release:v1.32.0"│
│  4. 写入 ClusterVersion.status.desired                       │
└─────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 2: 查询 UpgradePath 验证升级路径合法性                    │
│                                                              │
│  1. 读取 UpgradePath (name=global)                           │
│     → 不存在则使用默认值并自动创建 CR                         │
│  2. 在 graph.edges 中查找 from=actualVersion, to=desired     │
│  3. 验证: 边存在 + recommended=true (或 force=true)           │
│  4. 验证: 不在 blockedUpgrades 中                            │
│  5. 计算 availableUpdates (BFS 遍历 edges)                   │
│  6. 写入 ClusterVersion.status.availableUpdates              │
└─────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 3: 拉取 ReleaseImage OCI 镜像                           │
│                                                              │
│  1. crane.Pull(image, auth)                                 │
│  2. 提取 release.json → 创建/更新 ReleaseImage CR            │
│  3. 提取 upgrade-graph.json → 解析升级依赖图                   │
│  4. 提取 manifests/ → 暂存                                   │
│  5. 提取 scripts/ → 暂存                                    │
│  6. 验证 contentHash + 签名                                  │
│  7. 验证 previousVersions 包含 actualVersion                 │
└─────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 4: 前置验证                                             │
│                                                              │
│  1. 集群健康: 所有节点 Ready                                  │
│  2. 写入 history 条目 (state=Partial)                        │
└─────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 5: 按 upgradeGraph 顺序升级                             │
│                                                              │
│  按 order 排序 phases, 遍历:                                  │
│  ├── 构建组件依赖 DAG (dependsOn)                            │
│  ├── 拓扑排序执行组件                                         │
│  ├── 无依赖的并行, 有依赖的等待                               │
│  └── 每个组件:                                               │
│      ├── manifests → kubectl apply -f                        │
│      ├── scripts → SSH 执行                                  │
│      └── healthCheck → 等待通过 (超时重试)                    │
│                                                              │
│  blocking=true 的 phase 必须全部成功                          │
└─────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 6: 升级完成                                             │
│                                                              │
│  成功:                                                       │
│  ├── actualVersion = desiredVersion                          │
│  ├── history[0].state = Completed                            │
│  ├── Available=True, Progressing=False, Failing=False        │
│  └── 清理临时文件                                             │
│                                                              │
│  失败:                                                       │
│  ├── Failing=True, history[0].state = Partial                │
│  └── 等待用户干预 (修改 desiredUpdate 或 force)               │
└─────────────────────────────────────────────────────────────┘
```

### 7.9 Controller 调和逻辑

```go
func (r *ClusterVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cv := &infrav1.ClusterVersion{}
	if err := r.Get(ctx, req.NamespacedName, cv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper := patch.NewHelper(cv, r.Client)
	defer patchHelper.Patch(ctx, cv)

	// 1. 同步 UpgradePath (拉取 OCI 镜像)
	if err := r.syncUpgradePath(ctx, cv); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// 2. 同步 ReleaseCatalog (拉取 OCI 镜像)
	if err := r.syncReleaseCatalog(ctx, cv); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// 3. 计算 availableUpdates
	r.computeAvailableUpdates(ctx, cv)

	// 4. 检查是否已达到目标版本
	if cv.Spec.DesiredUpdate == nil || cv.Status.ActualVersion == cv.Spec.DesiredUpdate.Version {
		meta.SetStatusCondition(&cv.Status.Conditions, metav1.Condition{
			Type: infrav1.UpgradeAvailable, Status: metav1.ConditionTrue, Reason: "AsExpected",
		})
		meta.SetStatusCondition(&cv.Status.Conditions, metav1.Condition{
			Type: infrav1.UpgradeProgressing, Status: metav1.ConditionFalse, Reason: "AsExpected",
		})
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// 5. 前置验证
	if err := r.validateUpgrade(ctx, cv); err != nil {
		meta.SetStatusCondition(&cv.Status.Conditions, metav1.Condition{
			Type: infrav1.UpgradeFailing, Status: metav1.ConditionTrue,
			Reason: "ValidationFailed", Message: err.Error(),
		})
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// 6. 拉取 ReleaseImage OCI 镜像
	releaseImage, err := r.fetchReleaseImage(ctx, cv)
	if err != nil {
		meta.SetStatusCondition(&cv.Status.Conditions, metav1.Condition{
			Type: infrav1.UpgradeFailing, Status: metav1.ConditionTrue,
			Reason: "PullFailed", Message: err.Error(),
		})
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// 7. 更新 history
	cv.Status.History = prependHistory(cv.Status.History, infrav1.UpdateHistory{
		State: infrav1.PartialUpdate, Version: cv.Spec.DesiredUpdate.Version,
		Image: releaseImage.Spec.Image, Verified: true, StartedTime: metav1.Now(),
	})

	// 8. 执行升级图
	if err := r.executeUpgradeGraph(ctx, cv, releaseImage); err != nil {
		meta.SetStatusCondition(&cv.Status.Conditions, metav1.Condition{
			Type: infrav1.UpgradeFailing, Status: metav1.ConditionTrue,
			Reason: "UpgradeFailed", Message: err.Error(),
		})
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// 9. 升级完成
	cv.Status.ActualVersion = cv.Spec.DesiredUpdate.Version
	cv.Status.History[0].State = infrav1.CompletedUpdate
	cv.Status.History[0].CompletionTime = ptr.To(metav1.Now())
	meta.SetStatusCondition(&cv.Status.Conditions, metav1.Condition{
		Type: infrav1.UpgradeAvailable, Status: metav1.ConditionTrue, Reason: "AsExpected",
	})
	meta.SetStatusCondition(&cv.Status.Conditions, metav1.Condition{
		Type: infrav1.UpgradeProgressing, Status: metav1.ConditionFalse, Reason: "AsExpected",
	})

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}
```

### 7.10 安装流程

```
新集群创建
    │
    ├── 1. 查询 ReleaseCatalog.status.channels[stable-v1.31]
    │   → 获取最新版本 "v1.32.0"
    │
    ├── 2. 从 ReleaseCatalog 获取 image 地址
    │
    ├── 3. 拉取 ReleaseImage OCI 镜像
    │
    ├── 4. 解析 components 获取所有组件版本
    │
    ├── 5. 按 upgradeGraph 执行安装
    │
    └── 6. 创建 ClusterVersion CR (记录初始版本)
```

### 7.11 资源职责总结

| 问题 | 查询的资源 | 字段路径 |
|------|-----------|---------|
| 有哪些版本可用? | ReleaseCatalog | `status.releases` |
| v1.32.0 的镜像地址? | ReleaseCatalog | `status.releases[].image` |
| 从 v1.31.0 能升级到 v1.32.0 吗? | UpgradePath | `spec.graph.edges` |
| v1.32.0 包含哪些组件? | ReleaseImage | `spec.components` |
| v1.32.0 的升级顺序? | ReleaseImage | `spec.upgradeGraph` |
| 当前集群升级状态? | ClusterVersion | `status.actualVersion`, `status.history`, `status.conditions` |
| 安装时应该用哪个版本? | ReleaseCatalog | `status.channels[stable-v1.31]` 的最新版本 |

### 7.12 实施计划

#### 7.12.1 新增文件

| 文件 | 说明 |
|------|------|
| `api/v1beta1/clusterversion_types.go` | ClusterVersion CRD 类型 |
| `api/v1beta1/releaseimage_types.go` | ReleaseImage CRD 类型 |
| `api/v1beta1/upgradepath_types.go` | UpgradePath CRD 类型 |
| `api/v1beta1/releasecatalog_types.go` | ReleaseCatalog CRD 类型 |
| `internal/controllers/clusterversion_controller.go` | ClusterVersion Controller |
| `internal/upgrader/oci_puller.go` | OCI 镜像拉取 |
| `internal/upgrader/graph_executor.go` | 升级图执行器 |
| `internal/upgrader/health_checker.go` | 健康检查 |
| `config/crd/bases/infrastructure.cluster.x-k8s.io_clusterversions.yaml` | CRD YAML |
| `config/crd/bases/infrastructure.cluster.x-k8s.io_releaseimages.yaml` | CRD YAML |
| `config/crd/bases/infrastructure.cluster.x-k8s.io_upgradepaths.yaml` | CRD YAML |
| `config/crd/bases/infrastructure.cluster.x-k8s.io_releasecatalogs.yaml` | CRD YAML |

#### 7.12.2 开发阶段

| 阶段 | 内容 | 工作量 |
|------|------|--------|
| **Phase 15** | CRD 类型定义 (ClusterVersion/ReleaseImage/UpgradePath/ReleaseCatalog) | 1.5 周 |
| **Phase 16** | OCI 镜像拉取模块 (go-containerregistry 集成) | 1 周 |
| **Phase 17** | ClusterVersion Controller 基础调和逻辑 | 1.5 周 |
| **Phase 18** | 升级图执行器 (DAG 拓扑排序 + 并行执行) | 1.5 周 |
| **Phase 19** | HealthCheck 模块 (6 种检查类型) | 1 周 |
| **Phase 20** | 升级历史、Conditions、状态上报 | 1 周 |
| **Phase 21** | E2E 测试 + 文档 | 2 周 |
| **总计** | | **9.5 周** |
