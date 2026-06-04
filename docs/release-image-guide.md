# ReleaseImage 使用指南

## 1. 概述

ReleaseImage 是 CAPBM 的组件版本管理核心 CRD，定义完整的 Kubernetes 发行版本，包括组件版本、Addon 定义和升级图。

**核心特性**:
- **自描述**: 一个 CRD 定义所有组件版本和升级配置
- **版本一致**: 所有组件版本由 ReleaseImage 统一管理
- **多架构支持**: 支持 amd64 和 arm64
- **多 OS 支持**: 支持 Ubuntu、CentOS、Rocky 等
- **高内聚配置**: 每个组件自带安装、升级、备份、回滚配置
- **离线支持**: 支持 air-gapped 环境的离线安装

## 2. ReleaseImage CRD

### 2.1 API 信息

| 属性 | 值 |
|------|-----|
| **API Group** | `cvo.capbm.io` |
| **Version** | `v1beta1` |
| **Kind** | `ReleaseImage` |
| **Scope** | Namespaced |

### 2.2 创建 ReleaseImage

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
  labels:
    capbm.io/release-channel: stable
spec:
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
      namespace: kube-system
      ...
  
  # 升级图
  upgradeGraph:
    - name: phase-1-runtime
      order: 1
      blocking: true
      components: [containerd]
    ...
  
  # 内容校验和
  contentHash: sha256:abc123...
```

### 2.3 应用 ReleaseImage

```bash
# 应用 ReleaseImage
kubectl apply -f release-image/release.json

# 查看 ReleaseImage
kubectl get releaseimage

# 查看 ReleaseImage 详情
kubectl get releaseimage v1.31.1 -o yaml

# 查看 ReleaseImage 状态
kubectl get releaseimage v1.31.1 -o jsonpath='{.status}'
```

## 3. ReleaseImage 内容目录

### 3.1 目录结构

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

### 3.2 release.json 示例

完整的 `release.json` 包含:
- 基础信息 (version, image, channels, previousVersions)
- HTTP 服务器配置
- 镜像仓库配置
- 二进制组件定义 (kubernetes, containerd, helm, cniPlugins)
- Addon 定义 (calico, ceph-csi, capi-core-controller, metallb, gateway-api, envoy-gateway)
- 升级图 (7 个阶段)
- 内容校验和

完整示例请参考 `release-image/release.json`。

## 4. 构建 ReleaseImage OCI 镜像

### 4.1 准备内容

1. 填充 `release-image/binaries/` 目录
2. 填充 `release-image/images/` 目录
3. 填充 `release-image/charts/` 目录
4. 填充 `release-image/manifests/` 目录
5. 更新 `release-image/release.json`

### 4.2 构建镜像

```bash
# 构建 ReleaseImage
make release-image-build RELEASE_IMG=registry.example.com/capbm/release:v1.31.1

# 推送 ReleaseImage
make release-image-push RELEASE_IMG=registry.example.com/capbm/release:v1.31.1

# 或者一步完成
make release-image RELEASE_IMG=registry.example.com/capbm/release:v1.31.1
```

### 4.3 Dockerfile.release

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
LABEL org.opencontainers.image.description="CAPBM Release Image containing binaries, charts, manifests, and scripts"
LABEL org.opencontainers.image.version="v1.31.1"
```

## 5. 使用 ReleaseImage 进行升级

### 5.1 创建 ClusterVersion 触发升级

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

```bash
kubectl apply -f clusterversion.yaml
```

### 5.2 升级流程

```
1. CVO Controller 检测 ClusterVersion 变更
2. 验证升级路径 (UpgradePath)
3. 获取目标 ReleaseImage
4. Phase 1: K8S 升级 (containerd → kubernetes)
5. Phase 2: Addon 升级 (calico → ceph-csi → ...)
6. 更新 ClusterVersion 状态
```

### 5.3 仅 Addon 升级 (K8S 版本不变)

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
    version: v1.31.0     # K8S 版本不变
    image: registry.example.com/capbm/release:v1.31.0-patch1  # 新 ReleaseImage
```

## 6. 监控 ReleaseImage 状态

### 6.1 查看 ReleaseImage 状态

```bash
# 查看 ReleaseImage
kubectl get releaseimage v1.31.1

# 查看详细信息
kubectl get releaseimage v1.31.1 -o yaml

