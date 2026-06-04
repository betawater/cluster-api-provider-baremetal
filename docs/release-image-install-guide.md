# 使用 ReleaseImage 安装集群指南

## 1. 概述

ReleaseImage 是 CAPBM 的组件版本管理核心，通过自包含的 OCI 镜像分发所有组件，支持在线和离线（air-gapped）安装模式。本指南介绍如何使用 ReleaseImage 安装和升级 Kubernetes 集群。

---

## 2. 架构概览

### 2.1 核心 CRD

| CRD | 作用域 | 说明 |
|-----|--------|------|
| `ReleaseImage` | Namespaced | 定义完整的 Kubernetes 版本，包含组件版本、addons、升级图 |
| `ClusterVersion` | Namespaced | 触发安装/升级，跟踪集群版本状态和升级历史 |
| `ClusterAddon` | Namespaced | 跟踪每个 addon 的安装状态 |
| `UpgradePath` | Global | 定义合法的升级路径和兼容性规则 |
| `ReleaseCatalog` | Global | 全局版本目录索引 |

### 2.2 控制器组件

| 组件 | 模块 | 职责 |
|------|------|------|
| `ClusterVersionReconciler` | CVO | 主编排器：同步 UpgradePath、ReleaseCatalog，执行 K8S + addon 升级 |
| `ReleaseImageReconciler` | CVO | 验证内容哈希，管理 ReleaseImage 生命周期 |
| `ClusterAddonReconciler` | CVO | 通过 Helm 或 Manifest 安装/升级 addon |
| `GraphExecutor` | CVO | 执行升级图，应用 manifests，运行脚本，执行健康检查 |
| `Installer` | CAPBM | 通过 SSH 在节点上安装组件（containerd + Kubernetes） |

---

## 3. 前置条件

- 引导集群（Management Cluster）已安装并运行
- 目标裸金属节点已准备就绪（BareMetalMachine）
- ReleaseImage OCI 镜像已推送到可访问的镜像仓库
- 节点网络可访问引导集群 API Server

---

## 4. 引导集群安装

### 4.1 环境要求

| 组件 | 版本要求 | 说明 |
|------|----------|------|
| Kubernetes | v1.31+ | 引导集群版本 |
| kubectl | v1.31+ | 已配置并连接到引导集群 |
| clusterctl | v1.13+ | CAPI 命令行工具 |
| kustomize | v5.4.3+ | 用于手动部署（可选） |

**裸金属节点要求：**
- 操作系统：Ubuntu 20.04+、CentOS 7+、Rocky 8+、AlmaLinux、Debian
- SSH 访问已启用
- 最低配置：2GB RAM、20GB 磁盘（单节点建议 4GB/40GB）
- Swap 已禁用
- 内核版本 >= 3.10（建议 >= 5.4）
- 节点之间网络互通，且可访问引导集群 API Server

### 4.2 安装 clusterctl

#### Linux (amd64)

```bash
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/clusterctl-linux-amd64 -o clusterctl
chmod +x clusterctl
sudo mv clusterctl /usr/local/bin/
```

#### Linux (arm64)

```bash
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/clusterctl-linux-arm64 -o clusterctl
chmod +x clusterctl
sudo mv clusterctl /usr/local/bin/
```

#### macOS (amd64)

```bash
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/clusterctl-darwin-amd64 -o clusterctl
chmod +x clusterctl
sudo mv clusterctl /usr/local/bin/
```

#### macOS (arm64 / Apple Silicon)

```bash
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/clusterctl-darwin-arm64 -o clusterctl
chmod +x clusterctl
sudo mv clusterctl /usr/local/bin/
```

#### 验证安装

```bash
clusterctl version
```

### 4.3 创建引导集群

#### 方式一：使用 kind（开发/测试）

```bash
# 创建 kind 集群
kind create cluster --name capbm-mgmt --image kindest/node:v1.32.0

# 验证连接
kubectl cluster-info
kubectl get nodes
```

#### 方式二：使用 kubeadm（生产环境）

```bash
# 在目标节点上初始化控制平面
sudo kubeadm init --pod-network-cidr=192.168.0.0/16

# 配置 kubectl
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config

# 安装 CNI 插件（例如 Calico）
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.28.1/manifests/calico.yaml

# 验证集群状态
kubectl get nodes
kubectl get pods -n kube-system
```

