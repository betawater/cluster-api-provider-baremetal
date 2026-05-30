# ReleaseImage 使用指南

## 1. 概述

ReleaseImage 是 CAPBM 的组件版本管理核心，通过 OCI 镜像分发所有组件，支持在线和离线安装模式。

**核心特性**:
- **自包含**: 一个镜像包含所有组件，无需外网访问
- **版本一致**: 所有组件版本由 ReleaseImage 统一管理
- **多架构支持**: 支持 linux-amd64 和 linux-arm64
- **多 OS 支持**: 支持 Ubuntu (deb) 和 RHEL (rpm)
- **组件类型自描述**: 通过 index.json 标识 binary/manifest/helm 类型

## 2. 镜像内容

ReleaseImage 是一个自包含的 OCI 镜像，内置 nginx HTTP 服务器，提供所有组件下载服务：

```
/release/
├── images/                           # 容器镜像 (按组件分类)
│   ├── kubernetes/
│   │   └── v1.31.0/
│   │       ├── kube-apiserver.tar
│   │       ├── kube-controller-manager.tar
│   │       ├── kube-scheduler.tar
│   │       ├── kube-proxy.tar
│   │       ├── coredns.tar
│   │       ├── etcd.tar
│   │       └── pause.tar
│   │
│   ├── calico/
│   │   └── v3.27.0/
│   │       ├── calico-node.tar
│   │       ├── calico-kube-controllers.tar
│   │       └── calico-cni.tar
│   │
│   ├── cilium/
│   │   └── v1.15.0/
│   │       ├── cilium.tar
│   │       ├── cilium-operator.tar
│   │       └── hubble-relay.tar
│   │
│   ├── flannel/
│   │   └── v0.24.0/
│   │       └── flannel.tar
│   │
│   ├── ceph-csi/
│   │   └── v3.9.0/
│   │       ├── cephcsi.tar
│   │       ├── csi-attacher.tar
│   │       ├── csi-provisioner.tar
│   │       ├── csi-snapshotter.tar
│   │       ├── csi-resizer.tar
│   │       └── csi-node-driver-registrar.tar
│   │
│   ├── local-path-provisioner/
│   │   └── v0.0.26/
│   │       ├── local-path-provisioner.tar
│   │       └── helper.tar
│   │
│   ├── nfs-csi/
│   │   └── v4.5.0/
│   │       ├── nfs.tar
│   │       └── csi-node-driver-registrar.tar
│   │
│   ├── envoy-gateway/
│   │   └── v1.1.0/
│   │       ├── envoy-gateway.tar
│   │       └── envoy-proxy.tar
│   │
│   └── metallb/
│       └── v0.14.0/
│           ├── metallb-controller.tar
│           └── metallb-speaker.tar
│
├── runtime/                          # 容器运行时二进制
│   └── containerd/
│       └── v1.7.0/
│           ├── linux-amd64/
│           └── linux-arm64/
│
├── kubernetes/                       # Kubernetes 核心二进制
│   └── v1.31.0/
│       ├── ubuntu/
│       │   ├── linux-amd64/
│       │   └── linux-arm64/
│       └── rhel/
│           ├── linux-amd64/
│           └── linux-arm64/
│
├── cni/                              # CNI 网络插件
│   ├── plugins/
│   │   └── v1.3.0/
│   │       ├── linux-amd64/
│   │       └── linux-arm64/
│   ├── calico/
│   │   └── v3.27.0/
│   ├── cilium/
│   │   └── v1.15.0/
│   └── flannel/
│       └── v0.24.0/
│
├── csi/                              # CSI 存储驱动
│   ├── ceph-csi/
│   ├── local-path-provisioner/
│   └── nfs-csi/
│
├── gateway/                          # 网关组件
│   ├── gateway-api/
│   └── envoy-gateway/
│
├── metallb/                          # 负载均衡器
│
├── scripts/                          # 辅助脚本
│   └── load-images.sh
│
├── index.json                        # 组件索引
└── checksums.sha256                  # 校验和
```

## 3. 部署方式

### 3.1 部署到管理集群 (推荐)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: capbm-release-server
  namespace: capbm-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: capbm-release-server
  template:
    metadata:
      labels:
        app: capbm-release-server
    spec:
      containers:
      - name: release-server
        image: capbm-release:v1.31.0
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
---
apiVersion: v1
kind: Service
metadata:
  name: capbm-release-server
  namespace: capbm-system
