# CAPBM Namespace 设计

## 概述

本文档描述 Cluster API Provider Bare Metal (CAPBM) 的 Namespace 设计，实现多集群资源隔离和权限管理。

## 架构设计

### Namespace 层级结构

```
┌─────────────────────────────────────────────────────────────┐
│ 集群架构                                                     │
│                                                             │
│  capbm-system                    # CAPBM 系统命名空间         │
│  ├── Deployment: capbm-controller-manager                   │
│  ├── ServiceAccount: capbm-controller-manager               │
│  ├── ClusterRole: capbm-manager-role                        │
│  └── ClusterClass: baremetal-clusterclass-v0.1.0            │
│                                                             │
│  cluster-my-cluster              # 集群 my-cluster 命名空间   │
│  ├── Cluster: my-cluster                                    │
│  ├── BareMetalCluster: my-cluster                           │
│  ├── KubeadmControlPlane: my-cluster-control-plane          │
│  ├── BareMetalMachineTemplate: my-cluster-cp                │
│  ├── BareMetalMachine: my-cluster-cp-xxx (x3)               │
│  ├── MachineDeployment: my-cluster-md-0                     │
│  ├── BareMetalMachineTemplate: my-cluster-md-0              │
│  ├── BareMetalMachine: my-cluster-md-0-xxx (x2)             │
│  └── BareMetalHostInventory: my-cluster-hosts               │
│                                                             │
│  cluster-prod-cluster            # 另一个集群                │
│  └── ...                                                    │
└─────────────────────────────────────────────────────────────┘
```

### 资源归属规则

| 资源类型 | Namespace | 说明 |
|---------|-----------|------|
| CAPBM Controller | `capbm-system` | 系统组件，全局共享 |
| ClusterClass | `capbm-system` 或 `default` | 模板，可跨集群复用 |
| Cluster | 集群 namespace | 集群资源 |
| BareMetalCluster | 集群 namespace | 集群基础设施 |
| BareMetalMachine | 集群 namespace | 机器资源 |
| BareMetalHostInventory | 集群 namespace | 机器池，集群专用 |
| KubeadmControlPlane | 集群 namespace | 控制面 |
| MachineDeployment | 集群 namespace | 工作节点 |

## 命名规范

### Namespace 命名

```
格式: cluster-<cluster-name>

示例:
- cluster-my-cluster
- cluster-prod-cluster
- cluster-staging-cluster
- cluster-dev-cluster
```

**命名约束**:
- 必须符合 Kubernetes namespace 命名规则 (DNS-1123 label)
- 最大长度 63 字符
- 只能包含小写字母、数字和连字符

### 资源标签

所有集群相关资源添加以下标签：

```yaml
labels:
  cluster.x-k8s.io/cluster-name: my-cluster   # CAPI 标准标签
  capbm.capbm.io/cluster-namespace: cluster-my-cluster  # CAPBM 扩展标签
```

## 用户工作流

### 创建集群

```bash
# 1. 创建集群 namespace
kubectl create namespace cluster-my-cluster

# 2. 创建 BareMetalHostInventory
kubectl apply -f inventory.yaml -n cluster-my-cluster

# 3. 创建集群
kubectl apply -f cluster.yaml -n cluster-my-cluster

# 4. 查看资源
kubectl get all -n cluster-my-cluster
kubectl get baremetalmachines -n cluster-my-cluster

# 5. 获取 kubeconfig
clusterctl get kubeconfig my-cluster -n cluster-my-cluster > workload-kubeconfig

# 6. 删除集群 (自动清理所有资源)
kubectl delete namespace cluster-my-cluster
```

### 多集群管理

```
clusters/
├── my-cluster/
│   ├── kustomization.yaml
│   ├── cluster.yaml
│   └── inventory.yaml
├── prod-cluster/
│   ├── kustomization.yaml
│   ├── cluster.yaml
│   └── inventory.yaml
└── staging-cluster/
    ├── kustomization.yaml
    ├── cluster.yaml
    └── inventory.yaml
```

## RBAC 配置

### 集群管理员 Role

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: cluster-admin
  namespace: cluster-my-cluster
rules:
- apiGroups: ["cluster.x-k8s.io", "infrastructure.cluster.x-k8s.io"]
  resources: ["*"]
  verbs: ["*"]
- apiGroups: [""]
  resources: ["secrets", "configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: cluster-admin-binding
  namespace: cluster-my-cluster
subjects:
- kind: User
  name: developer
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: cluster-admin
  apiGroup: rbac.authorization.k8s.io
```

### 只读用户 Role

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: cluster-viewer
  namespace: cluster-my-cluster
rules:
- apiGroups: ["cluster.x-k8s.io", "infrastructure.cluster.x-k8s.io"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: cluster-viewer-binding
  namespace: cluster-my-cluster
subjects:
- kind: User
  name: viewer
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: cluster-viewer
  apiGroup: rbac.authorization.k8s.io
```

## 迁移方案

### 从 default namespace 迁移

```bash
#!/bin/bash
# migrate-cluster.sh

CLUSTER_NAME=$1
OLD_NS=${2:-default}
NEW_NS="cluster-${CLUSTER_NAME}"

if [ -z "$CLUSTER_NAME" ]; then
  echo "Usage: $0 <cluster-name> [old-namespace]"
  exit 1
fi

echo "Migrating cluster '$CLUSTER_NAME' from '$OLD_NS' to '$NEW_NS'"

# Create new namespace
kubectl create namespace $NEW_NS

# Export and migrate resources
RESOURCES=("cluster" "baremetalcluster" "baremetalhostinventory")
for resource in "${RESOURCES[@]}"; do
  kubectl get $resource $CLUSTER_NAME -n $OLD_NS -o yaml | \
    sed "s/namespace: $OLD_NS/namespace: $NEW_NS/g" | \
    kubectl apply -f - -n $NEW_NS
done

echo "Migration complete. Verify with: kubectl get all -n $NEW_NS"
```

## 设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **Namespace 策略** | 每集群独立 vs 共享 | 每集群独立 | 完全隔离，易管理 |
| **命名规范** | `cluster-<name>` vs `<name>` | `cluster-<name>` | 避免命名冲突 |
| **ClusterClass 位置** | 每集群 vs 系统级 | 系统级 (capbm-system) | 复用，避免重复 |
| **Namespace 创建** | 手动 vs 自动 | 手动 (推荐) | 明确控制 |
| **RBAC 粒度** | ClusterRole vs Role | Role (按 namespace) | 最小权限原则 |

## 实施步骤

1. **更新文档**: 在使用指南中添加 namespace 使用说明
2. **更新示例**: 所有 YAML 示例使用 `cluster-<name>` namespace
3. **添加迁移指南**: 提供从 default namespace 迁移的步骤
4. **更新 RBAC**: 添加 namespace 级别的 Role 示例
5. **测试验证**: 在测试环境验证多集群隔离
