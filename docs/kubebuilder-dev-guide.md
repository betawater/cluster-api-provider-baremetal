# CAPBM Kubebuilder 开发指南

本文档描述如何基于 Kubebuilder 框架开发和维护 CAPBM (Cluster API Provider Bare Metal) 项目。

---

## 1. 项目概述

CAPBM 是一个基于 Cluster API 的裸金属基础设施提供者，采用多模块架构设计，包含两个独立的管理器：

| 模块 | API Group | 用途 |
|------|-----------|------|
| **CVO** (Cluster Version Operator) | `cvo.capbm.io` | 集群版本管理和升级协调 |
| **CAPBM** (Cluster API Provider Bare Metal) | `infrastructure.cluster.x-k8s.io` | 裸金属基础设施管理 |

### 核心 CRD

#### CVO 模块
- `ClusterVersion` - 集群版本状态和升级目标
- `ReleaseImage` - 发布版本镜像和组件定义
- `UpgradePath` - 升级路径和兼容性规则
- `ReleaseCatalog` - 可用发布版本目录
- `ClusterAddon` - 集群插件生命周期管理

#### CAPBM 模块
- `BareMetalCluster` - 裸金属集群基础设施
- `BareMetalMachine` - 裸金属机器实例
- `BareMetalHostInventory` - 主机池管理
- `BareMetalClusterTemplate` - 集群模板
- `BareMetalMachineTemplate` - 机器模板

---

## 2. 项目结构

```
cluster-api-provider-baremetal/
├── go.work                              # Go workspace 配置
├── Makefile                             # 构建脚本
├── PROJECT                              # Kubebuilder 项目配置
├── Dockerfile.cvo                       # CVO Docker 构建
├── Dockerfile.capbm                     # CAPBM Docker 构建
├── modules/
│   ├── cvo/                             # CVO 模块
│   │   ├── go.mod
│   │   ├── go.sum
│   │   ├── api/v1beta1/                 # CVO API 类型定义
│   │   ├── cmd/manager/                 # CVO 管理器入口
│   │   ├── internal/                    # CVO 内部实现
│   │   │   ├── controllers/             # CVO 控制器
│   │   │   ├── upgrader/                # 升级逻辑
│   │   │   ├── addon/                   # 插件管理
│   │   │   └── registry/                # 镜像注册
│   │   ├── pkg/ssh/                     # 公开 SSH 包
│   │   └── config/                      # CVO 部署配置
│   │       ├── crd/bases/               # 生成的 CRD YAML
│   │       ├── rbac/                    # RBAC 配置
│   │       └── manager/                 # 管理器部署
│   │
│   └── capbm/                           # CAPBM 模块
│       ├── go.mod
│       ├── go.sum
│       ├── api/v1beta1/                 # CAPBM API 类型定义
│       ├── cmd/manager/                 # CAPBM 管理器入口
│       ├── internal/                    # CAPBM 内部实现
│       │   ├── controllers/             # CAPBM 控制器
│       │   ├── ssh/                     # SSH 客户端
│       │   ├── installer/               # 组件安装
│       │   ├── lb/                      # 负载均衡
│       │   ├── cni/                     # CNI 插件
│       │   ├── csi/                     # CSI 驱动
│       │   └── ...
│       └── config/                      # CAPBM 部署配置
│           ├── crd/bases/               # 生成的 CRD YAML
│           ├── rbac/                    # RBAC 配置
│           ├── manager/                 # 管理器部署
│           └── clusterclass/            # ClusterClass 模板
│
├── docs/                                # 设计文档
├── hack/                                # 辅助脚本
├── templates/                           # 模板文件
├── test/                                # e2e 测试
└── .github/workflows/                   # CI/CD 配置
```

---

## 3. 开发环境设置

### 3.1 前置要求

- Go 1.25+
- Docker
- kustomize v5.4.3+
- kubectl
- 一个 Kubernetes 集群 (用于测试)

### 3.2 克隆项目

