# BareMetalClusterTemplate

## BareMetalClusterSpec 与 BareMetalClusterTemplate 的关联机制
在 ClusterClass 模式下，`BareMetalClusterSpec` 与 `BareMetalClusterTemplate` 的关联是通过 **ClusterClass 的 Patch 机制** 和 **CAPI 的 ClusterTopology Controller** 自动完成的。

### 关联流程图
```
┌─────────────────────────────────────────────────────────────────┐
│ 1. 用户创建 Cluster 资源                                         │
│                                                                 │
│ spec:                                                           │
│   topology:                                                     │
│     classRef:                                                   │
│       name: baremetal-clusterclass-v0.1.0                       │
│     variables:                                                  │
│     - name: controlPlaneEndpoint                                │
│       value:                                                    │
│         host: "lb.example.com"                                  │
│         port: 6443                                              │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. ClusterTopology Controller 读取 ClusterClass                 │
│                                                                 │
│ ClusterClass.spec.infrastructure:                               │
│   templateRef:                                                  │
│     kind: BareMetalClusterTemplate                              │
│     name: baremetal-clusterclass-v0.1.0                         │
│                                                                 │
│ ClusterClass.spec.patches:                                      │
│ - name: controlPlaneEndpoint                                    │
│   definitions:                                                  │
│   - selector:                                                   │
│       kind: BareMetalClusterTemplate                            │
│     jsonPatches:                                                │
│     - op: add                                                   │
│       path: /spec/template/spec/controlPlaneEndpoint/host       │
│       valueFrom:                                                │
│         variable: controlPlaneEndpoint.host                     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. ClusterTopology Controller 应用 Patches                       │
│                                                                 │
│ 从 BareMetalClusterTemplate 生成 BareMetalCluster:              │
│                                                                 │
│ apiVersion: infrastructure.cluster.x-k8s.io/v1beta1             │
│ kind: BareMetalCluster                                          │
│ metadata:                                                       │
│   name: my-cluster                                              │
│   ownerReferences:                                              │
│   - apiVersion: cluster.x-k8s.io/v1beta2                        │
│     kind: Cluster                                               │
│     name: my-cluster                                            │
│ spec:                                                           │
│   controlPlaneEndpoint:                                         │
│     host: "lb.example.com"     ← 从变量注入                     │
│     port: 6443                 ← 从变量注入                     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. BareMetalCluster Controller 处理生成的资源                    │
│                                                                 │
│ - 验证 ControlPlaneEndpoint 是否有效                            │
│ - 设置 status.ready = true                                      │
│ - 更新 Conditions                                               │
└─────────────────────────────────────────────────────────────────┘
```

### 关键关联点
| 关联点 | 说明 |
|--------|------|
| **templateRef** | ClusterClass.spec.infrastructure.templateRef 指向 BareMetalClusterTemplate |
| **Patches** | ClusterClass.spec.patches 定义如何将变量注入到模板中 |
| **变量定义** | ClusterClass.spec.variables 定义用户可配置的变量 |
| **OwnerReference** | 生成的 BareMetalCluster 的 ownerReferences 指向 Cluster |

### 代码中的关联
```yaml
# ClusterClass 定义
spec:
  infrastructure:
    templateRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: BareMetalClusterTemplate
      name: baremetal-clusterclass-v0.1.0
  variables:
  - name: controlPlaneEndpoint
    required: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          host:
            type: string
          port:
            type: integer
  patches:
  - name: controlPlaneEndpoint
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: BareMetalClusterTemplate
        matchResources:
          infrastructureCluster: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/controlPlaneEndpoint/host
        valueFrom:
          variable: controlPlaneEndpoint.host
```

### 数据流总结
```
Cluster.spec.topology.variables
         │
         ▼ (ClusterTopology Controller 读取)
ClusterClass.spec.variables + patches
         │
         ▼ (应用 JSON Patch)
BareMetalClusterTemplate.spec.template.spec
         │
         ▼ (实例化)
BareMetalCluster.spec
         │
         ▼ (BareMetalCluster Controller 处理)
BareMetalCluster.status
```

**核心要点**：
1. 用户**不直接创建** BareMetalCluster，而是通过 Cluster 的 topology 引用 ClusterClass
2. ClusterClass 定义了**模板**和**变量注入规则**
3. CAPI 的 ClusterTopology Controller **自动生成** BareMetalCluster 资源
4. 生成的 BareMetalCluster 的 spec 来自模板 + 变量的 patch 结果

