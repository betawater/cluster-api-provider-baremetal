# ReleaseImage 目录规范

## 1. 概述

ReleaseImage 是 CAPBM 的组件版本管理核心，通过自包含的目录结构分发所有组件，支持在线和离线（air-gapped）安装模式。

**核心特性**:
- **自包含**: 一个目录包含所有组件，无需外网访问
- **版本一致**: 所有组件版本由 ReleaseImage 统一管理
- **多架构支持**: 支持 linux/amd64 和 linux/arm64
- **多 OS 支持**: 支持 Ubuntu、Debian 和 CentOS/RHEL
- **组件类型自描述**: 通过 release.json 标识 binary/manifest/helm 类型
- **可验证**: 提供 SHA256 校验和文件验证完整性

---

## 2. 目录结构

### 2.1 完整目录树

```
release-image/
├── release.json                          # ReleaseImage spec 定义
├── binaries/                             # 二进制组件
│   ├── kubernetes/                       # Kubernetes 二进制（OS 特定）
│   │   └── {version}/                    # 例如 v1.31.1
│   │       ├── ubuntu/                   # Ubuntu 平台（deb 包）
│   │       │   ├── amd64/
│   │       │   │   ├── kubeadm
│   │       │   │   ├── kubelet
│   │       │   │   └── kubectl
│   │       │   └── arm64/
│   │       │       ├── kubeadm
│   │       │       ├── kubelet
│   │       │       └── kubectl
│   │       ├── debian/                   # Debian 平台（deb 包）
│   │       │   ├── amd64/
│   │       │   └── arm64/
│   │       └── centos/                   # CentOS/RHEL 平台（rpm 包）
│   │           ├── amd64/
│   │           └── arm64/
│   │       ├── upgrade.sh                # Kubernetes 升级脚本
│   │       └── rollback.sh               # Kubernetes 回滚脚本
│   ├── containerd/                       # Containerd（Linux 通用）
│   │   └── {version}/                    # 例如 1.7.24
│   │       └── linux/
│   │           ├── amd64/
│   │           │   └── containerd.tar.gz
│   │           └── arm64/
│   │               └── containerd.tar.gz
│   │       ├── upgrade.sh                # Containerd 升级脚本
│   │       └── rollback.sh               # Containerd 回滚脚本
│   ├── helm/                             # Helm（Linux 通用）
│   │   └── {version}/                    # 例如 v3.15.0
│   │       └── linux/
│   │           ├── amd64/
│   │           │   └── helm.tar.gz
│   │           └── arm64/
│   │               └── helm.tar.gz
│   └── cni-plugins/                      # CNI 插件（Linux 通用）
│       └── {version}/                    # 例如 v1.5.0
│           └── linux/
│               ├── amd64/
│               │   └── cni-plugins.tgz
│               └── arm64/
│                   └── cni-plugins.tgz
├── addons/                               # 附加组件
│   ├── calico/                           # Calico CNI
│   │   └── {version}/                    # 例如 v3.28.1
│   │       ├── charts/
│   │       │   └── tigera-operator.tgz
│   │       └── rollback.sh
│   ├── ceph-csi/                         # Ceph CSI
│   │   └── {version}/
│   │       ├── charts/
│   │       │   └── ceph-csi-rbd.tgz
│   │       └── rollback.sh
│   ├── metallb/                          # MetalLB
│   │   └── {version}/
│   │       ├── manifests/
│   │       │   └── metallb-native.yaml
│   │       └── rollback.sh
│   ├── gateway-api/                      # Gateway API
│   │   └── {version}/
│   │       ├── manifests/
│   │       │   └── standard-install.yaml
│   │       └── rollback.sh
│   ├── capi-core/                        # CAPI Core Controller
│   │   └── {version}/
│   │       ├── charts/
│   │       │   └── capi-core-controller.tgz
│   │       └── rollback.sh
│   └── envoy-gateway/                    # Envoy Gateway
│       └── {version}/
│           ├── charts/
│           │   └── envoy-gateway.tgz
│           └── rollback.sh
├── images/                               # 容器镜像 tar 包
│   ├── {registry}/                       # 例如 registry.k8s.io
│   │   ├── {image}/                      # 例如 kube-apiserver
│   │   │   └── {tag}.tar                 # 例如 v1.31.1.tar
│   │   └── {repo}/                       # 例如 coredns
│   │       └── {image}/                  # 例如 coredns
│   │           └── {tag}.tar             # 例如 v1.11.1.tar
│   └── docker.io/
│       └── calico/
│           └── node/
│               └── v3.28.1.tar
└── checksums/
    └── sha256sums.txt                    # 所有文件的 SHA256 校验和
```

