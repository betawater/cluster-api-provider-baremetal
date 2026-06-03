## 七、集群升级设计 (CVO 机制)

参考 OpenShift Cluster Version Operator (CVO) 机制，设计裸金属集群的升级管理能力。

### 7.1 核心架构

采用四个 CRD 资源，职责清晰、不耦合：

| 资源 | 作用域 | 数量 | API Group | 核心职责 |
|------|--------|------|-----------|---------|
| **ClusterVersion** | 每集群 | 每集群 1 个 | `cvo.capbm.io` | 升级目标、策略、状态、历史 |
| **ReleaseImage** | 每版本 | 每版本 1 个 | `cvo.capbm.io` | 组件版本映射、升级依赖图、Manifest |
| **UpgradePath** | 全局 | 全局 1 个 | `cvo.capbm.io` | 升级图 (edges)、兼容性规则 |
| **ReleaseCatalog** | 全局 | 全局 1 个 | `cvo.capbm.io` | 所有已发布版本列表、通道索引 |

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
│  │         ClusterVersion Controller (CVO)           │             │
│  │  modules/cvo/internal/controllers/                │             │
│  │                                                   │             │
│  │  1. 从 ReleaseCatalog 查询目标版本信息             │             │
│  │  2. 从 UpgradePath 验证升级路径合法性              │             │
│  │  3. 从 ReleaseImage 获取升级依赖图和 Manifest      │             │
│  │  4. 执行升级 (K8S + Addon)                         │             │
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
│  │   addonStatus    │              └──────────────────┘            │
│  └──────────────────┘                                              │
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

### 7.2 CVO 模块代码位置

| 组件 | 代码路径 |
|------|---------|
| ClusterVersion CRD | `modules/cvo/api/v1beta1/clusterversion_types.go` |
| ReleaseImage CRD | `modules/cvo/api/v1beta1/releaseimage_types.go` |
| UpgradePath CRD | `modules/cvo/api/v1beta1/upgradepath_types.go` |
| ReleaseCatalog CRD | `modules/cvo/api/v1beta1/releasecatalog_types.go` |
| ClusterAddon CRD | `modules/cvo/api/v1beta1/clusteraddon_types.go` |
| ClusterVersion Controller | `modules/cvo/internal/controllers/clusterversion_controller.go` |
| Graph Executor | `modules/cvo/internal/upgrader/graph_executor.go` |
| Addon Upgrader | `modules/cvo/internal/addon/upgrader.go` |
| Backup/Rollback | `modules/cvo/internal/upgrader/backup_rollback.go` |

### 7.3 ClusterVersion CRD 设计

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
  channel: stable
  desiredUpdate:
    version: v1.31.1
    image: registry.example.com/capbm/release:v1.31.1
    force: false
status:
  observedGeneration: 1
  desired:
    version: v1.31.1
    image: registry.example.com/capbm/release:v1.31.1
  actualVersion: v1.31.0
  history:
    - state: Completed
      version: v1.31.0
      image: registry.example.com/capbm/release:v1.31.0
      verified: true
      startedTime: "2024-01-15T10:30:00Z"
      completionTime: "2024-01-15T10:45:00Z"
  conditions:
    - type: Available
      status: "True"
      reason: AsExpected
    - type: Progressing
      status: "False"
      reason: AsExpected
  availableUpdates:
    - version: v1.31.1
      image: registry.example.com/capbm/release:v1.31.1
  componentStatus:
    - name: containerd
      version: 1.7.24
      targetVersion: 1.7.24
      phase: Pending
    - name: kubernetes
      version: v1.31.0
      targetVersion: v1.31.1
      phase: Pending
  addonStatus:
    - name: calico
      version: v3.28.0
      targetVersion: v3.28.1
      phase: Upgrading
    - name: ceph-csi
      version: v3.11.0
      phase: Installed
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
    ObservedGeneration int64                `json:"observedGeneration"`
    Desired            Release              `json:"desired"`
    ActualVersion      string               `json:"actualVersion"`
    History            []UpdateHistory      `json:"history,omitempty"`
    Conditions         []metav1.Condition   `json:"conditions,omitempty"`
    AvailableUpdates   []Release            `json:"availableUpdates,omitempty"`
    ComponentStatus    []ComponentStatus    `json:"componentStatus,omitempty"`
    AddonStatus        []AddonVersionStatus `json:"addonStatus,omitempty"`
}
```

### 7.4 ReleaseImage CRD 设计

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
      version: v3.28.1
      contentPath: charts/calico-v3.28.1.tgz
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

### 7.5 升级触发机制

#### 7.5.1 K8S + Addon 同时升级

```yaml
# 更新 DesiredUpdate.Version 触发 K8S + Addon 升级
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

#### 7.5.2 仅 Addon 升级 (K8S 版本不变)

