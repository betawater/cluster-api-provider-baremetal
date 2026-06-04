# ReleaseImage 构建设计文档

## 1. 概述

### 1.1 设计目标

ReleaseImage 是 CAPBM 的组件版本管理核心，通过自包含的目录结构分发所有组件，支持在线和离线（air-gapped）安装模式。

**核心特性**:
- **自包含**: 一个目录包含所有组件，无需外网访问
- **版本一致**: 所有组件版本由 ReleaseImage 统一管理
- **多架构支持**: 支持 linux-amd64 和 linux-arm64
- **多 OS 支持**: 支持 Ubuntu、Debian 和 CentOS/RHEL
- **组件类型自描述**: 通过 release.json 标识 binary/manifest/helm 类型
- **可验证**: 提供 SHA256 校验和文件验证完整性

### 1.2 使用场景

| 场景 | 说明 |
|------|------|
| **在线安装** | 从 ReleaseImage 目录直接安装组件 |
| **离线安装** | 将整个 ReleaseImage 目录传输到 air-gapped 环境 |
| **版本升级** | 通过 ClusterVersion 触发升级到新版本 |
| **版本回滚** | 通过 ClusterVersion 回滚到旧版本 |

---

## 2. 目录结构

### 2.1 完整目录结构

```
release-image/
├── release.json                          # ReleaseImage spec 定义
├── binaries/                             # 二进制组件
│   ├── kubernetes/                       # Kubernetes 二进制
│   │   ├── ubuntu/                       # Ubuntu 平台
│   │   │   ├── amd64/
│   │   │   │   ├── kubeadm_1.31.1-00_amd64.deb
│   │   │   │   ├── kubelet_1.31.1-00_amd64.deb
│   │   │   │   └── kubectl_1.31.1-00_amd64.deb
│   │   │   └── arm64/
│   │   │       ├── kubeadm_1.31.1-00_arm64.deb
│   │   │       ├── kubelet_1.31.1-00_arm64.deb
│   │   │       └── kubectl_1.31.1-00_arm64.deb
│   │   ├── debian/                       # Debian 平台
│   │   │   ├── amd64/
│   │   │   └── arm64/
│   │   └── centos/                       # CentOS/RHEL 平台
│   │       ├── amd64/
│   │       │   ├── kubeadm-1.31.1-0.x86_64.rpm
│   │       │   ├── kubelet-1.31.1-0.x86_64.rpm
│   │       │   └── kubectl-1.31.1-0.x86_64.rpm
│   │       └── arm64/
│   │           ├── kubeadm-1.31.1-0.aarch64.rpm
│   │           ├── kubelet-1.31.1-0.aarch64.rpm
│   │           └── kubectl-1.31.1-0.aarch64.rpm
│   ├── containerd/                       # Containerd 二进制
│   │   ├── containerd-1.7.24-linux-amd64.tar.gz
│   │   └── containerd-1.7.24-linux-arm64.tar.gz
│   ├── helm/                             # Helm 二进制
│   │   ├── helm-v3.15.0-linux-amd64.tar.gz
│   │   └── helm-v3.15.0-linux-arm64.tar.gz
│   └── cni-plugins/                      # CNI 插件二进制
│       ├── cni-plugins-linux-amd64-v1.5.0.tgz
│       └── cni-plugins-linux-arm64-v1.5.0.tgz
├── images/                               # 容器镜像 tar 包
│   ├── registry.k8s.io_kube-apiserver_v1.31.1.tar
│   ├── registry.k8s.io_kube-controller-manager_v1.31.1.tar
│   ├── registry.k8s.io_kube-scheduler_v1.31.1.tar
│   ├── registry.k8s.io_kube-proxy_v1.31.1.tar
│   ├── registry.k8s.io_pause_3.9.tar
│   ├── registry.k8s.io_etcd_3.5.15-0.tar
│   ├── registry.k8s.io_coredns_coredns_v1.11.1.tar
│   ├── docker.io_calico_node_v3.28.1.tar
│   ├── docker.io_calico_kube-controllers_v3.28.1.tar
│   ├── docker.io_calico_cni_v3.28.1.tar
│   ├── quay.io_cephcsi_cephcsi_v3.11.0.tar
│   ├── quay.io_k8scsi_csi-attacher_v4.4.0.tar
│   ├── quay.io_k8scsi_csi-provisioner_v3.6.0.tar
│   ├── quay.io_k8scsi_csi-snapshotter_v6.3.0.tar
│   ├── quay.io_k8scsi_csi-resizer_v1.9.0.tar
│   ├── quay.io_k8scsi_csi-node-driver-registrar_v2.9.0.tar
│   ├── quay.io_metallb_controller_v0.14.8.tar
│   └── quay.io_metallb_speaker_v0.14.8.tar
├── charts/                               # Helm charts
│   ├── tigera-operator-v3.28.1.tgz
│   └── ceph-csi-rbd-v3.11.0.tgz
├── manifests/                            # Kubernetes manifests
│   ├── metallb-v0.14.8.yaml
│   └── gateway-api-v1.1.0.yaml
├── scripts/                              # 升级/回滚脚本
│   ├── upgrade-containerd.sh
│   ├── upgrade-kubernetes.sh
│   ├── rollback-containerd.sh
│   ├── rollback-kubernetes.sh
│   ├── rollback-calico.sh
│   ├── rollback-ceph-csi.sh
│   ├── rollback-capi-core.sh
│   ├── rollback-metallb.sh
│   ├── rollback-gateway-api.sh
│   └── rollback-envoy-gateway.sh
└── checksums/                            # 校验和文件
    ├── sha256sums.txt
    └── sha256sums.txt.sig
```