spec:
  type: ClusterIP
  selector:
    app: capbm-release-server
  ports:
  - port: 8080
    targetPort: 8080
```

### 3.2 独立服务器部署

```bash
# 在独立服务器上运行
docker run -d --name release-server \
  -p 8080:8080 \
  capbm-release:v1.31.0

# 验证服务
curl http://<server-ip>:8080/release/index.json
```

### 3.3 NodePort 方式 (供工作集群访问)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: capbm-release-server
  namespace: capbm-system
spec:
  type: NodePort
  selector:
    app: capbm-release-server
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30080
```

## 4. 配置集群使用 ReleaseImage

### 4.1 组件安装配置

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-baremetal-cluster
spec:
  topology:
    classRef:
      name: baremetal-clusterclass-v0.1.0
    version: v1.31.0
    controlPlane:
      replicas: 3
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
        replicas: 2
    variables:
    # 组件安装 - 使用 ReleaseImage HTTP 服务器
    - name: componentInstall
      value:
        enabled: true
        airGap:
          enabled: true
          binarySource: "HTTPServer"
          httpServerConfig:
            baseUrl: "http://capbm-release-server.capbm-system.svc.cluster.local:8080/release"
    
    # CNI 插件 - 使用 ReleaseImage
    - name: cni
      value:
        enabled: true
        type: "calico"
        version: "3.27.0"
        airGap:
          enabled: true
          manifestSource: "HTTPServer"
          httpServerConfig:
            baseUrl: "http://capbm-release-server.capbm-system.svc.cluster.local:8080/release"
    
    # CSI 驱动 - 使用 ReleaseImage
    - name: csi
      value:
        enabled: false
        driver: "ceph-csi"
        version: "3.9.0"
        airGap:
          enabled: true
          manifestSource: "HTTPServer"
          httpServerConfig:
            baseUrl: "http://capbm-release-server.capbm-system.svc.cluster.local:8080/release"
```

### 4.2 ReleaseImage CRD 引用

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.0
spec:
  version: "v1.31.0"
  image: "capbm-release:v1.31.0"
  httpServer:
    port: 8080
    basePath: "/release"
  components:
    kubernetes:
      version: "v1.31.0"
      type: "binary"
      path: "kubernetes/v1.31.0"
      platforms:
        ubuntu:
          architectures: ["linux-amd64", "linux-arm64"]
        rhel:
          architectures: ["linux-amd64", "linux-arm64"]
    containerd:
      version: "v1.7.0"
      type: "binary"
      path: "runtime/containerd/v1.7.0"
      architectures: ["linux-amd64", "linux-arm64"]
    calico:
      version: "v3.27.0"
      type: "manifest"
      path: "cni/calico/v3.27.0"
      files:
        manifest: "calico.yaml"
        chart: "calico.tgz"
      images: "images/calico-v3.27.0.tar"
    cilium:
      version: "v1.15.0"
      type: "helm"
      path: "cni/cilium/v1.15.0"
      files:
        chart: "cilium.tgz"
      images: "images/cilium-v1.15.0.tar"
      helmValues:
        ipam.mode: "kubernetes"
        kubeProxyReplacement: "partial"
```

## 5. 安装流程

```
┌─────────────────────────────────────────────────────────────┐
│ 1. 部署 ReleaseImage HTTP 服务器                            │
│    ├── docker pull capbm-release:v1.31.0                   │
│    ├── kubectl apply -f release-server.yaml                │
│    └── 验证: curl http://<ip>:8080/release/index.json      │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. 创建集群并配置 HTTP 服务器地址                            │
│    ├── 设置 componentInstall.airGap.httpServerConfig        │
│    └── baseUrl = "http://<release-server>:8080/release"    │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. CAPBM 自动安装组件                                       │
│    ├── 检测节点 OS 类型和架构                               │
│    ├── 从 ReleaseImage 获取组件路径                         │
│    ├── fetch_resource("kubernetes/v1.31.0/ubuntu/...")      │
│    ├── fetch_resource("runtime/containerd/v1.7.0/...")      │
│    ├── fetch_resource("cni/calico/v3.27.0/calico.yaml")     │
│    └── 安装并验证                                           │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. 自动加载容器镜像 (离线模式)                               │
│    ├── 加载 Kubernetes 核心镜像                             │
│    │   ├── kube-apiserver.tar                               │
│    │   ├── kube-controller-manager.tar                      │
│    │   └── ...                                              │
│    ├── 加载 CNI 镜像 (根据安装的 CNI 类型)                   │
│    │   ├── calico-node.tar                                  │
│    │   ├── calico-kube-controllers.tar                      │
│    │   └── calico-cni.tar                                   │
│    └── 加载其他组件镜像                                     │
└─────────────────────────────────────────────────────────────┘
```