#### 方式三：使用现有 Kubernetes 集群

如果您已有可用的 Kubernetes 集群（云提供商或本地），确保 `kubectl` 已正确配置：

```bash
kubectl cluster-info
kubectl get nodes
```

### 4.4 安装 CAPI 核心组件

```bash
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm
```

这将安装：
- CAPI 核心控制器（`capi-system` 命名空间）
- Kubeadm Bootstrap Provider（`capi-kubeadm-bootstrap-system` 命名空间）
- Kubeadm Control Plane Provider（`capi-kubeadm-control-plane-system` 命名空间）

**离线环境安装：**

```bash
CAPI_VERSION="v1.13.0"

kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/${CAPI_VERSION}/cluster-api-components.yaml
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/${CAPI_VERSION}/bootstrap-kubeadm-components.yaml
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/${CAPI_VERSION}/control-plane-kubeadm-components.yaml
```

### 4.5 安装 CAPBM Provider

```bash
# 安装 CRDs 和控制器
kubectl apply -k modules/capbm/config/
```

这将部署：
- CRDs: `BareMetalCluster`、`BareMetalMachine`、`BareMetalHostInventory`、`BareMetalClusterTemplate`、`BareMetalMachineTemplate`
- 控制器: `capbm-controller-manager`（`capbm-system` 命名空间）

**开发环境部署（自定义镜像）：**

```bash
# 安装 CRDs
make install-capbm

# 部署控制器
make deploy-capbm CAPBM_IMG=your-registry/capbm-manager:v0.1.0
```

### 4.6 安装 CVO（Cluster Version Operator）

```bash
# 安装 CRDs 和控制器
kubectl apply -k modules/cvo/config/
```

这将部署：
- CRDs: `ClusterVersion`、`ReleaseImage`、`UpgradePath`、`ReleaseCatalog`、`ClusterAddon`
- 控制器: `cvo-controller-manager`（`cvo-system` 命名空间）

**开发环境部署（自定义镜像）：**

```bash
# 安装 CRDs
make install-cvo

# 部署控制器
make deploy-cvo CVO_IMG=your-registry/cvo-manager:v0.1.0
```

### 4.7 部署 ClusterClass 模板

```bash
kubectl apply -k modules/capbm/config/clusterclass/
```

这将部署：
- `ClusterClass`: `baremetal-clusterclass-v0.1.0`
- `BareMetalClusterTemplate`
- `BareMetalMachineTemplate`（控制平面和工作节点）
- `KubeadmControlPlaneTemplate`
- `KubeadmConfigTemplate`

### 4.8 验证安装

```bash
# 检查所有控制器运行状态
kubectl get pods -n capi-system
kubectl get pods -n capi-kubeadm-bootstrap-system
kubectl get pods -n capi-kubeadm-control-plane-system
kubectl get pods -n capbm-system
kubectl get pods -n cvo-system

# 验证 CRDs
kubectl get crd | grep -E "baremetal|cluster.x-k8s.io|cvo.capbm.io"

# 等待控制器就绪
kubectl wait --for=condition=Available deployment/capbm-controller-manager -n capbm-system --timeout=120s
kubectl wait --for=condition=Available deployment/cvo-controller-manager -n cvo-system --timeout=120s
```

### 4.9 完整安装脚本

