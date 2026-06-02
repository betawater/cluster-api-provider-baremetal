# Addon 升级触发机制设计

## 概述

本文档描述基于 `ClusterVersion` 目标版本变更的 Addon 升级触发机制。该机制复用现有的 K8S 版本变更触发流程，在 K8S 升级完成后自动触发 Addon 升级，同时支持 K8S 版本不变时仅触发 Addon 升级的场景。

---

## 1. 当前升级触发机制分析

### 1.1 触发链路

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

### 1.2 关键触发条件

```go
// 触发条件: cv.Status.ActualVersion != cv.Spec.DesiredUpdate.Version
if cv.Spec.DesiredUpdate == nil || cv.Status.ActualVersion == cv.Spec.DesiredUpdate.Version {
    return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}
```

---

## 2. KCP 版本变更触发控制面升级流程

### 2.1 KCP 检测机制 (CAPI 原生)

```
KubeadmControlPlane Controller (CAPI 内置)
    │
    ├── Watch: KubeadmControlPlane 资源
    │
    └── Reconcile Loop:
            │
            ├── 1. 检测 spec.version 变更
            │   └── kcp.Spec.Version != kcp.Status.Version
            │
            ├── 2. 获取当前控制面 Machine 列表
            │   └── 通过 label selector 匹配
            │
            ├── 3. 计算需要升级的节点
            │   └── 比较 Machine 的 version 与 spec.version
            │
            ├── 4. 执行滚动升级策略
            │   ├── MaxSurge: 允许同时创建的新节点数
            │   └── 逐节点升级:
            │       ├── 4.1 创建新 Machine (新版本)
            │       ├── 4.2 等待新 Machine Ready
            │       ├── 4.3 删除旧 Machine
            │       └── 4.4 重复直到所有节点升级
            │
            └── 5. 更新状态
                ├── kcp.Status.Version = spec.Version
                └── kcp.Status.Ready = True
```

### 2.2 CAPBM 与 KCP 的协调流程

```
CAPBM GraphExecutor.executeComponent()
    │
    ├── 步骤 1: applyManifests()
    │   │
    │   └── 应用 K8S manifests 到管理集群
    │       │
    │       └── 更新 KubeadmControlPlane.spec.version
    │           │
    │           ├── Before:
    │           │   spec.version: v1.31.0
    │           │   status.version: v1.31.0
    │           │
    │           └── After:
    │               spec.version: v1.32.0   ← 版本变更
    │               status.version: v1.31.0  ← 尚未升级
    │
    ├── 步骤 2: KCP Controller 自动响应
    │   │
    │   ├── 检测到 spec.version != status.version
    │   │
    │   └── 触发控制面滚动升级:
    │       │
    │       ├── 节点 1 (etcd leader):
    │       │   ├── 创建新 Machine (v1.32.0)
    │       │   ├── 等待 etcd 成员加入
    │       │   ├── 等待 API Server Ready
    │       │   ├── 删除旧 Machine (v1.31.0)
    │       │   └── 等待 etcd 健康检查通过
    │       │
    │       ├── 节点 2:
    │       │   └── 同上...
    │       │
    │       └── 节点 3:
    │           └── 同上...
    │
    ├── 步骤 3: waitForKCPUpgrade()
    │   │
    │   └── 轮询等待:
    │       ├── 条件 1: kcp.Status.Version == spec.Version
    │       ├── 条件 2: kcp.Status.Ready == True
    │       ├── 条件 3: kcp.Status.UpdatedReplicas == kcp.Status.Replicas
    │       └── 超时: 10 分钟
    │
    └── 步骤 4: 继续后续 Phase
        └── worker 节点升级
```

### 2.3 完整时序图

```
User          ClusterVersion      GraphExecutor      KCP (CAPI)        Machines
 │                  │                   │                │                │
 │ 修改 version     │                   │                │                │
 │─────────────────►│                   │                │                │
 │                  │                   │                │                │
 │                  │ executeUpgrade()  │                │                │
 │                  │──────────────────►│                │                │
 │                  │                   │                │                │
 │                  │                   │ applyManifests()               │
 │                  │                   │───────────────►│                │
 │                  │                   │  (patch KCP    │                │
 │                  │                   │   spec.version)│                │
 │                  │                   │                │                │
 │                  │                   │                │ 检测 version 变更
 │                  │                   │                │───────────────►│
 │                  │                   │                │                │
 │                  │                   │                │  滚动升级节点 1
 │                  │                   │                │ ◄─────────────►│
 │                  │                   │                │                │
 │                  │                   │                │  滚动升级节点 2
 │                  │                   │                │ ◄─────────────►│
 │                  │                   │                │                │
 │                  │                   │                │  滚动升级节点 3
 │                  │                   │                │ ◄─────────────►│
 │                  │                   │                │                │
 │                  │                   │ waitForKCP()   │                │
 │                  │                   │◄───────────────│                │
 │                  │                   │  (poll status) │                │
 │                  │                   │                │                │
 │                  │                   │◄───────────────│                │
 │                  │                   │  upgrade done  │                │
 │                  │◄──────────────────│                │                │
 │                  │  complete         │                │                │
 │◄─────────────────│                   │                │                │
 │  done            │                   │                │                │
```

---

## 3. Addon 升级触发方案设计

### 3.1 核心设计原则

