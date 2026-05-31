# CAPBM 使用指南

## 目录

- [1. 前置条件](#1-前置条件)
- [2. 安装 CAPBM Provider](#2-安装-capbm-provider)
- [3. 部署 ClusterClass 模板](#3-部署-clusterclass-模板)
- [4. 配置机器池 (BareMetalHostInventory)](#4-配置机器池-baremetalhostinventory)
- [5. 创建裸金属集群](#5-创建裸金属集群)
- [6. 负载均衡器配置](#6-负载均衡器配置)
- [7. K8s 核心组件配置定制](#7-k8s-核心组件配置定制)
- [8. 单节点集群安装](#8-单节点集群安装)
- [9. 集群管理](#9-集群管理)
- [10. 多 Worker 池配置](#10-多-worker-池配置)
- [11. 升级集群](#11-升级集群)
- [12. 删除集群](#12-删除集群)
- [13. 故障排查](#13-故障排查)
- [14. 常见问题](#14-常见问题)

---

## 1. 前置条件

### 1.1 管理集群

- Kubernetes v1.32+ 管理集群
- `kubectl` 已配置并连接到管理集群
- `clusterctl` v1.13+ 已安装

### 1.2 裸金属机器要求

| 项目 | 最低要求 | 推荐配置 |
|------|----------|----------|
| 操作系统 | Ubuntu 20.04+, CentOS 7+, Rocky 8+ | Ubuntu 22.04 LTS |
| CPU | 2 核 | 4 核+ |
| 内存 | 2 GB | 4 GB+ |
| 磁盘 | 20 GB 可用空间 | 50 GB+ SSD |
| 网络 | SSH 可达，外网可访问 | 千兆以太网 |
| 内核版本 | >= 3.10 | >= 5.4 |

### 1.3 网络要求

- 所有裸金属机器之间网络互通
- 管理集群可通过 SSH 访问所有裸金属机器
- 外部负载均衡器地址（用于 API Server 端点）

### 1.4 机器信息配置 (BareMetalHostInventory)

在 ClusterClass 模式下，裸金属机器的具体信息（IP、主机名、凭据）通过 **BareMetalHostInventory** 资源统一管理。这是一个机器池概念，CAPBM 会从中自动分配可用机器给集群使用。

**为什么需要机器池？**
- ClusterClass 使用 `replicas` 指定节点数量，但不指定具体机器
- 裸金属机器是预先存在的物理服务器，需要有一个地方定义它们的信息
- 机器池支持多集群共享、自动分配和释放

**机器池工作流程**：
```
1. 创建 BareMetalHostInventory 定义所有可用机器
2. 创建 Cluster 时引用机器池
3. CAPBM 自动从池中分配可用机器（根据 role 过滤）
4. 删除集群时机器自动释放回池中
```

---

## 2. 安装 CAPBM Provider

### 2.1 安装 Cluster API 核心组件

```bash
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm \
  --infrastructure baremetal
```

### 2.2 验证安装

```bash
kubectl get pods -n capi-system
kubectl get pods -n capi-kubeadm-bootstrap-system
kubectl get pods -n capi-kubeadm-control-plane-system
kubectl get pods -n capbm-system

kubectl get crd | grep -E "baremetal|cluster.x-k8s.io"
```

---

## 3. 部署 ClusterClass 模板

### 3.1 应用 ClusterClass 及相关模板

```bash
kubectl apply -f config/clusterclass/
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

## 4. 配置机器池 (BareMetalHostInventory)

### 4.1 创建集群 Namespace

推荐为每个集群创建独立的 namespace 以实现资源隔离：

```bash
# 创建集群 namespace
kubectl create namespace cluster-my-cluster
```

### 4.2 创建机器池定义

机器池定义了所有可用的裸金属机器信息，包括 IP、主机名、凭据和角色：

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: datacenter-a-hosts
  namespace: cluster-my-cluster   # 使用集群 namespace
spec:
  hosts:
  # 控制面节点
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

在集群 namespace 中为每台机器创建独立的凭据 Secret（或使用统一的凭据）：

```bash
# 方式一：每台机器独立凭据
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

### 4.3 应用机器池配置

```bash
kubectl apply -f baremetalhostinventory.yaml -n cluster-my-cluster

kubectl get baremetalhostinventory datacenter-a-hosts -n cluster-my-cluster
kubectl get baremetalhostinventory datacenter-a-hosts -n cluster-my-cluster -o yaml
```

### 4.4 查看机器池状态

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
# 北京数据中心机器池
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
# 上海数据中心机器池
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

## 5. 创建裸金属集群

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
      namespace: capbm-system     # ClusterClass 在系统 namespace
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

## 6. 负载均衡器配置

CAPBM 支持自动将 control-plane 节点注册到外部负载均衡器，并支持 Ingress 流量负载均衡。

### 6.1 API Server 负载均衡器

CAPBM 支持以下负载均衡器类型：

| 类型 | 注册方式 | 适用场景 |
|------|---------|---------|
| HAProxy | Runtime API / SSH | 中小型集群 |
| F5 BIG-IP | iControl REST API | 企业级硬件 LB |
| Keepalived | VIP 故障转移 | 无外部 LB 的裸金属 |
| MetalLB | BGP / L2 宣告 | 裸金属 Service LB |

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

#### Keepalived 配置 (无外部 LB)

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

### 6.2 Ingress 负载均衡器

配置 worker 节点的 Ingress 流量负载均衡：

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

### 6.3 验证负载均衡器状态

```bash
# 检查 API Server LB 状态
kubectl get baremetalcluster my-baremetal-cluster -o jsonpath='{.status.conditions[?(@.type=="LoadBalancerReady")]}'

# 检查 Ingress LB 状态
kubectl get baremetalcluster my-baremetal-cluster -o jsonpath='{.status.conditions[?(@.type=="IngressLoadBalancerReady")]}'

# 查看 HAProxy 后端
echo "show servers state k8s-apiserver" | socat stdio tcp:10.0.0.50:9999
```

---

## 7. K8s 核心组件配置定制

CAPBM 支持通过 ClusterClass 变量定制节点上安装的 Kubernetes 核心组件。

### 7.1 容器运行时配置

支持 containerd、CRI-O 和 Docker 三种容器运行时：

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

自定义 kubelet 参数和组件仓库：

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

支持 Calico、Cilium 和 Flannel：

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

支持 Ceph-CSI、Cinder-CSI、Local-CSI 和 NFS-CSI：

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

CAPBM 支持使用 ReleaseImage OCI 镜像作为组件源。ReleaseImage 是一个自包含的镜像，内置 HTTP 服务器，提供所有组件的下载服务。

#### 7.6.1 部署 ReleaseImage HTTP 服务器

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
├── runtime/                    # 容器运行时
│   └── containerd/
│       └── v1.7.0/
│           ├── linux-amd64/
│           └── linux-arm64/
├── kubernetes/                 # Kubernetes 核心二进制
│   └── v1.31.0/
│       ├── ubuntu/
│       │   ├── linux-amd64/
│       │   └── linux-arm64/
│       └── rhel/
│           ├── linux-amd64/
│           └── linux-arm64/
├── cni/                        # CNI 网络插件
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
├── csi/                        # CSI 存储驱动
│   ├── ceph-csi/
│   ├── local-path-provisioner/
│   └── nfs-csi/
├── gateway/                    # 网关组件
│   ├── gateway-api/
│   └── envoy-gateway/
├── metallb/                    # 负载均衡器
├── images/                     # 容器镜像包
├── index.json                  # 组件索引
└── checksums.sha256            # 校验和
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

## 8. 单节点集群安装

CAPBM 支持创建单节点 Kubernetes 集群（control-plane 和 worker 合并），适用于开发测试或边缘计算场景。

### 8.1 机器池配置

单节点集群只需一台机器，role 设置为 `control-plane`：

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

设置 `controlPlane.replicas: 1`，不配置 worker 节点：

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
    # 不配置 workers，control-plane 节点同时承担 worker 角色
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

### 8.3 允许控制面节点调度工作负载

默认情况下，control-plane 节点有污点 `node-role.kubernetes.io/control-plane:NoSchedule`。如需在单节点上运行工作负载，需移除污点：

```bash
# 获取 kubeconfig 后执行
kubectl --kubeconfig workload-kubeconfig taint nodes --all node-role.kubernetes.io/control-plane-

# 或者在集群配置中通过 KubeadmConfigTemplate 设置
```

### 8.4 注意事项

| 项目 | 说明 |
|------|------|
| 高可用 | 单节点集群无高可用能力，不适用于生产环境 |
| etcd | 单节点 etcd，数据丢失风险 |
| 升级 | 升级期间服务不可用 |
| 负载均衡器 | 可直接使用节点 IP 作为 controlPlaneEndpoint |

---

## 9. 集群管理

### 9.1 扩缩容

```bash
# 扩容 Worker 节点到 5 个
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

### 9.3 健康检查

```bash
kubectl get machinehealthcheck -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster

kubectl --kubeconfig workload-kubeconfig get nodes
kubectl --kubeconfig workload-kubeconfig describe node <node-name>
```

---

## 10. 多 Worker 池配置

### 10.1 创建多 Worker 池集群

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

### 10.2 使用节点选择器调度工作负载

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

## 11. 升级集群

### 11.1 升级 Kubernetes 版本

```bash
kubectl patch cluster my-baremetal-cluster --type='merge' -p '{
  "spec": {
    "topology": {
      "version": "v1.32.0"
    }
  }
}'
```

### 11.2 升级流程说明

```
1. 控制面升级
   - etcd 集群逐个升级
   - API Server 逐个滚动升级
   - 等待控制面健康

2. Worker 节点升级
   - 逐个滚动升级 MachineDeployment
   - 每个节点升级前执行预检
   - 等待新节点 Ready 后驱逐旧节点

3. 升级完成
   - 所有节点运行新版本
```

### 11.3 监控升级进度

```bash
clusterctl describe cluster my-baremetal-cluster --show-conditions all

kubectl get machines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster \
  -o custom-columns=NAME:.metadata.name,VERSION:.spec.version,PHASE:.status.phase
```

---

## 12. 删除集群

### 12.1 删除集群

```bash
kubectl delete cluster my-baremetal-cluster

clusterctl delete cluster my-baremetal-cluster
```

### 12.2 清理凭据

```bash
kubectl delete secret baremetal-ssh-credentials
```

### 12.3 卸载 Provider

```bash
clusterctl delete --infrastructure baremetal

clusterctl delete --all
```

---

## 13. 故障排查

### 13.1 集群创建失败

```bash
kubectl describe cluster my-baremetal-cluster

kubectl describe baremetalcluster my-baremetal-cluster

kubectl describe baremetalmachine <machine-name>

kubectl logs -n capbm-system -l control-plane=controller-manager --tail=100
```

### 13.2 SSH 连接问题

```bash
kubectl get secret baremetal-ssh-credentials

kubectl get secret baremetal-ssh-credentials -o jsonpath='{.data.username}' | base64 -d

ssh -o StrictHostKeyChecking=no root@<machine-ip> echo "SSH connection successful"
```

### 13.3 预检失败

```bash
kubectl get baremetalmachine <machine-name> -o jsonpath='{.status.conditions}'
```

常见预检失败原因：
1. OS 不支持 - 检查 /etc/os-release
2. 内核版本过低 - 检查 uname -r
3. 磁盘空间不足 - 检查 df -h
4. 内存不足 - 检查 free -g
5. Swap 未关闭 - 检查 swapon --show

### 13.4 网络问题

```bash
curl -k https://lb.example.com:6443/version

kubectl --kubeconfig workload-kubeconfig exec -it <pod-name> -- ping <other-node-ip>

kubectl --kubeconfig workload-kubeconfig run -it dns-test --image=busybox:1.28 --restart=Never -- nslookup kubernetes.default
```

---

## 14. 常见问题

### Q1: 如何添加新的裸金属机器？

更新 BareMetalHostInventory 添加新机器，然后增加 replicas：

```bash
kubectl edit baremetalhostinventory datacenter-a-hosts

kubectl patch cluster my-baremetal-cluster --type='merge' -p \
  '{"spec":{"topology":{"workers":{"machineDeployments":[{"class":"default-worker","name":"md-0","replicas":3}]}}}}'
```

### Q2: 如何更换 SSH 凭据？

更新 Secret 并重新触发调和：

```bash
kubectl create secret generic baremetal-ssh-credentials \
  --from-literal=username=newuser \
  --from-literal=password=newpassword \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl annotate baremetalmachine <machine-name> force-reconcile=$(date +%s)
```

### Q3: 如何禁用预检？

```yaml
variables:
- name: preFlightChecks
  value:
    enabled: false
```

### Q4: 支持哪些操作系统？

- Ubuntu 20.04, 22.04
- CentOS 7, 8
- Rocky Linux 8, 9
- AlmaLinux 8, 9
- Debian 10, 11, 12

### Q5: 如何自定义 Pod CIDR 和 Service CIDR？

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

### A. ClusterClass 变量参考

| 变量名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| controlPlaneEndpoint | object | 是 | - | 控制面负载均衡地址 (host, port) |
| credentialsSecret | string | 是 | - | SSH 凭据 Secret 名称 |
| hostInventoryRef | string | 是 | - | BareMetalHostInventory 资源名称 |
| kubernetesVersion | string | 是 | - | Kubernetes 版本 (格式: vX.Y.Z) |
| podCIDR | string | 否 | 10.244.0.0/16 | Pod 网络 CIDR |
| serviceCIDR | string | 否 | 10.96.0.0/12 | Service 网络 CIDR |
| preFlightChecks | object | 否 | enabled: true | 预检配置 |
| componentInstall | object | 否 | enabled: true | 组件安装配置 (containerd/kubelet/kubeadm) |
| cni | object | 否 | enabled: true, type: calico | CNI 插件配置 |
| csi | object | 否 | enabled: false | CSI 驱动配置 |
| loadBalancer | object | 否 | - | API Server 负载均衡器配置 |
| ingressLoadBalancer | object | 否 | enabled: false | Ingress 负载均衡器配置 |

### B. 项目结构

```
cluster-api-provider-baremetal/
├── api/v1beta1/              # CRD 类型定义
│   ├── baremetalcluster_types.go
│   ├── baremetalclustertemplate_types.go
│   ├── baremetalmachine_types.go
│   ├── baremetalmachinetemplate_types.go
│   ├── baremetalhostinventory_types.go
│   ├── groupversion_info.go
│   ├── conditions.go
│   └── zz_generated.deepcopy.go
│
├── cmd/                      # 入口文件
│   └── main.go
│
├── internal/
│   ├── controllers/          # 控制器实现
│   │   ├── baremetalcluster_controller.go
│   │   ├── baremetalmachine_controller.go
│   │   ├── baremetalhostinventory_controller.go
│   │   └── suite_test.go
│   │
│   ├── lb/                   # 负载均衡器 Provider
│   │   ├── manager.go
│   │   ├── haproxy.go
│   │   ├── keepalived.go
│   │   ├── f5.go
│   │   └── metallb.go
│   │
│   ├── gateway/              # Gateway API 组件管理
│   │   ├── gatewayapi.go
│   │   ├── envoygateway.go
│   │   └── metallb.go
│   │
│   ├── installer/            # 组件安装模块
│   │   ├── installer.go
│   │   ├── scripts.go
│   │   ├── detector.go
│   │   └── progress.go
│   │
│   ├── cni/                  # CNI 插件安装
│   │   └── installer.go
│   │
│   ├── csi/                  # CSI 驱动安装
│   │   └── installer.go
│   │
│   ├── ssh/                  # SSH 连接管理
│   │   ├── manager.go
│   │   ├── client.go
│   │   └── preflight.go
│   │
│   ├── network/              # 网络配置模块
│   │   ├── firewall.go
│   │   └── selinux.go
│   │
│   ├── health/               # 安装验证
│   │   └── verify.go
│   │
│   └── upgrader/             # 升级编排模块
│       ├── graph_executor.go
│       ├── diff_components.go
│       ├── health_checker.go
│       └── oci_puller.go
│
├── config/
│   ├── crd/                  # CRD YAML 定义
│   │   ├── bases/
│   │   └── kustomization.yaml
│   │
│   ├── clusterclass/         # ClusterClass 模板
│   │   ├── baremetal-clusterclass.yaml
│   │   ├── baremetal-cluster-template.yaml
│   │   ├── baremetal-machine-template-cp.yaml
│   │   ├── baremetal-machine-template-worker.yaml
│   │   ├── kubeadm-controlplane-template.yaml
│   │   ├── kubeadm-config-template.yaml
│   │   └── kustomization.yaml
│   │
│   ├── rbac/                 # RBAC 配置
│   ├── manager/              # Controller 部署
│   └── default/              # Kustomize 入口
│
├── templates/clusterclass/   # clusterctl 模板
│   └── baremetal-clusterclass-v0.1.0.yaml
│
├── test/                     # 测试代码
│   ├── e2e/
│   └── utils/
│
├── docs/                     # 文档
│   ├── design.md
│   ├── design-ex.md
│   ├── user-guide.md
│   └── cluster-upgrade-cvo.md
│
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
