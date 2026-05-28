# BareMetalCluster Controller 端点来源优化说明

## 优化背景

在 Cluster API 架构中，ControlPlaneEndpoint 有三种提供方式：
1. **用户提供** - 通过 Cluster.spec.controlPlaneEndpoint 或 ClusterClass variables
2. **Control Plane Provider 提供** - 如 KubeadmControlPlane 自动生成
3. **Infrastructure Provider 提供** - 如 AWS/GCP 自动创建负载均衡器

对于 CAPBM（裸金属），端点通常由**用户外部提供**（已有负载均衡器），CAPBM 不负责创建端点。

## 优化内容

### 1. 智能端点解析

新增 `resolveControlPlaneEndpoint` 方法，按优先级解析端点来源：

```go
func (r *BareMetalClusterReconciler) resolveControlPlaneEndpoint(...) (clusterv1.APIEndpoint, string, error) {
    clusterEndpoint := capiCluster.Spec.ControlPlaneEndpoint
    infraEndpoint := bmCluster.Spec.ControlPlaneEndpoint

    switch {
    case clusterValid && infraValid:
        // 两者都有效时，优先使用 Cluster 的端点，并记录日志警告不一致
        return clusterEndpoint, "cluster", nil
    case clusterValid:
        // Cluster 有效时使用 Cluster 的端点
        return clusterEndpoint, "cluster", nil
    case infraValid:
        // 仅 BareMetalCluster 有效时使用其端点
        return infraEndpoint, "infrastructure", nil
    default:
        // 都无效时返回空，等待端点设置
        return clusterv1.APIEndpoint{}, "", nil
    }
}
```

### 2. 端点来源追踪

通过 Annotation 记录端点来源，便于调试和审计：

```go
bmCluster.Annotations[EndpointSourceAnnotation] = source
// 示例: baremetal.cluster.x-k8s.io/endpoint-source: "cluster"
```

### 3. 监听 Cluster 资源变化

Controller 现在 watch Cluster 资源，当 Cluster 的 ControlPlaneEndpoint 变化时自动触发调谐：

```go
func (r *BareMetalClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&infrav1.BareMetalCluster{}).
        Watches(
            &clusterv1.Cluster{},
            handler.EnqueueRequestsFromMapFunc(clusterToBareMetalCluster(mgr)),
        ).
        Complete(r)
}
```

### 4. 符合 v1beta2 合约

新增 `status.initialization.provisioned` 字段，符合 CAPI v1beta2 合约要求：

```go
provisioned := true
bmCluster.Status.Initialization = &infrav1.BareMetalClusterInitializationStatus{
    Provisioned: &provisioned,
}
```

### 5. 网络配置自动同步

从 Cluster 自动同步网络配置到 BareMetalCluster：

```go
func (r *BareMetalClusterReconciler) reconcileNetworkConfig(...) error {
    // PodCIDR
    if bmCluster.Spec.Network.PodCIDR == "" {
        if len(clusterNetwork.Pods.CIDRBlocks) > 0 {
            bmCluster.Spec.Network.PodCIDR = clusterNetwork.Pods.CIDRBlocks[0]
        }
    }
    // ServiceCIDR
    // DNSDomain
}
```

## 端点来源优先级

```
Cluster.spec.controlPlaneEndpoint (最高优先级)
         │
         ▼ (如果无效)
BareMetalCluster.spec.controlPlaneEndpoint
         │
         ▼ (如果无效)
等待端点设置 (RequeueAfter: 10s)
```

## 适用场景

| 场景 | 端点来源 | Annotation 值 |
|------|----------|---------------|
| ClusterClass 模式 | Cluster (从 variables 注入) | `cluster` |
| 手动创建 Cluster | Cluster (用户直接设置) | `cluster` |
| 仅设置 BareMetalCluster | BareMetalCluster | `infrastructure` |
| 两者都设置 | Cluster (优先) | `cluster` |

## 修改的文件

| 文件 | 变更 |
|------|------|
| `internal/controllers/baremetalcluster_controller.go` | 核心优化逻辑 |
| `api/v1beta1/baremetalcluster_types.go` | 新增 Initialization 状态 |
| `api/v1beta1/zz_generated.deepcopy.go` | 新增 DeepCopy 方法 |
| `config/crd/bases/...baremetalclusters.yaml` | 更新 CRD 定义 |