## 6. 镜像加载脚本

ReleaseImage 提供辅助脚本用于手动加载镜像：

```bash
#!/bin/bash
# scripts/load-images.sh
# 用法: ./load-images.sh <component> [version]
# 示例: ./load-images.sh calico v3.27.0

set -euo pipefail

RELEASE_SERVER="${RELEASE_SERVER:-http://localhost:8080/release}"
COMPONENT="${1:-all}"
VERSION="${2:-}"

load_images() {
    local component="$1"
    local version="$2"
    local image_path="${RELEASE_SERVER}/images/${component}/${version}"
    
    echo "=== 加载 ${component} v${version} 镜像 ==="
    
    # 获取镜像列表 (从 index.json)
    local images
    images=$(curl -fsSL "${RELEASE_SERVER}/index.json" | jq -r ".images.\"${component}\".images[]")
    
    for image_tar in $images; do
        local tar_url="${image_path}/${image_tar}"
        local dest="/tmp/${image_tar}"
        
        echo "下载: ${image_tar}"
        curl -fsSL "$tar_url" -o "$dest"
        
        echo "导入: ${image_tar}"
        ctr -n k8s.io images import "$dest"
        rm -f "$dest"
    done
    
    echo "=== ${component} v${version} 镜像加载完成 ==="
}

case "$COMPONENT" in
    all)
        # 加载所有镜像
        for comp in kubernetes calico cilium envoy-gateway metallb; do
            if [ -n "$VERSION" ]; then
                load_images "$comp" "$VERSION"
            else
                # 从 index.json 获取版本
                ver=$(curl -fsSL "${RELEASE_SERVER}/index.json" | jq -r ".components.\"${comp}\".version")
                if [ "$ver" != "null" ] && [ -n "$ver" ]; then
                    load_images "$comp" "$ver"
                fi
            fi
        done
        ;;
    *)
        if [ -z "$VERSION" ]; then
            VERSION=$(curl -fsSL "${RELEASE_SERVER}/index.json" | jq -r ".components.\"${COMPONENT}\".version")
        fi
        load_images "$COMPONENT" "$VERSION"
        ;;
esac
```

使用示例：

```bash
# 加载所有镜像
./load-images.sh all

# 仅加载 Calico 镜像
./load-images.sh calico v3.27.0

# 仅加载 Kubernetes 镜像
./load-images.sh kubernetes v1.31.0
```

## 6. 组件路径自动适配

CAPBM 根据节点 OS 和架构自动选择正确的组件路径：

```bash
# 节点检测
OS_TYPE="ubuntu"     # 或 "rhel"
ARCH="linux-amd64"   # 或 "linux-arm64"

# 自动构建路径
K8S_PATH="kubernetes/v1.31.0/${OS_TYPE}/${ARCH}/kubelet.deb"
CONTAINERD_PATH="runtime/containerd/v1.7.0/${ARCH}/containerd.tar.gz"
CNI_PLUGINS_PATH="cni/plugins/v1.3.0/${ARCH}/cni-plugins.tgz"
CALICO_PATH="cni/calico/v3.27.0/calico.yaml"
```

## 7. 验证 ReleaseImage 内容

```bash
# 查看组件索引
curl http://<release-server>:8080/release/index.json | jq

# 查看目录结构
curl http://<release-server>:8080/release/

# 下载特定组件
curl -O http://<release-server>:8080/release/cni/calico/v3.27.0/calico.yaml

# 验证校验和
curl http://<release-server>:8080/release/checksums.sha256 | sha256sum -c
```

## 8. 多版本管理