### 2.2 目录说明

| 目录 | 内容 | 大小估算 |
|------|------|---------|
| `binaries/` | 二进制组件（K8s、containerd、Helm、CNI） | ~500MB |
| `images/` | 容器镜像 tar 包（~20 个镜像） | ~5-8GB |
| `charts/` | Helm charts（Calico、Ceph CSI 等） | ~50MB |
| `manifests/` | Kubernetes manifests（MetalLB、Gateway API） | ~10MB |
| `scripts/` | 升级/回滚脚本 | ~50KB |
| `checksums/` | SHA256 校验和文件 | ~10KB |
| **总计** | | **~6-10GB** |

---

## 3. 组件版本

### 3.1 核心组件

| 组件 | 版本 | 类型 | 说明 |
|------|------|------|------|
| Kubernetes | v1.31.1 | binary | K8s 核心组件（kubeadm、kubelet、kubectl） |
| Containerd | 1.7.24 | binary | 容器运行时 |
| Helm | v3.15.0 | binary | Helm 包管理器 |
| CNI Plugins | v1.5.0 | binary | CNI 网络插件 |

### 3.2 Addon 组件

| 组件 | 版本 | 类型 | 说明 |
|------|------|------|------|
| Calico | v3.28.1 | helm | CNI 网络插件 |
| Ceph CSI | v3.11.0 | helm | CSI 存储驱动 |
| MetalLB | v0.14.8 | manifest | 负载均衡器 |
| Gateway API | v1.1.0 | manifest | Gateway API CRDs |
| CAPI Core | v1.7.0 | helm | Cluster API 核心控制器 |

### 3.3 容器镜像

| 镜像 | 版本 | 来源 |
|------|------|------|
| kube-apiserver | v1.31.1 | registry.k8s.io |
| kube-controller-manager | v1.31.1 | registry.k8s.io |
| kube-scheduler | v1.31.1 | registry.k8s.io |
| kube-proxy | v1.31.1 | registry.k8s.io |
| pause | 3.9 | registry.k8s.io |
| etcd | 3.5.15-0 | registry.k8s.io |
| coredns | v1.11.1 | registry.k8s.io |
| calico/node | v3.28.1 | docker.io |
| calico/kube-controllers | v3.28.1 | docker.io |
| calico/cni | v3.28.1 | docker.io |
| cephcsi | v3.11.0 | quay.io |
| csi-attacher | v4.4.0 | quay.io |
| csi-provisioner | v3.6.0 | quay.io |
| csi-snapshotter | v6.3.0 | quay.io |
| csi-resizer | v1.9.0 | quay.io |
| csi-node-driver-registrar | v2.9.0 | quay.io |
| metallb/controller | v0.14.8 | quay.io |
| metallb/speaker | v0.14.8 | quay.io |

---

## 4. 构建脚本

### 4.1 脚本位置

```
scripts/build-release-image.sh
```

### 4.2 脚本功能

