# CAPBM 使用指南

## 目录

- [1. 前置条件](#1-前置条件)
- [2. 安装 CAPBM Provider](#2-安装-capbm-provider)
- [3. 部署 ClusterClass 模板](#3-部署-clusterclass-模板)
- [4. 创建裸金属集群](#4-创建裸金属集群)
- [5. 集群管理](#5-集群管理)
- [6. 多 Worker 池配置](#6-多-worker-池配置)
- [7. 升级集群](#7-升级集群)
- [8. 删除集群](#8-删除集群)
- [9. 故障排查](#9-故障排查)
- [10. 常见问题](#10-常见问题)

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

---

## 2. 安装 CAPBM Provider

### 2.1 安装 Cluster API 核心组件

```bash
# 安装 CAPI 核心、kubeadm bootstrap 和 control-plane provider
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm \
  --infrastructure baremetal
```

### 2.2 验证安装

```bash
# 检查所有组件是否正常运行
kubectl get pods -n capi-system
kubectl get pods -n capi-kubeadm-bootstrap-system
kubectl get pods -n capi-kubeadm-control-plane-system
kubectl get pods -n capbm-system

# 检查 CRD 是否已注册
kubectl get crd | grep -E "baremetal|cluster.x-k8s.io"
```

---

## 3. 部署 ClusterClass 模板

### 3.1 应用 ClusterClass 及相关模板

```bash
# 部署完整的 ClusterClass 定义和所有关联模板
kubectl apply -f config/clusterclass/
```

### 3.2 验证部署

```bash
# 检查 ClusterClass 是否已创建
kubectl get clusterclass baremetal-clusterclass-v0.1.0

# 检查所有模板资源
kubectl get baremetalclustertemplate
kubectl get baremetalmachinetemplate
kubectl get kubeadmcontrolplanetemplate
kubectl get kubeadmconfigtemplate
```

---

## 4. 创建裸金属集群

### 4.1 创建 SSH 凭据 Secret

```bash
# 方式一：使用密码认证
kubectl create secret generic baremetal-ssh-credentials \
  --from-literal=username=root \
  --from-literal=password=your-secure-password

# 方式二：使用 SSH Key 认证（推荐生产环境）
kubectl create secret generic baremetal-ssh-credentials \
  --from-literal=username=root \
  --from-file=ssh-privatekey=/path/to/ssh/private-key
```

### 4.2 使用 clusterctl 生成集群配置

```bash
# 生成集群 YAML 配置
clusterctl generate cluster my-baremetal-cluster \
  --from templates/clusterclass/baremetal-clusterclass-v0.1.0.yaml \
  --variable CLUSTER_NAME=my-baremetal-cluster \
  --variable NAMESPACE=default \
  --variable KUBERNETES_VERSION=v1.31.0 \
  --variable CONTROL_PLANE_MACHINE_COUNT=3 \
  --variable WORKER_MACHINE_COUNT=2 \
  --variable CONTROL_PLANE_ENDPOINT_HOST=lb.example.com \
  --variable CONTROL_PLANE_ENDPOINT_PORT=6443 \
  --variable SSH_CREDENTIALS_SECRET=baremetal-ssh-credentials \
  --variable SSH_USERNAME=root \
  --variable SSH_PASSWORD=your-secure-password \
  > cluster.yaml
```

### 4.3 手动编写集群配置（可选）

如果您需要更精细的控制，可以手动编写 Cluster 资源：

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
        annotations:
          description: "Control plane nodes"
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
        replicas: 2
        metadata:
          labels:
            role: worker
          annotations:
            description: "Worker nodes"
    variables:
    - name: controlPlaneEndpoint
      value:
        host: "lb.example.com"
        port: 6443
    - name: credentialsSecret
      value: "baremetal-ssh-credentials"
    - name: kubernetesVersion
      value: "v1.31.0"
    - name: podCIDR
      value: "10.244.0.0/16"
    - name: serviceCIDR
      value: "10.96.0.0/12"
---
apiVersion: v1
kind: Secret
metadata:
  name: baremetal-ssh-credentials
  namespace: default
stringData:
  username: "root"
  password: "your-secure-password"
```

### 4.4 应用集群配置

```bash
# 应用集群配置
kubectl apply -f cluster.yaml

# 查看集群创建进度
clusterctl describe cluster my-baremetal-cluster

# 或使用 kubectl 查看
kubectl get cluster my-baremetal-cluster
kubectl get baremetalcluster my-baremetal-cluster
```

### 4.5 监控集群创建过程

```bash
# 实时监控集群状态
kubectl get cluster my-baremetal-cluster --watch

# 查看基础设施状态
kubectl get baremetalcluster my-baremetal-cluster -o yaml

# 查看 Machine 状态
kubectl get machines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster

# 查看 BareMetalMachine 状态
kubectl get baremetalmachines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster
```

### 4.6 获取工作集群 kubeconfig

```bash
# 获取工作集群的 kubeconfig
clusterctl get kubeconfig my-baremetal-cluster > workload-kubeconfig

# 使用 kubeconfig 访问工作集群
kubectl --kubeconfig workload-kubeconfig get nodes
kubectl --kubeconfig workload-kubeconfig get pods -A
```

---

## 5. 集群管理

### 5.1 扩缩容

#### 扩容 Worker 节点

```bash
# 将 Worker 节点扩容到 5 个
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

# 验证扩容
kubectl get machines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster
```

#### 缩容控制面节点

```bash
# 将控制面节点缩容到 1 个（仅用于测试环境）
kubectl patch cluster my-baremetal-cluster --type='merge' -p '{
  "spec": {
    "topology": {
      "controlPlane": {
        "replicas": 1
      }
    }
  }
}'
```

### 5.2 查看集群详情

```bash
# 使用 clusterctl 查看集群拓扑
clusterctl describe cluster my-baremetal-cluster --show-conditions all

# 查看 ClusterClass 生成的资源
kubectl get all -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster
```

### 5.3 健康检查

```bash
# 查看 MachineHealthCheck 状态
kubectl get machinehealthcheck -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster

# 查看节点健康状态
kubectl --kubeconfig workload-kubeconfig get nodes
kubectl --kubeconfig workload-kubeconfig describe node <node-name>
```

---

## 6. 多 Worker 池配置

### 6.1 创建多 Worker 池集群

当您需要不同规格的 Worker 节点时，可以定义多个 MachineDeployment：

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
    - name: kubernetesVersion
      value: "v1.31.0"
```

### 6.2 使用节点选择器调度工作负载

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

## 7. 升级集群

### 7.1 升级 Kubernetes 版本

```bash
# 升级到 v1.32.0
kubectl patch cluster my-baremetal-cluster --type='merge' -p '{
  "spec": {
    "topology": {
      "version": "v1.32.0"
    }
  }
}'
```

### 7.2 升级流程说明

ClusterClass 会自动编排升级流程：

```
1. 控制面升级
   ├── etcd 集群逐个升级
   ├── API Server 逐个滚动升级
   └── 等待控制面健康

2. Worker 节点升级
   ├── 逐个滚动升级 MachineDeployment
   ├── 每个节点升级前执行预检
   └── 等待新节点 Ready 后驱逐旧节点

3. 升级完成
   └── 所有节点运行新版本
```

### 7.3 监控升级进度

```bash
# 查看升级状态
clusterctl describe cluster my-baremetal-cluster --show-conditions all

# 查看各 Machine 的版本
kubectl get machines -l cluster.x-k8s.io/cluster-name=my-baremetal-cluster \
  -o custom-columns=NAME:.metadata.name,VERSION:.spec.version,PHASE:.status.phase
```

---

## 8. 删除集群

### 8.1 删除集群

```bash
# 删除集群（会级联删除所有关联资源）
kubectl delete cluster my-baremetal-cluster

# 或使用 clusterctl
clusterctl delete cluster my-baremetal-cluster
```

### 8.2 清理凭据

```bash
# 删除 SSH 凭据 Secret
kubectl delete secret baremetal-ssh-credentials
```

### 8.3 卸载 Provider（可选）

```bash
# 卸载 CAPBM Provider
clusterctl delete --infrastructure baremetal

# 卸载所有 CAPI 组件
clusterctl delete --all
```

---

## 9. 故障排查

### 9.1 集群创建失败

```bash
# 查看集群事件
kubectl describe cluster my-baremetal-cluster

# 查看 BareMetalCluster 事件
kubectl describe baremetalcluster my-baremetal-cluster

# 查看 BareMetalMachine 事件
kubectl describe baremetalmachine <machine-name>

# 查看 Controller 日志
kubectl logs -n capbm-system -l control-plane=controller-manager --tail=100
```

### 9.2 SSH 连接问题

```bash
# 检查凭据 Secret 是否存在
kubectl get secret baremetal-ssh-credentials

# 检查 Secret 内容
kubectl get secret baremetal-ssh-credentials -o jsonpath='{.data.username}' | base64 -d
kubectl get secret baremetal-ssh-credentials -o jsonpath='{.data.password}' | base64 -d

# 测试 SSH 连接
ssh -o StrictHostKeyChecking=no root@<machine-ip> echo "SSH connection successful"
```

### 9.3 预检失败

```bash
# 查看预检条件状态
kubectl get baremetalmachine <machine-name> -o jsonpath='{.status.conditions}'

# 常见预检失败原因：
# 1. OS 不支持 - 检查 /etc/os-release
# 2. 内核版本过低 - 检查 uname -r
# 3. 磁盘空间不足 - 检查 df -h
# 4. 内存不足 - 检查 free -g
# 5. Swap 未关闭 - 检查 swapon --show
```

### 9.4 网络问题

```bash
# 检查负载均衡器是否可达
curl -k https://lb.example.com:6443/version

# 检查节点间网络连通性
kubectl --kubeconfig workload-kubeconfig exec -it <pod-name> -- ping <other-node-ip>

# 检查 DNS 解析
kubectl --kubeconfig workload-kubeconfig run -it dns-test --image=busybox:1.28 --restart=Never -- nslookup kubernetes.default
```

---

## 10. 常见问题

### Q1: 如何添加新的裸金属机器？

**A:** 确保新机器的 IP 和凭据已配置，然后增加 `replicas` 值：

```bash
kubectl patch cluster my-baremetal-cluster --type='merge' -p \
  '{"spec":{"topology":{"workers":{"machineDeployments":[{"class":"default-worker","name":"md-0","replicas":3}]}}}}'
```

### Q2: 如何更换 SSH 凭据？

**A:** 更新 Secret 并重新触发调和：

```bash
# 更新 Secret
kubectl create secret generic baremetal-ssh-credentials \
  --from-literal=username=newuser \
  --from-literal=password=newpassword \
  --dry-run=client -o yaml | kubectl apply -f -

# 触发重新调和（给 BareMetalMachine 添加 annotation）
kubectl annotate baremetalmachine <machine-name> force-reconcile=$(date +%s)
```

### Q3: 如何禁用预检？

**A:** 在 Cluster 的 variables 中设置：

```yaml
variables:
- name: preFlightChecks
  value:
    enabled: false
```

### Q4: 支持哪些操作系统？

**A:** 目前支持以下操作系统：
- Ubuntu 20.04, 22.04
- CentOS 7, 8
- Rocky Linux 8, 9
- AlmaLinux 8, 9
- Debian 10, 11, 12

### Q5: 如何自定义 Pod CIDR 和 Service CIDR？

**A:** 在 Cluster 的 variables 中设置：

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

### Q7: 如何自定义 ClusterClass？

**A:** 编辑 ClusterClass 资源并应用：

```bash
kubectl edit clusterclass baremetal-clusterclass-v0.1.0
```

修改后，现有的 Cluster 会自动根据新的 ClusterClass 进行调和。

---

## 附录

### A. ClusterClass 变量参考

| 变量名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| controlPlaneEndpoint | object | 是 | - | 控制面负载均衡地址 (host, port) |
| credentialsSecret | string | 是 | - | SSH 凭据 Secret 名称 |
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
