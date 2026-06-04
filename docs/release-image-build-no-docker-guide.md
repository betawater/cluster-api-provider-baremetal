# ReleaseImage 构建指南（无需 Docker 和 Helm）

## 1. 概述

本指南介绍如何在不安装 Docker 和 Helm CLI 的情况下构建完整的 ReleaseImage 离线包。

### 核心优势

| 优势 | 说明 |
|------|------|
| **无需 Docker daemon** | 使用 skopeo 或 crane 直接操作 registry |
| **无需 Helm CLI** | 直接从 GitHub Releases 下载 Helm charts |
| **轻量级** | 仅需 curl、sha256sum 和 skopeo/crane |
| **完整离线包** | 包含所有二进制文件、容器镜像、charts 和 manifests |

---

## 2. 依赖要求

### 必需工具

| 工具 | 用途 | 安装方式 |
|------|------|---------|
| curl | 下载文件 | `sudo apt-get install -y curl` |
| sha256sum | 生成校验和 | `sudo apt-get install -y coreutils` |
| skopeo 或 crane | 拉取容器镜像 | 见下方安装指南 |

### 可选工具

| 工具 | 用途 | 说明 |
|------|------|------|
| skopeo | 拉取容器镜像（推荐） | 功能更全面，支持多种镜像格式 |
| crane | 拉取容器镜像（备选） | 更轻量，Google 维护 |

---

## 3. 安装指南

### 3.1 安装 skopeo（推荐）

#### Ubuntu/Debian

```bash
sudo apt-get update
sudo apt-get install -y skopeo

# 验证安装
skopeo --version
```

#### CentOS/RHEL

```bash
sudo yum install -y skopeo

# 验证安装
skopeo --version
```

#### 从 GitHub Releases 安装（所有平台）

```bash
# 下载 skopeo 二进制
curl -L -o skopeo.tar.gz https://github.com/containers/skopeo/releases/download/v1.15.0/skopeo-linux-amd64.tar.gz

# 解压
tar -xzf skopeo.tar.gz

# 安装
sudo mv skopeo /usr/local/bin/

# 验证
skopeo --version
```

### 3.2 安装 crane（备选）

#### 从 GitHub Releases 安装

```bash
# 下载 crane 二进制
curl -L -o crane.tar.gz https://github.com/google/go-containerregistry/releases/download/v0.20.0/go-containerregistry_Linux_x86_64.tar.gz

# 解压
tar -xzf crane.tar.gz

# 安装
sudo mv crane /usr/local/bin/

# 验证
crane version
```

### 3.3 修复 Docker 权限（如果使用 build-release-image.sh）

如果您选择使用 `build-release-image.sh`（需要 Docker 的版本），可能会遇到 Docker 权限问题：

```
permission denied while trying to connect to the docker API at unix:///var/run/docker.sock
```

**解决方案**：

```bash
# 1. 将当前用户添加到 docker 组
sudo usermod -aG docker $USER

# 2. 重新登录或运行以下命令使更改生效
newgrp docker

# 3. 验证 Docker 权限
docker ps
```

**验证 Docker 安装**：

```bash
# 检查 Docker 是否安装
docker --version

# 检查 Docker 服务是否运行
sudo systemctl status docker

# 如果未运行，启动 Docker
sudo systemctl start docker
sudo systemctl enable docker
```

**注意**：
- 添加用户到 docker 组后，需要重新登录或运行 `newgrp docker` 才能使更改生效
- 如果使用 SSH 连接，需要断开并重新连接
- 在 CI/CD 环境中，可能需要使用 `sudo` 运行 Docker 命令

---

## 4. 构建 ReleaseImage

### 4.1 使用 skopeo（默认）

```bash
# 1. 赋予脚本执行权限
chmod +x scripts/build-release-image-no-docker.sh

# 2. 运行构建脚本（默认使用 skopeo）
./scripts/build-release-image-no-docker.sh

# 3. 验证构建结果
ls -la release-image/
cat release-image/checksums/sha256sums.txt | head -20
```

### 4.2 使用 crane

```bash
# 1. 设置环境变量使用 crane
export IMAGE_TOOL=crane

# 2. 运行构建脚本
./scripts/build-release-image-no-docker.sh

# 3. 验证构建结果
ls -la release-image/
```

### 4.3 自定义版本

```bash
# 设置自定义版本
export RELEASE_VERSION=v1.32.0
export CONTAINERD_VERSION=1.7.25
export CALICO_VERSION=v3.29.0
export CEPH_CSI_VERSION=v3.12.0

# 运行构建
./scripts/build-release-image-no-docker.sh
```

### 4.4 自定义输出目录

```bash
# 设置自定义输出目录
export OUTPUT_DIR=/tmp/my-release-image

# 运行构建
./scripts/build-release-image-no-docker.sh
```

---

## 5. 输出内容

### 5.1 目录结构

