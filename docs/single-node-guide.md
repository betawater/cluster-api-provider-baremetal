# CAPBM 单节点集群完整安装指南

本指南详细说明如何使用 CAPBM (Cluster API Provider Bare Metal) 安装单节点 Kubernetes 集群，适用于开发测试或边缘计算场景。

## 一、环境准备

### 1.1 架构说明

```
┌─────────────────────────────────────────────────────────────┐
│ 管理集群 (Management Cluster)                                │
│ - Kubernetes v1.32+                                         │
│ - CAPBM Provider 已安装                                      │
│ - kubectl 已配置                                            │
└───────────────────┬─────────────────────────────────────────┘
                    │ SSH 连接
                    ▼
┌─────────────────────────────────────────────────────────────┐
│ 目标裸金属机器 (Target Bare Metal Machine)                   │
│ - 单节点 (control-plane + worker 合并)                       │
│ - Ubuntu 22.04 LTS / CentOS 7+ / Rocky 8+                   │
│ - SSH 可达                                                  │
│ - 已安装基础操作系统                                         │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 管理集群要求

| 项目 | 要求 |
|------|------|
| Kubernetes 版本 | v1.32+ |
| kubectl | 已配置并连接到管理集群 |
| clusterctl | v1.13+ 已安装 |
| 网络 | 可 SSH 访问目标裸金属机器 |

### 1.3 目标机器要求

| 项目 | 最低要求 | 推荐配置 |
|------|----------|----------|
| 操作系统 | Ubuntu 20.04+, CentOS 7+, Rocky 8+ | Ubuntu 22.04 LTS |
| CPU | 2 核 | 4 核+ |
| 内存 | 4 GB | 8 GB+ |
| 磁盘 | 40 GB 可用空间 | 100 GB+ SSD |
| 网络 | SSH 可达，IP 固定 | 千兆以太网 |
| 内核版本 | >= 3.10 | >= 5.4 |
| Swap | 已关闭 | 已关闭 |

### 1.4 目标机器预检

在目标机器上执行以下检查：

```bash
# 1. 检查操作系统
cat /etc/os-release

# 2. 检查 CPU
nproc

# 3. 检查内存
free -h

# 4. 检查磁盘空间
df -h /

# 5. 检查内核版本
uname -r

# 6. 检查 Swap 状态 (必须关闭)
swapon --show

# 7. 检查 SSH 服务
systemctl status sshd

# 8. 检查网络连通性
ping -c 3 <管理集群 IP>
```

如果 Swap 未关闭，执行：

```bash
sudo swapoff -a
sudo sed -i '/ swap / s/^/#/' /etc/fstab
```

### 1.5 安装 clusterctl

`clusterctl` 是 Cluster API 的命令行工具，用于初始化管理集群和 Provider。

#### 方式一：使用官方安装脚本（推荐）

```bash
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/clusterctl-linux-amd64 -o clusterctl
chmod +x clusterctl
sudo mv clusterctl /usr/local/bin/
```

#### 方式二：使用 Homebrew（macOS）

```bash
brew install clusterctl
```

#### 方式三：手动下载

```bash
# 从 GitHub Releases 下载
wget https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/clusterctl-linux-amd64
chmod +x clusterctl-linux-amd64
sudo mv clusterctl-linux-amd64 /usr/local/bin/clusterctl
```

#### 验证安装

```bash
clusterctl version
```

预期输出：
```
clusterctl version: &version.Info{Major:"1", Minor:"13", GitVersion:"v1.13.0", ...}
```

---

## 二、安装 CAPBM Provider

### 2.1 使用 clusterctl 安装（推荐）

在管理集群上执行：

```bash
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm \
  --infrastructure baremetal
```

### 2.2 验证安装

```bash
# 检查 Pod 状态
kubectl get pods -n capi-system
kubectl get pods -n capi-kubeadm-bootstrap-system
kubectl get pods -n capi-kubeadm-control-plane-system
kubectl get pods -n capbm-system

# 检查 CRD
kubectl get crd | grep -E "baremetal|cluster.x-k8s.io"
```

预期输出：

```
NAME                                                  READY   STATUS
capbm-controller-manager-xxxxx-xxxxx                  1/1     Running
cvo-controller-manager-xxxxx-xxxxx                    1/1     Running
capi-controller-manager-xxxxx-xxxxx                   1/1     Running
capi-kubeadm-bootstrap-controller-xxxxx-xxxxx         1/1     Running
capi-kubeadm-control-plane-controller-xxxxx-xxxxx     1/1     Running
```

### 2.3 不使用 clusterctl 安装（高级/离线环境）

对于离线环境或需要完全控制的场景，可以手动安装所有组件。

#### 2.3.1 安装 CAPI 核心组件

```bash
# 下载 CAPI 核心 CRDs 和 Controller
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/cluster-api-components.yaml

