# Baremetal 自升级方案设计

## 1. 概述

### 1.1 设计目标

Baremetal 自升级方案旨在实现管理集群（Management Cluster）自身的零中断或最小中断升级，包括：

- **CVO Manager** 控制器升级
- **CAPBM Manager** 控制器升级
- **CRD 定义** 更新
- **Webhook 配置** 更新
- **RBAC 策略** 更新

### 1.2 核心原则

| 原则 | 说明 |
|------|------|
| **零中断优先** | 通过 `maxUnavailable=0`、`maxSurge=1` 确保服务连续性 |
| **分阶段执行** | CRD → RBAC → Webhook → CVO → CAPBM，严格依赖顺序 |
| **自动回滚** | 任何阶段失败自动回滚到先前版本 |
| **状态持久化** | 升级状态存储在 CR 中，支持控制器重启后恢复 |
| **代码复用** | 与工作集群升级共用 GraphExecutor、HealthChecker、BackupRollback 等组件 |

### 1.3 业务中断风险分析

| 阶段 | 风险 | 预计中断时间 | 缓解措施 |
|------|------|-------------|----------|
| CRD 更新（兼容） | 字段删除/类型变更 | 0 秒 | 只增不改、保留旧字段、转换 Webhook |
| RBAC 更新 | 权限变更 | 0 秒 | 原子更新，不影响现有连接 |
| Webhook 配置更新 | 端点不可达 | 5-15 秒 | `failurePolicy: Ignore`、双端点并行 |
| Deployment 滚动更新 | 旧 Pod 终止、新 Pod 未就绪 | 10-30 秒 | `maxUnavailable=0`、`minReadySeconds=30` |
| etcd 备份期间 | 快照占用 I/O | 写入延迟 | 低峰期执行、限速 |

---

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Management Cluster                           │
│                                                                     │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐ │
│  │   CVO Manager   │    │  CAPBM Manager  │    │  CAPI Core      │ │
│  │  (cvo-system)   │    │ (capbm-system)  │    │  (capi-system)  │ │
│  └────────┬────────┘    └────────┬────────┘    └────────┬────────┘ │
│           │                      │                      │          │
│           └──────────────────────┼──────────────────────┘          │
│                                  │                                 │
│                    ┌─────────────▼─────────────┐                   │
│                    │   SelfUpgradeController   │                   │
│                    │   (new - cvo-system)      │                   │
│                    └─────────────┬─────────────┘                   │
│                                  │                                 │
│                    ┌─────────────▼─────────────┐                   │
│                    │   SelfUpgrade CRD         │                   │
│                    │   spec:                   │                   │
│                    │     targetVersion         │                   │
│                    │     components:           │                   │
│                    │       - cvo-manager       │                   │
│                    │       - capbm-manager     │                   │
│                    │       - crds              │                   │
│                    │     strategy: Rolling     │                   │
│                    └───────────────────────────┘                   │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 代码复用架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                        代码复用架构                                  │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                   共用层 (Shared Layer)                       │  │
│  │                                                              │  │
│  │  GraphExecutor │ HealthChecker │ BackupRollback │ OCIPuller  │  │
│  └───────────────────────────┬──────────────────────────────────┘  │
│                              │                                     │
│              ┌───────────────┼───────────────┐                     │
│              ▼               ▼               ▼                     │
│  ┌───────────────┐ ┌───────────────┐ ┌───────────────┐           │
│  │ Workload      │ │ SelfUpgrade   │ │ AddonUpgrade  │           │
│  │ Cluster       │ │ Controller    │ │ Controller    │           │
│  │ Upgrader      │ │ (New)         │ │ (Existing)    │           │
│  └───────────────┘ └───────────────┘ └───────────────┘           │
│         │                  │                                     │
│         ▼                  ▼                                     │
│  ┌───────────────┐ ┌───────────────┐                            │
│  │ SSH + Scripts │ │ K8s API Only  │                            │
│  │ (Node-level)  │ │ (Cluster-level)│                           │
│  └───────────────┘ └───────────────┘                            │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.3 可共用组件清单

