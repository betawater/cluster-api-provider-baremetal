# CAPBM 使用指南

## 目录

- [1. 前置条件](#1-前置条件)
- [2. 安装 CAPBM Provider](#2-安装-capbm-provider)
- [3. 部署 ClusterClass 模板](#3-部署-clusterclass-模板)
- [4. 创建 ReleaseImage](#4-创建-releaseimage)
- [5. 配置机器池 (BareMetalHostInventory)](#5-配置机器池-baremetalhostinventory)
- [6. 创建裸金属集群](#6-创建裸金属集群)
- [7. 负载均衡器配置](#7-负载均衡器配置)
- [8. K8s 核心组件配置定制](#8-k8s-核心组件配置定制)
- [9. 单节点集群安装](#9-单节点集群安装)
- [10. 集群管理](#10-集群管理)
- [11. 多 Worker 池配置](#11-多-worker-池配置)
- [12. 升级集群](#12-升级集群)
- [13. 删除集群](#13-删除集群)
- [14. 故障排查](#14-故障排查)
- [15. 常见问题](#15-常见问题)

---

## 1. 前置条件

### 1.1 管理集群

- Kubernetes v1.32+ 管理集群
- `kubectl` 已配置并连接到管理集�?
- `clusterctl` v1.13+ 已安�?

### 1.2 裸金属机器要�?

| 项目 | 最低要�?| 推荐配置 |
|------|----------|----------|
| 操作系统 | Ubuntu 20.04+, CentOS 7+, Rocky 8+ | Ubuntu 22.04 LTS |
| CPU | 2 �?| 4 �? |
| 内存 | 2 GB | 4 GB+ |
| 磁盘 | 20 GB 可用空间 | 50 GB+ SSD |
| 网络 | SSH 可达，外网可访问 | 千兆以太�?|
| 内核版本 | >= 3.10 | >= 5.4 |

### 1.3 网络要求

- 所有裸金属机器之间网络互�?
- 管理集群可通过 SSH 访问所有裸金属机器
- 外部负载均衡器地址（用�?API Server 端点�?

### 1.4 机器信息配置 (BareMetalHostInventory)

�?ClusterClass 模式下，裸金属机器的具体信息（IP、主机名、凭据）通过 **BareMetalHostInventory** 资源统一管理。这是一个机器池概念，CAPBM 会从中自动分配可用机器给集群使用�?

**为什么需要机器池�?*
- ClusterClass 使用 `replicas` 指定节点数量，但不指定具体机�?
- 裸金属机器是预先存在的物理服务器，需要有一个地方定义它们的信息
- 机器池支持多集群共享、自动分配和释放

**机器池工作流�?*�?
```
1. 创建 BareMetalHostInventory 定义所有可用机�?
2. 创建 Cluster 时引用机器池
3. CAPBM 自动从池中分配可用机器（根据 role 过滤�?
4. 删除集群时机器自动释放回池中
```

---

## 2. 安装 CAPBM Provider

### 2.1 安装 Cluster API 核心组件

```bash
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm
```

### 2.2 安装 CAPBM Provider

```bash
# 使用 kustomize 安装
kubectl apply -k modules/capbm/config/default/
```

### 2.3 安装 CVO (Cluster Version Operator)

```bash
# 安装 CVO
kubectl apply -k modules/cvo/config/default/
```

### 2.4 验证安装

```bash
# 验证 CAPI 核心组件
kubectl get pods -n capi-system
kubectl get pods -n capi-kubeadm-bootstrap-system
kubectl get pods -n capi-kubeadm-control-plane-system

# 验证 CAPBM
kubectl get pods -n capbm-system

# 验证 CVO
kubectl get pods -n cvo-system

# 验证 CRDs
kubectl get crd | grep -E "baremetal|cluster.x-k8s.io|cvo.capbm.io"
```

---

## 3. 部署 ClusterClass 模板

### 3.1 应用 ClusterClass 及相关模�?

```bash
kubectl apply -k modules/capbm/config/clusterclass/
```

### 3.2 验证部署

```bash
kubectl get clusterclass baremetal-clusterclass-v0.1.0
kubectl get baremetalclustertemplate
kubectl get baremetalmachinetemplate
kubectl get kubeadmcontrolplanetemplate
kubectl get kubeadmconfigtemplate
```

---

## 4. 创建 ReleaseImage

ReleaseImage 定义了完整的 Kubernetes 发行版本，包括组件版本、Addon 定义和升级图�?

### 4.1 创建 ReleaseImage

```bash
# 应用 ReleaseImage
kubectl apply -f release-image/release.json
```

### 4.2 验证 ReleaseImage

```bash
# 查看 ReleaseImage 状�?
kubectl get releaseimage v1.31.1 -o yaml

# 查看状态字�?
kubectl get releaseimage v1.31.1 -o jsonpath='{.status}'
```

### 4.3 ReleaseImage 结构

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
spec:
  version: v1.31.1
  image: registry.example.com/capbm/release:v1.31.1
  
  # 二进制组�?
  components:
    kubernetes:
      version: v1.31.1
      type: binary
      platforms:
        ubuntu:
          architectures: [amd64, arm64]
          packages:
            kubeadm: kubeadm_1.31.1-00
            kubelet: kubelet_1.31.1-00
            kubectl: kubectl_1.31.1-00
    containerd:
      version: 1.7.24
      type: binary
      # ...
  
  # Addon 定义
  addons:
    - name: calico
      type: helm
      version: v3.28.1
      contentPath: charts/calico-v3.28.1.tgz
      namespace: kube-system
      # ...
  
  # 升级�?
  upgradeGraph:
    - name: phase-1-runtime
      order: 1
      blocking: true
      components: [containerd]
    # ...
```

---

## 5. 配置机器�?(BareMetalHostInventory)

### 4.1 创建集群 Namespace

推荐为每个集群创建独立的 namespace 以实现资源隔离：

```bash
# 创建集群 namespace
kubectl create namespace cluster-my-cluster
```

### 4.2 创建机器池定�?

机器池定义了所有可用的裸金属机器信息，包括 IP、主机名、凭据和角色�?

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: datacenter-a-hosts
  namespace: cluster-my-cluster   # 使用集群 namespace
spec:
  hosts:
  # 控制面节�?
  - name: node-01
    hostName: "node-01"
    ipAddress: "192.168.1.101"
    sshPort: 22
    credentialsRef:
      name: node-01-credentials
    role: "control-plane"
    labels:
      rack: "rack-1"
      zone: "zone-a"
  - name: node-02
    hostName: "node-02"
    ipAddress: "192.168.1.102"
    sshPort: 22
    credentialsRef:
      name: node-02-credentials
    role: "control-plane"
    labels:
      rack: "rack-1"
      zone: "zone-a"
  - name: node-03
    hostName: "node-03"
    ipAddress: "192.168.1.103"
    sshPort: 22
    credentialsRef:
      name: node-03-credentials
    role: "control-plane"
    labels:
      rack: "rack-2"
      zone: "zone-b"
  # Worker 节点
  - name: node-04
    hostName: "node-04"
    ipAddress: "192.168.1.104"
    sshPort: 22
    credentialsRef:
      name: node-04-credentials
    role: "worker"
    labels:
      rack: "rack-2"
      zone: "zone-b"
  - name: node-05
    hostName: "node-05"
    ipAddress: "192.168.1.105"
    sshPort: 22
    credentialsRef:
      name: node-05-credentials
    role: "worker"
    labels:
      rack: "rack-3"
      zone: "zone-a"
```

### 4.2 创建机器凭据 Secret

在集�?namespace 中为每台机器创建独立的凭�?Secret（或使用统一的凭据）�?

```bash
# 方式一：每台机器独立凭�?
kubectl create secret generic node-01-credentials \
  --from-literal=username=root \
  --from-literal=password=node01-password \
  -n cluster-my-cluster

kubectl create secret generic node-02-credentials \
  --from-literal=username=root \
  --from-literal=password=node02-password \
  -n cluster-my-cluster

# 方式二：使用统一凭据（所有机器相同）
kubectl create secret generic baremetal-unified-credentials \
  --from-literal=username=root \
  --from-literal=password=unified-password \
  -n cluster-my-cluster
```

### 4.3 应用机器池配�?

```bash
kubectl apply -f baremetalhostinventory.yaml -n cluster-my-cluster

kubectl get baremetalhostinventory datacenter-a-hosts -n cluster-my-cluster
kubectl get baremetalhostinventory datacenter-a-hosts -n cluster-my-cluster -o yaml
```

### 4.4 查看机器池状�?

```bash
kubectl get baremetalhostinventory datacenter-a-hosts -n cluster-my-cluster -o jsonpath='{.status}'
```

输出示例:
```json
{
  "totalHosts": 5,
  "availableHosts": 5,
  "allocatedHosts": 0,
  "hostsStatus": [
    {"name": "node-01", "state": "Available"},
    {"name": "node-02", "state": "Available"}
  ]
}
```

### 4.5 多机器池管理

您可以创建多个机器池来管理不同位置或类型的机器（每个集群使用独立 namespace）：

```yaml
# 北京数据中心机器�?
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: beijing-datacenter-hosts
  namespace: cluster-beijing   # 北京集群 namespace
spec:
  hosts:
  - name: bj-node-01
    hostName: "bj-node-01"
    ipAddress: "10.0.1.101"
    credentialsRef:
      name: beijing-credentials
    role: "control-plane"
---
# 上海数据中心机器�?
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: shanghai-datacenter-hosts
  namespace: cluster-shanghai   # 上海集群 namespace
spec:
  hosts:
  - name: sh-node-01
    hostName: "sh-node-01"
    ipAddress: "10.0.2.101"
    credentialsRef:
      name: shanghai-credentials
    role: "control-plane"
```

创建集群时通过 `hostInventoryRef` 变量指定使用哪个机器池：

```yaml
variables:
- name: hostInventoryRef
  value: "beijing-datacenter-hosts"
```

---

## 5. 创建裸金属集�?

### 5.1 使用 clusterctl 生成集群配置

```bash
clusterctl generate cluster my-baremetal-cluster \
  --from templates/clusterclass/baremetal-clusterclass-v0.1.0.yaml \
  --variable CLUSTER_NAME=my-baremetal-cluster \
  --variable NAMESPACE=cluster-my-cluster \
  --variable KUBERNETES_VERSION=v1.31.0 \
  --variable CONTROL_PLANE_MACHINE_COUNT=3 \
  --variable WORKER_MACHINE_COUNT=2 \
  --variable CONTROL_PLANE_ENDPOINT_HOST=lb.example.com \
  --variable CONTROL_PLANE_ENDPOINT_PORT=6443 \
  --variable HOST_INVENTORY_REF=datacenter-a-hosts \
  --variable SSH_CREDENTIALS_SECRET=baremetal-ssh-credentials \
  > cluster.yaml
```

### 5.2 手动编写集群配置

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-baremetal-cluster
  namespace: cluster-my-cluster   # 使用集群 namespace
spec:
  topology:
    classRef:
      name: baremetal-clusterclass-v0.1.0
      namespace: capbm-system     # ClusterClass 在系�?namespace
    version: v1.31.0
    controlPlane:
      replicas: 3
      metadata:
        labels:
          role: control-plane
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
        replicas: 2
        metadata:
          labels:
            role: worker
    variables:
    - name: controlPlaneEndpoint
      value:
        host: "lb.example.com"
        port: 6443
    - name: credentialsSecret
      value: "baremetal-ssh-credentials"
    - name: hostInventoryRef
      value: "datacenter-a-hosts"
    - name: kubernetesVersion
      value: "v1.31.0"
    - name: podCIDR
      value: "10.244.0.0/16"
    - name: serviceCIDR
      value: "10.96.0.0/12"
```

### 5.3 应用集群配置

```bash
kubectl apply -f cluster.yaml -n cluster-my-cluster

clusterctl describe cluster my-baremetal-cluster -n cluster-my-cluster

kubectl get cluster my-baremetal-cluster -n cluster-my-cluster
kubectl get baremetalcluster my-baremetal-cluster -n cluster-my-cluster
```

### 5.4 监控集群创建过程

```bash
kubectl get cluster my-baremetal-cluster -n cluster-my-cluster --watch

kubectl get baremetalcluster my-baremetal-cluster -n cluster-my-cluster -o yaml

kubectl get machines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster -n cluster-my-cluster

kubectl get baremetalmachines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster -n cluster-my-cluster
```

### 5.5 获取工作集群 kubeconfig

```bash
clusterctl get kubeconfig my-baremetal-cluster -n cluster-my-cluster > workload-kubeconfig

kubectl --kubeconfig workload-kubeconfig get nodes
kubectl --kubeconfig workload-kubeconfig get pods -A
```

---

## 6. 负载均衡器配�?

CAPBM 支持自动�?control-plane 节点注册到外部负载均衡器，并支持 Ingress 流量负载均衡�?

### 6.1 API Server 负载均衡�?

CAPBM 支持以下负载均衡器类型：

| 类型 | 注册方式 | 适用场景 |
|------|---------|---------|
| HAProxy | Runtime API / SSH | 中小型集�?|
| F5 BIG-IP | iControl REST API | 企业级硬�?LB |
| Keepalived | VIP 故障转移 | 无外�?LB 的裸金属 |
| MetalLB | BGP / L2 宣告 | 裸金�?Service LB |

#### HAProxy 配置

```yaml
variables:
- name: loadBalancer
  value:
    provider: "haproxy"
    healthCheck:
      enabled: true
      path: "/healthz"
      interval: "5s"
    haproxy:
      adminHost: "10.0.0.50"
      adminPort: 9999
      backendName: "k8s-apiserver"
      reloadCommand: "systemctl reload haproxy"
```

#### F5 BIG-IP 配置

```yaml
variables:
- name: loadBalancer
  value:
    provider: "f5"
    f5:
      host: "f5.example.com"
      port: 443
      credentialsRef:
        name: "f5-credentials"
      partition: "Common"
      poolName: "k8s-apiserver-pool"
```

#### Keepalived 配置 (无外�?LB)

```yaml
variables:
- name: loadBalancer
  value:
    provider: "keepalived"
    keepalived:
      virtualIP: "10.0.0.100"
      interface: "eth0"
      virtualRouterID: 51
      priority: 100
```

### 6.2 Ingress 负载均衡�?

配置 worker 节点�?Ingress 流量负载均衡�?

```yaml
variables:
- name: ingressLoadBalancer
  value:
    enabled: true
    provider: "haproxy"
    haproxy:
      adminHost: "10.0.0.50"
      backendName: "k8s-ingress"
      httpPort: 30080          # worker 节点 HTTP NodePort
      httpsPort: 30443         # worker 节点 HTTPS NodePort
```

### 6.3 验证负载均衡器状�?

```bash
# 检�?API Server LB 状�?
kubectl get baremetalcluster my-baremetal-cluster -o jsonpath='{.status.conditions[?(@.type=="LoadBalancerReady")]}'

# 检�?Ingress LB 状�?
kubectl get baremetalcluster my-baremetal-cluster -o jsonpath='{.status.conditions[?(@.type=="IngressLoadBalancerReady")]}'

# 查看 HAProxy 后端
echo "show servers state k8s-apiserver" | socat stdio tcp:10.0.0.50:9999
```

---

## 7. K8s 核心组件配置定制

CAPBM 支持通过 ClusterClass 变量定制节点上安装的 Kubernetes 核心组件�?

### 7.1 容器运行时配�?

支持 containerd、CRI-O �?Docker 三种容器运行时：

```yaml
variables:
- name: componentInstall
  value:
    enabled: true
    strategy: "InstallIfMissing"  # InstallIfMissing | AlwaysInstall | Skip
    containerRuntime:
      type: "containerd"          # containerd | cri-o | docker
      version: "1.7.0"
      config:
        systemdCgroup: true
        sandboxImage: "registry.k8s.io/pause:3.9"
        registryMirrors:
          - host: "docker.io"
            endpoints: ["https://mirror.example.com"]
        maxConcurrentDownloads: 3
        rawConfig: |
          [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia]
            runtime_type = "io.containerd.runc.v2"
```

### 7.2 Kubernetes 组件配置

自定�?kubelet 参数和组件仓库：

```yaml
variables:
- name: componentInstall
  value:
    kubernetes:
      version: "1.31.0"
      repository:
        baseUrl: "https://pkgs.k8s.io/core:/stable:/v1.31/deb/"
        gpgKey: "https://pkgs.k8s.io/core:/stable:/v1.31/deb/Release.key"
      config:
        kubelet:
          cgroupDriver: "systemd"
          maxPods: 250
          extraArgs:
            feature-gates: "RotateKubeletServerCertificate=true"
            kube-reserved: "cpu=250m,memory=512Mi"
```

### 7.3 CNI 插件配置

支持 Calico、Cilium �?Flannel�?

```yaml
variables:
- name: cni
  value:
    enabled: true
    type: "calico"           # calico | cilium | flannel
    version: "3.27.0"
    config:
      podCIDR: "10.244.0.0/16"
      calico:
        ipam: "CalicoIPAM"
        bgp:
          enabled: false
      cilium:
        kubeProxyReplacement: "partial"
        routingMode: "tunnel"
```

### 7.4 CSI 驱动配置

支持 Ceph-CSI、Cinder-CSI、Local-CSI �?NFS-CSI�?

```yaml
variables:
- name: csi
  value:
    enabled: false
    driver: "ceph-csi"       # ceph-csi | cinder-csi | local-csi | nfs-csi
    version: "3.9.0"
    config:
      cephCsi:
        clusterID: "my-ceph-cluster"
        monitors:
          - "10.0.0.10:6789"
        rbd:
          enabled: true
          pool: "kubernetes"
        storageClass:
          name: "ceph-rbd"
          reclaimPolicy: "Delete"
```

### 7.5 离线/Air-Gap 安装

在内网或隔离环境中配置离线安装：

```yaml
variables:
- name: componentInstall
  value:
    airGap:
      enabled: true
      binarySource: "HTTPServer"  # HTTPServer | ConfigMap | LocalPath
      httpServerConfig:
        baseUrl: "http://10.0.0.50:30080/release"
      preloadImages:
        - "registry.k8s.io/kube-apiserver:v1.31.0"
        - "registry.k8s.io/kube-controller-manager:v1.31.0"
        - "registry.k8s.io/kube-scheduler:v1.31.0"
```

### 7.6 ReleaseImage 镜像安装

CAPBM 支持使用 ReleaseImage OCI 镜像作为组件源。ReleaseImage 是一个自包含的镜像，内置 HTTP 服务器，提供所有组件的下载服务�?

#### 7.6.1 部署 ReleaseImage HTTP 服务�?

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
---
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

#### 7.6.2 ReleaseImage 目录结构

ReleaseImage 镜像使用统一的目录结构规范：`分类/组件名称/版本/平台/组件内容`

```
/release/
├── runtime/                    # 容器运行�?
�?  └── containerd/
�?      └── v1.7.0/
�?          ├── linux-amd64/
�?          └── linux-arm64/
├── kubernetes/                 # Kubernetes 核心二进�?
�?  └── v1.31.0/
�?      ├── ubuntu/
�?      �?  ├── linux-amd64/
�?      �?  └── linux-arm64/
�?      └── rhel/
�?          ├── linux-amd64/
�?          └── linux-arm64/
├── cni/                        # CNI 网络插件
�?  ├── plugins/
�?  �?  └── v1.3.0/
�?  �?      ├── linux-amd64/
�?  �?      └── linux-arm64/
�?  ├── calico/
�?  �?  └── v3.27.0/
�?  ├── cilium/
�?  �?  └── v1.15.0/
�?  └── flannel/
�?      └── v0.24.0/
├── csi/                        # CSI 存储驱动
�?  ├── ceph-csi/
�?  ├── local-path-provisioner/
�?  └── nfs-csi/
├── gateway/                    # 网关组件
�?  ├── gateway-api/
�?  └── envoy-gateway/
├── metallb/                    # 负载均衡�?
├── images/                     # 容器镜像�?
├── index.json                  # 组件索引
└── checksums.sha256            # 校验�?
```

#### 7.6.3 配置集群使用 ReleaseImage

```yaml
variables:
- name: componentInstall
  value:
    enabled: true
    airGap:
      enabled: true
      binarySource: "HTTPServer"
      httpServerConfig:
        baseUrl: "http://10.0.0.50:30080/release"
- name: cni
  value:
    enabled: true
    type: "calico"
    version: "3.27.0"
    airGap:
      enabled: true
      manifestSource: "HTTPServer"
      httpServerConfig:
        baseUrl: "http://10.0.0.50:30080/release"
```

---

## 8. 单节点集群安�?

CAPBM 支持创建单节�?Kubernetes 集群（control-plane �?worker 合并），适用于开发测试或边缘计算场景�?

### 8.1 机器池配�?

单节点集群只需一台机器，role 设置�?`control-plane`�?

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: single-node-hosts
  namespace: default
spec:
  hosts:
  - name: node-01
    hostName: "node-01"
    ipAddress: "192.168.1.101"
    sshPort: 22
    credentialsRef:
      name: node-01-credentials
    role: "control-plane"
```

### 8.2 集群配置

设置 `controlPlane.replicas: 1`，不配置 worker 节点�?

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: single-node-cluster
  namespace: default
spec:
  topology:
    classRef:
      name: baremetal-clusterclass-v0.1.0
    version: v1.31.0
    controlPlane:
      replicas: 1
      metadata:
        labels:
          role: control-plane
    # 不配�?workers，control-plane 节点同时承担 worker 角色
    variables:
    - name: controlPlaneEndpoint
      value:
        host: "192.168.1.101"  # 直接使用节点 IP
        port: 6443
    - name: credentialsSecret
      value: "node-01-credentials"
    - name: hostInventoryRef
      value: "single-node-hosts"
    - name: kubernetesVersion
      value: "v1.31.0"
```

### 8.3 允许控制面节点调度工作负�?

默认情况下，control-plane 节点有污�?`node-role.kubernetes.io/control-plane:NoSchedule`。如需在单节点上运行工作负载，需移除污点�?

```bash
# 获取 kubeconfig 后执�?
kubectl --kubeconfig workload-kubeconfig taint nodes --all node-role.kubernetes.io/control-plane-

# 或者在集群配置中通过 KubeadmConfigTemplate 设置
```

### 8.4 注意事项

| 项目 | 说明 |
|------|------|
| 高可�?| 单节点集群无高可用能力，不适用于生产环�?|
| etcd | 单节�?etcd，数据丢失风�?|
| 升级 | 升级期间服务不可�?|
| 负载均衡�?| 可直接使用节�?IP 作为 controlPlaneEndpoint |

---

## 9. 集群管理

### 9.1 扩缩�?

```bash
# 扩容 Worker 节点�?5 �?
kubectl patch cluster my-baremetal-cluster --type='merge' -p '{
  "spec": {
    "topology": {
      "workers": {
        "machineDeployments": [
          {
            "class": "default-worker",
            "name": "md-0",
            "replicas": 5
          }
        ]
      }
    }
  }
}'

kubectl get machines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster
```

### 9.2 查看集群详情

```bash
clusterctl describe cluster my-baremetal-cluster --show-conditions all

kubectl get all -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster
```

### 9.3 健康检�?

```bash
kubectl get machinehealthcheck -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster

kubectl --kubeconfig workload-kubeconfig get nodes
kubectl --kubeconfig workload-kubeconfig describe node <node-name>
```

---

## 10. �?Worker 池配�?

### 10.1 创建�?Worker 池集�?

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: multi-pool-cluster
  namespace: default
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
        name: general-purpose
        replicas: 3
        metadata:
          labels:
            node-type: general
      - class: default-worker
        name: high-memory
        replicas: 2
        metadata:
          labels:
            node-type: high-memory
        variables:
          overrides:
          - name: preFlightChecks
            value:
              enabled: true
              minMemoryGB: 8
              minDiskGB: 50
    variables:
    - name: controlPlaneEndpoint
      value:
        host: "lb.example.com"
        port: 6443
    - name: credentialsSecret
      value: "baremetal-ssh-credentials"
    - name: hostInventoryRef
      value: "datacenter-a-hosts"
    - name: kubernetesVersion
      value: "v1.31.0"
```

### 10.2 使用节点选择器调度工作负�?

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: memory-intensive-app
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: memory-intensive
  template:
    spec:
      nodeSelector:
        node-type: high-memory
      containers:
      - name: app
        image: my-app:latest
        resources:
          requests:
            memory: "4Gi"
```

---

## 12. 升级集群

### 12.1 使用 ClusterVersion 升级

CAPBM 使用 ClusterVersion 资源来管理集群升级。首先确保已创建 ReleaseImage：

```bash
# 确保 ReleaseImage 已创建
kubectl get releaseimage v1.31.1
```

### 12.2 创建 ClusterVersion 触发升级

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
  namespace: default
spec:
  clusterRef:
    name: my-cluster
    namespace: default
  desiredUpdate:
    version: v1.31.1
    image: registry.example.com/capbm/release:v1.31.1
```

```bash
# 应用 ClusterVersion 触发升级
kubectl apply -f clusterversion.yaml
```

### 12.3 升级流程说明

```
1. CVO Controller 检测 ClusterVersion 变更
2. 验证升级路径 (UpgradePath)
3. 获取目标 ReleaseImage
4. Phase 1: K8S 升级 (containerd → kubernetes)
   - 控制面节点逐个升级
   - etcd 集群逐个升级
   - API Server 逐个滚动升级
   - 等待控制面健康
5. Phase 2: Addon 升级 (calico → ceph-csi → ...)
   - 按依赖顺序升级 Addon
   - 等待每个 Addon 就绪
6. 更新 ClusterVersion 状态
```

### 12.4 仅 Addon 升级 (K8S 版本不变)

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
  namespace: default
spec:
  clusterRef:
    name: my-cluster
    namespace: default
  desiredUpdate:
    version: v1.31.0     # K8S 版本不变
    image: registry.example.com/capbm/release:v1.31.0-patch1  # 新 ReleaseImage
```

### 12.5 监控升级进度

```bash
# 查看 ClusterVersion 状态
kubectl get clusterversion my-cluster -o yaml

# 查看 ReleaseImage 状态
kubectl get releaseimage v1.31.1 -o yaml

# 查看 ClusterAddon 状态
kubectl get clusteraddon -n default

# 查看升级日志
kubectl logs -n cvo-system -l control-plane=controller-manager --tail=100
```

---

## 13. 删除集群

### 13.1 删除集群

```bash
kubectl delete cluster my-baremetal-cluster

clusterctl delete cluster my-baremetal-cluster
```

### 13.2 清理凭据

```bash
kubectl delete secret baremetal-ssh-credentials
```

### 13.3 卸载 CVO

```bash
kubectl delete -k modules/cvo/config/default/
```

### 13.4 卸载 CAPBM Provider

```bash
kubectl delete -k modules/capbm/config/default/

clusterctl delete --infrastructure baremetal

clusterctl delete --all
```

---

## 14. 故障排查

### 14.1 集群创建失败

```bash
kubectl describe cluster my-baremetal-cluster

kubectl describe baremetalcluster my-baremetal-cluster

kubectl describe baremetalmachine <machine-name>

kubectl logs -n capbm-system -l control-plane=controller-manager --tail=100
```

### 14.2 SSH 连接问题

```bash
kubectl get secret baremetal-ssh-credentials

kubectl get secret baremetal-ssh-credentials -o jsonpath='{.data.username}' | base64 -d

ssh -o StrictHostKeyChecking=no root@<machine-ip> echo "SSH connection successful"
```

### 14.3 预检失败

```bash
kubectl get baremetalmachine <machine-name> -o jsonpath='{.status.conditions}'
```

常见预检失败原因�?
1. OS 不支�?- 检�?/etc/os-release
2. 内核版本过低 - 检�?uname -r
3. 磁盘空间不足 - 检�?df -h
4. 内存不足 - 检�?free -g
5. Swap 未关�?- 检�?swapon --show

### 14.4 网络问题

```bash
curl -k https://lb.example.com:6443/version

kubectl --kubeconfig workload-kubeconfig exec -it <pod-name> -- ping <other-node-ip>

kubectl --kubeconfig workload-kubeconfig run -it dns-test --image=busybox:1.28 --restart=Never -- nslookup kubernetes.default
```

---

## 15. 常见问题

### Q1: 如何添加新的裸金属机器？

更新 BareMetalHostInventory 添加新机器，然后增加 replicas�?

```bash
kubectl edit baremetalhostinventory datacenter-a-hosts

kubectl patch cluster my-baremetal-cluster --type='merge' -p \
  '{"spec":{"topology":{"workers":{"machineDeployments":[{"class":"default-worker","name":"md-0","replicas":3}]}}}}'
```

### Q2: 如何更换 SSH 凭据�?

更新 Secret 并重新触发调和：

```bash
kubectl create secret generic baremetal-ssh-credentials \
  --from-literal=username=newuser \
  --from-literal=password=newpassword \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl annotate baremetalmachine <machine-name> force-reconcile=$(date +%s)
```

### Q3: 如何禁用预检�?

```yaml
variables:
- name: preFlightChecks
  value:
    enabled: false
```

### Q4: 支持哪些操作系统�?

- Ubuntu 20.04, 22.04
- CentOS 7, 8
- Rocky Linux 8, 9
- AlmaLinux 8, 9
- Debian 10, 11, 12

### Q5: 如何自定�?Pod CIDR �?Service CIDR�?

```yaml
variables:
- name: podCIDR
  value: "192.168.0.0/16"
- name: serviceCIDR
  value: "172.16.0.0/12"
```

### Q6: 如何查看 ClusterClass 的完整定义？

```bash
kubectl get clusterclass baremetal-clusterclass-v0.1.0 -o yaml
```

### Q7: 如何查看机器池分配状态？

```bash
kubectl get baremetalhostinventory datacenter-a-hosts -o jsonpath='{.status.hostsStatus}'
```

---

## 附录

### A. ClusterClass 变量参�?

| 变量�?| 类型 | 必填 | 默认�?| 说明 |
|--------|------|------|--------|------|
| controlPlaneEndpoint | object | �?| - | 控制面负载均衡地址 (host, port) |
| credentialsSecret | string | �?| - | SSH 凭据 Secret 名称 |
| hostInventoryRef | string | �?| - | BareMetalHostInventory 资源名称 |
| kubernetesVersion | string | �?| - | Kubernetes 版本 (格式: vX.Y.Z) |
| podCIDR | string | �?| 10.244.0.0/16 | Pod 网络 CIDR |
| serviceCIDR | string | �?| 10.96.0.0/12 | Service 网络 CIDR |
| preFlightChecks | object | �?| enabled: true | 预检配置 |
| componentInstall | object | �?| enabled: true | 组件安装配置 (containerd/kubelet/kubeadm) |
| cni | object | �?| enabled: true, type: calico | CNI 插件配置 |
| csi | object | �?| enabled: false | CSI 驱动配置 |
| loadBalancer | object | �?| - | API Server 负载均衡器配�?|
| ingressLoadBalancer | object | �?| enabled: false | Ingress 负载均衡器配�?|

### B. 项目结构

```
cluster-api-provider-baremetal/
├── modules/
│   ├── cvo/                    # Cluster Version Operator
│   │   ├── go.mod
│   │   ├── api/v1beta1/        # CVO API types
│   │   │   ├── releaseimage_types.go
│   │   │   ├── clusterversion_types.go
│   │   │   ├── clusteraddon_types.go
│   │   │   ├── upgradepath_types.go
│   │   │   ├── releasecatalog_types.go
│   │   │   ├── component_types.go
│   │   │   ├── addon_types.go
│   │   │   └── upgrade_types.go
│   │   ├── cmd/manager/        # CVO entry point
│   │   ├── internal/           # CVO controllers & logic
│   │   │   ├── controllers/
│   │   │   │   ├── releaseimage_controller.go
│   │   │   │   ├── clusterversion_controller.go
│   │   │   │   └── clusteraddon_controller.go
│   │   │   ├── upgrader/
│   │   │   │   ├── graph_executor.go
│   │   │   │   ├── diff_components.go
│   │   │   │   ├── health_checker.go
│   │   │   │   └── oci_puller.go
│   │   │   └── addon/
│   │   │       ├── helm_installer.go
│   │   │       └── manifest_installer.go
│   │   └── config/             # CVO deployment configs
│   │       ├── crd/
│   │       ├── rbac/
│   │       └── manager/
│   │
│   └── capbm/                  # CAPBM Infrastructure Provider
│       ├── go.mod
│       ├── api/v1beta1/        # CAPBM API types
│       │   ├── baremetalcluster_types.go
│       │   ├── baremetalclustertemplate_types.go
│       │   ├── baremetalmachine_types.go
│       │   ├── baremetalmachinetemplate_types.go
│       │   ├── baremetalhostinventory_types.go
│       │   ├── groupversion_info.go
│       │   └── conditions.go
│       ├── cmd/manager/        # CAPBM entry point
│       ├── internal/           # Controllers, SSH, LB, etc.
│       │   ├── controllers/
│       │   │   ├── baremetalcluster_controller.go
│       │   │   ├── baremetalmachine_controller.go
│       │   │   └── baremetalhostinventory_controller.go
│       │   ├── lb/
│       │   ├── gateway/
│       │   ├── installer/
│       │   ├── cni/
│       │   ├── csi/
│       │   ├── ssh/
│       │   ├── network/
│       │   └── health/
│       └── config/             # CAPBM deployment configs
│           ├── crd/
│           ├── clusterclass/
│           ├── rbac/
│           ├── manager/
│           └── default/
│
├── release-image/              # ReleaseImage 内容目录
│   ├── release.json
│   ├── binaries/
│   ├── images/
│   ├── charts/
│   ├── manifests/
│   └── scripts/
│
├── go.work                     # Go workspace definition
├── Makefile
├── Dockerfile.cvo              # CVO Docker build
├── Dockerfile.capbm            # CAPBM Docker build
├── Dockerfile.release          # ReleaseImage Docker build
├── docs/                       # 文档
├── templates/                  # 模板
└── test/                       # 测试代码
```
cluster-api-provider-baremetal/
├── api/v1beta1/              # CRD 类型定义
�?  ├── baremetalcluster_types.go
�?  ├── baremetalclustertemplate_types.go
�?  ├── baremetalmachine_types.go
�?  ├── baremetalmachinetemplate_types.go
�?  ├── baremetalhostinventory_types.go
�?  ├── groupversion_info.go
�?  ├── conditions.go
�?  └── zz_generated.deepcopy.go
�?
├── cmd/                      # 入口文件
�?  └── main.go
�?
├── internal/
�?  ├── controllers/          # 控制器实�?
�?  �?  ├── baremetalcluster_controller.go
�?  �?  ├── baremetalmachine_controller.go
�?  �?  ├── baremetalhostinventory_controller.go
�?  �?  └── suite_test.go
�?  �?
�?  ├── lb/                   # 负载均衡�?Provider
�?  �?  ├── manager.go
�?  �?  ├── haproxy.go
�?  �?  ├── keepalived.go
�?  �?  ├── f5.go
�?  �?  └── metallb.go
�?  �?
�?  ├── gateway/              # Gateway API 组件管理
�?  �?  ├── gatewayapi.go
�?  �?  ├── envoygateway.go
�?  �?  └── metallb.go
�?  �?
�?  ├── installer/            # 组件安装模块
�?  �?  ├── installer.go
�?  �?  ├── scripts.go
�?  �?  ├── detector.go
�?  �?  └── progress.go
�?  �?
�?  ├── cni/                  # CNI 插件安装
�?  �?  └── installer.go
�?  �?
�?  ├── csi/                  # CSI 驱动安装
�?  �?  └── installer.go
�?  �?
�?  ├── ssh/                  # SSH 连接管理
�?  �?  ├── manager.go
�?  �?  ├── client.go
�?  �?  └── preflight.go
�?  �?
�?  ├── network/              # 网络配置模块
�?  �?  ├── firewall.go
�?  �?  └── selinux.go
�?  �?
�?  ├── health/               # 安装验证
�?  �?  └── verify.go
�?  �?
�?  └── upgrader/             # 升级编排模块
�?      ├── graph_executor.go
�?      ├── diff_components.go
�?      ├── health_checker.go
�?      └── oci_puller.go
�?
├── config/
�?  ├── crd/                  # CRD YAML 定义
�?  �?  ├── bases/
�?  �?  └── kustomization.yaml
�?  �?
�?  ├── clusterclass/         # ClusterClass 模板
�?  �?  ├── baremetal-clusterclass.yaml
�?  �?  ├── baremetal-cluster-template.yaml
�?  �?  ├── baremetal-machine-template-cp.yaml
�?  �?  ├── baremetal-machine-template-worker.yaml
�?  �?  ├── kubeadm-controlplane-template.yaml
�?  �?  ├── kubeadm-config-template.yaml
�?  �?  └── kustomization.yaml
�?  �?
�?  ├── rbac/                 # RBAC 配置
�?  ├── manager/              # Controller 部署
�?  └── default/              # Kustomize 入口
�?
├── templates/clusterclass/   # clusterctl 模板
�?  └── baremetal-clusterclass-v0.1.0.yaml
�?
├── test/                     # 测试代码
�?  ├── e2e/
�?  └── utils/
�?
├── docs/                     # 文档
�?  ├── design.md
�?  ├── design-ex.md
�?  ├── user-guide.md
�?  └── cluster-upgrade-cvo.md
�?
├── Makefile
├── Dockerfile
├── PROJECT
├── go.mod
└── go.sum
```

### C. 相关资源

- [Cluster API 官方文档](https://cluster-api.sigs.k8s.io/)
- [ClusterClass 文档](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/)
- [Gateway API 文档](https://gateway-api.sigs.k8s.io/)
- [Envoy Gateway 文档](https://gateway.envoyproxy.io/)
- [MetalLB 文档](https://metallb.universe.tf/)
- [kubeadm 文档](https://kubernetes.io/docs/reference/setup-tools/kubeadm/)
