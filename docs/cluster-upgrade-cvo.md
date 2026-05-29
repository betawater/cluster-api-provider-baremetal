## 七、集群升级设计 (CVO 机制)

参考 OpenShift Cluster Version Operator (CVO) 机制，设计裸金属集群的升级管理能力。

### 7.1 核心架构

采用四个 CRD 资源，职责清晰、不耦合：

| 资源 | 作用域 | 数量 | 核心职责 |
|------|--------|------|---------|
| **ClusterVersion** | 每集群 | 每集群 1 个 | 升级目标、策略、状态、历史 |
| **ReleaseImage** | 每版本 | 每版本 1 个 | 组件版本映射、升级依赖图、Manifest |
| **UpgradePath** | 全局 | 全局 1 个 | 升级图 (edges)、兼容性规则 |
| **ReleaseCatalog** | 全局 | 全局 1 个 | 所有已发布版本列表、通道索引 |

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
│                                                                     │
│  ┌──────────────────┐                                              │
│  │ BareMetalMachine │                                              │
│  │ (node-01)        │                                              │
│  │                  │                                              │
│  │ spec:            │                                              │
│  │   releaseImageRef│─── 从 ReleaseImage 获取组件版本               │
│  │   componentInstall│─── 保留运行时配置 (registry mirrors 等)       │
│  │ status:          │                                              │
│  │   installedComponents                                           │
│  └──────────────────┘                                              │
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
    - state: "Partial"
      version: "v1.32.0"
      image: "registry.example.com/capbm/release:v1.32.0"
      verified: true
      startedTime: "2024-02-20T14:00:00Z"
  conditions:
    - type: "Available"
      status: "True"
      reason: "AsExpected"
    - type: "Progressing"
      status: "True"
      reason: "Upgrading"
      message: "Upgrading to v1.32.0"
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
    - name: "containerd"
      version: "1.7.12"
      targetVersion: "1.7.13"
      phase: "Completed"
    - name: "kubelet"
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
    - phase: "ControlPlane"
      order: 200
      blocking: true
      components:
        - name: "etcd"
          manifests: ["control-plane/etcd.yaml"]
          blocking: true
        - name: "kube-apiserver"
          manifests: ["control-plane/apiserver.yaml"]
          blocking: true
          dependsOn: ["etcd"]
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
        - name: "ceph-csi"
          manifests: ["infrastructure/ceph-csi.yaml"]
          dependsOn: ["kube-apiserver"]
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
	UpgradeAvailable   clusterv1.ConditionType = "Available"
	UpgradeProgressing clusterv1.ConditionType = "Progressing"
	UpgradeFailing     clusterv1.ConditionType = "Failing"
	UpgradeUpgradeable clusterv1.ConditionType = "Upgradeable"
	UpgradeRetrieved   clusterv1.ConditionType = "RetrievedUpdates"
)

// ==================== ReleaseImage ====================

type ReleaseImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   ReleaseImageSpec   `json:"spec,omitempty"`
	Status ReleaseImageStatus `json:"status,omitempty"`
}

type ReleaseImageSpec struct {
	Version          string                 `json:"version"`
	Image            string                 `json:"image"`
	Channels         []string               `json:"channels,omitempty"`
	PreviousVersions []string               `json:"previousVersions,omitempty"`
	Components       ReleaseComponentVersions `json:"components"`
	UpgradeGraph     []UpgradePhase         `json:"upgradeGraph"`
	ContentHash      string                 `json:"contentHash,omitempty"`
}

type ReleaseComponentVersions struct {
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
	LastSyncTime  metav1.Time               `json:"lastSyncTime,omitempty"`
	SyncSucceeded bool                      `json:"syncSucceeded"`
	ImageDigest   string                    `json:"imageDigest,omitempty"`
	Releases      []ReleaseEntry            `json:"releases,omitempty"`
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

### 7.8 版本检测设计 (核心)

#### 7.8.1 核心原则

**不使用 shell 命令获取组件版本**，所有版本信息直接从 ReleaseImage 的 `spec.components` 获取。

#### 7.8.2 版本检测流程

```
┌─────────────────────────────────────────────────────────────┐
│ 1. 获取当前 ReleaseImage (基于 actualVersion)                │
│    currentRI = GetReleaseImage("v1.31.0")                   │
│                                                             │
│ 2. 获取目标 ReleaseImage (基于 desiredUpdate.version)        │
│    targetRI = GetReleaseImage("v1.32.0")                    │
│                                                             │
│ 3. 对比两个 ReleaseImage 的 components (纯内存操作)          │
│    diff = DiffComponents(currentRI, targetRI)               │
│                                                             │
│ 4. 生成升级计划                                              │
│    diff.changed = ["kubelet", "containerd", "calico"]       │
│    diff.unchanged = ["kubeadm", "kubectl"]                  │
│                                                             │
│ 5. 只升级 changed 列表中的组件                               │
│    按 upgradeGraph 顺序执行                                  │
└─────────────────────────────────────────────────────────────┘
```

#### 7.8.3 DiffComponents 实现

```go
type ComponentDiff struct {
	Changed   []ComponentChange `json:"changed"`
	Unchanged []string          `json:"unchanged"`
	Added     []string          `json:"added"`
	Removed   []string          `json:"removed"`
}

type ComponentChange struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"currentVersion"`
	TargetVersion  string `json:"targetVersion"`
}