# 下载 Kubeadm Bootstrap Provider
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/bootstrap-kubeadm-components.yaml

# 下载 Kubeadm Control Plane Provider
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.13.0/control-plane-kubeadm-components.yaml
```

#### 2.3.2 安装 CAPBM Provider

```bash
kubectl apply -k modules/capbm/config/default/
```

#### 2.3.3 安装 CVO (Cluster Version Operator)

```bash
kubectl apply -k modules/cvo/config/default/
```

#### 2.3.4 离线环境安装

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
kubectl apply -k /path/to/capbm/config/default/
kubectl apply -k /path/to/cvo/config/default/
```

---

## 三、部署 ClusterClass 模板

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

## 四、配置单节点机器池

### 4.1 创建机器凭据 Secret

```bash
kubectl create secret generic single-node-credentials \
  --from-literal=username=root \
  --from-literal=password=<your-password> \
  -n default
```

> **注意**: 如果使用 SSH 密钥认证，使用以下方式：
> ```bash
> kubectl create secret generic single-node-credentials \
>   --from-literal=username=root \
>   --from-file=ssh-privatekey=~/.ssh/id_rsa \
>   -n cluster-single-node
> ```

### 4.2 创建单节点机器池

创建 `single-node-inventory.yaml`:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalHostInventory
metadata:
  name: single-node-hosts
  namespace: cluster-single-node   # 使用集群 namespace
spec:
  hosts:
  - name: single-node-01
    hostName: "single-node-01"
    ipAddress: "192.168.1.101"    # 替换为实际 IP
    sshPort: 22
    credentialsRef:
      name: single-node-credentials
    role: "control-plane"
    labels:
      rack: "rack-1"
      zone: "zone-a"
```

应用配置：

```bash
kubectl apply -f single-node-inventory.yaml -n cluster-single-node

# 验证
kubectl get baremetalhostinventory single-node-hosts -n cluster-single-node -o yaml
```

---

## 五、创建单节点集群

### 5.1 创建集群配置

创建 `single-node-cluster.yaml`:

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: single-node-cluster
  namespace: cluster-single-node   # 使用集群 namespace
spec:
  topology:
    classRef:
      name: baremetal-clusterclass-v0.1.0
      namespace: capbm-system     # ClusterClass 在系统 namespace
    version: v1.31.0
    controlPlane:
      replicas: 1              # 单节点
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
      value: "single-node-credentials"
    - name: hostInventoryRef
      value: "single-node-hosts"
    - name: kubernetesVersion
      value: "v1.31.0"
    - name: podCIDR
      value: "10.244.0.0/16"
    - name: serviceCIDR
      value: "10.96.0.0/12"
    # 可选：配置 CNI 插件
    - name: cni
      value:
        enabled: true
        type: "calico"
        version: "3.27.0"
```

### 5.2 应用集群配置

```bash
kubectl apply -f single-node-cluster.yaml -n cluster-single-node
```

### 5.3 监控集群创建过程

```bash
# 查看集群状态
kubectl get cluster single-node-cluster -n cluster-single-node --watch

# 查看详细信息
clusterctl describe cluster single-node-cluster -n cluster-single-node --show-conditions all

# 查看 BareMetalCluster 状态
kubectl get baremetalcluster single-node-cluster -n cluster-single-node -o yaml

# 查看 Machine 状态
kubectl get machines -l cluster.x-k8s.io/cluster-name=single-node-cluster -n cluster-single-node

# 查看 BareMetalMachine 状态
kubectl get baremetalmachines -l cluster.x-k8s.io/cluster-name=single-node-cluster -n cluster-single-node
```

### 5.4 集群创建流程

```
1. ClusterTopology Controller 解析 ClusterClass
   ↓
2. 创建 BareMetalCluster, KubeadmControlPlane, BareMetalMachineTemplate
   ↓
3. BareMetalMachine Controller 从机器池分配主机
   ↓
4. SSH 连接到目标机器
   ↓
5. 执行预检 (OS/网络/内核/Swap)
   ↓
6. 安装 containerd + Kubernetes 组件
   ↓
7. kubeadm init 初始化控制面
   ↓
8. 安装 CNI 插件 (Calico/Cilium)
   ↓
9. 集群 Ready
```