```bash
#!/bin/bash
set -e

echo "=== CAPBM 引导集群安装脚本 ==="

# 前置检查
echo "[1/7] 检查前置条件..."
kubectl version --client >/dev/null 2>&1 || { echo "kubectl 未安装"; exit 1; }
clusterctl version >/dev/null 2>&1 || { echo "clusterctl 未安装"; exit 1; }
echo "  ✓ kubectl 和 clusterctl 已安装"

# 创建引导集群（如需要）
echo "[2/7] 检查引导集群连接..."
kubectl cluster-info >/dev/null 2>&1 || {
    echo "  未检测到引导集群，使用 kind 创建..."
    kind create cluster --name capbm-mgmt --image kindest/node:v1.32.0
}
echo "  ✓ 引导集群已连接"

# 安装 CAPI 核心组件
echo "[3/7] 安装 CAPI 核心组件..."
clusterctl init --core cluster-api --bootstrap kubeadm --control-plane kubeadm
echo "  ✓ CAPI 核心组件已安装"

# 安装 CAPBM Provider
echo "[4/7] 安装 CAPBM Provider..."
kubectl apply -k modules/capbm/config/
echo "  ✓ CAPBM Provider 已安装"

# 安装 CVO
echo "[5/7] 安装 CVO..."
kubectl apply -k modules/cvo/config/
echo "  ✓ CVO 已安装"

# 部署 ClusterClass 模板
echo "[6/7] 部署 ClusterClass 模板..."
kubectl apply -k modules/capbm/config/clusterclass/
echo "  ✓ ClusterClass 模板已部署"

# 验证安装
echo "[7/7] 验证安装..."
kubectl wait --for=condition=Available deployment/capbm-controller-manager -n capbm-system --timeout=120s
kubectl wait --for=condition=Available deployment/cvo-controller-manager -n cvo-system --timeout=120s

echo ""
echo "=== 安装完成 ==="
echo ""
echo "控制器状态："
kubectl get pods -n capi-system
kubectl get pods -n capbm-system
kubectl get pods -n cvo-system
echo ""
echo "已安装的 CRDs："
kubectl get crd | grep -E "baremetal|cluster.x-k8s.io|cvo.capbm.io"
```

保存为 `install-mgmt-cluster.sh` 并执行：

```bash
chmod +x install-mgmt-cluster.sh
./install-mgmt-cluster.sh
```

---

## 5. 构建并发布 ReleaseImage 镜像

### 4.1 构建镜像

```bash
# 设置版本环境变量
export RELEASE_VERSION=v1.31.1
export K8S_VERSION=v1.31.1
export CONTAINERD_VERSION=1.7.24

# 构建 release image
make release-image-build RELEASE_IMG=registry.example.com/capbm/release:${RELEASE_VERSION}

# 推送镜像
make release-image-push RELEASE_IMG=registry.example.com/capbm/release:${RELEASE_VERSION}
```

### 4.2 使用构建脚本

```bash
# 使用 Docker 构建
./scripts/build-release-image.sh

# 使用 skopeo/crane 构建（无需 Docker）
./scripts/build-release-image-no-docker.sh

# 强制重新下载所有文件
FORCE_DOWNLOAD=true ./scripts/build-release-image-no-docker.sh
```

---

## 6. 应用 ReleaseImage CR

创建 ReleaseImage 资源定义目标版本的所有组件：

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-31-1
  namespace: capbm-system
