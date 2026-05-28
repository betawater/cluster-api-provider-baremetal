# CAPBM 使用指南

## 目录

- [1. 前置条件](#1-前置条件)
- [2. 安装 CAPBM Provider](#2-安装-capbm-provider)
- [3. 部署 ClusterClass 模板](#3-部署-clusterclass-模板)
- [4. 配置机器池 (BareMetalHostInventory)](#4-配置机器池-baremetalhostinventory)
- [5. 创建裸金属集群](#5-创建裸金属集群)
- [6. 集群管理](#6-集群管理)
- [7. 多 Worker 池配置](#7-多-worker-池配置)
- [8. 升级集群](#8-升级集群)
- [9. 删除集群](#9-删除集群)
- [10. 故障排查](#10-故障排查)
- [11. 常见问题](#11-常见问题)

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

### 4.1 创建机器池定义

机器池定义了所有可用的裸金属机器信息，包括 IP、主机名、凭据和角色：

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: datacenter-a-hosts
  namespace: default
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

为每台机器创建独立的凭据 Secret（或使用统一的凭据）：

```bash
# 方式一：每台机器独立凭据
kubectl create secret generic node-01-credentials \
  --from-literal=username=root \
  --from-literal=password=node01-password

kubectl create secret generic node-02-credentials \
  --from-literal=username=root \
  --from-literal=password=node02-password

# 方式二：使用统一凭据（所有机器相同）
kubectl create secret generic baremetal-unified-credentials \
  --from-literal=username=root \
  --from-literal=password=unified-password
```

### 4.3 应用机器池配置

```bash
kubectl apply -f baremetalhostinventory.yaml

kubectl get baremetalhostinventory datacenter-a-hosts
kubectl get baremetalhostinventory datacenter-a-hosts -o yaml
```

### 4.4 查看机器池状态

```bash
kubectl get baremetalhostinventory datacenter-a-hosts -o jsonpath='{.status}'
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

您可以创建多个机器池来管理不同位置或类型的机器：

```yaml
# 北京数据中心机器池
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: beijing-datacenter-hosts
  namespace: default
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
  namespace: default
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
  --variable NAMESPACE=default \
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
  namespace: default
spec:
  topology:
    classRef:
      name: baremetal-clusterclass-v0.1.0
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
kubectl apply -f cluster.yaml

clusterctl describe cluster my-baremetal-cluster

kubectl get cluster my-baremetal-cluster
kubectl get baremetalcluster my-baremetal-cluster
```

### 5.4 监控集群创建过程

```bash
kubectl get cluster my-baremetal-cluster --watch

kubectl get baremetalcluster my-baremetal-cluster -o yaml

kubectl get machines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster

kubectl get baremetalmachines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster
```

### 5.5 获取工作集群 kubeconfig

```bash
clusterctl get kubeconfig my-baremetal-cluster > workload-kubeconfig

kubectl --kubeconfig workload-kubeconfig get nodes
kubectl --kubeconfig workload-kubeconfig get pods -A
```

---

## 6. 集群管理

### 6.1 扩缩容

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

### 6.2 查看集群详情

```bash
clusterctl describe cluster my-baremetal-cluster --show-conditions all

kubectl get all -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster
```

### 6.3 健康检查

```bash
kubectl get machinehealthcheck -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster

kubectl --kubeconfig workload-kubeconfig get nodes
kubectl --kubeconfig workload-kubeconfig describe node <node-name>
```

---

## 7. 多 Worker 池配置

### 7.1 创建多 Worker 池集群

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

### 7.2 使用节点选择器调度工作负载

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

## 8. 升级集群

### 8.1 升级 Kubernetes 版本

```bash
kubectl patch cluster my-baremetal-cluster --type='merge' -p '{
  "spec": {
    "topology": {
      "version": "v1.32.0"
    }
  }
}'
```

### 8.2 升级流程说明

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

### 8.3 监控升级进度

```bash
clusterctl describe cluster my-baremetal-cluster --show-conditions all

kubectl get machines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster \
  -o custom-columns=NAME:.metadata.name,VERSION:.spec.version,PHASE:.status.phase
```

---

## 9. 删除集群

### 9.1 删除集群

```bash
kubectl delete cluster my-baremetal-cluster

clusterctl delete cluster my-baremetal-cluster
```

### 9.2 清理凭据

```bash
kubectl delete secret baremetal-ssh-credentials
```

### 9.3 卸载 Provider

```bash
clusterctl delete --infrastructure baremetal

clusterctl delete --all
```

---

## 10. 故障排查

### 10.1 集群创建失败

```bash
kubectl describe cluster my-baremetal-cluster

kubectl describe baremetalcluster my-baremetal-cluster

kubectl describe baremetalmachine <machine-name>

kubectl logs -n capbm-system -l control-plane=controller-manager --tail=100
```

### 10.2 SSH 连接问题

```bash
kubectl get secret baremetal-ssh-credentials

kubectl get secret baremetal-ssh-credentials -o jsonpath='{.data.username}' | base64 -d

ssh -o StrictHostKeyChecking=no root@<machine-ip> echo "SSH connection successful"
```

### 10.3 预检失败

```bash
kubectl get baremetalmachine <machine-name> -o jsonpath='{.status.conditions}'
```

常见预检失败原因：
1. OS 不支持 - 检查 /etc/os-release
2. 内核版本过低 - 检查 uname -r
3. 磁盘空间不足 - 检查 df -h
4. 内存不足 - 检查 free -g
5. Swap 未关闭 - 检查 swapon --show

### 10.4 网络问题

```bash
curl -k https://lb.example.com:6443/version

kubectl --kubeconfig workload-kubeconfig exec -it <pod-name> -- ping <other-node-ip>

kubectl --kubeconfig workload-kubeconfig run -it dns-test --image=busybox:1.28 --restart=Never -- nslookup kubernetes.default
```

---

## 11. 常见问题

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

### B. 项目结构

```
cluster-api-provider-baremetal/
├── api/v1beta1/          # CRD 类型定义
├── cmd/                  # 入口文件
├── config/
│   ├── crd/              # CRD YAML 定义
│   ├── clusterclass/     # ClusterClass 模板
│   ├── rbac/             # RBAC 配置
│   └── manager/          # Controller 部署
├── internal/
│   ├── controllers/      # 控制器实现
│   └── ssh/              # SSH 连接管理
├── templates/clusterclass/ # clusterctl 模板
└── test/                 # 测试代码
```

### C. 相关资源

- [Cluster API 官方文档](https://cluster-api.sigs.k8s.io/)
- [ClusterClass 文档](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/)
- [kubeadm 文档](https://kubernetes.io/docs/reference/setup-tools/kubeadm/)
