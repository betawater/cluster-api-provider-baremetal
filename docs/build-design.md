# CAPBM 构建设计

## 概述

本文档描述 Cluster API Provider Bare Metal (CAPBM) 的构建和发布流程，使得用户可以通过 `clusterctl init --infrastructure baremetal` 安装到 Kubernetes 集群。

## 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│ 开发者工作流程                                               │
│                                                             │
│  1. 修改代码                                                 │
│  2. 运行测试 (make test)                                    │
│  3. 创建 Git tag (git tag v0.1.0)                           │
│  4. 推送 tag (git push origin v0.1.0)                       │
│  5. 触发 GitHub Actions Release workflow                    │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ GitHub Actions Release Workflow                              │
│                                                             │
│  1. 构建 Docker 镜像并推送到 GHCR                           │
│  2. 生成 infrastructure-components.yaml                     │
│  3. 创建 GitHub Release 并上传 manifests                    │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 用户安装流程                                                 │
│                                                             │
│  clusterctl init --infrastructure baremetal                 │
│                                                             │
│  1. 读取 metadata.yaml 获取版本信息                         │
│  2. 下载 infrastructure-components.yaml                     │
│  3. 应用 manifests 到集群                                   │
│  4. 创建 capbm-system namespace                             │
│  5. 部署 CAPBM Controller                                   │
└─────────────────────────────────────────────────────────────┘
```

## 核心文件

### metadata.yaml

定义 API 版本契约和发布系列。clusterctl 使用此文件确定哪些 provider 版本与当前 Cluster API 版本兼容。

```yaml
apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
kind: Metadata
releaseSeries:
  - major: 0
    minor: 1
    contract: v1beta1
```

**字段说明**:
- `contract`: 对应的 Cluster API contract 版本 (v1beta1)
- `major/minor`: provider 版本号

### infrastructure-components.yaml

通过 `kustomize build config/default` 生成的完整 manifests，包含：
- CRDs (所有自定义资源定义)
- Namespace (capbm-system)
- ServiceAccount
- ClusterRole / ClusterRoleBinding
- Deployment (controller-manager)

### Makefile Targets

| Target | 说明 |
|--------|------|
| `make build` | 构建 manager 二进制 |
| `make test` | 运行测试 |
| `make docker-build` | 构建 Docker 镜像 |
| `make docker-push` | 推送 Docker 镜像 |
| `make release` | 生成 release manifests |
| `make release-manifests VERSION=v0.1.0` | 生成发布目录 |
| `make deploy` | 部署到当前集群 |

## 发布流程

### 1. 准备发布

```bash
# 确保代码已提交
git add .
git commit -m "Prepare release v0.1.0"

# 更新 metadata.yaml (如果需要添加新版本)
# 当前已包含 v0.1.0 系列
```

### 2. 创建并推送 tag

```bash
git tag v0.1.0
git push origin v0.1.0
```

### 3. 自动发布

GitHub Actions 自动执行：
1. 构建 Docker 镜像并推送到 `ghcr.io`
2. 生成 `infrastructure-components.yaml`
3. 创建 GitHub Release 并附加 manifests

### 4. 用户安装

```bash
# 安装最新版本
clusterctl init --infrastructure baremetal

# 安装特定版本
clusterctl init --infrastructure baremetal:v0.1.0
```

## 目录结构

```
cluster-api-provider-baremetal/
├── metadata.yaml                    # 版本元数据 (clusterctl 需要)
├── Makefile                         # 构建目标
├── Dockerfile                       # 容器镜像定义
├── PROJECT                          # kubebuilder 项目配置
├── .github/
│   └── workflows/
│       └── release.yaml             # 自动发布 workflow
├── config/
│   ├── crd/                         # CRD 定义
│   ├── rbac/                        # RBAC 配置
│   ├── manager/                     # Manager Deployment
│   ├── default/                     # 默认 kustomize 入口
│   └── clusterclass/                # ClusterClass 模板
├── api/                             # API 类型定义
├── internal/                        # 控制器和工具
├── cmd/                             # 入口文件
└── releases/                        # 发布目录 (本地构建时生成)
    └── v0.1.0/
        ├── infrastructure-components.yaml
        └── metadata.yaml
```

## 镜像仓库

| 镜像 | 仓库 | 说明 |
|------|------|------|
| CAPBM Controller | `ghcr.io/betawater/cluster-api-provider-baremetal` | 主控制器镜像 |

## 版本策略

### SemVer 版本

遵循语义化版本规范：
- **Major**: 不兼容的 API 变更
- **Minor**: 向后兼容的功能新增
- **Patch**: 向后兼容的 bug 修复

### Contract 版本

- `v1beta1`: 对应 Cluster API `cluster.x-k8s.io/v1beta1` API

### 版本兼容性

| CAPBM 版本 | Cluster API 版本 | Kubernetes 版本 |
|-----------|-----------------|----------------|
| v0.1.x | v1.7+ | v1.30+ |

## 本地开发

### 构建和测试

```bash
# 下载依赖
go mod download

# 运行测试
make test

# 构建二进制
make build

# 构建 Docker 镜像
make docker-build IMG=capbm:local
```

### 本地部署

```bash
# 部署到当前集群
make deploy IMG=capbm:local

# 查看部署状态
kubectl get pods -n capbm-system

# 卸载
make undeploy
```

## 故障排查

### clusterctl 无法找到 provider

确保：
1. `metadata.yaml` 存在于 release 中
2. `infrastructure-components.yaml` 存在于 release 中
3. tag 格式正确 (vX.Y.Z)

### 镜像拉取失败

检查：
1. 镜像名称和 tag 是否正确
2. GHCR 访问权限
3. 网络连通性

## 参考资源

- [Cluster API Provider 开发指南](https://cluster-api.sigs.k8s.io/developer/providers/getting-started)
- [clusterctl 文档](https://cluster-api.sigs.k8s.io/clusterctl/overview)
- [Kubebuilder 文档](https://book.kubebuilder.io/)