# 查看状态字段
kubectl get releaseimage v1.31.1 -o jsonpath='{.status}'
```

### 6.2 状态字段说明

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

## 7. 离线环境使用

### 7.1 配置 HTTP 服务器

```yaml
spec:
  httpServer:
    enabled: true
    port: 8080
    basePath: /release/v1.31.1
    baseUrl: http://192.168.1.100:8080
    insecureSkipVerify: true
```

### 7.2 配置镜像仓库

```yaml
spec:
  imageRegistry:
    enabled: true
    registry: 192.168.1.100:5000
    repository: capbm
    insecureSkipVerify: true
```

## 8. 组件定义详解

### 8.1 二进制组件

```yaml
components:
  kubernetes:
    version: v1.31.1
    type: binary
    path: /opt/capbm/binaries/kubernetes
    platforms:
      ubuntu:
        architectures: [amd64, arm64]
        packages:
          kubeadm: kubeadm_1.31.1-00
          kubelet: kubelet_1.31.1-00
          kubectl: kubectl_1.31.1-00
    installStrategy:
      timeout: 600s
      retryCount: 3
      method: package
      serviceName: kubelet
    upgradeStrategy:
      type: Rolling
      maxConcurrent: 1
      timeout: 900s
      retryCount: 3
      drain: true
    upgrade:
      backup:
        enabled: true
        config:
          - path: /etc/kubernetes
            type: directory
        etcdSnapshot: true
      rollback:
        script: scripts/rollback-kubernetes.sh
        timeout: 600s
      healthCheck:
        command: kubectl get nodes
        timeout: 60s
        retries: 3
```

### 8.2 Addon 定义

```yaml
addons:
  - name: calico
    type: helm
    version: v3.28.1
    contentPath: charts/calico-v3.28.1.tgz
    namespace: kube-system
    dependencies: []
    variables:
      - name: podCIDR
        type: string
        description: Pod network CIDR
        required: true
    defaultValues:
      ipam: calico-ipam
    installStrategy:
      timeout: 300s
      retryCount: 3
      createNamespace: true
      wait: true
    upgradeStrategy:
      type: Rolling
      maxUnavailable: 0
      timeout: 300s
      retryCount: 3
    upgrade:
      backup:
        enabled: true
        config:
          - path: /etc/cni/net.d
            type: directory
      rollback:
        script: scripts/rollback-calico.sh
        timeout: 300s
      healthCheck:
        command: kubectl get pods -n kube-system -l k8s-app=calico-node
        timeout: 60s
        retries: 3
```

## 9. 升级图详解

### 9.1 升级阶段

```yaml
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
        scripts:
          - scripts/upgrade-containerd.sh
        healthCheck:
          type: ServiceRunning
          name: containerd
          timeout: 30s

  - name: phase-2-kubernetes
    order: 2
    blocking: true
    components:
      - name: kubernetes
        blocking: true
        dependsOn: [containerd]
        scripts:
          - scripts/upgrade-kubernetes.sh
        healthCheck:
          type: DeploymentReady
          namespace: kube-system
          name: kube-apiserver
          timeout: 60s

  - name: phase-3-addons
    order: 3
    blocking: false
    components:
      - name: calico
        blocking: true
        dependsOn: [kubernetes]
        healthCheck:
          type: DaemonSetReady
          namespace: kube-system
          name: calico-node
          timeout: 120s
```

### 9.2 升级顺序

```
Phase 1: containerd (运行时)
    ↓
Phase 2: kubernetes (K8S 核心)
    ↓
Phase 3: calico (CNI)
    ↓
Phase 4: ceph-csi (CSI)
    ↓
Phase 5: gateway-api → envoy-gateway (Gateway)
    ↓
Phase 6: metallb (负载均衡)
    ↓
Phase 7: capi-core-controller (CAPI Core)
```

## 10. 故障排查

### 10.1 ReleaseImage 验证失败

```bash
# 查看 ReleaseImage 事件
kubectl describe releaseimage v1.31.1

# 查看 CVO 日志
kubectl logs -n cvo-system -l control-plane=controller-manager
```

### 10.2 升级失败

```bash
# 查看 ClusterVersion 状态
kubectl get clusterversion my-cluster -o yaml

# 查看 ClusterAddon 状态
kubectl get clusteraddon -n default

# 查看升级日志
kubectl logs -n cvo-system -l control-plane=controller-manager --tail=100
```

## 11. 相关资源

- [ReleaseImage 实现设计文档](./releaseimage-implementation-design.md)
- [ClusterVersion 升级设计](./cluster-upgrade-cvo.md)
- [Addon 升级触发设计](./addon-upgrade-trigger-design.md)
- [CAPBM 用户指南](./user-guide.md)