| 组件 | 位置 | 共用方式 | 说明 |
|------|------|----------|------|
| **GraphExecutor** | `modules/cvo/internal/upgrader/graph_executor.go` | ✅ 直接复用 | 升级图执行逻辑相同 |
| **HealthChecker** | `modules/cvo/internal/upgrader/health_checker.go` | ✅ 直接复用 | DeploymentReady、CRDEstablished 等检查通用 |
| **BackupRollbackExecutor** | `modules/cvo/internal/upgrader/backup_rollback.go` | ✅ 直接复用 | 备份/回滚逻辑通用 |
| **OCIPuller** | `modules/cvo/internal/upgrader/oci_puller.go` | ✅ 直接复用 | 拉取 ReleaseImage 逻辑相同 |
| **SSHManager** | `modules/cvo/pkg/ssh/manager.go` | ❌ 自升级不需要 | 自升级不通过 SSH，使用 Kubernetes API |
| **ControlPlaneUpgrader** | `modules/cvo/internal/upgrader/control_plane_upgrader.go` | ❌ 不适用 | 工作集群升级需要 drain/uncordon，自升级不需要 |
| **AddonUpgrader** | `modules/cvo/internal/addon/upgrader.go` | ⚠️ 部分复用 | Helm/Manifest 安装逻辑可复用，但自升级不需要 addon 概念 |

### 2.4 需要新建的组件

| 组件 | 说明 |
|------|------|
| **SelfUpgrade CRD** | 自升级资源定义，包含目标版本、组件列表、策略 |
| **SelfUpgradeController** | 自升级专属协调器，基于阶段的状态机 |
| **DeploymentUpgrader** | 更新 Deployment 镜像、等待滚动完成 |
| **CRDUpgrader** | 安全更新 CRD（只增不改策略） |
| **WebhookUpgrader** | 安全更新 Webhook 配置 |
| **SelfUpgradeStateStore** | 持久化升级状态，支持控制器重启后恢复 |

---

## 3. CRD 设计

### 3.1 SelfUpgrade CRD

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: SelfUpgrade
metadata:
  name: self-upgrade-v0-9-0
  namespace: cvo-system
spec:
  # 目标版本
  targetVersion: v0.9.0
  
  # ReleaseImage OCI 镜像引用
  releaseImage: registry.example.com/capbm/release:v0.9.0
  
  # 升级策略
  strategy:
    type: Rolling
    maxUnavailable: 0
    maxSurge: 1
    minReadySeconds: 30
    timeout: 30m
    autoRollback: true
  
  # 升级前钩子
  preUpgradeHooks:
    - name: backup-crds
      command: kubectl get crds -o yaml > /tmp/crds-backup.yaml
      timeout: 60s
    - name: backup-etcd
      command: etcdctl snapshot save /tmp/etcd-snapshot.db
      timeout: 300s
  
  # 升级后钩子
  postUpgradeHooks:
    - name: verify-integration
      command: kubectl run integration-test --image=test:latest --restart=Never
      timeout: 120s
  
  # 升级组件列表
  components:
    - name: crds
      type: crd
      order: 1
      blocking: true
      dependsOn: []
      
    - name: rbac
      type: rbac
      order: 2
      blocking: true
      dependsOn: ["crds"]
      
    - name: webhooks
      type: webhook
      order: 3
      blocking: true
      dependsOn: ["crds", "rbac"]
      healthCheck:
        type: EndpointHealthy
        timeout: 30s
        
    - name: cvo-manager
      type: deployment
      order: 4
      blocking: true
      dependsOn: ["crds", "rbac", "webhooks"]
      healthCheck:
        type: DeploymentReady
        namespace: cvo-system
        name: cvo-controller-manager
        timeout: 120s
        
    - name: capbm-manager
      type: deployment
      order: 5
      blocking: true
      dependsOn: ["crds", "rbac", "webhooks", "cvo-manager"]
      healthCheck:
        type: DeploymentReady
        namespace: capbm-system
        name: capbm-controller-manager
        timeout: 120s