```bash
# 拉取多个版本的 ReleaseImage
docker pull capbm-release:v1.31.0
docker pull capbm-release:v1.32.0

# 部署不同版本的 ReleaseImage 服务器
kubectl apply -f release-server-v1.31.0.yaml
kubectl apply -f release-server-v1.32.0.yaml
```

不同集群可以引用不同版本的 ReleaseImage：

```yaml
# 集群 A 使用 v1.31.0
variables:
- name: componentInstall
  value:
    airGap:
      httpServerConfig:
        baseUrl: "http://release-server-v1.31.0:8080/release"

# 集群 B 使用 v1.32.0
variables:
- name: componentInstall
  value:
    airGap:
      httpServerConfig:
        baseUrl: "http://release-server-v1.32.0:8080/release"
```

## 9. 离线环境使用

### 9.1 在有网环境构建镜像

```bash
# 运行构建脚本
./build-release-image.sh v1.31.0

# 导出镜像为 tar 文件
docker save capbm-release:v1.31.0 -o capbm-release-v1.31.0.tar
```

### 9.2 传输到离线环境

```bash
# 通过 SCP 传输
scp capbm-release-v1.31.0.tar offline-server:/tmp/

# 或通过 USB/光盘等物理介质传输
```

### 9.3 在离线环境加载并运行

```bash
# 加载镜像
docker load -i /tmp/capbm-release-v1.31.0.tar

# 运行 ReleaseImage 服务器
docker run -d --name release-server \
  -p 8080:8080 \
  capbm-release:v1.31.0

# 验证服务
curl http://localhost:8080/release/index.json
```

### 9.4 配置集群使用

```yaml
variables:
- name: componentInstall
  value:
    enabled: true
    airGap:
      enabled: true
      binarySource: "HTTPServer"
      httpServerConfig:
        baseUrl: "http://offline-server:8080/release"
- name: cni
  value:
    enabled: true
    type: "calico"
    version: "3.27.0"
    airGap:
      enabled: true
      manifestSource: "HTTPServer"
      httpServerConfig:
        baseUrl: "http://offline-server:8080/release"
```

## 10. 构建 ReleaseImage

### 10.1 构建脚本