### 2.2 目录层级规则

| 层级 | 规则 | 示例 |
|------|------|------|
| **Kubernetes** | `binaries/kubernetes/{version}/{os}/{arch}/` | `binaries/kubernetes/v1.31.1/ubuntu/amd64/` |
| **Linux 通用组件** | `binaries/{component}/{version}/linux/{arch}/` | `binaries/containerd/1.7.24/linux/amd64/` |
| **容器镜像** | `images/{registry}/{repo}/{image}/{tag}.tar` | `images/registry.k8s.io/kube-apiserver/v1.31.1.tar` |
| **Addon Charts** | `addons/{addon}/{version}/charts/` | `addons/calico/v3.28.1/charts/` |
| **Addon Manifests** | `addons/{addon}/{version}/manifests/` | `addons/metallb/v0.14.8/manifests/` |
| **升级/回滚脚本** | `binaries/{component}/{version}/upgrade.sh` | `binaries/kubernetes/v1.31.1/upgrade.sh` |
| **Addon 脚本** | `addons/{addon}/{version}/rollback.sh` | `addons/calico/v3.28.1/rollback.sh` |

### 2.3 OS 与架构矩阵

| 组件 | ubuntu | debian | centos | linux (通用) |
|------|--------|--------|--------|-------------|
| kubernetes | ✅ (deb) | ✅ (deb) | ✅ (rpm) | - |
| containerd | - | - | - | ✅ (tar.gz) |
| helm | - | - | - | ✅ (tar.gz) |
| cni-plugins | - | - | - | ✅ (tgz) |

| 架构 | amd64 | arm64 |
|------|-------|-------|
| 支持状态 | ✅ | ✅ |

---

## 3. release.json 规范

### 3.1 顶层结构

```json
{
  "version": "v1.31.1",
  "image": "registry.example.com/capbm/release:v1.31.1",
  "httpServer": { ... },
  "imageRegistry": { ... },
  "channels": ["stable", "fast"],
  "previousVersions": ["v1.31.0", "v1.30.0"],
  "components": { ... },
  "addons": [ ... ],
  "upgradeGraph": [ ... ],
  "contentHash": "sha256:..."
}
```

### 3.2 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `version` | string | ReleaseImage 版本标识 |
| `image` | string | OCI 镜像引用 |
| `httpServer` | object | HTTP 服务配置（端口、基础路径） |
| `imageRegistry` | object | 镜像仓库配置 |
| `channels` | string[] | 发布通道（stable/fast） |
| `previousVersions` | string[] | 可升级的先前版本 |
| `components` | object | 核心组件定义（kubernetes/containerd/helm/cniPlugins） |
| `addons` | array | 附加组件定义（calico/ceph-csi/metallb 等） |
| `upgradeGraph` | array | 升级阶段和依赖关系 |
| `contentHash` | string | 内容 SHA256 校验和 |

### 3.3 组件定义 (components)

#### Kubernetes 组件

```json
"kubernetes": {
  "version": "v1.31.1",
  "type": "binary",
  "path": "/opt/capbm/binaries/kubernetes/v1.31.1",
  "platforms": {
    "ubuntu": {
      "architectures": ["amd64", "arm64"],
      "packages": {
        "kubeadm": "kubeadm_1.31.1-00",
        "kubelet": "kubelet_1.31.1-00",
        "kubectl": "kubectl_1.31.1-00"
      }
    },
    "debian": { ... },
    "centos": { ... }
  },
  "imageList": [
    "registry.k8s.io/kube-apiserver:v1.31.1",
    ...
  ],
  "installStrategy": { ... },
  "upgradeStrategy": { ... },
  "upgrade": { ... }
}
```

#### Linux 通用组件

```json
"containerd": {
  "version": "1.7.24",
  "type": "binary",
  "path": "/opt/capbm/binaries/containerd/1.7.24",
  "architectures": ["amd64", "arm64"],
  "files": {
    "archive": "containerd-1.7.24-linux-amd64.tar.gz"
  },
  "installStrategy": { ... },
  "upgradeStrategy": { ... },
  "upgrade": { ... }
}
```

### 3.4 附加组件定义 (addons)

