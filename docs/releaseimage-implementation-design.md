# ReleaseImage 完整端到端实现设计文档

## 一、概述

### 1.1 设计目标

ReleaseImage 是 CAPBM 项目的核心 CRD，用于定义一个完整的 Kubernetes 发行版本。本设计文档描述了 ReleaseImage 的完整端到端实现方案，包括核心基础设施、控制器实现、内容目录结构和构建部署流程。

| 目标 | 说明 |
|------|------|
| **版本管理** | 定义集群所有组件的版本映射 |
| **升级编排** | 定义升级顺序、依赖关系和策略 |
| **高内聚配置** | 每个组件自带安装、升级、备份、回滚配置 |
| **多架构支持** | 支持 amd64、arm64 等多种架构 |
| **多 OS 支持** | 支持 Ubuntu、CentOS、Rocky 等操作系统 |
| **离线支持** | 支持 air-gapped 环境的离线安装 |

### 1.2 API 信息

| 属性 | 值 |
|------|-----|
| **API Group** | `cvo.capbm.io` |
| **Version** | `v1beta1` |
| **Kind** | `ReleaseImage` |
| **Scope** | Namespaced |
| **Storage** | etcd |

---

## 二、架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                        管理集群 (Management Cluster)                  │
│                                                                     │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                    CVO Controller Manager                      │  │
│  │                                                               │  │
│  │  ┌─────────────────────┐  ┌────────────────────────────────┐  │  │
│  │  │ ReleaseImage        │  │ ClusterVersion                 │  │  │
│  │  │ Controller          │  │ Controller                     │  │  │
│  │  │ - 验证 contentHash  │  │ - 监控 DesiredUpdate 变更      │  │  │
│  │  │ - 统计 manifests    │  │ - 验证升级路径                 │  │  │
│  │  │ - 更新 status       │  │ - 获取 ReleaseImage            │  │  │
│  │  └─────────────────────┘  │ - 执行升级                     │  │  │
│  │                           └───────────────┬────────────────┘  │  │
│  │  ┌─────────────────────┐                  │                   │  │
│  │  │ ClusterAddon        │                  │                   │  │
│  │  │ Controller          │◄─────────────────┘                   │  │
│  │  │ - 监听 ClusterAddon │                                      │  │
│  │  │ - 比较版本          │                                      │  │
│  │  │ - 执行安装/升级     │                                      │  │
│  │  └─────────────────────┘                                      │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                    核心基础设施                                │  │
│  │                                                               │  │
│  │  ┌─────────────────────┐  ┌────────────────────────────────┐  │  │
│  │  │ OCIPuller           │  │ ContentFetcher                 │  │  │
│  │  │ - 拉取 OCI 镜像     │  │ - 从 HTTP/本地获取内容         │  │  │
│  │  │ - 解析 release.json │  │ - 获取 charts/manifests/scripts│  │  │
│  │  └─────────────────────┘  └────────────────────────────────┘  │  │
│  │                                                               │  │
│  │  ┌─────────────────────┐  ┌────────────────────────────────┐  │  │
│  │  │ decodeYAML          │  │ Installer 接口                 │  │  │
│  │  │ - 解码 YAML 为      │  │ - HelmInstaller                │  │  │
│  │  │   client.Object     │  │ - ManifestInstaller            │  │  │
│  │  └─────────────────────┘  └────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
         │
         ▼ (OCI 镜像)
┌─────────────────────────────────────────────────────────────────────┐
│                    ReleaseImage OCI 镜像                             │
│                                                                     │
│  release-image/                                                     │
│  ├── release.json                  # ReleaseImage spec              │
│  ├── binaries/                     # 二进制组件                     │
│  │   ├── kubernetes/               # K8S 二进制                     │
│  │   ├── containerd/               # containerd 二进制              │
│  │   ├── helm/                     # Helm 二进制                    │
│  │   └── cni-plugins/              # CNI 插件二进制                 │
│  ├── images/                       # 容器镜像 tar 包                │
│  ├── charts/                       # Helm charts                    │
│  ├── manifests/                    # Kubernetes manifests           │
│  ├── scripts/                      # 升级/回滚脚本                  │
│  └── checksums/                    # 校验和文件                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 模块依赖关系

