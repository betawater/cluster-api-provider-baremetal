# CAPBM 安装指南

## 目录

- [1. 前置条件](#1-前置条件)
- [2. 安装 CAPI 核心组件](#2-安装-capi-核心组件)
- [3. 部署 CAPBM Provider](#3-部署-capbm-provider)
- [4. 部署 ClusterClass 模板](#4-部署-clusterclass-模板)
- [5. 验证部署](#5-验证部署)
- [6. 配置本地 Provider 源（可选）](#6-配置本地-provider-源可选)
- [7. 常见问题](#7-常见问题)

---

## 1. 前置条件

### 1.1 管理集群

- Kubernetes v1.32+ 管理集群
- `kubectl` 已配置并连接到管理集群
- `clusterctl` v1.13+ 已安装
- `make` 已安装

### 1.2 本地开发环境

- Go 1.25+ 已安装
- Docker（用于构建镜像）
- kustomize（用于部署）

---

## 2. 安装 CAPI 核心组件

首先安装 Cluster API 的核心组件（这些组件已发布到官方仓库）：

```bash
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm
```

验证安装：

```bash
kubectl get pods -n capi-system
kubectl get pods -n capi-kubeadm-bootstrap-system
kubectl get pods -n capi-kubeadm-control-plane-system
```

---

## 3. 部署 CAPBM Provider

由于 `baremetal` provider 是本地开发的，尚未发布到 CAPI 官方仓库，需要通过 `make deploy` 方式部署。

### 3.1 部署 CAPBM Controller

```bash
# 部署 CAPBM CRDs 和 Controller
make deploy-capbm

# 部署 CVO (Cluster Version Operator) CRDs 和 Controller
make deploy-cvo
```

### 3.2 手动部署（备选）

如果 `make deploy` 不可用，可以手动部署：

```bash
# 安装 CAPBM CRDs
kubectl apply -k modules/capbm/config/crd

# 部署 CAPBM Controller
kubectl apply -k modules/capbm/config

# 安装 CVO CRDs
kubectl apply -k modules/cvo/config/crd

# 部署 CVO Controller
kubectl apply -k modules/cvo/config
```

### 3.3 验证部署

```bash
# 检查 CRDs
kubectl get crd | grep baremetal
kubectl get crd | grep versionmanifest
kubectl get crd | grep upgrade
kubectl get crd | grep clusterversion

# 检查 Controller Pods
kubectl get pods -n capbm-system
kubectl get pods -n cvo-system

# 预期输出
# NAME                                  READY   STATUS    RESTARTS   AGE
# capbm-controller-manager-xxxxx        1/1     Running   0          30s
# cvo-controller-manager-xxxxx          1/1     Running   0          30s
```

---

## 4. 部署 ClusterClass 模板

```bash
# 使用 make 部署
make deploy-clusterclass

# 或手动部署
kubectl apply -k modules/capbm/config/clusterclass
```

验证 ClusterClass 部署：

```bash
kubectl get clusterclass baremetal-clusterclass-v0.1.0
kubectl get baremetalclustertemplate
kubectl get baremetalmachinetemplate
kubectl get kubeadmcontrolplanetemplate
kubectl get kubeadmconfigtemplate
```

---

## 5. 验证部署

### 5.1 检查所有 CRDs

```bash
# CAPBM CRDs
kubectl get crd | grep infrastructure.cluster.x-k8s.io

# 预期输出:
# baremetalclusters.infrastructure.cluster.x-k8s.io
# baremetalclustertemplates.infrastructure.cluster.x-k8s.io
# baremetalhostinventories.infrastructure.cluster.x-k8s.io
# baremetalmachines.infrastructure.cluster.x-k8s.io
# baremetalmachinetemplates.infrastructure.cluster.x-k8s.io

# CVO CRDs
kubectl get crd | grep capbm.cluster.x-k8s.io

# 预期输出:
# clusterversions.capbm.cluster.x-k8s.io
# upgradegraphs.capbm.cluster.x-k8s.io
# versioncatalogs.capbm.cluster.x-k8s.io
# versionmanifests.capbm.cluster.x-k8s.io
```

### 5.2 检查 Controller 日志

```bash
# 查看 CAPBM Controller 日志
kubectl logs -n capbm-system -l control-plane=controller-manager --tail=50

# 查看 CVO Controller 日志
kubectl logs -n cvo-system -l control-plane=controller-manager --tail=50
```

---

## 6. 配置本地 Provider 源（可选）

如果你希望使用 `clusterctl init --infrastructure baremetal` 方式安装，需要配置本地 Provider 源。

### 6.1 生成 Release Manifest

```bash
make release-capbm
# 这会生成 infrastructure-components.yaml
```

### 6.2 创建 clusterctl 配置文件

创建 `~/.clusterctl.yaml` 文件：

```yaml
providers:
  - name: baremetal
    url: file:///path/to/cluster-api-provider-baremetal/infrastructure-components.yaml
    type: InfrastructureProvider
```

> **注意**: 将 `/path/to/cluster-api-provider-baremetal` 替换为实际的项目路径。

### 6.3 使用 clusterctl 安装

```bash
clusterctl init --infrastructure baremetal
```

---

## 7. 常见问题

### Q1: `clusterctl init --infrastructure baremetal` 报错 "release not found"

**原因**: `baremetal` provider 尚未发布到 CAPI 官方仓库。

**解决方案**: 使用 `make deploy-capbm` 直接部署，或配置本地 Provider 源（见第 6 节）。

### Q2: 部署后 CRDs 未创建

**检查步骤**:

```bash
# 检查 kustomize 是否正确安装
make kustomize

# 手动构建并应用
kubectl apply -k modules/capbm/config/crd
kubectl apply -k modules/capbm/config
```

### Q3: Controller Pod 处于 CrashLoopBackOff 状态

**排查步骤**:

```bash
# 查看 Pod 详情
kubectl describe pod -n capbm-system -l control-plane=controller-manager

# 查看日志
kubectl logs -n capbm-system -l control-plane=controller-manager --tail=100
```

**常见原因**:
- RBAC 权限不足
- 镜像拉取失败
- 配置错误

### Q4: 如何卸载 Provider

```bash
# 卸载 CAPBM
make undeploy-capbm

# 卸载 CVO
make undeploy-cvo

# 卸载 ClusterClass
kubectl delete -k modules/capbm/config/clusterclass

# 卸载 CAPI 核心组件
clusterctl delete --core cluster-api --bootstrap kubeadm --control-plane kubeadm
```

---

## 附录：完整安装流程示例

```bash
# 1. 安装 CAPI 核心组件
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm

# 2. 部署 CAPBM Provider
make deploy-capbm
make deploy-cvo

# 3. 部署 ClusterClass
make deploy-clusterclass

# 4. 验证部署
kubectl get crd | grep baremetal
kubectl get pods -n capbm-system
kubectl get clusterclass

# 5. 创建机器池和集群（参考 user-guide.md）
```
