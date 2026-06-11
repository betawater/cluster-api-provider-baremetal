# controlPlaneEndpoint
## Cluster.spec.controlPlaneEndpoint 数据来源
根据 CAPI 官方文档，`Cluster.spec.controlPlaneEndpoint` 有**三个可能的来源**：

### 来源 1：用户提供（最常见）
**手动模式**：用户直接在 Cluster 资源中设置
```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  controlPlaneEndpoint:
    host: "lb.example.com"
    port: 6443
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: BareMetalCluster
    name: my-cluster
```

**ClusterClass 模式**：通过 variables 注入
```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  topology:
    classRef:
      name: baremetal-clusterclass
    variables:
    - name: controlPlaneEndpoint
      value:
        host: "lb.example.com"
        port: 6443
```

### 来源 2：Infrastructure Provider 提供
某些云提供商（AWS/GCP/Azure）会自动创建负载均衡器，并将端点写回：
```
InfraCluster.spec.controlPlaneEndpoint (Provider 创建 LB)
         │
         ▼ (CAPI Cluster Controller 读取)
Cluster.spec.controlPlaneEndpoint
```
**CAPBM 不适用此方式**，因为裸金属环境通常使用外部已有的负载均衡器。

### 来源 3：Control Plane Provider 提供
某些 Control Plane Provider 也可以提供端点。

## CAPBM 场景的完整数据流
```
用户设置 Cluster.spec.topology.variables.controlPlaneEndpoint
         │
         ▼ (ClusterTopology Controller 应用 Patches)
BareMetalCluster.spec.controlPlaneEndpoint
         │
         ▼ (CAPI Cluster Controller 读取 InfraCluster)
Cluster.spec.controlPlaneEndpoint
         │
         ▼ (用于生成 kubeconfig、证书 SANs 等)
KubeadmControlPlane / Machine / Node
```

### 关键控制器交互
| 控制器 | 职责 |
|--------|------|
| **ClusterTopology Controller** | 将 variables 通过 patches 注入到 BareMetalCluster.spec.controlPlaneEndpoint |
| **BareMetalCluster Controller** | 验证端点，设置 Ready 状态 |
| **Cluster Controller** | 从 BareMetalCluster.spec.controlPlaneEndpoint 读取并同步到 Cluster.spec.controlPlaneEndpoint |
| **KubeadmControlPlane Controller** | 使用 Cluster.spec.controlPlaneEndpoint 生成 kubeadm 配置和证书 |

### 为什么需要双向同步？
```
Cluster.spec.controlPlaneEndpoint ←→ BareMetalCluster.spec.controlPlaneEndpoint
```

1. **Cluster → BareMetalCluster**：ClusterClass 模式下，用户通过 variables 设置端点，需要传递给 InfraCluster
2. **BareMetalCluster → Cluster**：Provider 创建端点后，需要上报给 Cluster（云提供商场景）

对于 CAPBM，主要是**第一个方向**（Cluster → BareMetalCluster），因为端点由用户提供。