```bash
#!/bin/bash
# build-release-image.sh

set -euo pipefail

RELEASE_VERSION="v1.31.0"
OUTPUT_DIR="./release-image-content/release"
ARCHS=("amd64" "arm64")

# 创建目录结构
mkdir -p "$OUTPUT_DIR"/{runtime/containerd/v1.7.0,kubernetes/$RELEASE_VERSION/{ubuntu,rhel}/{amd64,arm64},cni/plugins/v1.3.0/{linux-amd64,linux-arm64},cni/{calico/v3.27.0,cilium/v1.15.0,flannel/v0.24.0},csi/{ceph-csi/v3.9.0,local-path-provisioner/v0.0.26,nfs-csi/v4.5.0},gateway/{gateway-api/v1.2.0,envoy-gateway/v1.1.0},metallb/v0.14.0,images/{kubernetes/$RELEASE_VERSION,calico/v3.27.0,cilium/v1.15.0,flannel/v0.24.0,ceph-csi/v3.9.0,local-path-provisioner/v0.0.26,nfs-csi/v4.5.0,envoy-gateway/v1.1.0,metallb/v0.14.0},scripts}

# 下载 Kubernetes 组件 (多架构多 OS)
for arch in "${ARCHS[@]}"; do
    download_k8s_debs "$RELEASE_VERSION" "$arch" "$OUTPUT_DIR/kubernetes/$RELEASE_VERSION/ubuntu/$arch"
    download_k8s_rpms "$RELEASE_VERSION" "$arch" "$OUTPUT_DIR/kubernetes/$RELEASE_VERSION/rhel/$arch"
done

# 下载 containerd (多架构)
for arch in "${ARCHS[@]}"; do
    curl -fsSL "https://github.com/containerd/containerd/releases/download/v1.7.0/containerd-1.7.0-linux-${arch}.tar.gz" \
      -o "$OUTPUT_DIR/runtime/containerd/v1.7.0/linux-${arch}/containerd-1.7.0-linux-${arch}.tar.gz"
done

# 下载 CNI plugins (多架构)
for arch in "${ARCHS[@]}"; do
    curl -fsSL "https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-${arch}-v1.3.0.tgz" \
      -o "$OUTPUT_DIR/cni/plugins/v1.3.0/linux-${arch}/cni-plugins-linux-${arch}-v1.3.0.tgz"
done

# 下载 Manifest/Helm 组件
curl -fsSL "https://raw.githubusercontent.com/projectcalico/calico/v3.27.0/manifests/calico.yaml" \
  -o "$OUTPUT_DIR/cni/calico/v3.27.0/calico.yaml"

curl -fsSL "https://raw.githubusercontent.com/cilium/cilium/v1.15.0/install/kubernetes/cilium/quick-install.yaml" \
  -o "$OUTPUT_DIR/cni/cilium/v1.15.0/cilium.yaml"

# 下载并保存容器镜像
save_container_images() {
    local output_dir="$1"
    
    # Kubernetes 核心镜像
    for image in kube-apiserver kube-controller-manager kube-scheduler kube-proxy coredns etcd pause; do
        docker pull "registry.k8s.io/${image}:v1.31.0"
        docker save "registry.k8s.io/${image}:v1.31.0" -o "$output_dir/images/kubernetes/v1.31.0/${image}.tar"
    done
    
    # Calico 镜像
    for image in calico/node calico/kube-controllers calico/cni; do
        docker pull "docker.io/${image}:v3.27.0"
        docker save "docker.io/${image}:v3.27.0" -o "$output_dir/images/calico/v3.27.0/$(basename $image).tar"
    done
    
    # Cilium 镜像
    for image in cilium/cilium cilium/operator cilium/hubble-relay; do
        docker pull "quay.io/${image}:v1.15.0"
        docker save "quay.io/${image}:v1.15.0" -o "$output_dir/images/cilium/v1.15.0/$(basename $image).tar"
    done
    
    # Envoy Gateway 镜像
    for image in envoyproxy/gateway envoyproxy/envoy; do
        docker pull "docker.io/${image}:v1.1.0"
        docker save "docker.io/${image}:v1.1.0" -o "$output_dir/images/envoy-gateway/v1.1.0/$(basename $image).tar"
    done
    
    # MetalLB 镜像
    for image in metallb/controller metallb/speaker; do
        docker pull "quay.io/${image}:v0.14.0"
        docker save "quay.io/${image}:v0.14.0" -o "$output_dir/images/metallb/v0.14.0/$(basename $image).tar"
    done
}

save_container_images "$OUTPUT_DIR"

# 生成 index.json
generate_index_json "$OUTPUT_DIR"

# 生成 checksums
cd "$OUTPUT_DIR" && find . -type f -exec sha256sum {} + > checksums.sha256

# 构建 OCI 镜像
docker build -t capbm-release:$RELEASE_VERSION .
docker push capbm-release:$RELEASE_VERSION
```

### 10.2 Dockerfile

```dockerfile
FROM nginx:alpine AS release-server

COPY release/ /usr/share/nginx/html/release/

# 启用目录浏览
RUN echo 'server { \
    listen 8080; \
    server_name localhost; \
    location / { \
        autoindex on; \
        autoindex_exact_size off; \
        autoindex_localtime on; \
    } \
}' > /etc/nginx/conf.d/default.conf

EXPOSE 8080
CMD ["nginx", "-g", "daemon off;"]
```

## 11. index.json 索引文件