spec:
  version: v1.31.1
  image: registry.example.com/capbm/release:v1.31.1
  httpServer:
    enabled: true
    port: 8080
    basePath: /release/v1.31.1
  imageRegistry:
    enabled: true
    registry: registry.example.com
    repository: capbm
  channels: ["stable", "fast"]
  previousVersions: ["v1.31.0", "v1.30.0"]
  components:
    kubernetes:
      version: v1.31.1
      type: binary
      path: /opt/capbm/binaries/kubernetes/v1.31.1
      platforms:
        ubuntu:
          architectures: ["amd64", "arm64"]
          packages:
            kubeadm: "kubeadm_1.31.1-00"
            kubelet: "kubelet_1.31.1-00"
            kubectl: "kubectl_1.31.1-00"
        centos:
          architectures: ["amd64", "arm64"]
          packages:
            kubeadm: "kubeadm-1.31.1-0"
            kubelet: "kubelet-1.31.1-0"
            kubectl: "kubectl-1.31.1-0"
      imageList:
        - "registry.k8s.io/kube-apiserver:v1.31.1"
        - "registry.k8s.io/kube-controller-manager:v1.31.1"
        - "registry.k8s.io/kube-scheduler:v1.31.1"
        - "registry.k8s.io/kube-proxy:v1.31.1"
        - "registry.k8s.io/pause:3.9"
        - "registry.k8s.io/etcd:3.5.15-0"
        - "registry.k8s.io/coredns/coredns:v1.11.1"
    containerd:
      version: 1.7.24
      type: binary
      path: /opt/capbm/binaries/containerd/1.7.24
      architectures: ["amd64", "arm64"]
  addons:
    - name: calico
      type: helm
      version: v3.28.1
      contentPath: addons/calico/v3.28.1/charts/tigera-operator.tgz
      namespace: kube-system
      dependencies: []
    - name: ceph-csi
      type: helm
      version: v3.11.0
      contentPath: addons/ceph-csi/v3.11.0/charts/ceph-csi-rbd.tgz
      namespace: ceph-csi
      dependencies: ["calico"]
    - name: metallb
      type: manifest
      version: v0.14.8
      contentPath: addons/metallb/v0.14.8/manifests/metallb-native.yaml
      namespace: metallb-system
      dependencies: ["calico"]
    - name: gateway-api
      type: manifest
      version: v1.1.0
      contentPath: addons/gateway-api/v1.1.0/manifests/standard-install.yaml
      namespace: gateway-system
      dependencies: []
    - name: capi-core-controller
      type: manifest
      version: v1.7.0
      contentPath: addons/capi-core/v1.7.0/manifests/cluster-api-components.yaml
      namespace: capi-system
      dependencies: []
  upgradeGraph:
    - name: phase-1-runtime
      order: 1
      blocking: true
      rollingUpdate:
        maxUnavailable: 1
      components:
        - name: containerd
          blocking: true
          dependsOn: []
          scripts: ["binaries/containerd/1.7.24/upgrade.sh"]
          healthCheck:
            type: ServiceRunning
            name: containerd
            timeout: 30s
    - name: phase-2-kubernetes
      order: 2
      blocking: true
      rollingUpdate:
        maxUnavailable: 1
      components:
        - name: kubernetes
          blocking: true
          dependsOn: ["containerd"]
          scripts: ["binaries/kubernetes/v1.31.1/upgrade.sh"]
          healthCheck:
            type: DeploymentReady
            namespace: kube-system
            name: kube-apiserver
            timeout: 60s
    - name: phase-3-cni
      order: 3
      blocking: true
      components:
        - name: calico
          blocking: true
          dependsOn: ["kubernetes"]
          healthCheck:
            type: DaemonSetReady
            namespace: kube-system
            name: calico-node
            timeout: 120s
    - name: phase-4-csi
      order: 4
      blocking: false
      components:
        - name: ceph-csi
          blocking: false
          dependsOn: ["calico"]
          healthCheck:
            type: DeploymentReady
            namespace: ceph-csi
            name: ceph-csi-rbdplugin-provisioner
            timeout: 120s
    - name: phase-5-gateway
      order: 5
      blocking: false
      components:
        - name: gateway-api
          blocking: true
          dependsOn: ["kubernetes"]
          healthCheck:
            type: CRDEstablished
            name: gateways.gateway.networking.k8s.io
            timeout: 60s
    - name: phase-6-loadbalancer
      order: 6
      blocking: false
      components:
        - name: metallb
          blocking: false
          dependsOn: ["calico"]
          healthCheck:
            type: DeploymentReady
            namespace: metallb-system
            name: metallb-controller
            timeout: 60s
    - name: phase-7-capi-core
      order: 7
      blocking: true
      components:
        - name: capi-core-controller
          blocking: true
          dependsOn: ["kubernetes"]
          healthCheck:
            type: DeploymentReady
            namespace: capi-system
            name: capi-controller-manager
            timeout: 60s
```

应用配置：

```bash
kubectl apply -f release-image.yaml
```

---

## 7. 创建 ClusterVersion 触发安装

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster-version
  namespace: capbm-system
spec:
  clusterName: my-baremetal-cluster
  desiredUpdate:
    version: v1.31.1
    releaseImageRef: v1-31-1
```

应用配置：

```bash
kubectl apply -f clusterversion.yaml
```

---

## 8. 安装执行流程

