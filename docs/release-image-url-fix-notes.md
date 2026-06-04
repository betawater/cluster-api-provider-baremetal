# ReleaseImage 构建脚本 URL 修复说明

## 修复概述

本次修复解决了 `build-release-image.sh` 和 `build-release-image-no-docker.sh` 中的下载 URL 问题。

---

## 修复内容

### 1. Kubernetes 二进制文件 URL

**问题**:
- 原脚本使用 `pkgs.k8s.io` 的 URL，但 URL 格式不正确
- 包命名格式错误（`-1.1` 和 `-150500.1.1` 后缀不存在）

**修复方案**:
- 改用 `dl.k8s.io` 下载 Kubernetes server 包
- 从 server 包中提取 kubeadm、kubelet、kubectl 二进制文件

**修复后的 URL**:
```bash
https://dl.k8s.io/v1.31.1/kubernetes-server-linux-amd64.tar.gz
```

**下载流程**:
1. 下载 server 包
2. 解压到临时目录
3. 提取 `kubernetes/server/bin/kubeadm`、`kubelet`、`kubectl`
4. 复制到输出目录（所有 OS 平台共用相同的二进制文件）

---

### 2. Helm Charts 下载

**问题**:
- 原脚本尝试直接从 GitHub Releases 下载 charts
- 但 Calico 和 Ceph CSI charts 通常不在 GitHub Releases 中

**修复方案**:

#### `build-release-image.sh`（需要 Helm CLI）
使用 Helm CLI 从 Helm repo 下载 charts：

```bash
helm repo add projectcalico https://docs.tigera.io/calico/charts
helm pull projectcalico/tigera-operator --version v3.28.1 -d charts/

helm repo add ceph-csi https://ceph.github.io/csi-charts
helm pull ceph-csi/ceph-csi-rbd --version v3.11.0 -d charts/
```

#### `build-release-image-no-docker.sh`（无需 Helm CLI）
从 Helm repo index.yaml 解析 charts 的下载 URL：

```bash
# 从 index.yaml 解析 URL
curl -sL "https://docs.tigera.io/calico/charts/index.yaml" | \
    grep -A5 "tigera-operator-v3.28.1" | \
    grep "url:" | head -1 | \
    sed 's/.*url: //'
```

如果解析失败，尝试从 GitHub Releases 下载作为备选方案。

---

### 3. 其他组件 URL（无需修复）

| 组件 | URL | 状态 |
|------|-----|------|
| **Containerd** | `https://github.com/containerd/containerd/releases/download/...` | ✅ 正确 |
| **CNI Plugins** | `https://github.com/containernetworking/plugins/releases/download/...` | ✅ 正确 |
| **Helm** | `https://get.helm.sh/helm-v3.15.0-linux-amd64.tar.gz` | ✅ 正确 |
| **MetalLB Manifest** | `https://raw.githubusercontent.com/metallb/metallb/...` | ✅ 正确 |
| **Gateway API Manifest** | `https://github.com/kubernetes-sigs/gateway-api/releases/...` | ✅ 正确 |

---

## 验证方法

### 测试 Kubernetes 下载

```bash
# 测试 dl.k8s.io URL
curl -I https://dl.k8s.io/v1.31.1/kubernetes-server-linux-amd64.tar.gz
# 应该返回 HTTP 200 或 302
```

### 测试 Helm Charts 下载

```bash
# 测试 Calico Helm repo
curl -sL https://docs.tigera.io/calico/charts/index.yaml | head -20

# 测试 Ceph CSI Helm repo
curl -sL https://ceph.github.io/csi-charts/index.yaml | head -20
```

### 测试容器镜像拉取

```bash
# 使用 skopeo 测试
skopeo inspect docker://registry.k8s.io/kube-apiserver:v1.31.1

# 使用 crane 测试
crane digest registry.k8s.io/kube-apiserver:v1.31.1
```

---

## 已知限制

### 网络问题

| 问题 | 影响 | 解决方案 |
|------|------|---------|
| 中国大陆访问 docker.io/quay.io 受限 | 容器镜像拉取失败 | 使用代理或镜像 registry |
| GitHub Releases 访问受限 | 二进制文件下载失败 | 使用国内镜像站 |
| Helm repo 访问受限 | Charts 下载失败 | 手动下载 charts 并放入目录 |

### 备选方案

对于完全离线或网络受限的环境：

1. **预构建包**: 在有网络的环境预先构建完整的 release-image
2. **内部仓库**: 将组件上传到内部文件服务器或 artifact repository
3. **手动下载**: 手动下载所有组件并放入 `release-image/` 目录

---

## 相关文档

- [ReleaseImage 构建指南（无需 Docker 和 Helm）](./release-image-build-no-docker-guide.md)
- [ReleaseImage 构建设计](./release-image-build-design.md)
- [ReleaseImage 实现设计](./releaseimage-implementation-design.md)