func DiffComponents(current, target *infrav1.ReleaseImage) *ComponentDiff {
	diff := &ComponentDiff{}

	// 对比 containerd
	if current.Spec.Components.Containerd != target.Spec.Components.Containerd {
		if target.Spec.Components.Containerd != "" {
			diff.Changed = append(diff.Changed, ComponentChange{
				Name:           "containerd",
				CurrentVersion: current.Spec.Components.Containerd,
				TargetVersion:  target.Spec.Components.Containerd,
			})
		}
	} else if current.Spec.Components.Containerd != "" {
		diff.Unchanged = append(diff.Unchanged, "containerd")
	}

	// 对比 kubernetes 子组件
	for name, targetVer := range target.Spec.Components.Kubernetes {
		currentVer := current.Spec.Components.Kubernetes[name]
		if currentVer != targetVer {
			diff.Changed = append(diff.Changed, ComponentChange{
				Name:           name,
				CurrentVersion: currentVer,
				TargetVersion:  targetVer,
			})
		} else {
			diff.Unchanged = append(diff.Unchanged, name)
		}
	}

	// 对比 CNI/CSI 组件
	cniComponents := map[string]string{
		"calico":  target.Spec.Components.Calico,
		"cilium":  target.Spec.Components.Cilium,
		"cephCsi": target.Spec.Components.CephCsi,
	}
	for name, targetVer := range cniComponents {
		currentVer := getComponentVersionByName(current, name)
		if currentVer != targetVer && targetVer != "" {
			diff.Changed = append(diff.Changed, ComponentChange{
				Name:           name,
				CurrentVersion: currentVer,
				TargetVersion:  targetVer,
			})
		} else if targetVer != "" {
			diff.Unchanged = append(diff.Unchanged, name)
		}
	}

	return diff
}
```

#### 7.8.4 升级场景示例

**场景 1: 小版本升级 (v1.31.0 → v1.31.1)**
```
currentRI (v1.31.0):
  components:
    kubernetes:
      kubelet: "v1.31.0"
      kubeadm: "v1.31.0"
      kubectl: "v1.31.0"
    containerd: "1.7.12"

targetRI (v1.31.1):
  components:
    kubernetes:
      kubelet: "v1.31.1"    # 变化
      kubeadm: "v1.31.1"    # 变化
      kubectl: "v1.31.1"    # 变化
    containerd: "1.7.12"    # 不变

diff:
  changed: [kubelet, kubeadm, kubectl]
  unchanged: [containerd]

→ 只升级 kubelet, kubeadm, kubectl，跳过 containerd
```

**场景 2: 大版本升级 (v1.31.0 → v1.32.0)**
```
currentRI (v1.31.0):
  components:
    kubernetes:
      kubelet: "v1.31.0"
      kubeadm: "v1.31.0"
    containerd: "1.7.12"
    calico: "3.26.1"

targetRI (v1.32.0):
  components:
    kubernetes:
      kubelet: "v1.32.0"    # 变化
      kubeadm: "v1.32.0"    # 变化
    containerd: "1.7.13"    # 变化
    calico: "3.27.0"        # 变化

diff:
  changed: [kubelet, kubeadm, containerd, calico]