| 原则 | 说明 |
|------|------|
| **统一触发** | 通过 `ClusterVersion.Spec.DesiredUpdate` 变更触发 |
| **K8S 优先** | K8S 升级先于 Addon 升级执行 |
| **版本对比** | 比较 `ClusterAddon.Status.Version` 与 `ReleaseImage.Addon.Version` |
| **幂等性** | 版本相同时跳过升级 |
| **依赖顺序** | Addon 按依赖关系拓扑排序依次升级 |

### 3.2 触发条件判断

```
Reconcile Loop
    │
    ├── 获取目标 ReleaseImage
    │
    ├── 判断 K8S 升级
    │   └── cv.Status.ActualVersion != cv.Spec.DesiredUpdate.Version
    │
    └── 判断 Addon 升级
        └── 遍历 releaseImage.Spec.Addons:
            ├── 获取当前 ClusterAddon
            ├── 如果 ClusterAddon 不存在 → 需要安装
            ├── 如果 ClusterAddon.Status.Version != addonDef.Version → 需要升级
            └── 否则 → 跳过
```

### 3.3 升级执行流程

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

---

## 4. 使用场景

### 4.1 场景 A: K8S + Addon 同时升级

```yaml
# 初始状态
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
status:
  actualVersion: v1.31.0
  addonStatus:
    - name: calico
      version: v3.27.0
      phase: Installed

# 用户修改 DesiredUpdate
spec:
  desiredUpdate:
    version: v1.32.0  # K8S 版本变更

# 触发:
#   Phase 1: K8S v1.31.0 → v1.32.0
#   Phase 2: calico v3.27.0 → v3.28.0 (从 ReleaseImage v1.32.0 获取)
```

### 4.2 场景 B: 仅 Addon 升级 (K8S 版本不变)

```yaml
# 初始状态
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
status:
  actualVersion: v1.31.0
  addonStatus:
    - name: calico
      version: v3.27.0
      phase: Installed

# 用户修改 DesiredUpdate (K8S 版本不变，但指向新 ReleaseImage)
spec:
  desiredUpdate:
    version: v1.31.0     # K8S 版本不变
    image: registry.example.com/capbm/release:v1.31.0-patch1  # 新 ReleaseImage

# 触发:
#   Phase 1: 跳过 (K8S 版本未变)
#   Phase 2: calico v3.27.0 → v3.27.1 (从新 ReleaseImage 获取)
```

### 4.3 场景 C: 多 Addon 升级 (依赖顺序)

```yaml
# ReleaseImage 定义
spec:
  addons:
    - name: calico
      version: v3.28.0
      dependencies: []
    - name: ceph-csi
      version: v3.11.0
      dependencies: [calico]
    - name: monitoring
      version: v0.70.0
      dependencies: [calico]

# 触发顺序:
#   1. calico (无依赖)
#   2. ceph-csi (依赖 calico)
#   3. monitoring (依赖 calico)
```

---

## 5. 状态追踪设计

### 5.1 ClusterVersionStatus 扩展

```go
type ClusterVersionStatus struct {
    // ... existing fields ...
    
    // AddonStatus tracks addon versions after upgrade.
    // +optional
    AddonStatus []AddonVersionStatus `json:"addonStatus,omitempty"`
}

type AddonVersionStatus struct {
    Name    string `json:"name"`
    Version string `json:"version"`
    Phase   string `json:"phase"`  // Installed, Upgrading, Failed
}
```

### 5.2 Status 示例

```yaml
status:
  actualVersion: v1.31.0
  addonStatus:
    - name: calico
      version: v3.28.0
      phase: Installed
    - name: ceph-csi
      version: v3.10.0
      phase: Upgrading
    - name: monitoring
      version: v0.69.0
      phase: Installed
  conditions:
    - type: AddonUpgradeProgressing
      status: "True"
      reason: UpgradingCephCSI
      message: "Upgrading ceph-csi from v3.10.0 to v3.11.0"
```

---

## 6. 关键设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **触发字段** | 新增字段 vs 复用现有 | 复用 `DesiredUpdate` | 保持 CRD 简洁，统一入口 |
| **升级顺序** | Addon 先 vs K8S 先 | K8S 先 | Addon 通常兼容多个 K8S 版本 |
| **版本来源** | ReleaseImage vs 直接指定 | ReleaseImage | 保持单一数据源 |
| **并发控制** | 串行 vs 并行 | 串行 (依赖顺序) | 保证依赖关系 |
| **回滚策略** | 自动 vs 手动 | 手动 | Addon 回滚需要用户确认 |

---

## 7. 实施步骤

| 阶段 | 内容 | 文件 |
|------|------|------|
| **Phase 1** | 扩展 `ClusterVersionStatus` 添加 `AddonStatus` | `modules/cvo/api/v1beta1/clusterversion_types.go` |
| **Phase 2** | 添加 Addon 升级条件类型 | `modules/cvo/api/v1beta1/clusterversion_types.go` |
| **Phase 3** | 实现 `needsAddonUpgrade()` 判断逻辑 | `modules/cvo/internal/controllers/clusterversion_controller.go` |
| **Phase 4** | 实现 `executeAddonUpgrades()` 执行逻辑 | `modules/cvo/internal/controllers/clusterversion_controller.go` |
| **Phase 5** | 实现 Addon 状态更新逻辑 | `modules/cvo/internal/controllers/clusterversion_controller.go` |
| **Phase 6** | 更新 reconcile 循环逻辑 | `modules/cvo/internal/controllers/clusterversion_controller.go` |
| **Phase 7** | 重新生成 deepcopy 代码 | `make generate` |
| **Phase 8** | 验证构建和测试 | `make test` |