```
modules/cvo/
├── api/v1beta1/          # CRD 类型定义
│   ├── releaseimage_types.go
│   ├── clusterversion_types.go
│   ├── clusteraddon_types.go
│   ├── component_types.go
│   ├── addon_types.go
│   └── upgrade_types.go
├── cmd/manager/          # CVO 管理器入口
├── internal/
│   ├── controllers/      # 控制器实现
│   │   ├── releaseimage_controller.go
│   │   ├── clusterversion_controller.go
│   │   └── clusteraddon_controller.go
│   ├── upgrader/         # 升级逻辑
│   │   ├── oci_puller.go
│   │   ├── graph_executor.go
│   │   └── ...
│   └── addon/            # Addon 安装逻辑
│       ├── helm_installer.go
│       └── manifest_installer.go
└── config/               # 部署配置
    ├── crd/
    ├── rbac/
    └── manager/

modules/capbm/
├── api/v1beta1/          # CAPBM CRD 类型定义
├── cmd/manager/          # CAPBM 管理器入口
├── internal/             # CAPBM 内部实现
└── config/               # 部署配置
```

---

## 三、CRD 设计

### 3.1 ReleaseImage Spec

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
spec:
  # 基础信息
  version: v1.31.1
  image: registry.example.com/capbm/release:v1.31.1
  
  # HTTP 服务器配置 (用于离线环境分发)
  httpServer:
    enabled: true
    port: 8080
    basePath: /release/v1.31.1
  
  # 镜像仓库配置 (用于导入到目标环境)
  imageRegistry:
    enabled: true
    registry: registry.example.com
    repository: capbm
  
  # 发布通道
  channels: [stable, fast]
  
  # 可升级的前置版本
  previousVersions: [v1.31.0, v1.30.0]
  
  # 二进制组件定义
  components:
    kubernetes: {...}
    containerd: {...}
    helm: {...}
    cniPlugins: {...}
  
  # Addon 定义
  addons:
    - name: calico
      type: helm
      version: v3.28.1
      contentPath: charts/calico-v3.28.1.tgz
      ...
  
  # 升级图
  upgradeGraph:
    - name: phase-1-runtime
      order: 1
      blocking: true
      components: [containerd]
    - name: phase-2-kubernetes
      order: 2
      blocking: true
      components: [kubernetes]
    ...
  
  # 内容校验和
  contentHash: sha256:abc123...
```

### 3.2 ReleaseImage Status

```yaml
status:
  verified: true                    # 是否已验证
  manifestCount: 15                 # Manifest 文件数量
  imagesImported: true              # 镜像是否已导入
  importJobName: release-image-import-v1.31.1
  importStatus: Completed
  importMessage: All images imported successfully
  importedImages:
    - component: kubernetes
      image: kube-apiserver
      targetRef: registry.example.com/capbm/kube-apiserver:v1.31.1
      status: imported
```

---

## 四、核心基础设施

### 4.1 OCI Puller

**文件**: `modules/cvo/internal/upgrader/oci_puller.go`

**功能**:
- `PullAndParseReleaseImage()` - 拉取并解析 release.json
- `PullAndParseCatalog()` - 拉取并解析 catalog.json
- `PullAndParseUpgradePath()` - 拉取并解析 upgrade-path.json
- `GetManifestDir()` - 获取 manifests 目录路径
- `GetScriptsDir()` - 获取 scripts 目录路径
- `PullFile()` - 拉取特定文件
- `PullChart()` - 拉取 Helm chart
- `PullManifest()` - 拉取 manifest
- `PullScript()` - 拉取脚本
- `WithAuth()` - 配置 OCI 认证

**实现细节**:
```go
type OCIPuller struct {
    workDir string
    auth    *AuthConfig
}

func (p *OCIPuller) PullAndParseReleaseImage(ctx context.Context, image string) (*cfov1.ReleaseImageSpec, error) {
    dir, err := p.pullImage(ctx, image, "release")
    if err != nil {
        return nil, fmt.Errorf("failed to pull release image: %w", err)
    }

    releaseData, err := os.ReadFile(filepath.Join(dir, "release.json"))
    if err != nil {
        return nil, fmt.Errorf("failed to read release.json: %w", err)
    }

    var spec cfov1.ReleaseImageSpec
    if err := json.Unmarshal(releaseData, &spec); err != nil {
        return nil, fmt.Errorf("failed to parse release.json: %w", err)
    }

    return &spec, nil
}
```

### 4.2 ContentFetcher

**文件**: `modules/cvo/internal/addon/manifest_installer.go`

**功能**:
- `FetchFromReleaseImage()` - 从 HTTP server 或本地目录获取内容
- `FetchChart()` - 获取 Helm chart
- `FetchManifest()` - 获取 manifest
- `FetchScript()` - 获取脚本

**实现细节**:
```go
type ContentFetcher struct {
    releaseServer string
    localDir      string
}