```

### 3.2 Go 类型定义

```go
// SelfUpgradeSpec defines the desired state of SelfUpgrade
type SelfUpgradeSpec struct {
    // TargetVersion is the target version to upgrade to
    TargetVersion string `json:"targetVersion"`
    
    // ReleaseImage is the OCI image reference containing upgrade components
    ReleaseImage string `json:"releaseImage,omitempty"`
    
    // Strategy defines the upgrade strategy
    Strategy SelfUpgradeStrategy `json:"strategy,omitempty"`
    
    // PreUpgradeHooks are hooks to run before upgrade
    PreUpgradeHooks []Hook `json:"preUpgradeHooks,omitempty"`
    
    // PostUpgradeHooks are hooks to run after upgrade
    PostUpgradeHooks []Hook `json:"postUpgradeHooks,omitempty"`
    
    // Components defines which components to upgrade
    Components []SelfUpgradeComponent `json:"components,omitempty"`
}

type SelfUpgradeStrategy struct {
    // Type is the strategy type (Rolling, Recreate)
    Type StrategyType `json:"type,omitempty"`
    
    // MaxUnavailable is the maximum number of components that can be unavailable
    MaxUnavailable int `json:"maxUnavailable,omitempty"`
    
    // MaxSurge is the maximum number of extra components that can be created
    MaxSurge int `json:"maxSurge,omitempty"`
    
    // MinReadySeconds is the minimum time a component must be ready before continuing
    MinReadySeconds int `json:"minReadySeconds,omitempty"`
    
    // Timeout is the maximum time for the entire upgrade
    Timeout metav1.Duration `json:"timeout,omitempty"`
    
    // AutoRollback enables automatic rollback on failure
    AutoRollback bool `json:"autoRollback,omitempty"`
}

type SelfUpgradeComponent struct {
    // Name is the component name
    Name string `json:"name"`
    
    // Type is the component type (deployment, crd, webhook, rbac)
    Type ComponentType `json:"type"`
    
    // Order defines the upgrade order
    Order int `json:"order,omitempty"`
    
    // Blocking indicates if this component must succeed before continuing
    Blocking bool `json:"blocking,omitempty"`
    
    // DependsOn lists component dependencies
    DependsOn []string `json:"dependsOn,omitempty"`
    
    // HealthCheck defines the health check for this component
    HealthCheck *HealthCheck `json:"healthCheck,omitempty"`
}

