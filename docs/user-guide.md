# CAPBM 使用指南

## 目录

- [1. 前置条件](#1-前置条件)
- [2. 安装 clusterctl](#2-安装-clusterctl)
- [3. 安装 CAPBM Provider](#3-安装-capbm-provider)
- [4. 部署 ClusterClass 模板](#4-部署-clusterclass-模板)
- [5. 创建 ReleaseImage](#5-创建-releaseimage)
- [6. 配置机器池 (BareMetalHostInventory)](#6-配置机器池-baremetalhostinventory)
- [7. 创建裸金属集群](#7-创建裸金属集群)
- [8. 负载均衡器配置](#8-负载均衡器配置)
- [9. K8s 核心组件配置定制](#9-k8s-核心组件配置定制)
- [10. 单节点集群安装](#10-单节点集群安装)
- [11. 集群管理](#11-集群管理)
- [12. 多 Worker 池配置](#12-多-worker-池配置)
- [13. 升级集群](#13-升级集群)
- [14. 删除集群](#14-删除集群)
- [15. 故障排查](#15-故障排查)
- [16. 常见问题](#16-常见问题)

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

**机器池工作流程：**
```
1. 创建 BareMetalHostInventory 定义所有可用机器
2. 创建 Cluster 时引用机器池
3. CAPBM 自动从池中分配可用机器（根据 role 过滤）
4. 删除集群时机器自动释放回池中
```

---

## 2. 安装 clusterctl

`clusterctl` 是 Cluster API 的命令行工具，用于初始化管理集群和 Provider。

### 2.1 方式一：使用官方安装脚本（推荐）

```bash
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/clusterctl-linux-amd64 -o clusterctl
chmod +x clusterctl
sudo mv clusterctl /usr/local/bin/
```

### 2.2 方式二：使用 Homebrew（macOS）

```bash
brew install clusterctl
```

### 2.3 方式三：手动下载

```bash
# 从 GitHub Releases 下载
wget https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/clusterctl-linux-amd64
chmod +x clusterctl-linux-amd64
sudo mv clusterctl-linux-amd64 /usr/local/bin/clusterctl
```

### 2.4 验证安装

```bash
clusterctl version
```

预期输出：
```
clusterctl version: &version.Info{Major:"1", Minor:"13", GitVersion:"v1.13.0", ...}
```

---

## 3. 安装 CAPBM Provider

### 3.1 安装 Cluster API 核心组件

```bash
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm
```

### 3.2 安装 CAPBM Provider

```bash
# 使用 kustomize 安装
kubectl apply -k modules/capbm/config/default/
```

### 3.3 安装 CVO (Cluster Version Operator)

```bash
# 安装 CVO
kubectl apply -k modules/cvo/config/default/
```

### 3.4 验证安装

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

### 3.5 不使用 clusterctl 安装（高级/离线环境）

对于离线环境或需要完全控制的场景，可以手动安装所有组件。

#### 3.5.1 安装 CAPI 核心组件

```bash
# 下载 CAPI 核心 CRDs 和 Controller
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/cluster-api-components.yaml

# 下载 Kubeadm Bootstrap Provider
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/bootstrap-kubeadm-components.yaml

# 下载 Kubeadm Control Plane Provider
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/control-plane-kubeadm-components.yaml
```

#### 3.5.2 验证 CAPI 安装

```bash
kubectl get pods -n capi-system
kubectl get pods -n capi-kubeadm-bootstrap-system
kubectl get pods -n capi-kubeadm-control-plane-system
```

#### 3.5.3 离线环境安装

对于完全离线的环境：

```bash
# 1. 在有网络的环境下载所有 manifests
mkdir -p capi-offline
cd capi-offline

# 下载 CAPI 核心组件
curl -L -o cluster-api-components.yaml \
  https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/cluster-api-components.yaml

# 下载 Kubeadm Bootstrap Provider
curl -L -o bootstrap-kubeadm-components.yaml \
  https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/bootstrap-kubeadm-components.yaml

# 下载 Kubeadm Control Plane Provider
curl -L -o control-plane-kubeadm-components.yaml \
  https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/control-plane-kubeadm-components.yaml

# 2. 传输到离线环境（使用 U 盘、scp 等）
scp -r capi-offline user@offline-machine:/tmp/

# 3. 在离线环境按顺序应用
ssh user@offline-machine
cd /tmp/capi-offline
kubectl apply -f cluster-api-components.yaml
kubectl apply -f bootstrap-kubeadm-components.yaml
kubectl apply -f control-plane-kubeadm-components.yaml
```

---

## 4. 部署 ClusterClass 模板

### 3.1 应用 ClusterClass 及相关模板

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

ReleaseImage 定义了完整的 Kubernetes 发行版本，包括组件版本、Addon 定义和升级图。

### 4.1 创建 ReleaseImage

```bash
# 应用 ReleaseImage
kubectl apply -f release-image/release.json
```

### 4.2 验证 ReleaseImage

```bash
# 查看 ReleaseImage 状态
kubectl get releaseimage v1.31.1 -o yaml

# 查看状态字段
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
  
  # 二进制组件
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
  
  # 升级图
  upgradeGraph:
    - name: phase-1-runtime
      order: 1
      blocking: true
      components: [containerd]
    # ...
```

---

## 5. 配置机器池 (BareMetalHostInventory)

### 5.1 创建集群 Namespace

推荐为每个集群创建独立的 namespace 以实现资源隔离：

```bash
# 创建集群 namespace
kubectl create namespace cluster-my-cluster
```

### 5.2 创建机器池定义

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
      rack: "rack-1"
      zone: "zone-a"
  - name: node-05
    hostName: "node-05"
    ipAddress: "192.168.1.105"
    sshPort: 22
    credentialsRef:
      name: node-05-credentials
    role: "worker"
    labels:
      rack: "rack-2"
      zone: "zone-b"
```

### 5.3 创建节点凭据

为每台机器创建 SSH 凭据 Secret：

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: node-01-credentials
  namespace: cluster-my-cluster
type: Opaque
stringData:
  username: "root"
  password: "your-password"
  # 或使用 SSH 私钥
  # privateKey: |
  #   -----BEGIN RSA PRIVATE KEY-----
  #   ...
  #   -----END RSA PRIVATE KEY-----
```

```bash
# 批量创建凭据
for i in 01 02 03 04 05; do
  kubectl create secret generic node-${i}-credentials \
    --from-literal=username=root \
    --from-literal=password=password123 \
    -n cluster-my-cluster
done
```

### 5.4 应用机器池

```bash
kubectl apply -f baremetalhostinventory.yaml

# 验证机器池
kubectl get baremetalhostinventory datacenter-a-hosts -n cluster-my-cluster
```

---

## 6. 创建裸金属集群

### 6.1 创建 Cluster 资源

使用 ClusterClass 拓扑模式创建集群：

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
  namespace: cluster-my-cluster
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

```bash
kubectl apply -f cluster.yaml
```

### 6.2 监控集群创建

```bash
# 查看集群状态
kubectl get cluster my-cluster -n cluster-my-cluster

# 查看详细信息
clusterctl describe cluster my-cluster -n cluster-my-cluster

# 查看 Machine 状态
kubectl get machines -n cluster-my-cluster

# 查看 BareMetalMachine 状态
kubectl get baremetalmachine -n cluster-my-cluster
```

### 6.3 获取 workload 集群 kubeconfig

```bash
clusterctl get kubeconfig my-cluster -n cluster-my-cluster > workload-kubeconfig

# 测试连接
kubectl --kubeconfig workload-kubeconfig get nodes
```

---

## 7. 负载均衡器配置

### 7.1 支持的负载均衡器类型

CAPBM 支持多种负载均衡器类型：

| 类型 | 说明 | 适用场景 |
|------|------|---------|
| HAProxy | 基于软件的负载均衡 | 通用场景 |
| Keepalived | VRRP 协议高可用 | 简单高可用 |
| F5 | F5 BIG-IP 硬件负载均衡 | 企业级 |
| MetalLB | Kubernetes 原生负载均衡 | 云原生环境 |

### 7.2 配置 API Server 负载均衡

在 Cluster 变量中配置：

```yaml
variables:
- name: loadBalancer
  value:
    enabled: true
    type: "haproxy"
    haproxy:
      frontendPort: 6443
      backendPort: 6443
      healthCheck:
        enabled: true
        interval: 5s
```

### 7.3 配置 Ingress 负载均衡

```yaml
variables:
- name: ingressLoadBalancer
  value:
    enabled: true
    type: "metallb"
    metallb:
      ipAddressPool:
        - 192.168.1.200-192.168.1.250
```

---

## 8. K8s 核心组件配置定制

### 8.1 自定义 Pod CIDR 和 Service CIDR

```yaml
variables:
- name: podCIDR
  value: "10.244.0.0/16"
- name: serviceCIDR
  value: "10.96.0.0/12"
```

### 8.2 自定义 kubeadm 配置

```yaml
variables:
- name: kubeadmConfig
  value:
    apiServer:
      extraArgs:
        authorization-mode: "Node,RBAC"
        enable-admission-plugins: "NodeRestriction"
    controllerManager:
      extraArgs:
        node-cidr-mask-size: "24"
    scheduler:
      extraArgs:
        bind-address: "0.0.0.0"
```

### 8.3 自定义 kubelet 配置

```yaml
variables:
- name: kubeletConfig
  value:
    maxPods: 250
    kubeReserved:
      cpu: "100m"
      memory: "256Mi"
    systemReserved:
      cpu: "100m"
      memory: "256Mi"
```

---

## 9. 单节点集群安装

CAPBM 支持创建单节点 Kubernetes 集群（control-plane 和 worker 合并），适用于开发测试或边缘计算场景。

### 9.1 机器池配置

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: single-node-hosts
  namespace: cluster-single-node
spec:
  hosts:
  - name: single-node-01
    hostName: "single-node-01"
    ipAddress: "192.168.1.101"
    sshPort: 22
    credentialsRef:
      name: single-node-credentials
    role: "control-plane"
    labels:
      type: "single-node"
```

### 9.2 创建单节点集群

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: single-node-cluster
  namespace: cluster-single-node
spec:
  topology:
    classRef:
      name: baremetal-clusterclass-v0.1.0
    version: v1.31.0
    controlPlane:
      replicas: 1
    variables:
    - name: controlPlaneEndpoint
      value:
        host: "192.168.1.101"
        port: 6443
    - name: credentialsSecret
      value: "single-node-credentials"
    - name: hostInventoryRef
      value: "single-node-hosts"
    - name: kubernetesVersion
      value: "v1.31.0"
    - name: singleNode
      value:
        enabled: true
```

### 9.3 单节点注意事项

- 不支持高可用
- etcd 使用单节点模式
- 不适用于生产环境

---

## 10. 集群管理

### 10.1 扩缩容 Worker 节点

```bash
# 扩展 Worker 节点
kubectl patch cluster my-cluster -n cluster-my-cluster --type='merge' -p \
  '{"spec":{"topology":{"workers":{"machineDeployments":[{"class":"default-worker","name":"md-0","replicas":5}]}}}}'

# 缩减 Worker 节点
kubectl patch cluster my-cluster -n cluster-my-cluster --type='merge' -p \
  '{"spec":{"topology":{"workers":{"machineDeployments":[{"class":"default-worker","name":"md-0","replicas":1}]}}}}'
```

### 10.2 查看集群详情

```bash
clusterctl describe cluster my-baremetal-cluster --show-conditions all

kubectl get all -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster
```

### 10.3 健康检查

```bash
kubectl get machinehealthcheck -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster

kubectl --kubeconfig workload-kubeconfig get nodes
kubectl --kubeconfig workload-kubeconfig describe node <node-name>
```

---

## 11. 多 Worker 池配置

### 11.1 创建多 Worker 池集群

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

### 11.2 使用节点选择器调度工作负载

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

常见预检失败原因：
1. OS 不支持 - 检查 /etc/os-release
2. 内核版本过低 - 检查 uname -r
3. 磁盘空间不足 - 检查 df -h
4. 内存不足 - 检查 free -g
5. Swap 未关闭 - 检查 swapon --show

### 14.4 网络问题

```bash
curl -k https://lb.example.com:6443/version

kubectl --kubeconfig workload-kubeconfig exec -it <pod-name> -- ping <other-node-ip>

kubectl --kubeconfig workload-kubeconfig run -it dns-test --image=busybox:1.28 --restart=Never -- nslookup kubernetes.default
```

---

## 15. 常见问题

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

### C. 相关资源

- [Cluster API 官方文档](https://cluster-api.sigs.k8s.io/)
- [ClusterClass 文档](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/)
- [Gateway API 文档](https://gateway-api.sigs.k8s.io/)
- [Envoy Gateway 文档](https://gateway.envoyproxy.io/)
- [MetalLB 文档](https://metallb.universe.tf/)
- [kubeadm 文档](https://kubernetes.io/docs/reference/setup-tools/kubeadm/)