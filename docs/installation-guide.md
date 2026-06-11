# CAPBM 安装指南

## 目录

- [1. 前置条件](#1-前置条件)
- [2. 安装 CAPI 核心组件](#2-安装-capi-核心组件)
- [3. 部署 ClusterClass 模板](#3-部署-clusterclass-模板)
- [4. 验证部署](#4-验证部署)
- [5. 常见问题](#5-常见问题)

---

## 1. 前置条件

### 1.1 管理集群

- Kubernetes v1.32+ 管理集群
- `kubectl` 已配置并连接到管理集群
- `clusterctl` v1.13+ 已安装

### 1.2 本地开发环境（可选）

- Go 1.25+ 已安装
- Docker（用于构建镜像）
- kustomize（用于部署）

---

## 2. 安装 CAPI 核心组件

> **重要**: ClusterClass 功能需要启用 `ClusterTopology` 功能门控。
> 以下安装方法已自动启用此功能。

### 2.1 使用自动化安装脚本（推荐）

这是最简单的安装方式，脚本会自动处理所有依赖和配置：

```bash
# 下载并执行安装脚本
curl -fsSL https://raw.githubusercontent.com/betawater/cluster-api-provider-baremetal/main/scripts/install-capbm.sh | bash
```

或者，如果你已克隆仓库：

```bash
chmod +x scripts/install-capbm.sh
./scripts/install-capbm.sh
```

脚本会自动：
1. 安装 CAPI 核心组件并启用 `ClusterTopology`
2. 修复 kubeadm control plane provider 的 Feature Gates
3. 部署 CAPBM CRDs 和 Controller
4. 部署 ClusterClass 模板
5. 验证安装状态

### 2.2 使用 clusterctl 配置文件

如果你希望手动控制安装过程，可以使用项目提供的 `clusterctl.yaml` 配置文件：

```bash
# 使用本地 clusterctl 配置安装
clusterctl init --config clusterctl.yaml \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm \
  --infrastructure baremetal \
  --feature-gates=ClusterTopology=true
```

#### 生成集群配置

```bash
clusterctl generate cluster my-cluster \
  --from templates/clusterclass/baremetal-clusterclass.yaml \
  --variable CLUSTER_NAME=my-cluster \
  --variable NAMESPACE=default \
  --variable KUBERNETES_VERSION=v1.31.1 \
  --variable CONTROL_PLANE_MACHINE_COUNT=3 \
  --variable WORKER_MACHINE_COUNT=2 \
  --variable CONTROL_PLANE_ENDPOINT_HOST=lb.example.com \
  --variable CONTROL_PLANE_ENDPOINT_PORT=6443 \
  --variable SSH_CREDENTIALS_SECRET=baremetal-ssh-credentials \
  --variable SSH_USERNAME=root \
  --variable SSH_PASSWORD=your-password \
  --variable HOST_INVENTORY_REF=datacenter-a-hosts \
  --variable POD_CIDR=10.244.0.0/16 \
  --variable SERVICE_CIDR=10.96.0.0/12 \
  > cluster.yaml
```

**变量说明**：

| 变量名 | 说明 | 示例值 |
|--------|------|--------|
| `CLUSTER_NAME` | 集群名称 | `my-cluster` |
| `NAMESPACE` | 命名空间 | `default` |
| `KUBERNETES_VERSION` | Kubernetes 版本 | `v1.31.1` |
| `CONTROL_PLANE_MACHINE_COUNT` | 控制面节点数 | `3` |
| `WORKER_MACHINE_COUNT` | Worker 节点数 | `2` |
| `CONTROL_PLANE_ENDPOINT_HOST` | API Server 地址 | `lb.example.com` |
| `CONTROL_PLANE_ENDPOINT_PORT` | API Server 端口 | `6443` |
| `SSH_CREDENTIALS_SECRET` | SSH 凭据 Secret 名称 | `baremetal-ssh-credentials` |
| `SSH_USERNAME` | SSH 用户名 | `root` |
| `SSH_PASSWORD` | SSH 密码 | `your-password` |
| `HOST_INVENTORY_REF` | BareMetalHostInventory 名称 | `datacenter-a-hosts` |
| `POD_CIDR` | Pod 网络 CIDR（可选） | `10.244.0.0/16` |
| `SERVICE_CIDR` | Service 网络 CIDR（可选） | `10.96.0.0/12` |

### 2.3 手动安装（高级）

如果你需要完全控制安装过程，可以按以下步骤手动安装：

#### 步骤 1: 安装 CAPI 核心组件

```bash
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm \
  --feature-gates=ClusterTopology=true
```

#### 步骤 2: 修复 kubeadm control plane provider 的 Feature Gates

> **注意**: kubeadm control plane provider 默认禁用 `ClusterTopology`，需要手动启用。

```bash
kubectl patch deployment capi-kubeadm-control-plane-controller-manager \
  -n capi-kubeadm-control-plane-system \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/args", "value": ["--feature-gates=MachinePool=true,ClusterTopology=true,KubeadmBootstrapFormatIgnition=false,PriorityQueue=true,ReconcilerRateLimiting=true,InPlaceUpdates=false,MachineTaintPropagation=false","--leader-elect","--diagnostics-address=:8443","--insecure-diagnostics=false"]}]'

# 等待 rollout 完成
kubectl rollout status deployment capi-kubeadm-control-plane-controller-manager -n capi-kubeadm-control-plane-system
```

#### 步骤 3: 部署 CAPBM Provider

```bash
# 部署 CAPBM CRDs 和 Controller
kubectl apply -k modules/capbm/config/crd/
kubectl apply -k modules/capbm/config/

# 部署 CVO (Cluster Version Operator) CRDs 和 Controller
kubectl apply -k modules/cvo/config/crd/
kubectl apply -k modules/cvo/config/
```

### 验证安装

```bash
# 检查 CAPI Controllers
kubectl get pods -n capi-system
kubectl get pods -n capi-kubeadm-bootstrap-system
kubectl get pods -n capi-kubeadm-control-plane-system

# 检查 CAPBM Controller
kubectl get pods -n capbm-system

# 检查 ClusterClass
kubectl get clusterclass baremetal-clusterclass
```

预期输出：

```
NAME                                  READY   STATUS
capi-controller-manager-xxxxx         1/1     Running
capi-kubeadm-bootstrap-controller-xxxxx  1/1     Running
capi-kubeadm-control-plane-controller-xxxxx  1/1     Running
capbm-controller-manager-xxxxx        1/1     Running
cvo-controller-manager-xxxxx          1/1     Running
```

---

## 3. 部署 ClusterClass 模板

> **注意**: 如果使用自动化安装脚本（`scripts/install-capbm.sh`），ClusterClass 模板已自动部署。

```bash
# 使用 kustomize 部署
kubectl apply -k modules/capbm/config/clusterclass/
```

验证 ClusterClass 部署：

```bash
kubectl get clusterclass baremetal-clusterclass
kubectl get baremetalclustertemplate
kubectl get baremetalmachinetemplate
kubectl get kubeadmcontrolplanetemplate
kubectl get kubeadmconfigtemplate
```

---

## 4. 验证部署

### 4.1 检查所有 CRDs

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

### 4.2 检查 Controller 日志

```bash
# 查看 CAPBM Controller 日志
kubectl logs -n capbm-system -l control-plane=controller-manager --tail=50

# 查看 CVO Controller 日志
kubectl logs -n cvo-system -l control-plane=controller-manager --tail=50
```

---

## 5. 常见问题

### Q1: `clusterctl init --infrastructure baremetal` 报错 "release not found"

**原因**: `baremetal` provider 尚未发布到 CAPI 官方仓库。

**解决方案**: 使用自动化安装脚本（`scripts/install-capbm.sh`）或配置本地 Provider 源（见 2.2 节）。

### Q2: 部署后 CRDs 未创建

**检查步骤**:

```bash
# 手动构建并应用
kubectl apply -k modules/capbm/config/crd/
kubectl apply -k modules/capbm/config/
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

### Q4: `admission webhook denied the request: spec: Forbidden: can be set only if the ClusterTopology feature flag is enabled`

**原因**: kubeadm control plane provider 默认禁用 `ClusterTopology`。

**解决方案**: 执行步骤 2.3 中的步骤 2，patch Deployment 的 Feature Gates。

### Q5: `ClusterClass water/baremetal-clusterclass not found`

**原因**: ClusterClass 和 Cluster 不在同一个命名空间。

**解决方案**: 确保 ClusterClass 和 Cluster 在同一个命名空间，或者在 Cluster 的 `spec.topology.classRef` 中指定 `namespace` 字段。

### Q6: 如何卸载 Provider

```bash
# 卸载 ClusterClass
kubectl delete -k modules/capbm/config/clusterclass/

# 卸载 CAPBM
kubectl delete -k modules/capbm/config/
kubectl delete -k modules/capbm/config/crd/

# 卸载 CVO
kubectl delete -k modules/cvo/config/
kubectl delete -k modules/cvo/config/crd/

# 卸载 CAPI 核心组件
clusterctl delete --core cluster-api --bootstrap kubeadm --control-plane kubeadm
```

---

## 附录：完整安装流程示例

```bash
# 1. 使用自动化脚本安装（推荐）
./scripts/install-capbm.sh

# 或者手动安装：
# 1. 安装 CAPI 核心组件
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm \
  --feature-gates=ClusterTopology=true

# 2. 修复 kubeadm control plane provider 的 Feature Gates
kubectl patch deployment capi-kubeadm-control-plane-controller-manager \
  -n capi-kubeadm-control-plane-system \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/args", "value": ["--feature-gates=MachinePool=true,ClusterTopology=true,KubeadmBootstrapFormatIgnition=false,PriorityQueue=true,ReconcilerRateLimiting=true,InPlaceUpdates=false,MachineTaintPropagation=false","--leader-elect","--diagnostics-address=:8443","--insecure-diagnostics=false"]}]'

# 3. 部署 CAPBM CRDs 和 Controller
kubectl apply -k modules/capbm/config/crd/
kubectl apply -k modules/capbm/config/

# 4. 部署 ClusterClass
kubectl apply -k modules/capbm/config/clusterclass/

# 5. 验证部署
kubectl get crd | grep baremetal
kubectl get pods -n capbm-system
kubectl get clusterclass

# 6. 创建机器池和集群（参考 single-node-guide.md）
```