```yaml
# 更新 DesiredUpdate.Image (K8S 版本不变，但指向新 ReleaseImage)
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

### 7.6 升级执行流程

```go
// modules/cvo/internal/controllers/clusterversion_controller.go
func (r *ClusterVersionReconciler) executeUpgrade(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage, needsK8SUpgrade bool) error {
    // Phase 1: K8S 升级 (仅当 K8S 版本变更时)
    if needsK8SUpgrade {
        // 初始化 ComponentStatus
        cv.Status.ComponentStatus = r.initComponentStatus(releaseImage)

        // 执行组件升级图
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

### 7.7 Addon 升级触发判断

```go
// modules/cvo/internal/controllers/clusterversion_controller.go
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

### 7.8 Addon 升级执行

```go
// modules/cvo/internal/controllers/clusterversion_controller.go
func (r *ClusterVersionReconciler) executeAddonUpgrades(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) error {
    addonUpgrader := addon.NewUpgrader(r.Client, "", cv.Namespace)

    // 构建 Addon 依赖图
    addonGraph := buildAddonDependencyGraph(releaseImage)
    sortedAddons := topologicalSortAddons(addonGraph)

    // 获取当前 ReleaseImage 用于版本比较
    currentRelease := &cfov1.ReleaseImage{}
    if err := r.Get(ctx, types.NamespacedName{Name: versionToName(cv.Status.ActualVersion)}, currentRelease); err != nil {
        currentRelease = nil  // 如果找不到，视为全新安装
    }

    // 按依赖顺序升级 Addon
    for _, addonName := range sortedAddons {
        addonDef := findAddonDefByName(releaseImage, addonName)
        if addonDef == nil {
            continue
        }

        // 获取或创建 ClusterAddon
        clusterAddon := &cfov1.ClusterAddon{}
        err := r.Get(ctx, types.NamespacedName{Name: addonName, Namespace: cv.Namespace}, clusterAddon)
        if apierrors.IsNotFound(err) {
            // 全新安装
            clusterAddon = &cfov1.ClusterAddon{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      addonName,
                    Namespace: cv.Namespace,
                },
                Spec: cfov1.ClusterAddonSpec{
                    ClusterRef:      cv.Spec.ClusterRef,
                    ReleaseImageRef: corev1.LocalObjectReference{Name: releaseImage.Name},
                    AddonName:       addonDef.Name,
                    Namespace:       addonDef.Namespace,
                },
            }
            if err := r.Client.Create(ctx, clusterAddon); err != nil {
                return fmt.Errorf("failed to create addon %s: %w", addonName, err)
            }
        } else if err != nil {
            return err
        }

        // 如果已经是目标版本，跳过
        if clusterAddon.Status.Version == addonDef.Version {
            continue
        }

        // 执行升级
        if err := addonUpgrader.Upgrade(ctx, clusterAddon, currentRelease, releaseImage); err != nil {
            return fmt.Errorf("failed to upgrade addon %s: %w", addonName, err)
        }
    }

    return nil
}
```

### 7.9 升级状态追踪

```go
// modules/cvo/internal/controllers/clusterversion_controller.go
func (r *ClusterVersionReconciler) updateAddonStatus(cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) {
    var addonStatus []cfov1.AddonVersionStatus

    for _, addonDef := range releaseImage.Spec.Addons {
        if addonDef.Version == "" {
            continue
        }

        status := cfov1.AddonVersionStatus{
            Name:          addonDef.Name,
            TargetVersion: addonDef.Version,
            Phase:         cfov1.AddonPhaseInstalled,
        }

        clusterAddon := &cfov1.ClusterAddon{}
        err := r.Get(context.Background(), types.NamespacedName{Name: addonDef.Name, Namespace: cv.Namespace}, clusterAddon)

        if apierrors.IsNotFound(err) {
            status.Phase = cfov1.AddonPhasePending
            status.Version = ""
        } else if err == nil {
            status.Version = clusterAddon.Status.Version
            if clusterAddon.Status.Version != addonDef.Version {
                status.Phase = cfov1.AddonPhaseUpgrading
            }
        }

        status.LastTransitionTime = metav1.Now()
        addonStatus = append(addonStatus, status)
    }

    cv.Status.AddonStatus = addonStatus
}
```

### 7.10 升级条件类型

```go
// modules/cvo/api/v1beta1/clusterversion_types.go
const (
    // K8S 升级条件
    UpgradeAvailable   clusterv1.ConditionType = "Available"
    UpgradeProgressing clusterv1.ConditionType = "Progressing"
    UpgradeFailing     clusterv1.ConditionType = "Failing"
    UpgradeUpgradeable clusterv1.ConditionType = "Upgradeable"
    UpgradeRetrieved   clusterv1.ConditionType = "RetrievedUpdates"

    // Addon 升级条件
    AddonUpgradeProgressing clusterv1.ConditionType = "AddonUpgradeProgressing"
    AddonUpgradeFailing     clusterv1.ConditionType = "AddonUpgradeFailing"
    AddonUpgradeCompleted   clusterv1.ConditionType = "AddonUpgradeCompleted"
)
```

### 7.11 升级路径验证

```go
// modules/cvo/internal/upgrader/graph_executor.go
func (e *GraphExecutor) ValidateUpgradePath(ctx context.Context, cv *cfov1.ClusterVersion) error {
    if cv.Spec.DesiredUpdate == nil || cv.Spec.DesiredUpdate.Version == "" {
        return nil
    }

    upgradePath := &cfov1.UpgradePath{}
    if err := e.client.Get(ctx, types.NamespacedName{Name: "global"}, upgradePath); err != nil {
        return fmt.Errorf("failed to get UpgradePath: %w", err)
    }

    from := cv.Status.ActualVersion
    to := cv.Spec.DesiredUpdate.Version

    if from == to {
        return nil
    }

    found := false
    for _, edge := range upgradePath.Spec.Graph.Edges {
        if matchVersion(edge.From, from) && matchVersion(edge.To, to) {
            found = true
            if !edge.Recommended && !cv.Spec.DesiredUpdate.Force {
                return fmt.Errorf("upgrade from %s to %s is not recommended, use force=true to override", from, to)
            }
            break
        }
    }

    if !found && !cv.Spec.DesiredUpdate.Force {
        return fmt.Errorf("no valid upgrade path from %s to %s", from, to)
    }

    for _, blocked := range upgradePath.Spec.Rules.BlockedUpgrades {
        if matchVersion(blocked.From, from) && matchVersion(blocked.To, to) {
            return fmt.Errorf("upgrade from %s to %s is blocked: %s", from, to, blocked.Reason)
        }
    }

    return nil
}
```

### 7.12 KCP 协调机制

控制面升级时，CVO 通过以下方式与 KCP 协调：

```go
// modules/cvo/internal/upgrader/control_plane_upgrader.go
func (u *ControlPlaneUpgrader) waitForKCPUpgrade(ctx context.Context, cv *cfov1.ClusterVersion) error {
    // 等待 KCP.status.version == desiredVersion
    // 等待 KCP.status.conditions[Ready] == True
    return wait.PollUntilContextTimeout(ctx, 10*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
        kcp := &controlplanev1.KubeadmControlPlane{}
        if err := u.client.Get(ctx, types.NamespacedName{
            Namespace: cv.Namespace,
            Name:      cv.Spec.ClusterRef.Name + "-control-plane",
        }, kcp); err != nil {
            return false, err
        }
        
        return kcp.Status.UpdatedReplicas == kcp.Status.Replicas &&
               kcp.Status.ReadyReplicas == kcp.Status.Replicas, nil
    })
}
```

### 7.13 升级流程图

```
用户修改 ClusterVersion.spec.desiredUpdate.version
    │
    ▼