```json
{
  "name": "calico",
  "type": "helm",
  "version": "v3.28.1",
  "contentPath": "addons/calico/v3.28.1/charts/tigera-operator.tgz",
  "namespace": "kube-system",
  "dependencies": [],
  "variables": [ ... ],
  "defaultValues": { ... },
  "installStrategy": { ... },
  "upgradeStrategy": { ... },
  "upgrade": {
    "rollback": {
      "script": "addons/calico/v3.28.1/rollback.sh",
      "timeout": "300s"
    }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 组件名称 |
| `type` | string | 类型：`helm` / `manifest` |
| `version` | string | 组件版本 |
| `contentPath` | string | 相对于 release-image 根目录的内容路径 |
| `namespace` | string | 安装目标命名空间 |
| `dependencies` | string[] | 依赖组件名称列表 |
| `variables` | array | 可配置变量定义 |
| `defaultValues` | object | 变量默认值 |

### 3.5 升级图 (upgradeGraph)

```json
{
  "name": "phase-1-runtime",
  "order": 1,
  "blocking": true,
  "rollingUpdate": {
    "maxUnavailable": 1
  },
  "components": [
    {
      "name": "containerd",
      "blocking": true,
      "dependsOn": [],
      "scripts": ["binaries/containerd/1.7.24/upgrade.sh"],
      "healthCheck": { ... }
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 阶段名称 |
| `order` | int | 执行顺序（从小到大） |
| `blocking` | bool | 是否阻塞后续阶段 |
| `rollingUpdate` | object | 滚动更新配置 |
| `components` | array | 阶段内组件列表 |
| `components[].dependsOn` | string[] | 组件依赖 |
| `components[].scripts` | string[] | 升级脚本路径 |
| `components[].healthCheck` | object | 健康检查配置 |

---

## 4. 构建脚本

### 4.1 脚本列表

| 脚本 | 说明 | 依赖 |
|------|------|------|
| `scripts/build-release-image.sh` | 基于 Docker 的构建脚本 | Docker, Helm, curl, sha256sum |
| `scripts/build-release-image-no-docker.sh` | 无 Docker 构建脚本 | curl, sha256sum, skopeo/crane |

### 4.2 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `RELEASE_VERSION` | `v1.31.1` | CAPBM 发布版本 |
| `K8S_VERSION` | `v1.31.1` | Kubernetes 版本 |
| `CONTAINERD_VERSION` | `1.7.24` | Containerd 版本 |
| `HELM_VERSION` | `v3.15.0` | Helm 版本 |
| `CNI_PLUGINS_VERSION` | `v1.5.0` | CNI 插件版本 |
| `CALICO_VERSION` | `v3.28.1` | Calico 版本 |
| `CEPH_CSI_VERSION` | `v3.11.0` | Ceph CSI 版本 |
| `METALLB_VERSION` | `v0.14.8` | MetalLB 版本 |
| `GATEWAY_API_VERSION` | `v1.1.0` | Gateway API 版本 |
| `OUTPUT_DIR` | `release-image` | 输出目录 |
| `FORCE_DOWNLOAD` | `false` | 强制重新下载所有文件 |
| `IMAGE_TOOL` | `auto` | 镜像工具：`auto`/`skopeo`/`crane` |

### 4.3 使用示例

```bash
# 默认构建
./scripts/build-release-image-no-docker.sh

# 指定版本和输出目录
RELEASE_VERSION=v1.32.0 K8S_VERSION=v1.32.0 \
  OUTPUT_DIR=release-image-v1.32.0 \
  ./scripts/build-release-image-no-docker.sh

# 强制重新下载
FORCE_DOWNLOAD=true ./scripts/build-release-image-no-docker.sh

# 使用 crane 替代 skopeo
IMAGE_TOOL=crane ./scripts/build-release-image-no-docker.sh
```

### 4.4 下载源

| 组件 | 下载源 |
|------|--------|
| Kubernetes | `https://dl.k8s.io/{version}/kubernetes-server-linux-{arch}.tar.gz` |
| Containerd | `https://github.com/containerd/containerd/releases/download/v{version}/containerd-{version}-linux-{arch}.tar.gz` |
| Helm | `https://get.helm.sh/helm-{version}-linux-{arch}.tar.gz` |
| CNI Plugins | `https://github.com/containernetworking/plugins/releases/download/{version}/cni-plugins-linux-{arch}-{version}.tgz` |
| Calico Chart | `https://docs.tigera.io/calico/charts/` |
| Ceph CSI Chart | `https://ceph.github.io/csi-charts/` |
| MetalLB Manifest | `https://raw.githubusercontent.com/metallb/metallb/{version}/config/manifests/metallb-native.yaml` |
| Gateway API Manifest | `https://github.com/kubernetes-sigs/gateway-api/releases/download/{version}/standard-install.yaml` |
| 容器镜像 | `registry.k8s.io`, `docker.io`, `quay.io` |

---

## 5. 文件命名规范

### 5.1 二进制文件

| 组件 | 命名格式 | 示例 |
|------|----------|------|
| Kubernetes (deb) | `kubeadm`, `kubelet`, `kubectl` | `kubeadm` (直接二进制) |
| Kubernetes (rpm) | `kubeadm`, `kubelet`, `kubectl` | `kubeadm` (直接二进制) |
| Containerd | `containerd.tar.gz` | `containerd.tar.gz` |
| Helm | `helm.tar.gz` | `helm.tar.gz` |
| CNI Plugins | `cni-plugins.tgz` | `cni-plugins.tgz` |

### 5.2 容器镜像

```
images/{registry}/{repo}/{image}/{tag}.tar
```

示例：
- `images/registry.k8s.io/kube-apiserver/v1.31.1.tar`
- `images/docker.io/calico/node/v3.28.1.tar`
- `images/quay.io/cephcsi/cephcsi/v3.11.0.tar`

### 5.3 Charts 和 Manifests

```
addons/{addon}/{version}/charts/{chart-name}.tgz
addons/{addon}/{version}/manifests/{manifest-name}.yaml
```

示例：
- `addons/calico/v3.28.1/charts/tigera-operator.tgz`
- `addons/ceph-csi/v3.11.0/charts/ceph-csi-rbd.tgz`
- `addons/metallb/v0.14.8/manifests/metallb-native.yaml`
- `addons/gateway-api/v1.1.0/manifests/standard-install.yaml`

### 5.4 升级/回滚脚本

```
binaries/{component}/{version}/upgrade.sh
binaries/{component}/{version}/rollback.sh
addons/{addon}/{version}/rollback.sh
```

示例：
- `binaries/kubernetes/v1.31.1/upgrade.sh`
- `binaries/containerd/1.7.24/rollback.sh`
- `addons/calico/v3.28.1/rollback.sh`

---

## 6. 校验和

所有文件（除 `.sig` 和校验和文件本身外）的 SHA256 校验和存储在 `checksums/sha256sums.txt`：

```
<sha256>  ./binaries/kubernetes/v1.31.1/ubuntu/amd64/kubeadm
<sha256>  ./binaries/containerd/1.7.24/linux/amd64/containerd.tar.gz
<sha256>  ./images/registry.k8s.io/kube-apiserver/v1.31.1.tar
...
```

验证命令：
```bash
cd release-image
sha256sum -c checksums/sha256sums.txt
```

---

## 7. 升级流程

### 7.1 升级阶段

```
Phase 1 (blocking): containerd
       ↓
Phase 2 (blocking): kubernetes
       ↓
Phase 3 (blocking): calico (CNI)
       ↓
Phase 4 (optional):  ceph-csi (CSI)
       ↓
Phase 5 (optional):  gateway-api, envoy-gateway
       ↓
Phase 6 (optional):  metallb (LoadBalancer)
       ↓
Phase 7 (blocking):  capi-core-controller
```

### 7.2 升级触发

```bash
# 1. 应用 ReleaseImage
kubectl apply -f release-image/release.json

# 2. 创建 ClusterVersion 触发升级
kubectl apply -f - <<EOF
apiVersion: capbm.io/v1alpha1
kind: ClusterVersion
metadata:
  name: upgrade-to-v1.31.1
spec:
  targetVersion: v1.31.1
  releaseImageRef: release-image
EOF
```

---

## 8. 版本兼容性

| CAPBM 版本 | Kubernetes | Containerd | Calico | Ceph CSI |
|-----------|-----------|-----------|--------|---------|
| v1.31.1 | v1.31.1 | 1.7.24 | v3.28.1 | v3.11.0 |
| v1.31.0 | v1.31.0 | 1.7.23 | v3.28.0 | v3.10.0 |
| v1.30.0 | v1.30.0 | 1.7.20 | v3.27.0 | v3.9.0 |