### 5.5 验证集群创建成功

```bash
# 检查集群状态
kubectl get cluster single-node-cluster -n cluster-single-node
# 输出: single-node-cluster   Provisioned   true    v1.31.0

# 检查节点状态
kubectl get baremetalcluster single-node-cluster -n cluster-single-node -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
# 输出: True
```

---

## 六、获取工作集群 kubeconfig

### 6.1 获取 kubeconfig

```bash
clusterctl get kubeconfig single-node-cluster -n cluster-single-node > workload-kubeconfig
```

### 6.2 验证连接

```bash
kubectl --kubeconfig workload-kubeconfig get nodes
# 输出:
# NAME              STATUS   ROLES           AGE   VERSION
# single-node-01    Ready    control-plane   5m    v1.31.0

kubectl --kubeconfig workload-kubeconfig get pods -A
# 输出:
# NAMESPACE     NAME                                    READY   STATUS
# kube-system   calico-node-xxxxx                       1/1     Running
# kube-system   calico-kube-controllers-xxxxx           1/1     Running
# kube-system   coredns-xxxxx-xxxxx                     1/1     Running
# kube-system   etcd-single-node-01                     1/1     Running
# kube-system   kube-apiserver-single-node-01           1/1     Running
# kube-system   kube-controller-manager-single-node-01  1/1     Running
# kube-system   kube-proxy-xxxxx                        1/1     Running
# kube-system   kube-scheduler-single-node-01           1/1     Running
```

---

## 七、允许控制面节点调度工作负载

### 7.1 移除控制面污点

默认情况下，control-plane 节点有污点 `node-role.kubernetes.io/control-plane:NoSchedule`。单节点集群需要移除污点以允许调度工作负载：

```bash
kubectl --kubeconfig workload-kubeconfig taint nodes --all node-role.kubernetes.io/control-plane-
```

### 7.2 验证污点已移除

```bash
kubectl --kubeconfig workload-kubeconfig describe node single-node-01 | grep Taints
# 输出: Taints: <none>
```

### 7.3 部署测试工作负载

```bash
kubectl --kubeconfig workload-kubeconfig create deployment nginx --image=nginx:latest
kubectl --kubeconfig workload-kubeconfig expose deployment nginx --port=80 --type=ClusterIP

# 验证 Pod 运行在单节点上
kubectl --kubeconfig workload-kubeconfig get pods -o wide
# 输出:
# NAME                     READY   STATUS    NODE
# nginx-xxxxx-xxxxx        1/1     Running   single-node-01
```

---

## 八、可选配置

### 8.1 配置私有镜像仓库认证

如果目标机器需要从私有镜像仓库拉取镜像：

```bash
# 在目标机器上配置 containerd
sudo mkdir -p /etc/containerd/certs.d/<registry-url>

cat <<EOF | sudo tee /etc/containerd/certs.d/<registry-url>/hosts.toml
server = "https://<registry-url>"

[host."https://<registry-url>"]
  capabilities = ["pull", "resolve"]
  [host."https://<registry-url>".header]
    Authorization = "Basic $(echo -n '<username>:<password>' | base64)"
EOF

# 重启 containerd
sudo systemctl restart containerd
```

### 8.2 配置镜像加速

```bash
# 在目标机器上配置 containerd 镜像加速
sudo cat <<EOF | sudo tee /etc/containerd/config.toml
version = 2

[plugins."io.containerd.grpc.v1.cri".registry]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
      endpoint = ["https://<mirror-url>"]
EOF

sudo systemctl restart containerd
```

### 8.3 配置持久化存储

单节点集群可使用 hostPath 或 local-path-provisioner 作为存储：

```bash
# 安装 local-path-provisioner
kubectl --kubeconfig workload-kubeconfig apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.26/deploy/local-path-storage.yaml

# 设置为默认 StorageClass
kubectl --kubeconfig workload-kubeconfig patch storageclass local-path -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
```

---

## 九、集群管理

### 9.1 查看集群状态

```bash
# 查看集群详情
clusterctl describe cluster single-node-cluster --show-conditions all

# 查看机器状态
kubectl get machines -l cluster.x-k8s.io/cluster-name=single-node-cluster -o wide

# 查看节点状态
kubectl --kubeconfig workload-kubeconfig get nodes -o wide
```