ClusterVersionReconciler.Reconcile() 被调用
    │
    ▼
第 79 行: 判断是否需要升级
┌─────────────────────────────────────────────────┐
│ if cv.Spec.DesiredUpdate == nil                 │
│    || cv.Status.ActualVersion ==                │
│       cv.Spec.DesiredUpdate.Version {           │
│     // 版本相同，无需升级                        │
│     return RequeueAfter: 5m                     │
│ }                                                │
└─────────────────────────────────────────────────┘
    │
    ▼ (版本不同，继续执行)
validateUpgrade() → preUpgradeHealthCheck() → fetchReleaseImage()
    │
    ▼
executeUpgrade()
    │
    ├── executeAddonUpgrades()              ← Addon 升级
    │
    └── GraphExecutor.ExecuteUpgradeGraph()  ← 组件升级
            │
            └── 遍历 UpgradeGraph Phases
                    │
                    └── Phase: "kubernetes"
                            │
                            └── applyManifests()
                                    │
                                    └── 更新 KubeadmControlPlane.spec.version
                                            │
                                            ▼
                                    KCP Controller 检测到 version 变更
                                            │
                                            ▼
                                    KCP 开始滚动升级控制面节点
```

### 7.14 升级顺序保证

```
Upgrade Flow:
    │
    ├── Phase 1: Addon Upgrades (in dependency order)
    │   ├── calico (no dependencies)
    │   ├── ceph-csi (depends on calico)
    │   └── monitoring (depends on calico)
    │
    └── Phase 2: K8S Component Upgrades
        ├── containerd
        ├── kubernetes (control plane)
        └── kubernetes (worker nodes)
```

### 7.15 关键设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **触发字段** | 新增字段 vs 复用现有 | 复用 `DesiredUpdate` | 保持 CRD 简洁，统一入口 |
| **升级顺序** | Addon 先 vs K8S 先 | K8S 先 | Addon 通常兼容多个 K8S 版本 |
| **版本来源** | ReleaseImage vs 直接指定 | ReleaseImage | 保持单一数据源 |
| **并发控制** | 串行 vs 并行 | 串行 (依赖顺序) | 保证依赖关系 |
| **回滚策略** | 自动 vs 手动 | 手动 | Addon 回滚需要用户确认 |
