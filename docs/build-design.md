# CAPBM 构建设计

## 概述

本文档描述 Cluster API Provider Bare Metal (CAPBM) 的构建和发布流程。项目采用多模块架构，包含 CVO (Cluster Version Operator) 和 CAPBM (Infrastructure Provider) 两个独立模块，以及 ReleaseImage 镜像构建。

## 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│ 开发者工作流程                                               │
│                                                             │
│  1. 修改代码 (modules/cvo/ 或 modules/capbm/)               │
│  2. 运行测试 (go test ./...)                                │
│  3. 创建 Git tag (git tag v0.1.0)                           │
│  4. 推送 tag (git push origin v0.1.0)                       │
│  5. 触发 GitHub Actions Release workflow                    │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ GitHub Actions Release Workflow                              │
│                                                             │
│  1. 构建 CVO Docker 镜像并推送到 GHCR                       │
│  2. 构建 CAPBM Docker 镜像并推送到 GHCR                     │
│  3. 构建 ReleaseImage OCI 镜像 (可选)                       │
│  4. 生成 infrastructure-components.yaml                     │
│  5. 生成 cvo-components.yaml                                │
│  6. 创建 GitHub Release 并上传 manifests                    │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 用户安装流程                                                 │
│                                                             │
│  # 安装 CAPI 核心组件                                       │
│  clusterctl init --core cluster-api --bootstrap kubeadm     │
│    --control-plane kubeadm                                  │
│                                                             │
│  # 安装 CAPBM provider                                      │
│  kubectl apply -k modules/capbm/config/default/             │
│                                                             │
│  # 安装 CVO (version operator)                              │
│  kubectl apply -k modules/cvo/config/default/               │
│                                                             │
│  # 部署 ClusterClass templates                              │
│  kubectl apply -k modules/capbm/config/clusterclass/        │
└─────────────────────────────────────────────────────────────┘
```

## 项目结构

```
cluster-api-provider-baremetal/
├── modules/
│   ├── cvo/                    # Cluster Version Operator
│   │   ├── go.mod
│   │   ├── api/v1beta1/        # CVO API types
│   │   ├── cmd/manager/        # CVO entry point
│   │   ├── internal/           # CVO controllers & logic
│   │   └── config/             # CVO deployment configs
│   │
│   └── capbm/                  # CAPBM Infrastructure Provider
│       ├── go.mod
│       ├── api/v1beta1/        # CAPBM API types
│       ├── cmd/manager/        # CAPBM entry point
│       ├── internal/           # Controllers, SSH, LB, etc.
│       └── config/             # CAPBM deployment configs
│
├── release-image/              # ReleaseImage 内容目录
│   ├── release.json            # ReleaseImage spec
│   ├── binaries/               # 二进制组件
│   ├── images/                 # 容器镜像
│   ├── charts/                 # Helm charts
│   ├── manifests/              # Kubernetes manifests
│   └── scripts/                # 升级/回滚脚本
│
├── go.work                     # Go workspace definition
├── Makefile
├── Dockerfile.cvo              # CVO Docker build
├── Dockerfile.capbm            # CAPBM Docker build
└── Dockerfile.release          # ReleaseImage Docker build
```

## 核心文件

### go.work

定义 Go workspace，包含两个模块：

```go
go 1.26

use (
    ./modules/cvo
    ./modules/capbm
)
```

### metadata.yaml

定义 API 版本契约和发布系列。clusterctl 使用此文件确定哪些 provider 版本与当前 Cluster API 版本兼容。

```yaml
apiVersion: clusterctl.cluster.x8s.io/v1alpha3
kind: Metadata
releaseSeries:
  - major: 0
    minor: 1
    contract: v1beta1
```

### Dockerfile.cvo

构建 CVO manager 镜像：

```dockerfile
FROM golang:1.26 AS builder
WORKDIR /workspace
COPY modules/cvo/go.mod modules/cvo/go.mod
COPY modules/cvo/go.sum modules/cvo/go.sum
RUN cd modules/cvo && go mod download
COPY modules/cvo/ modules/cvo/
RUN cd modules/cvo && go build -o /manager ./cmd/manager/

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
```

### Dockerfile.capbm

构建 CAPBM manager 镜像：

```dockerfile
FROM golang:1.26 AS builder
WORKDIR /workspace
COPY modules/capbm/go.mod modules/capbm/go.mod
COPY modules/capbm/go.sum modules/capbm/go.sum
COPY modules/cvo/go.mod modules/cvo/go.mod
COPY modules/cvo/go.sum modules/cvo/go.sum
RUN cd modules/capbm && go mod download
COPY modules/capbm/ modules/capbm/
COPY modules/cvo/ modules/cvo/
RUN cd modules/capbm && go build -o /manager ./cmd/manager/

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
```

### Dockerfile.release

构建 ReleaseImage OCI 镜像：

```dockerfile
FROM scratch

COPY release-image/release.json /release.json
COPY release-image/binaries/ /binaries/
COPY release-image/images/ /images/
COPY release-image/charts/ /charts/
COPY release-image/manifests/ /manifests/
COPY release-image/scripts/ /scripts/
COPY release-image/checksums/ /checksums/