### 9.2 升级 Kubernetes 版本

```bash
# 升级集群版本
kubectl patch cluster single-node-cluster --type='merge' -p '{
  "spec": {
    "topology": {
      "version": "v1.32.0"
    }
  }
}'

# 监控升级进度
clusterctl describe cluster single-node-cluster --show-conditions all
```

### 9.3 删除集群

```bash
# 删除集群
kubectl delete cluster single-node-cluster

# 验证机器已释放回机器池
kubectl get baremetalhostinventory single-node-hosts -o jsonpath='{.status.hostsStatus}'
```

---

## 十、故障排查

### 10.1 集群创建失败

```bash
# 查看集群事件
kubectl describe cluster single-node-cluster

# 查看 BareMetalCluster 状态
kubectl describe baremetalcluster single-node-cluster

# 查看 BareMetalMachine 状态
kubectl describe baremetalmachine <machine-name>

# 查看 CAPBM Controller 日志
kubectl logs -n capbm-system -l control-plane=controller-manager --tail=100
```

### 10.2 SSH 连接问题

```bash
# 验证凭据 Secret
kubectl get secret single-node-credentials -o jsonpath='{.data.username}' | base64 -d

# 测试 SSH 连接
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 root@192.168.1.101 echo "SSH connection successful"

# 检查目标机器 SSH 服务
ssh root@192.168.1.101 "systemctl status sshd"
```

### 10.3 预检失败

```bash
# 查看预检条件
kubectl get baremetalmachine <machine-name> -o jsonpath='{.status.conditions}'

# 常见预检失败原因：
# 1. OS 不支持 - 检查 /etc/os-release
# 2. 内核版本过低 - 检查 uname -r
# 3. 磁盘空间不足 - 检查 df -h
# 4. 内存不足 - 检查 free -g
# 5. Swap 未关闭 - 检查 swapon --show
```

### 10.4 网络问题

```bash
# 检查 API Server 可达性
curl -k https://192.168.1.101:6443/version

# 检查 Pod 网络
kubectl --kubeconfig workload-kubeconfig exec -it <pod-name> -- ping 10.244.0.1

# 检查 DNS 解析
kubectl --kubeconfig workload-kubeconfig run -it dns-test --image=busybox:1.28 --restart=Never -- nslookup kubernetes.default
```

### 10.5 常见问题

| 问题 | 原因 | 解决方案 |
|------|------|---------|
| 集群卡在 Provisioning | SSH 连接失败 | 检查网络、凭据、SSH 服务 |
| 节点 NotReady | CNI 未安装 | 检查 CNI 配置和 Pod 状态 |
| Pod 无法调度 | 控制面污点未移除 | 执行 taint nodes 命令 |
| 镜像拉取失败 | 网络或认证问题 | 检查私有仓库配置 |
| etcd 启动失败 | 磁盘空间不足 | 清理磁盘空间 |

---

## 十一、完整安装检查清单

- [ ] 管理集群 Kubernetes v1.32+ 已就绪
- [ ] clusterctl v1.13+ 已安装
- [ ] CAPBM Provider 已安装并运行
- [ ] ClusterClass 模板已部署
- [ ] 目标机器 OS 符合要求 (Ubuntu 20.04+/CentOS 7+/Rocky 8+)
- [ ] 目标机器 Swap 已关闭
- [ ] 目标机器 SSH 可达
- [ ] 机器凭据 Secret 已创建
- [ ] BareMetalHostInventory 已创建
- [ ] Cluster 资源已创建
- [ ] 集群状态为 Provisioned
- [ ] 节点状态为 Ready
- [ ] 控制面污点已移除
- [ ] 测试工作负载已部署并运行

---

## 十二、附录

### A. 单节点集群配置模板

```yaml
# single-node-cluster.yaml
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
    - name: podCIDR
      value: "10.244.0.0/16"
    - name: serviceCIDR
      value: "10.96.0.0/12"
    - name: cni
      value:
        enabled: true
        type: "calico"
        version: "3.27.0"
```

### B. 相关资源

- [Cluster API 官方文档](https://cluster-api.sigs.k8s.io/)
- [ClusterClass 文档](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/)
- [kubeadm 文档](https://kubernetes.io/docs/reference/setup-tools/kubeadm/)
- [Calico 文档](https://projectcalico.docs.tigera.io/)