```
ClusterVersion 创建
       │
       ▼
┌─────────────────────────────────────────────────┐
│ ClusterVersionReconciler                        │
│                                                 │
│ 1. syncUpgradePath()    → 拉取 UpgradePath 验证  │
│ 2. syncReleaseCatalog() → 拉取 ReleaseCatalog    │
│ 3. fetchReleaseImage()  → 拉取 ReleaseImage      │
│    - OCIPuller.PullAndParseReleaseImage()        │
│    - 解析 release.json，创建/更新 ReleaseImage CR │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 1: Container Runtime (blocking)           │
│                                                 │
│ - containerd 安装/升级                           │
│ - CAPBM Installer 通过 SSH 执行脚本              │
│ - 健康检查: systemctl is-active containerd       │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 2: Kubernetes 组件 (blocking)             │
│                                                 │
│ - kubeadm/kubelet/kubectl 安装/升级              │
│ - 依赖 containerd 完成                           │
│ - 健康检查: kube-apiserver Deployment Ready      │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 3: CNI - Calico (blocking)                │
│                                                 │
│ - Helm chart 安装                                │
│ - 依赖 kubernetes 完成                           │
│ - 健康检查: calico-node DaemonSet Ready          │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 4: CSI - Ceph CSI (non-blocking)          │
│                                                 │
│ - Helm chart 安装                                │
│ - 依赖 calico 完成                               │
│ - 健康检查: ceph-csi-rbdplugin-provisioner Ready │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 5: Gateway API (non-blocking)             │
│                                                 │
│ - Manifest 安装                                  │
│ - 依赖 kubernetes 完成                           │
│ - 健康检查: Gateway CRD Established              │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 6: LoadBalancer - MetalLB (non-blocking)  │
│                                                 │
│ - Manifest 安装                                  │
│ - 依赖 calico 完成                               │
│ - 健康检查: metallb-controller Deployment Ready  │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Phase 7: CAPI Core Controller (blocking)        │
│                                                 │
│ - Manifest 安装                                  │
│ - 依赖 kubernetes 完成                           │
│ - 健康检查: capi-controller-manager Deployment   │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
            安装完成，更新 ClusterVersion 状态
```

---

## 9. 查看安装状态

### 9.1 查看 ClusterVersion 状态

```bash
# 查看概要
kubectl get clusterversion -n capbm-system

# 查看详细状态
kubectl get clusterversion my-cluster-version -n capbm-system -o yaml
```

输出示例：

```yaml
status:
  conditions:
    - type: Available
      status: "True"
    - type: Progressing
      status: "False"
    - type: Upgradeable
      status: "True"
  currentVersion: v1.31.1
  desiredVersion: v1.31.1
  history:
    - version: v1.31.1
      startedTime: "2024-01-01T00:00:00Z"
      completionTime: "2024-01-01T00:15:00Z"
      state: Completed
  components:
    kubernetes:
      version: v1.31.1
      status: Installed
    containerd:
      version: 1.7.24
      status: Installed
  addons:
    - name: calico
      version: v3.28.1
      status: Installed
    - name: metallb
      version: v0.14.8
      status: Installed
```

### 9.2 查看 ReleaseImage 状态

```bash
kubectl get releaseimage -n capbm-system
kubectl get releaseimage v1-31-1 -n capbm-system -o yaml
```

### 9.3 查看 Addon 安装状态

```bash
kubectl get clusteraddon -n capbm-system
kubectl get clusteraddon calico -n capbm-system -o yaml
```

### 9.4 查看升级事件

```bash
kubectl get events -n capbm-system --field-selector involvedObject.kind=ClusterVersion
```

---

## 10. 升级集群

### 10.1 准备新版本 ReleaseImage

```bash
# 构建新版本
export RELEASE_VERSION=v1.32.0
export K8S_VERSION=v1.32.0

./scripts/build-release-image-no-docker.sh

# 推送镜像
make release-image-build RELEASE_IMG=registry.example.com/capbm/release:${RELEASE_VERSION}
make release-image-push RELEASE_IMG=registry.example.com/capbm/release:${RELEASE_VERSION}
```

### 10.2 应用新版本 ReleaseImage

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-32-0
  namespace: capbm-system
spec:
  version: v1.32.0
  image: registry.example.com/capbm/release:v1.32.0
  # ... 其他配置
```

### 10.3 更新 ClusterVersion 触发升级

```bash
kubectl patch clusterversion my-cluster-version -n capbm-system \
  --type='merge' \
  -p='{"spec":{"desiredUpdate":{"version":"v1.32.0","releaseImageRef":"v1-32-0"}}}'