func (f *ContentFetcher) FetchFromReleaseImage(ctx context.Context, releaseImage *cfov1.ReleaseImage, addonName string) ([]byte, error) {
    // 1. 查找 addon 定义
    // 2. 尝试从本地目录读取
    if f.localDir != "" {
        localPath := filepath.Join(f.localDir, addonDef.ContentPath)
        data, err := os.ReadFile(localPath)
        if err == nil {
            return data, nil
        }
    }
    
    // 3. 从 HTTP server 获取
    if f.releaseServer != "" {
        url := fmt.Sprintf("%s/%s", f.releaseServer, addonDef.ContentPath)
        // HTTP GET 请求...
    }
    
    return nil, fmt.Errorf("no content source configured")
}
```

### 4.3 decodeYAML

**文件**: `modules/cvo/internal/upgrader/graph_executor.go`

**功能**:
- 解码 YAML 为 `client.Object`
- 支持任何 Kubernetes 对象
- 使用 `k8s.io/client-go/kubernetes/scheme` 的 `UniversalDeserializer`

**实现细节**:
```go
func decodeYAML(data []byte) (client.Object, error) {
    decode := scheme.Codecs.UniversalDeserializer().Decode

    obj, _, err := decode(data, nil, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to decode YAML: %w", err)
    }

    clientObj, ok := obj.(client.Object)
    if !ok {
        return nil, fmt.Errorf("decoded object does not implement client.Object")
    }

    return clientObj, nil
}
```

---

## 五、控制器实现

### 5.1 ReleaseImage Controller

**文件**: `modules/cvo/internal/controllers/releaseimage_controller.go`

**调和流程**:
```
Reconcile
    │
    ├── 获取 ReleaseImage
    │
    ├── 处理删除 (移除 finalizer)
    │
    ├── 添加 finalizer (如果不存在)
    │
    ├── 验证 contentHash (如果指定且未验证)
    │   └── 更新 status.verified = true
    │
    ├── 统计 manifests (如果未统计)
    │   └── 更新 status.manifestCount
    │
    └── 5 分钟后重新调和
```

**RBAC 权限**:
- `releaseimages`: get, list, watch, create, update, patch, delete
- `releaseimages/status`: get, update, patch
- `releaseimages/finalizers`: update
- `jobs`: get, list, watch, create, update, patch, delete
- `secrets`: get, list, watch

### 5.2 ClusterAddon Controller

**文件**: `modules/cvo/internal/controllers/clusteraddon_controller.go`

**调和流程**:
```
Reconcile
    │
    ├── 获取 ClusterAddon
    │
    ├── 处理删除 (移除 finalizer)
    │
    ├── 添加 finalizer (如果不存在)
    │
    ├── 获取关联的 ReleaseImage
    │
    ├── 查找 addon 定义
    │
    ├── 比较当前版本与目标版本
    │   └── 如果不同:
    │       ├── 更新 phase = Upgrading
    │       ├── 创建 Installer (Helm/Manifest)
    │       ├── 执行 Install()
    │       ├── 等待 Job 完成
    │       └── 更新 status.version 和 status.phase
    │
    └── 5 分钟后重新调和