| 功能 | 说明 |
|------|------|
| `check_requirements` | 检查依赖工具（Docker、Helm、curl、sha256sum） |
| `create_directory_structure` | 创建完整的目录结构 |
| `download_kubernetes` | 下载 Kubernetes 二进制（Ubuntu/Debian deb + CentOS rpm） |
| `download_containerd` | 下载 Containerd 二进制 |
| `download_helm` | 下载 Helm 二进制 |
| `download_cni_plugins` | 下载 CNI 插件二进制 |
| `pull_and_save_images` | 拉取并保存容器镜像 |
| `download_charts` | 下载 Helm charts |
| `generate_manifests` | 生成 Kubernetes manifests |
| `generate_checksums` | 生成 SHA256 校验和文件 |
| `generate_release_json` | 生成 release.json |

### 4.3 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `RELEASE_VERSION` | v1.31.1 | Kubernetes 版本 |
| `CONTAINERD_VERSION` | 1.7.24 | Containerd 版本 |
| `HELM_VERSION` | v3.15.0 | Helm 版本 |
| `CNI_PLUGINS_VERSION` | v1.5.0 | CNI Plugins 版本 |
| `CALICO_VERSION` | v3.28.1 | Calico 版本 |
| `CEPH_CSI_VERSION` | v3.11.0 | Ceph CSI 版本 |
| `METALLB_VERSION` | v0.14.8 | MetalLB 版本 |
| `GATEWAY_API_VERSION` | v1.1.0 | Gateway API 版本 |
| `CAPI_CORE_VERSION` | v1.7.0 | CAPI Core 版本 |
| `OUTPUT_DIR` | release-image | 输出目录 |

### 4.4 使用方法

```bash
# 1. 设置环境变量（可选）
export RELEASE_VERSION=v1.31.1
export CONTAINERD_VERSION=1.7.24

# 2. 运行构建脚本
chmod +x scripts/build-release-image.sh
./scripts/build-release-image.sh

# 3. 验证构建结果
ls -la release-image/
cat release-image/checksums/sha256sums.txt | head -20
```

---

## 5. 平台支持

### 5.1 支持的架构

| 架构 | 说明 |
|------|------|
| amd64 | x86_64 架构 |
| arm64 | ARM64/AArch64 架构 |

### 5.2 支持的操作系统

| OS | 包格式 | 说明 |
|----|--------|------|
| Ubuntu | deb | Ubuntu 20.04, 22.04 |
| Debian | deb | Debian 10, 11, 12 |
| CentOS/RHEL | rpm | CentOS 7, 8, 9 / RHEL 8, 9 |

### 5.3 包命名规范

**Ubuntu/Debian (deb)**:
```
kubeadm_1.31.1-00_amd64.deb
kubelet_1.31.1-00_amd64.deb
kubectl_1.31.1-00_amd64.deb
```

**CentOS/RHEL (rpm)**:
```
kubeadm-1.31.1-0.x86_64.rpm
kubelet-1.31.1-0.x86_64.rpm
kubectl-1.31.1-0.x86_64.rpm
```

---

## 6. release.json 规范

### 6.1 结构

```json
{
  "version": "v1.31.1",
  "image": "registry.example.com/capbm/release:v1.31.1",
  "httpServer": {
    "enabled": true,
    "port": 8080,
    "basePath": "/release/v1.31.1"
  },
  "imageRegistry": {
    "enabled": true,
    "registry": "registry.example.com",
    "repository": "capbm",
    "imagePrefix": "release"
  },
  "channels": ["stable", "fast"],
  "previousVersions": ["v1.31.0", "v1.30.0"],
  "components": {
    "kubernetes": { ... },
    "containerd": { ... },
    "helm": { ... },
    "cniPlugins": { ... }
  },
  "addons": [ ... ],
  "upgradeGraph": [ ... ],
  "contentHash": "sha256:abc123..."
}
```

### 6.2 组件定义示例