```bash
git clone https://github.com/BetaWater/cluster-api-provider-baremetal.git
cd cluster-api-provider-baremetal
```

### 3.3 初始化依赖

```bash
# 初始化 Go workspace
go work sync

# 下载依赖
cd modules/cvo && go mod tidy
cd ../capbm && go mod tidy
```

---

## 4. 添加新的 CRD

### 4.1 在 CVO 模块添加 CRD

```bash
cd modules/cvo

# 使用 kubebuilder 创建 API
kubebuilder create api --group cvo --version v1beta1 --kind MyNewResource

# 或手动创建文件
cat > api/v1beta1/mynewresource_types.go << 'EOF'
package v1beta1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

type MyNewResource struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   MyNewResourceSpec   `json:"spec,omitempty"`
    Status MyNewResourceStatus `json:"status,omitempty"`
}

type MyNewResourceSpec struct {
    // 添加你的 spec 字段
}

type MyNewResourceStatus struct {
    Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
type MyNewResourceList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []MyNewResource `json:"items"`
}
EOF
```

### 4.2 在 CAPBM 模块添加 CRD

```bash
cd modules/capbm

# 使用 kubebuilder 创建 API
kubebuilder create api --group infrastructure --version v1beta1 --kind MyNewResource

# 注意: CAPBM 使用 infrastructure.cluster.x-k8s.io API group
```

### 4.3 注册到 Scheme

在 `api/v1beta1/groupversion_info.go` 的 `init()` 函数中注册新类型：

```go
func init() {
    SchemeBuilder.Register(
        &MyNewResource{},
        &MyNewResourceList{},
        // ... 其他类型
    )
}
```

### 4.4 生成 CRD manifests

```bash
# 在项目根目录执行
make manifests
```

生成的 CRD 文件将输出到:
- CVO: `modules/cvo/config/crd/bases/cvo.capbm.io_mynewresources.yaml`
- CAPBM: `modules/capbm/config/crd/bases/infrastructure.cluster.x-k8s.io_mynewresources.yaml`

---

## 5. 添加新的 Controller

### 5.1 创建 Controller

```bash
cd modules/cvo  # 或 modules/capbm

# 使用 kubebuilder 创建 controller
kubebuilder create controller --name mynewresource --group cvo --version v1beta1

# 或手动创建文件
cat > internal/controllers/mynewresource_controller.go << 'EOF'
package controllers

import (
    "context"

    "github.com/go-logr/logr"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"

    cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
)

type MyNewResourceReconciler struct {
    client.Client
    Log    logr.Logger
    Scheme *runtime.Scheme
}

func (r *MyNewResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    resource := &cfov1.MyNewResource{}
    if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 实现你的 reconcile 逻辑
    log.Info("Reconciling MyNewResource", "name", resource.Name)

    return ctrl.Result{}, nil
}

func (r *MyNewResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&cfov1.MyNewResource{}).
        Complete(r)
}
EOF
```

### 5.2 注册 Controller

在 `cmd/manager/main.go` 中注册新 controller：

```go
func main() {
    // ... 现有代码 ...

    if err = (&controllers.MyNewResourceReconciler{
        Client: mgr.GetClient(),
        Log:    ctrl.Log.WithName("controllers").WithName("MyNewResource"),
        Scheme: mgr.GetScheme(),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "MyNewResource")
        os.Exit(1)
    }

    // ... 现有代码 ...
}
```

---

## 6. 构建和测试

### 6.1 构建

```bash
# 构建所有模块
make build

# 单独构建
make build-cvo
make build-capbm
```

### 6.2 测试

```bash
# 运行所有测试
make test

# 运行特定模块测试
cd modules/cvo && go test ./...
cd modules/capbm && go test ./...

# 运行 e2e 测试
go test ./test/e2e/... -v
```

### 6.3 代码质量

```bash
# 格式化代码
make fmt

# 代码检查
make vet

# 生成 deepcopy
make generate
```

---

## 7. 部署

### 7.1 安装 CRD

```bash
# 安装 CVO CRD
make install-cvo

# 安装 CAPBM CRD
make install-capbm
```