```

---

## 11. 回滚

### 11.1 回滚到先前版本

```bash
# 更新 ClusterVersion 指向旧版本
kubectl patch clusterversion my-cluster-version -n capbm-system \
  --type='merge' \
  -p='{"spec":{"desiredUpdate":{"version":"v1.31.1","releaseImageRef":"v1-31-1"}}}'
```

### 11.2 使用回滚脚本

每个组件都有对应的回滚脚本，存储在 ReleaseImage 中：

```
binaries/kubernetes/v1.31.1/rollback.sh
binaries/containerd/1.7.24/rollback.sh
addons/calico/v3.28.1/rollback.sh
addons/metallb/v0.14.8/rollback.sh
```

---

## 12. 离线/Air-Gapped 环境安装

### 12.1 导出镜像

```bash
# 导出 OCI 镜像
docker save registry.example.com/capbm/release:v1.31.1 -o release-image.tar

# 导出容器镜像列表
# 从 release.json 获取 imageList 中的所有镜像
```

### 12.2 传输到离线环境

```bash
# 使用 USB/网络传输
scp release-image.tar offline-user@offline-host:/tmp/

# 在离线环境导入
docker load -i release-image.tar
docker push offline-registry.example.com/capbm/release:v1.31.1
```

### 12.3 配置离线 ReleaseImage

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-31-1-offline
  namespace: capbm-system
spec:
  version: v1.31.1
  image: offline-registry.example.com/capbm/release:v1.31.1
  httpServer:
    enabled: true
    port: 8080
    basePath: /release/v1.31.1
  imageRegistry:
    enabled: true
    registry: offline-registry.example.com
    repository: capbm
  # ... 其他配置
```

---

## 13. 安装源模式

CAPBM Installer 支持三种安装源模式：

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| `online` | 从官方仓库下载（apt/yum/zypper） | 在线环境 |
| `http` | 从 Release HTTP Server 下载 | 离线环境 |
| `local` | 从本地文件系统路径安装 | 离线环境 |

### 13.1 配置安装源

在 BareMetalCluster 或 BareMetalMachine 中配置：

```yaml
apiVersion: capbm.io/v1alpha1
kind: BareMetalCluster
metadata:
  name: my-cluster
spec:
  installSource:
    type: http
    baseURL: http://release-server:8080/release/v1.31.1
```

---

## 14. 故障排查

### 14.1 查看控制器日志

```bash
# CVO 控制器日志
kubectl logs -n capbm-system -l control-plane=cvo-controller-manager -f

# CAPBM 控制器日志
kubectl logs -n capbm-system -l control-plane=capbm-controller-manager -f
```

### 14.2 常见问题

| 问题 | 可能原因 | 解决方案 |
|------|----------|----------|
| ReleaseImage 拉取失败 | 镜像仓库不可达 | 检查网络，验证镜像存在 |
| 组件安装失败 | SSH 连接失败 | 检查节点 SSH 配置和凭据 |
| Addon 安装失败 | Helm/Monifest 错误 | 查看 ClusterAddon 事件 |
| 健康检查失败 | 组件未就绪 | 检查组件日志，手动验证 |

### 14.3 手动验证组件

```bash
# 检查 containerd
systemctl status containerd

# 检查 kubelet
systemctl status kubelet

# 检查 Kubernetes 组件
kubectl get nodes
kubectl get pods -n kube-system

# 检查 addon
kubectl get pods -n kube-system -l k8s-app=calico-node
kubectl get pods -n metallb-system
```

---

## 15. 参考文档

| 文档 | 说明 |
|------|------|
| [ReleaseImage 目录规范](./release-image-directory-spec.md) | OCI 镜像目录结构和文件命名规范 |
| [ReleaseImage 构建设计](./release-image-build-design.md) | 如何构建 ReleaseImage |
| [CVO 升级机制](./cluster-upgrade-cvo.md) | CVO 升级机制设计 |
| [原地升级设计](./in-place-upgrade-design.md) | 原地升级设计 |
| [控制平面升级设计](./control-plane-upgrade-design.md) | 控制平面升级设计 |