```

**RBAC 权限**:
- `clusteraddons`: get, list, watch, create, update, patch, delete
- `clusteraddons/status`: get, update, patch
- `clusteraddons/finalizers`: update
- `releaseimages`: get, list, watch
- `jobs`: get, list, watch, create, update, patch, delete
- `configmaps`, `secrets`: get, list, watch, create, update, patch, delete

### 5.3 Installer 接口

**文件**: `modules/cvo/internal/addon/helm_installer.go`

**接口定义**:
```go
type Installer interface {
    Install(ctx context.Context, addon *cfov1.ClusterAddon, releaseImage *cfov1.ReleaseImage, addonDef *cfov1.AddonDefinition) error
}
```

**实现**:
- `HelmInstaller` - 通过 Kubernetes Job 运行 Helm 安装
- `ManifestInstaller` - 通过 Kubernetes Job 应用 Manifest

---

## 六、内容目录结构

### 6.1 目录结构

```
release-image/
├── release.json                  # ReleaseImage spec JSON
├── binaries/
│   ├── kubernetes/
│   │   ├── ubuntu/amd64/
│   │   │   ├── kubeadm_1.31.1-00_amd64.deb
│   │   │   ├── kubelet_1.31.1-00_amd64.deb
│   │   │   └── kubectl_1.31.1-00_amd64.deb
│   │   └── centos/amd64/
│   │       ├── kubeadm-1.31.1-0.x86_64.rpm
│   │       ├── kubelet-1.31.1-0.x86_64.rpm
│   │       └── kubectl-1.31.1-0.x86_64.rpm
│   ├── containerd/
│   │   └── containerd-1.7.24-linux-amd64.tar.gz
│   ├── helm/
│   │   └── helm-v3.15.0-linux-amd64.tar.gz
│   └── cni-plugins/
│       └── cni-plugins-linux-amd64-v1.5.0.tgz
├── images/
│   ├── kube-apiserver_v1.31.1.tar
│   ├── kube-controller-manager_v1.31.1.tar
│   ├── kube-scheduler_v1.31.1.tar
│   ├── kube-proxy_v1.31.1.tar
│   ├── pause_3.9.tar
│   ├── etcd_3.5.15-0.tar
│   └── coredns_v1.11.1.tar
├── charts/
│   ├── calico-v3.28.1.tgz
│   ├── ceph-csi-rbd-v3.11.0.tgz
│   └── capi-core-controller-v1.7.0.tgz
├── manifests/
│   ├── metallb-v0.14.8.yaml
│   └── gateway-api-v1.1.0.yaml
├── scripts/
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
└── checksums/
    ├── sha256sums.txt
    └── sha256sums.txt.sig
```

### 6.2 release.json 示例

完整的 `release.json` 包含:
- 基础信息 (version, image, channels, previousVersions)
- HTTP 服务器配置
- 镜像仓库配置
- 二进制组件定义 (kubernetes, containerd, helm, cniPlugins)
- Addon 定义 (calico, ceph-csi, capi-core-controller, metallb, gateway-api, envoy-gateway)
- 升级图 (7 个阶段)
- 内容校验和

---

## 七、构建和部署

### 7.1 Dockerfile.release

```dockerfile
FROM scratch

# 复制 release.json
COPY release-image/release.json /release.json

# 复制二进制文件
COPY release-image/binaries/ /binaries/

# 复制镜像
COPY release-image/images/ /images/

# 复制 charts
COPY release-image/charts/ /charts/

# 复制 manifests
COPY release-image/manifests/ /manifests/

# 复制脚本
COPY release-image/scripts/ /scripts/

# 复制校验和
COPY release-image/checksums/ /checksums/

# 设置标签
LABEL org.opencontainers.image.title="CAPBM Release Image"
LABEL org.opencontainers.image.description="CAPBM Release Image containing binaries, charts, manifests, and scripts"
LABEL org.opencontainers.image.version="v1.31.1"
```

### 7.2 Makefile Targets

```makefile
# 环境变量
RELEASE_IMG ?= ghcr.io/betawater/capbm/release:v1.31.1

##@ Release Image

.PHONY: release-image-build
release-image-build: ## Build release image OCI image
	docker build -t ${RELEASE_IMG} -f Dockerfile.release .

.PHONY: release-image-push
release-image-push: ## Push release image OCI image
	docker push ${RELEASE_IMG}

.PHONY: release-image
release-image: release-image-build release-image-push ## Build and push release image
```

---

## 八、使用指南

### 8.1 构建 ReleaseImage

```bash
# 1. 填充 binaries/, charts/, manifests/, images/ 目录

# 2. 构建镜像
make release-image-build RELEASE_IMG=registry.example.com/capbm/release:v1.31.1

# 3. 推送镜像
make release-image-push RELEASE_IMG=registry.example.com/capbm/release:v1.31.1
```

### 8.2 部署 CVO 和 CAPBM

```bash
# 部署 CVO
make deploy-cvo CVO_IMG=ghcr.io/betawater/cvo-manager:v0.1.0

# 部署 CAPBM
make deploy-capbm CAPBM_IMG=ghcr.io/betawater/capbm-manager:v0.1.0
```

### 8.3 创建 ReleaseImage

```bash
kubectl apply -f release-image/release.json
```

### 8.4 创建 ClusterVersion 触发升级

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
spec:
  clusterRef:
    name: my-cluster
    namespace: default
  desiredUpdate:
    version: v1.31.1
    image: registry.example.com/capbm/release:v1.31.1
```