```
release-image/
├── release.json                          # ReleaseImage spec
├── binaries/                             # 二进制组件
│   ├── kubernetes/                       # Kubernetes 二进制
│   │   ├── ubuntu/                       # Ubuntu 平台
│   │   │   ├── amd64/
│   │   │   └── arm64/
│   │   ├── debian/                       # Debian 平台
│   │   │   ├── amd64/
│   │   │   └── arm64/
│   │   └── centos/                       # CentOS/RHEL 平台
│   │       ├── amd64/
│   │       └── arm64/
│   ├── containerd/                       # Containerd 二进制
│   └── cni-plugins/                      # CNI 插件二进制
├── images/                               # 容器镜像 tar 包
│   ├── registry.k8s.io_kube-apiserver_v1.31.1.tar
│   ├── registry.k8s.io_kube-controller-manager_v1.31.1.tar
│   ├── ... (其他镜像)
├── charts/                               # Helm charts
│   ├── tigera-operator-v3.28.1.tgz
│   └── ceph-csi-rbd-v3.11.0.tgz
├── manifests/                            # Kubernetes manifests
│   ├── metallb-v0.14.8.yaml
│   └── gateway-api-v1.1.0.yaml
├── scripts/                              # 升级/回滚脚本
└── checksums/                            # 校验和文件
    └── sha256sums.txt
```

### 5.2 文件大小估算

| 组件 | 大小估算 |
|------|---------|
| 二进制文件 | ~500MB |
| 容器镜像 | ~5-8GB |
| Helm charts | ~50MB |
| Manifests | ~10MB |
| 其他 | ~10MB |
| **总计** | **~6-10GB** |

---

## 6. 下载源说明

### 6.1 Kubernetes 二进制文件

脚本从 `dl.k8s.io` 下载 Kubernetes server 包，然后提取 kubeadm、kubelet、kubectl：

```bash
https://dl.k8s.io/v1.31.1/kubernetes-server-linux-amd64.tar.gz
```

### 6.2 容器镜像

使用 skopeo 或 crane 从以下 registry 拉取：

| Registry | 镜像 |
|----------|------|
| `registry.k8s.io` | kube-apiserver, kube-controller-manager, kube-scheduler, kube-proxy, pause, etcd, coredns |
| `docker.io` | calico/node, calico/kube-controllers, calico/cni |
| `quay.io` | cephcsi, csi-attacher, csi-provisioner, csi-snapshotter, csi-resizer, csi-node-driver-registrar, metallb |

### 6.3 Helm Charts

脚本从 Helm repo index.yaml 解析 charts 的下载 URL：

| Chart | Helm Repo |
|-------|-----------|
| Calico (tigera-operator) | https://docs.tigera.io/calico/charts |
| Ceph CSI (ceph-csi-rbd) | https://ceph.github.io/csi-charts |

### 6.4 Kubernetes Manifests

| Manifest | 下载源 |
|----------|--------|
| MetalLB | https://raw.githubusercontent.com/metallb/metallb/... |
| Gateway API | https://github.com/kubernetes-sigs/gateway-api/releases/... |

---

## 7. 使用 ReleaseImage

### 7.1 应用 ReleaseImage

```bash
kubectl apply -f release-image/release.json
```

### 7.2 创建 ClusterVersion 触发升级

```bash
kubectl apply -f clusterversion.yaml
```

### 7.3 验证校验和

```bash
cd release-image
sha256sum -c checksums/sha256sums.txt
```

---

## 8. 故障排查

### 8.1 常见问题

| 问题 | 原因 | 解决方案 |
|------|------|---------|
| skopeo/crane 未找到 | 未安装 | 安装 skopeo 或 crane |
| 下载失败 | 网络问题 | 检查网络连接，使用代理 |
| 拉取镜像失败 | 认证问题 | 配置 registry 认证 |
| 磁盘空间不足 | 空间不足 | 清理磁盘或增加空间 |
| 校验和不匹配 | 文件损坏 | 重新下载文件 |
| Docker 权限拒绝 | 用户不在 docker 组 | 运行 `sudo usermod -aG docker $USER && newgrp docker` |

### 8.2 调试模式

```bash
# 启用 bash 调试
bash -x scripts/build-release-image-no-docker.sh

# 跳过某些步骤
export SKIP_IMAGES=true   # 跳过容器镜像下载
export SKIP_CHARTS=true   # 跳过 Helm charts 下载
```

---

## 9. skopeo vs crane 对比

| 特性 | skopeo | crane |
|------|--------|-------|
| **开发者** | Red Hat/Containers | Google |
| **大小** | ~50MB | ~30MB |
| **功能** | 更全面 | 专注于 registry 操作 |
| **镜像格式** | 多种格式 | docker-archive, OCI |
| **认证** | 支持多种认证 | 支持标准认证 |
| **推荐场景** | 生产环境、多格式支持 | 轻量级、CI/CD |

### 选择建议

| 场景 | 推荐工具 |
|------|---------|
| 生产环境构建 | skopeo |
| CI/CD 环境 | crane（更轻量） |
| 需要多种镜像格式 | skopeo |
| 磁盘空间受限 | crane |

---

## 10. 相关资源

- [skopeo GitHub](https://github.com/containers/skopeo)
- [crane GitHub](https://github.com/google/go-containerregistry)
- [ReleaseImage 构建设计](./release-image-build-design.md)
- [ReleaseImage 实现设计](./releaseimage-implementation-design.md)
- [CAPBM 用户指南](./user-guide.md)