### 7.2 部署管理器

```bash
# 部署 CVO 管理器
make deploy-cvo CVO_IMG=ghcr.io/betawater/cvo-manager:latest

# 部署 CAPBM 管理器
make deploy-capbm CAPBM_IMG=ghcr.io/betawater/capbm-manager:latest
```

### 7.3 卸载

```bash
make undeploy-cvo
make undeploy-capbm
make uninstall-cvo
make uninstall-capbm
```

---

## 8. Docker 镜像构建

### 8.1 构建镜像

```bash
# 构建 CVO 镜像
make docker-build-cvo CVO_IMG=ghcr.io/betawater/cvo-manager:v0.1.0

# 构建 CAPBM 镜像
make docker-build-capbm CAPBM_IMG=ghcr.io/betawater/capbm-manager:v0.1.0
```

### 8.2 推送镜像

```bash
make docker-push-cvo CVO_IMG=ghcr.io/betawater/cvo-manager:v0.1.0
make docker-push-capbm CAPBM_IMG=ghcr.io/betawater/capbm-manager:v0.1.0
```

---

## 9. 发布流程

### 9.1 创建 Release

```bash
# 生成 release manifests
make release-manifests VERSION=v0.1.0

# 输出文件:
# - releases/v0.1.0/infrastructure-components.yaml
# - releases/v0.1.0/cvo-components.yaml
# - releases/v0.1.0/metadata.yaml
```

### 9.2 通过 Git Tag 发布

```bash
git tag v0.1.0
git push origin v0.1.0
```

CI 将自动:
1. 构建并推送 Docker 镜像
2. 生成 release manifests
3. 创建 GitHub Release

---

## 10. 常见问题

### Q: 如何添加新的 API 版本 (如 v1alpha1)?

```bash
cd modules/cvo

# 创建新的 API 版本目录
mkdir -p api/v1alpha1

# 创建 groupversion_info.go
cat > api/v1alpha1/groupversion_info.go << 'EOF'
package v1alpha1

import (
    "k8s.io/apimachinery/pkg/runtime/schema"
    "sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
    GroupVersion = schema.GroupVersion{Group: "cvo.capbm.io", Version: "v1alpha1"}
    SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
    AddToScheme = SchemeBuilder.AddToScheme
)
EOF
```

### Q: 如何修改现有 CRD?

1. 修改 `api/v1beta1/<kind>_types.go` 中的类型定义
2. 运行 `make manifests` 重新生成 CRD YAML
3. 运行 `make generate` 重新生成 deepcopy 代码
4. 提交更改

### Q: 如何调试 Controller?

```bash
# 本地运行 (无需 Docker)
make run-cvo
make run-capbm

# 或使用 delve 调试
cd modules/cvo
dlv debug ./cmd/manager/
```

### Q: 如何添加 Webhook?

```bash
cd modules/cvo

# 使用 kubebuilder 创建 webhook
kubebuilder create webhook --group cvo --version v1beta1 --kind MyNewResource --defaulting --programmatic-validation
```

---

## 11. 最佳实践

### 11.1 API 设计

- 使用 `+kubebuilder:validation:` markers 添加验证
- 为所有字段添加注释
- 使用 `omitempty` 标记可选字段
- 为 CRD 添加 `+kubebuilder:printcolumn` 便于 kubectl 输出

### 11.2 Controller 设计

- 使用 `ctrl.LoggerFrom(ctx)` 获取日志
- 使用 `client.IgnoreNotFound(err)` 处理 NotFound 错误
- 使用 `patch.Helper` 高效更新状态
- 添加 finalizer 处理资源清理

### 11.3 测试

- 为每个 controller 编写 `suite_test.go`
- 使用 `envtest` 进行集成测试
- 使用 `ginkgo` 编写 BDD 风格测试

### 11.4 多模块开发

- 在 `go.work` 中管理所有模块
- 模块间依赖通过 `replace` 指令处理
- 公共类型放在被依赖的模块中