### 8.5 监控升级状态

```bash
# 查看 ReleaseImage 状态
kubectl get releaseimage v1.31.1 -o yaml

# 查看 ClusterVersion 状态
kubectl get clusterversion my-cluster -o yaml

# 查看 ClusterAddon 状态
kubectl get clusteraddon -n default
```

---

## 九、实现状态

### 9.1 已完成

| 组件 | 状态 | 说明 |
|------|------|------|
| **CRD 定义** | ✅ 完成 | Go 类型和 CRD YAML 已生成 |
| **OCI Puller** | ✅ 完成 | 支持认证、拉取文件/charts/manifests/scripts |
| **ContentFetcher** | ✅ 完成 | 支持 HTTP server 和本地目录 |
| **decodeYAML** | ✅ 完成 | 使用 UniversalDeserializer 解码 |
| **ReleaseImage Controller** | ✅ 完成 | 验证 contentHash、统计 manifests |
| **ClusterAddon Controller** | ✅ 完成 | 版本比较、安装/升级、状态更新 |
| **Installer 接口** | ✅ 完成 | HelmInstaller 和 ManifestInstaller 实现 |
| **内容目录** | ✅ 完成 | release-image/ 目录结构和示例脚本 |
| **release.json** | ✅ 完成 | 完整的 ReleaseImage spec 示例 |
| **Dockerfile.release** | ✅ 完成 | 用于构建 ReleaseImage OCI 镜像 |
| **Makefile targets** | ✅ 完成 | release-image-build/push |

### 9.2 待完善

| 组件 | 状态 | 说明 |
|------|------|------|
| **OCI 完整拉取** | ⚠️ Stub | `pullImage()` 当前为 stub，需要完整的 oras-go 实现 |
| **contentHash 验证** | ⚠️ Stub | `verifyContentHash()` 当前标记为 verified，需要实际计算 |
| **镜像导入** | ⚠️ 未实现 | `imagesImported` 状态字段存在，但无导入逻辑 |
| **实际二进制文件** | ⚠️ 占位符 | binaries/ 目录只有 .gitkeep 占位符 |
| **实际 Charts/Manifests** | ⚠️ 占位符 | charts/ 和 manifests/ 目录只有 .gitkeep 占位符 |

---

## 十、设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **组件定义位置** | 独立 CRD vs ReleaseImage 内 | ReleaseImage 内 | 高内聚，版本与组件绑定 |
| **升级图定义** | 独立 CRD vs ReleaseImage 内 | ReleaseImage 内 | 升级顺序是版本特性 |
| **备份/回滚配置** | 独立配置 vs 组件内聚 | 组件内聚 | 每个组件负责自己的备份回滚 |
| **多架构支持** | 多个 ReleaseImage vs 单个 | 单个 | 简化版本管理 |
| **多 OS 支持** | 多个 ReleaseImage vs 单个 | 单个 | 简化版本管理 |
| **Addon 依赖** | 隐式 vs 显式 | 显式 (dependencies 字段) | 清晰可控 |
| **升级触发** | 独立字段 vs DesiredUpdate | DesiredUpdate | 统一入口 |
| **内容获取** | 仅 OCI vs OCI+HTTP+本地 | 多种数据源 | 支持离线环境 |

---

## 十一、未来扩展

### 11.1 计划中的扩展

| 扩展 | 说明 | 优先级 |
|------|------|--------|
| **组件签名验证** | 支持 GPG 签名验证组件完整性 | 高 |
| **增量升级** | 支持只升级变更的组件 | 中 |
| **升级回滚点** | 支持创建升级回滚点 | 中 |
| **组件兼容性矩阵** | 定义组件间兼容性规则 | 低 |
| **自动回滚** | 升级失败自动回滚 | 低 |

### 11.2 已知的限制

| 限制 | 说明 | 解决方案 |
|------|------|---------|
| **单镜像限制** | 一个 ReleaseImage 对应一个 OCI 镜像 | 使用 imageRegistry 导入到本地仓库 |
| **无组件覆盖** | 无法在 ClusterVersion 中覆盖组件版本 | 创建新的 ReleaseImage |
| **无动态变量** | 不支持运行时动态变量 | 使用 ClusterAddon.Spec.Values |