→ 按 upgradeGraph 顺序升级: containerd → kubelet → calico
```

**场景 3: 新节点安装**
```
currentVersion = "" → currentRI = targetRI
diff.changed = [] → 执行完整安装
```

### 7.9 完整升级流程

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

### 7.10 Controller 调和逻辑

#### 7.10.1 ClusterVersion Controller

```go
func (r *ClusterVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cv := &infrav1.ClusterVersion{}
	if err := r.Get(ctx, req.NamespacedName, cv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, _ := patch.NewHelper(cv, r.Client)
	defer patchHelper.Patch(ctx, cv)

	// 1. 同步 UpgradePath
	if err := r.syncUpgradePath(ctx, cv); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// 2. 同步 ReleaseCatalog
	if err := r.syncReleaseCatalog(ctx, cv); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// 3. 计算 availableUpdates
	r.computeAvailableUpdates(ctx, cv)

	// 4. 检查是否已达到目标版本
	if cv.Spec.DesiredUpdate == nil || cv.Status.ActualVersion == cv.Spec.DesiredUpdate.Version {
		setCVCondition(cv, infrav1.UpgradeAvailable, metav1.ConditionTrue, "AsExpected", "")
		setCVCondition(cv, infrav1.UpgradeProgressing, metav1.ConditionFalse, "AsExpected", "")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// 5. 前置验证
	if err := r.validateUpgrade(ctx, cv); err != nil {
		setCVCondition(cv, infrav1.UpgradeFailing, metav1.ConditionTrue, "ValidationFailed", err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// 6. 拉取 ReleaseImage
	releaseImage, err := r.fetchReleaseImage(ctx, cv)
	if err != nil {
		setCVCondition(cv, infrav1.UpgradeFailing, metav1.ConditionTrue, "PullFailed", err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// 7. 更新 history
	cv.Status.History = prependHistory(cv.Status.History, infrav1.UpdateHistory{
		State: infrav1.PartialUpdate, Version: cv.Spec.DesiredUpdate.Version,
		Image: releaseImage.Spec.Image, Verified: true, StartedTime: metav1.Now(),
	})

	// 8. 执行升级图
	if err := r.executeUpgradeGraph(ctx, cv, releaseImage); err != nil {
		setCVCondition(cv, infrav1.UpgradeFailing, metav1.ConditionTrue, "UpgradeFailed", err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// 9. 升级完成
	cv.Status.ActualVersion = cv.Spec.DesiredUpdate.Version
	cv.Status.History[0].State = infrav1.CompletedUpdate
	now := metav1.Now()
	cv.Status.History[0].CompletionTime = &now
	setCVCondition(cv, infrav1.UpgradeAvailable, metav1.ConditionTrue, "AsExpected", "")
	setCVCondition(cv, infrav1.UpgradeProgressing, metav1.ConditionFalse, "AsExpected", "")

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}
```

#### 7.10.2 BareMetalMachine Controller (安装集成)

```go
func (r *BareMetalMachineReconciler) installComponents(ctx context.Context, bmMachine *infrav1.BareMetalMachine, sshConn *ssh.SSHConnection) (*installer.InstallResult, error) {
	// 1. 获取当前版本和目标版本
	clusterName := bmMachine.Labels[clusterv1.ClusterNameLabel]
	cv := &infrav1.ClusterVersion{}
	if err := r.Get(ctx, types.NamespacedName{Name: clusterName}, cv); err != nil {
		return nil, err
	}

	currentVersion := cv.Status.ActualVersion
	targetVersion := currentVersion
	if cv.Spec.DesiredUpdate != nil && cv.Spec.DesiredUpdate.Version != "" {
		targetVersion = cv.Spec.DesiredUpdate.Version
	}

	// 2. 获取对应的 ReleaseImage
	currentRI, err := r.getReleaseImageByVersion(ctx, currentVersion)
	if err != nil {
		currentRI, err = r.getReleaseImageByVersion(ctx, targetVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to get target ReleaseImage: %w", err)
		}
	}

	targetRI, err := r.getReleaseImageByVersion(ctx, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get target ReleaseImage: %w", err)
	}

	// 3. 创建 Installer 并执行
	config := bmMachine.Spec.ComponentInstall
	if config == nil {
		config = &infrav1.ComponentInstallConfig{
			Enabled:  true,
			Strategy: infrav1.InstallIfMissing,
			ContainerRuntime: infrav1.ContainerRuntimeConfig{Type: "containerd"},
			MaxRetries: 3,
		}
	}

	role := bmMachine.Spec.Role
	if role == "" {
		role = "worker"
	}

	inst := installer.New(sshConn, currentRI, targetRI, config, role)
	return inst.Install(ctx)
}
```

### 7.11 Installer 重构

#### 7.11.1 新 Installer 结构

```go
type Installer struct {
	sshConn       *sshclient.SSHConnection
	currentRI     *infrav1.ReleaseImage
	targetRI      *infrav1.ReleaseImage
	config        *infrav1.ComponentInstallConfig
	role          string
	componentDiff *ComponentDiff
}

func New(sshConn *sshclient.SSHConnection,
	currentRI, targetRI *infrav1.ReleaseImage,
	config *infrav1.ComponentInstallConfig,
	role string) *Installer {

	return &Installer{
		sshConn:       sshConn,
		currentRI:     currentRI,
		targetRI:      targetRI,
		config:        config,
		role:          role,
		componentDiff: DiffComponents(currentRI, targetRI),
	}
}
```

#### 7.11.2 安装流程

```go
func (i *Installer) Install(ctx context.Context) (*InstallResult, error) {
	// 1. 如果没有变化，直接返回
	if len(i.componentDiff.Changed) == 0 && len(i.componentDiff.Added) == 0 {
		return &InstallResult{
			Completed: true,
			Success:   true,
			Progress:  "All components already at target versions",
		}, nil
	}

	// 2. 构建需要升级的组件集合
	needsUpgrade := make(map[string]bool)
	for _, c := range i.componentDiff.Changed {
		needsUpgrade[c.Name] = true
	}
	for _, name := range i.componentDiff.Added {
		needsUpgrade[name] = true
	}

	// 3. 按 upgradeGraph 顺序执行升级 (只升级需要升级的组件)
	for _, phase := range i.targetRI.Spec.UpgradeGraph {
		if phase.Name != "NodeComponents" {
			continue
		}

		for _, comp := range phase.Components {
			if !needsUpgrade[comp.Name] {
				log.Info("Skipping component (version unchanged)", "component", comp.Name)
				continue
			}

			if err := i.executeComponent(ctx, comp); err != nil {
				if comp.Blocking {
					return &InstallResult{
						Completed: false,
						Success:   false,
						Progress:  fmt.Sprintf("Component %s upgrade failed", comp.Name),
						Error:     err.Error(),
					}, err
				}
			}
		}
	}

	return &InstallResult{
		Completed: true,
		Success:   true,
		Progress:  fmt.Sprintf("Upgraded %d components", len(i.componentDiff.Changed)),
	}, nil
}
```

### 7.12 安装流程

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

### 7.13 资源职责总结

| 问题 | 查询的资源 | 字段路径 |
|------|-----------|---------|
| 有哪些版本可用? | ReleaseCatalog | `status.releases` |
| v1.32.0 的镜像地址? | ReleaseCatalog | `status.releases[].image` |
| 从 v1.31.0 能升级到 v1.32.0 吗? | UpgradePath | `spec.graph.edges` |
| v1.32.0 包含哪些组件? | ReleaseImage | `spec.components` |
| v1.32.0 的升级顺序? | ReleaseImage | `spec.upgradeGraph` |
| 当前集群升级状态? | ClusterVersion | `status.actualVersion`, `status.history`, `status.conditions` |
| 安装时应该用哪个版本? | ReleaseCatalog | `status.channels[stable-v1.31]` 的最新版本 |
| 哪些组件需要升级? | ReleaseImage (current vs target) | `DiffComponents(currentRI, targetRI)` |

### 7.14 实施计划

#### 7.14.1 新增文件

| 文件 | 说明 |
|------|------|
| `api/v1beta1/clusterversion_types.go` | ClusterVersion CRD 类型 |
| `api/v1beta1/releaseimage_types.go` | ReleaseImage CRD 类型 |
| `api/v1beta1/upgradepath_types.go` | UpgradePath CRD 类型 |
| `api/v1beta1/releasecatalog_types.go` | ReleaseCatalog CRD 类型 |
| `api/v1beta1/upgrade_deepcopy.go` | 手动 DeepCopy 实现 |
| `internal/upgrader/oci_puller.go` | OCI 镜像拉取 |
| `internal/upgrader/graph_executor.go` | 升级图执行器 |
| `internal/upgrader/health_checker.go` | 健康检查 |
| `internal/controllers/clusterversion_controller.go` | ClusterVersion Controller |

#### 7.14.2 修改文件

| 文件 | 说明 |
|------|------|
| `api/v1beta1/groupversion_info.go` | 注册新 CRD 类型 |
| `api/v1beta1/baremetalmachine_types.go` | 新增 ReleaseImageRef 字段 |
| `internal/controllers/baremetalmachine_controller.go` | 集成 ReleaseImage 安装 |
| `internal/installer/installer.go` | 重构为 ReleaseImage 驱动 |
| `cmd/main.go` | 注册 ClusterVersion Controller |

#### 7.14.3 开发阶段

| 阶段 | 内容 | 工作量 |
|------|------|--------|
| **Phase 1** | CRD 类型定义 + DeepCopy | 1.5 周 |
| **Phase 2** | OCI 拉取模块 | 1 周 |
| **Phase 3** | ClusterVersion Controller | 1.5 周 |
| **Phase 4** | 升级图执行器 + 健康检查 | 1.5 周 |
| **Phase 5** | Installer 重构 (ReleaseImage 驱动) | 1.5 周 |
| **Phase 6** | BareMetalMachine Controller 集成 | 1 周 |
| **Phase 7** | E2E 测试 + 文档 | 2 周 |
| **总计** | | **10 周** |