// SelfUpgradeStatus defines the observed state of SelfUpgrade
type SelfUpgradeStatus struct {
    // Phase is the current upgrade phase
    Phase SelfUpgradePhase `json:"phase,omitempty"`
    
    // StartedTime is when the upgrade started
    StartedTime metav1.Time `json:"startedTime,omitempty"`
    
    // CompletedTime is when the upgrade completed
    CompletedTime *metav1.Time `json:"completedTime,omitempty"`
    
    // CurrentVersion is the current version after upgrade
    CurrentVersion string `json:"currentVersion,omitempty"`
    
    // ComponentStatus tracks status of each component
    ComponentStatus []ComponentUpgradeStatus `json:"componentStatus,omitempty"`
    
    // History tracks upgrade history
    History []UpgradeHistory `json:"history,omitempty"`
    
    // Conditions represents the latest available observations
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type SelfUpgradePhase string

const (
    PhasePending       SelfUpgradePhase = "Pending"
    PhaseValidating    SelfUpgradePhase = "Validating"
    PhasePreUpgrade    SelfUpgradePhase = "PreUpgrade"
    PhaseUpgrading     SelfUpgradePhase = "Upgrading"
    PhaseVerifying     SelfUpgradePhase = "Verifying"
    PhaseCompleted     SelfUpgradePhase = "Completed"
    PhaseRollingBack   SelfUpgradePhase = "RollingBack"
    PhaseFailed        SelfUpgradePhase = "Failed"
)
```

---

## 4. 升级执行流程

```
SelfUpgrade 创建
       │
       ▼
┌─────────────────────────────────────────────────┐
│ Phase 1: Validating                             │
│                                                 │
│ - 验证目标版本存在                               │
│ - 验证升级路径合法                               │
│ - 验证当前集群健康                               │
│ - 验证 ReleaseImage 可拉取                       │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 2: PreUpgrade                             │
│                                                 │
│ - 执行 PreUpgradeHooks                           │
│ - 备份当前 CRD 定义                              │
│ - 备份当前 Deployment 配置                       │
│ - 备份 etcd (如果启用)                           │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 3: Upgrading (按顺序)                     │
│                                                 │
│ Step 1: CRDs                                    │
│   - kubectl apply -f crds/                      │
│   - 等待 CRD 就绪                                │
│   - 策略: 只增不改，保留旧字段                    │
│                                                 │
│ Step 2: RBAC                                    │
│   - kubectl apply -f rbac/                      │
│   - 原子更新，不影响现有连接                      │
│                                                 │
│ Step 3: Webhook 配置                            │
│   - kubectl apply -f webhooks/                  │
│   - failurePolicy: Ignore                       │
│   - 验证端点可达                                 │
│                                                 │
│ Step 4: CVO Manager                             │
│   - 更新 Deployment image                        │
│   - maxUnavailable: 0, maxSurge: 1              │
│   - 等待新 Pod 就绪 (minReadySeconds: 30)        │
│   - 健康检查                                     │
│                                                 │
│ Step 5: CAPBM Manager                           │
│   - 更新 Deployment image                        │
│   - maxUnavailable: 0, maxSurge: 1              │
│   - 等待新 Pod 就绪 (minReadySeconds: 30)        │
│   - 健康检查                                     │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 4: Verifying                              │
│                                                 │
│ - 验证所有 Deployments 就绪                      │
│ - 验证所有 CRDs 可用                             │
│ - 验证 Webhook 端点可达                          │
│ - 执行 PostUpgradeHooks                          │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 5: Completed                              │
│                                                 │
│ - 更新 CurrentVersion                            │
│ - 记录升级历史                                   │
│ - 发送完成事件                                   │
│                                                 │
│ 如果任何步骤失败:                                 │
│ - 进入 PhaseRollingBack                          │
│ - 按逆序回滚组件                                 │
│ - 更新状态为 PhaseFailed                         │
└─────────────────────────────────────────────────┘
```

---

## 5. 升级顺序和依赖

```yaml
upgradeGraph:
  - name: phase-1-crds
    order: 1
    blocking: true
    components:
      - name: crds
        type: crd
        blocking: true
        dependsOn: []
        healthCheck:
          type: CRDEstablished
          timeout: 60s

  - name: phase-2-rbac
    order: 2
    blocking: true
    components:
      - name: rbac
        type: rbac
        blocking: true
        dependsOn: ["crds"]

  - name: phase-3-webhooks
    order: 3
    blocking: true
    components:
      - name: webhooks
        type: webhook
        blocking: true
        dependsOn: ["crds", "rbac"]
        healthCheck:
          type: EndpointHealthy
          timeout: 30s

  - name: phase-4-cvo
    order: 4
    blocking: true
    components:
      - name: cvo-manager
        type: deployment
        blocking: true
        dependsOn: ["crds", "rbac", "webhooks"]
        healthCheck:
          type: DeploymentReady
          namespace: cvo-system
          name: cvo-controller-manager
          timeout: 120s

  - name: phase-5-capbm
    order: 5
    blocking: true
    components:
      - name: capbm-manager
        type: deployment
        blocking: true
        dependsOn: ["crds", "rbac", "webhooks", "cvo-manager"]
        healthCheck:
          type: DeploymentReady
          namespace: capbm-system
          name: capbm-controller-manager
          timeout: 120s
```

---

## 6. 回滚机制

### 6.1 自动回滚触发条件

| 条件 | 说明 |
|------|------|
| 健康检查失败 | 组件升级后健康检查未通过 |
| 超时 | 升级步骤超过配置的 timeout |
| CRD 不兼容 | 新 CRD 导致现有资源验证失败 |
| Deployment 滚动失败 | 新 Pod 无法就绪，旧 Pod 已终止 |

### 6.2 回滚执行流程

```go
func (r *SelfUpgradeReconciler) executeRollback(ctx context.Context, su *cvoapi.SelfUpgrade) error {
    // 1. 恢复 CRDs (逆序)
    if err := r.restoreCRDs(ctx, su); err != nil {
        return err
    }
    
    // 2. 恢复 RBAC
    if err := r.restoreRBAC(ctx, su); err != nil {
        return err
    }
    
    // 3. 恢复 Webhook 配置
    if err := r.restoreWebhooks(ctx, su); err != nil {
        return err
    }
    
    // 4. 恢复 CVO Manager
    if err := r.restoreDeployment(ctx, su, "cvo-controller-manager", "cvo-system"); err != nil {
        return err
    }
    
    // 5. 恢复 CAPBM Manager
    if err := r.restoreDeployment(ctx, su, "capbm-controller-manager", "capbm-system"); err != nil {
        return err
    }
    
    // 6. 验证回滚
    return r.verifyRollback(ctx, su)
}
```

### 6.3 备份策略

| 备份项 | 方式 | 存储位置 |
|--------|------|----------|
| CRD 定义 | kubectl get crd -o yaml | ConfigMap / etcd snapshot |
| Deployment 配置 | kubectl get deployment -o yaml | ConfigMap |
| Webhook 配置 | kubectl get mutatingwebhookconfiguration/validatingwebhookconfiguration -o yaml | ConfigMap |
| etcd 数据 | etcdctl snapshot save | 本地磁盘 / 对象存储 |

---

## 7. 零中断升级策略

### 7.1 Deployment 配置

```yaml
spec:
  strategy:
    rollingUpdate:
      maxUnavailable: 0    # 不允许不可用
      maxSurge: 1          # 先启动新 Pod
  minReadySeconds: 30      # 新 Pod 就绪后等待 30 秒
  revisionHistoryLimit: 3  # 保留 3 个版本用于回滚
  template:
    spec:
      readinessProbe:
        httpGet:
          path: /healthz
          port: 8081
        initialDelaySeconds: 10
        periodSeconds: 5
        failureThreshold: 3
```

### 7.2 CRD 更新策略

| 规则 | 说明 |
|------|------|
| **只增不改** | 只添加新字段，不删除或修改现有字段 |
| **保留旧字段** | 废弃字段保留至少 2 个版本 |
| **转换 Webhook** | 使用 Conversion Webhook 处理版本转换 |
| **存储版本** | 明确指定存储版本，避免歧义 |

### 7.3 Webhook 更新策略

| 规则 | 说明 |
|------|------|
| **failurePolicy: Ignore** | Webhook 不可用时不阻止 API 请求 |
| **双端点并行** | 新旧版本 Webhook 同时运行 |
| **sideEffects: None** | 确保 Webhook 无副作用 |
| **timeoutSeconds: 10** | 设置合理超时 |

---

## 8. 实现计划

### Phase 1: 核心功能 (Priority 1)

| 任务 | 说明 | 预估时间 |
|------|------|----------|
| 实现 OCI Puller | 使用 oras-go 拉取 OCI 镜像 | 3 天 |
| 实现脚本执行桥接 | NodeUpgradeExecutor 通过 SSH 执行脚本 | 2 天 |
| 实现 ControlPlaneUpgrader | drain/uncordon/containerd upgrade/etcd backup | 5 天 |
| 实现 Backup/Rollback | 配置备份、etcd 快照、回滚脚本执行 | 3 天 |

### Phase 2: 自升级功能 (Priority 2)

| 任务 | 说明 | 预估时间 |
|------|------|----------|
| 定义 SelfUpgrade CRD | API 类型定义和 CRD 生成 | 1 天 |
| 实现 SelfUpgradeController | 基于阶段的协调器 | 5 天 |
| 实现升级状态机 | Phase 跟踪和 Reconcile 循环 | 2 天 |
| 实现回滚机制 | 自动和手动回滚 | 3 天 |

### Phase 3: 增强功能 (Priority 3)

| 任务 | 说明 | 预估时间 |
|------|------|----------|
| 实现 RollingUpgradeCoordinator | 节点分批升级 | 3 天 |
| 实现 WorkerNodeUpgrader | Worker 节点升级 | 3 天 |
| 升级暂停/恢复 | Pause/Resume 功能 | 2 天 |
| 集成 Metrics | Prometheus 指标 | 2 天 |

### Phase 4: 安全和可观测性 (Priority 4)

| 任务 | 说明 | 预估时间 |
|------|------|----------|
| SSH 主机密钥验证 | 启用 KnownHosts 配置 | 1 天 |
| 升级前版本倾斜检查 | Kubernetes version skew policy | 1 天 |
| 升级事件和审计 | 详细的事件记录 | 1 天 |

---

## 9. 使用示例

### 9.1 创建 SelfUpgrade

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: SelfUpgrade
metadata:
  name: self-upgrade-v0-9-0
  namespace: cvo-system
spec:
  targetVersion: v0.9.0
  releaseImage: registry.example.com/capbm/release:v0.9.0
  strategy:
    type: Rolling
    maxUnavailable: 0
    maxSurge: 1
    minReadySeconds: 30
    timeout: 30m
    autoRollback: true
  preUpgradeHooks:
    - name: backup-crds
      command: kubectl get crds -o yaml > /tmp/crds-backup.yaml
      timeout: 60s
  components:
    - name: crds
      type: crd
      order: 1
      blocking: true
    - name: rbac
      type: rbac
      order: 2
      blocking: true
    - name: webhooks
      type: webhook
      order: 3
      blocking: true
    - name: cvo-manager
      type: deployment
      order: 4
      blocking: true
    - name: capbm-manager
      type: deployment
      order: 5
      blocking: true
```

### 9.2 查看升级状态

```bash
# 查看自升级状态
kubectl get selfupgrade -n cvo-system

# 查看详细状态
kubectl get selfupgrade self-upgrade-v0-9-0 -n cvo-system -o yaml

# 查看升级事件
kubectl get events -n cvo-system --field-selector involvedObject.kind=SelfUpgrade
```

---

## 10. 风险缓解

| 风险 | 缓解措施 |
|------|----------|
| 升级失败导致集群不可用 | 自动回滚、etcd 备份、分阶段升级 |
| CRD 不兼容 | 升级前验证、CRD 转换 webhook、只增不改策略 |
| Deployment 滚动更新失败 | maxUnavailable=0、健康检查、超时回滚 |
| 网络中断 | 重试机制、超时配置、断点续传 |
| 资源不足 | 升级前资源检查、Pod 优先级 |
| Webhook 端点不可达 | failurePolicy: Ignore、双端点并行 |

---

## 11. 参考文档

| 文档 | 说明 |
|------|------|
| [ReleaseImage 目录规范](./release-image-directory-spec.md) | OCI 镜像目录结构和文件命名规范 |
| [ReleaseImage 安装指南](./release-image-install-guide.md) | 使用 ReleaseImage 安装集群 |
| [CVO 升级机制](./cluster-upgrade-cvo.md) | CVO 升级机制设计 |
| [原地升级设计](./in-place-upgrade-design.md) | 原地升级设计 |
| [控制平面升级设计](./control-plane-upgrade-design.md) | 控制平面升级设计 |