```json
{
  "version": "v1.31.0",
  "components": {
    "kubernetes": {
      "version": "v1.31.0",
      "type": "binary",
      "path": "kubernetes/v1.31.0",
      "osSpecific": true,
      "platforms": {
        "ubuntu": {
          "architectures": ["linux-amd64", "linux-arm64"],
          "packages": ["kubeadm", "kubelet", "kubectl"]
        },
        "rhel": {
          "architectures": ["linux-amd64", "linux-arm64"],
          "packages": ["kubeadm", "kubelet", "kubectl"]
        }
      }
    },
    "containerd": {
      "version": "v1.7.0",
      "type": "binary",
      "path": "runtime/containerd/v1.7.0",
      "osSpecific": false,
      "architectures": ["linux-amd64", "linux-arm64"]
    },
    "cni-plugins": {
      "version": "v1.3.0",
      "type": "binary",
      "path": "cni/plugins/v1.3.0",
      "osSpecific": false,
      "architectures": ["linux-amd64", "linux-arm64"]
    },
    "calico": {
      "version": "v3.27.0",
      "type": "manifest",
      "path": "cni/calico/v3.27.0",
      "osSpecific": false,
      "installModes": ["manifest", "helm"],
      "files": {
        "manifest": "calico.yaml",
        "chart": "calico.tgz"
      },
      "images": "images/calico/v3.27.0",
      "imageList": [
        "calico-node.tar",
        "calico-kube-controllers.tar",
        "calico-cni.tar"
      ]
    },
    "cilium": {
      "version": "v1.15.0",
      "type": "helm",
      "path": "cni/cilium/v1.15.0",
      "osSpecific": false,
      "files": {
        "chart": "cilium.tgz"
      },
      "images": "images/cilium/v1.15.0",
      "imageList": [
        "cilium.tar",
        "cilium-operator.tar",
        "hubble-relay.tar"
      ],
      "helmValues": {
        "ipam.mode": "kubernetes",
        "kubeProxyReplacement": "partial"
      }
    }
  },
  "images": {
    "kubernetes": {
      "version": "v1.31.0",
      "path": "images/kubernetes/v1.31.0",
      "images": [
        "kube-apiserver.tar",
        "kube-controller-manager.tar",
        "kube-scheduler.tar",
        "kube-proxy.tar",
        "coredns.tar",
        "etcd.tar",
        "pause.tar"
      ]
    },
    "calico": {
      "version": "v3.27.0",
      "path": "images/calico/v3.27.0",
      "images": [
        "calico-node.tar",
        "calico-kube-controllers.tar",
        "calico-cni.tar"
      ]
    },
    "cilium": {
      "version": "v1.15.0",
      "path": "images/cilium/v1.15.0",
      "images": [
        "cilium.tar",
        "cilium-operator.tar",
        "hubble-relay.tar"
      ]
    },
    "envoy-gateway": {
      "version": "v1.1.0",
      "path": "images/envoy-gateway/v1.1.0",
      "images": [
        "envoy-gateway.tar",
        "envoy-proxy.tar"
      ]
    },
    "metallb": {
      "version": "v0.14.0",
      "path": "images/metallb/v0.14.0",
      "images": [
        "metallb-controller.tar",
        "metallb-speaker.tar"
      ]
    }
  }
}
```

## 12. 常见问题

### Q1: 如何更新 ReleaseImage 中的组件版本？

重新运行构建脚本并推送新镜像：

```bash
./build-release-image.sh v1.32.0
docker push capbm-release:v1.32.0
```

### Q2: 如何验证组件完整性？

```bash
# 下载 checksums 文件
curl http://<release-server>:8080/release/checksums.sha256 -o checksums.sha256

# 验证所有文件
cd /path/to/release && sha256sum -c checksums.sha256
```

### Q3: ReleaseImage 镜像有多大？

典型大小约 3-6 GB，包含：
- Kubernetes 二进制: ~200 MB
- containerd: ~100 MB
- CNI plugins: ~50 MB
- Kubernetes 镜像: ~500 MB (7 个镜像)
- Calico 镜像: ~200 MB (3 个镜像)
- Cilium 镜像: ~300 MB (3 个镜像)
- Envoy Gateway 镜像: ~150 MB (2 个镜像)
- MetalLB 镜像: ~50 MB (2 个镜像)
- Manifests/Helm charts: ~10 MB

### Q4: 可以同时部署多个版本的 ReleaseImage 吗？

可以。每个版本使用不同的 Service 名称：

```bash
kubectl apply -f release-server-v1.31.0.yaml
kubectl apply -f release-server-v1.32.0.yaml
```

不同集群通过 `baseUrl` 引用不同版本：

```yaml
# 集群 A
baseUrl: "http://release-server-v1.31.0:8080/release"

# 集群 B
baseUrl: "http://release-server-v1.32.0:8080/release"
```

### Q5: 如何在没有 Docker 的环境中使用？

可以使用 Podman 或其他 OCI 兼容运行时：

```bash
podman pull capbm-release:v1.31.0
podman save capbm-release:v1.31.0 -o capbm-release-v1.31.0.tar
podman load -i capbm-release-v1.31.0.tar
podman run -d --name release-server -p 8080:8080 capbm-release:v1.31.0
```