LABEL org.opencontainers.image.title="CAPBM Release Image"
LABEL org.opencontainers.image.version="v1.31.1"
```

## Makefile Targets

### 构建

| Target | 说明 |
|--------|------|
| `make build` | 构建 CVO 和 CAPBM manager 二进制 |
| `make build-cvo` | 构建 CVO manager 二进制 |
| `make build-capbm` | 构建 CAPBM manager 二进制 |

### 测试

| Target | 说明 |
|--------|------|
| `make test` | 运行所有测试 |

### Docker

| Target | 说明 |
|--------|------|
| `make docker-build-cvo` | 构建 CVO Docker 镜像 |
| `make docker-build-capbm` | 构建 CAPBM Docker 镜像 |
| `make docker-push-cvo` | 推送 CVO Docker 镜像 |
| `make docker-push-capbm` | 推送 CAPBM Docker 镜像 |
| `make docker-buildx` | 构建跨平台 Docker 镜像 |

### Release Image

| Target | 说明 |
|--------|------|
| `make release-image-build` | 构建 ReleaseImage OCI 镜像 |
| `make release-image-push` | 推送 ReleaseImage OCI 镜像 |
| `make release-image` | 构建并推送 ReleaseImage |

### 部署

| Target | 说明 |
|--------|------|
| `make install-cvo` | 安装 CVO CRDs |
| `make install-capbm` | 安装 CAPBM CRDs |
| `make deploy-cvo` | 部署 CVO controller |
| `make deploy-capbm` | 部署 CAPBM controller |
| `make deploy-clusterclass` | 部署 ClusterClass templates |
| `make undeploy-cvo` | 卸载 CVO controller |
| `make undeploy-capbm` | 卸载 CAPBM controller |

### Release

| Target | 说明 |
|--------|------|
| `make release-cvo` | 生成 CVO release manifests |
| `make release-capbm` | 生成 CAPBM release manifests |
| `make release-manifests VERSION=v0.1.0` | 生成发布目录 |

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CVO_IMG` | `ghcr.io/betawater/cvo-manager:v0.1.0` | CVO manager 镜像 |
| `CAPBM_IMG` | `ghcr.io/betawater/capbm-manager:v0.1.0` | CAPBM manager 镜像 |
| `RELEASE_IMG` | `ghcr.io/betawater/capbm/release:v1.31.1` | ReleaseImage 镜像 |
| `ENVTEST_K8S_VERSION` | `1.31.0` | envtest Kubernetes 版本 |

## 开发者工作流程

### 1. 本地开发

```bash
# 克隆仓库
git clone https://github.com/BetaWater/cluster-api-provider-baremetal.git
cd cluster-api-provider-baremetal

# 初始化 Go workspace
go work sync

# 下载依赖
cd modules/cvo && go mod download
cd ../capbm && go mod download
cd ../..

# 运行测试
make test

# 本地运行 CVO
make run-cvo

# 本地运行 CAPBM
make run-capbm
```

### 2. 构建和推送

```bash
# 构建 CVO 镜像
make docker-build-cvo CVO_IMG=your-registry/cvo-manager:v0.1.0

# 构建 CAPBM 镜像
make docker-build-capbm CAPBM_IMG=your-registry/capbm-manager:v0.1.0

# 推送镜像
make docker-push-cvo CVO_IMG=your-registry/cvo-manager:v0.1.0
make docker-push-capbm CAPBM_IMG=your-registry/capbm-manager:v0.1.0
```

### 3. 构建 ReleaseImage

```bash
# 1. 填充 release-image/ 目录
#    - binaries/
#    - images/
#    - charts/
#    - manifests/
#    - scripts/

# 2. 构建 ReleaseImage
make release-image-build RELEASE_IMG=your-registry/capbm/release:v1.31.1

# 3. 推送 ReleaseImage
make release-image-push RELEASE_IMG=your-registry/capbm/release:v1.31.1
```

### 4. 发布

```bash
# 创建 Git tag
git tag v0.1.0
git push origin v0.1.0

# 生成 release manifests
make release-manifests VERSION=v0.1.0

# 输出:
# - releases/v0.1.0/infrastructure-components.yaml
# - releases/v0.1.0/cvo-components.yaml
# - releases/v0.1.0/metadata.yaml
```

## CI/CD 流程

### GitHub Actions Workflows

| Workflow | 触发条件 | 说明 |
|----------|---------|------|
| `ci.yaml` | push to main, PR | 运行测试和 lint |
| `release.yaml` | push tag `v*` | 构建和发布 |

### Release Workflow

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Login to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push CVO Docker image
        run: |
          make docker-build-cvo CVO_IMG=ghcr.io/${{ github.repository }}/cvo-manager:${{ github.ref_name }}

      - name: Build and push CAPBM Docker image
        run: |
          make docker-build-capbm CAPBM_IMG=ghcr.io/${{ github.repository }}/capbm-manager:${{ github.ref_name }}

      - name: Generate CVO release manifests
        run: |
          make release-cvo CVO_IMG=ghcr.io/${{ github.repository }}/cvo-manager:${{ github.ref_name }}

      - name: Generate CAPBM release manifests
        run: |
          make release-capbm CAPBM_IMG=ghcr.io/${{ github.repository }}/capbm-manager:${{ github.ref_name }}

      - name: Create Release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            infrastructure-components.yaml
            cvo-components.yaml
            metadata.yaml
          generate_release_notes: true
```