```json
{
  "kubernetes": {
    "version": "v1.31.1",
    "type": "binary",
    "path": "/opt/capbm/binaries/kubernetes",
    "platforms": {
      "ubuntu": {
        "architectures": ["amd64", "arm64"],
        "packages": {
          "kubeadm": "kubeadm_1.31.1-00",
          "kubelet": "kubelet_1.31.1-00",
          "kubectl": "kubectl_1.31.1-00"
        }
      },
      "debian": {
        "architectures": ["amd64", "arm64"],
        "packages": {
          "kubeadm": "kubeadm_1.31.1-00",
          "kubelet": "kubelet_1.31.1-00",
          "kubectl": "kubectl_1.31.1-00"
        }
      },
      "centos": {
        "architectures": ["amd64", "arm64"],
        "packages": {
          "kubeadm": "kubeadm-1.31.1-0",
          "kubelet": "kubelet-1.31.1-0",
          "kubectl": "kubectl-1.31.1-0"
        }
      }
    },
    "imageList": [
      "registry.k8s.io/kube-apiserver:v1.31.1",
      "registry.k8s.io/kube-controller-manager:v1.31.1",
      "registry.k8s.io/kube-scheduler:v1.31.1",
      "registry.k8s.io/kube-proxy:v1.31.1",
      "registry.k8s.io/pause:3.9",
      "registry.k8s.io/etcd:3.5.15-0",
      "registry.k8s.io/coredns/coredns:v1.11.1"
    ],
    "installStrategy": {
      "timeout": "600s",
      "retryCount": 3,
      "method": "package",
      "serviceName": "kubelet"
    },
    "upgradeStrategy": {
      "type": "Rolling",
      "maxConcurrent": 1,
      "timeout": "900s",
      "retryCount": 3,
      "drain": true
    },
    "upgrade": {
      "backup": {
        "enabled": true,
        "config": [
          {"path": "/etc/kubernetes", "type": "directory"}
        ],
        "etcdSnapshot": true
      },
      "rollback": {
        "script": "scripts/rollback-kubernetes.sh",
        "timeout": "600s"
      },
      "healthCheck": {
        "command": "kubectl get nodes",
        "timeout": "60s",
        "retries": 3
      }
    }
  }
}
```

---

## 7. 校验和验证

### 7.1 生成校验和

```bash
cd release-image
find . -type f \
    -not -name "*.sig" \
    -not -name "sha256sums.txt" \
    -not -path "./checksums/*" \
    -exec sha256sum {} \; > checksums/sha256sums.txt
```

### 7.2 验证校验和

```bash
cd release-image
sha256sum -c checksums/sha256sums.txt
```

### 7.3 签名验证（可选）

```bash
# 使用 GPG 签名
gpg --sign --detach-sign checksums/sha256sums.txt

# 验证签名
gpg --verify checksums/sha256sums.txt.sig
```

---

## 8. 依赖要求

### 8.1 构建环境

| 工具 | 版本 | 用途 |
|------|------|------|
| Docker | 20.10+ | 拉取和保存容器镜像 |
| Helm | 3.15+ | 下载 Helm charts |
| curl | 7.68+ | 下载二进制文件和 manifests |
| sha256sum | coreutils 8.30+ | 生成校验和 |
| bash | 5.0+ | 脚本执行 |

### 8.2 网络要求

| 域名 | 用途 |
|------|------|
| github.com | 下载 Containerd、Helm、CNI Plugins |
| docker.io | 拉取 Calico 镜像 |
| quay.io | 拉取 Ceph CSI、MetalLB 镜像 |
| registry.k8s.io | 拉取 K8s 核心镜像 |
| pkgs.k8s.io | 下载 K8s 二进制包 |
| docs.tigera.io | 下载 Calico Helm chart |
| ceph.github.io | 下载 Ceph CSI Helm chart |

---

## 9. 磁盘空间需求

| 组件 | 大小估算 |
|------|---------|
| Kubernetes 二进制 | ~150MB |
| Containerd 二进制 | ~100MB |
| Helm 二进制 | ~50MB |
| CNI Plugins 二进制 | ~50MB |
| 容器镜像 | ~5-8GB |
| Helm charts | ~50MB |
| Manifests | ~10MB |
| 其他 | ~10MB |
| **总计** | **~6-10GB** |

---

## 10. 故障排查

### 10.1 常见问题

| 问题 | 原因 | 解决方案 |
|------|------|---------|
| 下载失败 | 网络问题 | 检查网络连接，使用代理 |
| Docker 拉取失败 | 认证问题 | 配置 Docker 认证 |
| Helm 仓库不可达 | 仓库地址变更 | 更新 Helm 仓库地址 |
| 磁盘空间不足 | 空间不足 | 清理磁盘或增加空间 |
| 校验和不匹配 | 文件损坏 | 重新下载文件 |

### 10.2 调试模式

```bash
# 启用调试输出
set -x

# 跳过某些步骤
export SKIP_IMAGES=true
export SKIP_CHARTS=true

# 指定输出目录
export OUTPUT_DIR=/tmp/release-image-test
```

---

## 11. 相关资源

- [ReleaseImage CRD 设计](./releaseimage-design.md)
- [ReleaseImage 实现设计](./releaseimage-implementation-design.md)
- [CAPBM 用户指南](./user-guide.md)
- [构建设计](./build-design.md)
