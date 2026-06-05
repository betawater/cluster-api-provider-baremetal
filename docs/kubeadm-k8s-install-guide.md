# kubeadm 安装 Kubernetes 集群完整指南

## 一、环境准备

### 1.1 节点规划

| 节点角色 | 主机名 | IP 地址 | 配置 |
|---------|--------|---------|------|
| Control Plane | k8s-master | 192.168.1.100 | 2C4G |
| Worker Node 1 | k8s-node1 | 192.168.1.101 | 2C4G |
| Worker Node 2 | k8s-node2 | 192.168.1.102 | 2C4G |

### 1.2 前置要求

- 操作系统：Ubuntu 20.04/22.04 或 CentOS 7/8
- 每台机器 2GB+ 内存
- 每台机器 2 核+ CPU
- 节点间网络互通
- 唯一的主机名、MAC 地址、product_uuid
- 禁用 swap

---

## 二、所有节点初始化（Master + Worker）

### 2.1 设置主机名

```bash
# 在 master 节点执行
hostnamectl set-hostname k8s-master

# 在 node1 节点执行
hostnamectl set-hostname k8s-node1

# 在 node2 节点执行
hostnamectl set-hostname k8s-node2
```

### 2.2 配置 hosts 解析

```bash
cat >> /etc/hosts << EOF
192.168.1.100 k8s-master
192.168.1.101 k8s-node1
192.168.1.102 k8s-node2
EOF
```

### 2.3 禁用 swap

```bash
# 临时禁用
swapoff -a

# 永久禁用（注释掉 /etc/fstab 中的 swap 行）
sed -i '/swap/s/^/#/' /etc/fstab

# 验证
free -m
```

### 2.4 加载内核模块

```bash
cat > /etc/modules-load.d/k8s.conf << EOF
overlay
br_netfilter
EOF

modprobe overlay
modprobe br_netfilter
```

### 2.5 配置内核参数

```bash
cat > /etc/sysctl.d/k8s.conf << EOF
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
EOF

sysctl --system
```

### 2.6 配置时间同步

```bash
# Ubuntu
apt install -y chrony
systemctl enable --now chrony

# CentOS
yum install -y chrony
systemctl enable --now chrony

# 验证
chronyc sources
```

### 2.7 关闭防火墙（或配置规则）

```bash
# Ubuntu
ufw disable

# CentOS
systemctl stop firewalld
systemctl disable firewalld
```

---

## 三、安装容器运行时（containerd）

### 3.1 安装 containerd

```bash
# Ubuntu
apt update
apt install -y containerd

# CentOS
yum install -y containerd.io
```

### 3.2 配置 containerd

```bash
# 生成默认配置
mkdir -p /etc/containerd
containerd config default > /etc/containerd/config.toml

# 修改配置：使用 systemd cgroup 驱动
sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

# 配置 sandbox 镜像（国内加速）
sed -i 's|sandbox_image = "registry.k8s.io/pause:.*"|sandbox_image = "registry.k8s.io/pause:3.9"|' /etc/containerd/config.toml
```

### 3.3 启动 containerd

```bash
systemctl daemon-reload
systemctl enable --now containerd
systemctl status containerd
```

---

## 四、安装 kubeadm、kubelet、kubectl

### 4.1 添加 Kubernetes 仓库

```bash
# Ubuntu
apt update
apt install -y apt-transport-https ca-certificates curl
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.29/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.29/deb/ /' > /etc/apt/sources.list.d/kubernetes.list

# CentOS
cat > /etc/yum.repos.d/kubernetes.repo << EOF
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/v1.29/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/v1.29/rpm/repodata/repomd.xml.key
EOF
```

### 4.2 安装组件

```bash
# Ubuntu
apt update
apt install -y kubelet kubeadm kubectl
apt-mark hold kubelet kubeadm kubectl

# CentOS
yum install -y kubelet kubeadm kubectl
systemctl enable kubelet
```

### 4.3 验证版本

```bash
kubeadm version
kubelet --version
kubectl version --client
```

### 4.4 升级版本

```bash
# 确认当前集群版本：
kubectl version --short
# install
apt-get update && apt-get install -y kubeadm=1.31.0-00 kubelet=1.31.0-00 kubectl=1.31.0-00 
```

```bash
cat <<EOF | sudo tee /etc/yum.repos.d/kubernetes.repo
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/v1.31/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/v1.31/rpm/repodata/repomd.xml.key
EOF

sudo yum install -y kubeadm-1.31.0 kubelet-1.31.0 kubectl-1.31.0
```

## 五、初始化 Control Plane（仅 Master 节点）

### 5.1 拉取镜像（可选，国内环境推荐）

```bash
# 查看需要的镜像
kubeadm config images list

# 拉取镜像（使用阿里云加速）
kubeadm config images pull --image-repository registry.aliyuncs.com/google_containers
```

### 5.2 初始化集群

```bash
kubeadm init \
  --apiserver-advertise-address=192.168.1.100 \
  --image-repository registry.aliyuncs.com/google_containers \
  --pod-network-cidr=10.244.0.0/16 \
  --service-cidr=10.96.0.0/12 \
  --kubernetes-version=v1.29.0
```

**参数说明：**
- `--apiserver-advertise-address`：API Server 监听地址（Master IP）
- `--image-repository`：镜像仓库地址（国内使用阿里云）
- `--pod-network-cidr`：Pod 网络 CIDR（Calico 默认 10.244.0.0/16）
- `--service-cidr`：Service 网络 CIDR
- `--kubernetes-version`：Kubernetes 版本

### 5.3 配置 kubectl

```bash
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config

# 验证
kubectl cluster-info
kubectl get nodes
```

**输出示例：**
```
NAME         STATUS     ROLES           AGE   VERSION
k8s-master   NotReady   control-plane   1m    v1.29.0
```

> 状态为 `NotReady` 是正常的，因为还没有安装网络插件。

### 5.4 保存 join 命令

初始化成功后会输出类似如下的 join 命令，**请保存好**：

```bash
kubeadm join 192.168.1.100:6443 --token abc123.def456 \
  --discovery-token-ca-cert-hash sha256:xxxxx
```

---

## 六、安装网络插件（CNI）

### 6.1 安装 Calico（推荐）

```bash
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/calico.yaml
```

### 6.2 或安装 Flannel

```bash
kubectl apply -f https://raw.githubusercontent.com/flannel-io/flannel/master/Documentation/kube-flannel.yml
```

### 6.3 验证网络插件

```bash
# 等待 Pod 启动
kubectl get pods -n kube-system

# 等待所有 Pod 状态变为 Running
kubectl get nodes
```

**输出示例：**
```
NAME         STATUS   ROLES           AGE   VERSION
k8s-master   Ready    control-plane   5m    v1.29.0
```

---

## 七、加入 Worker 节点

### 7.1 在 Worker 节点执行 join 命令

```bash
# 在 k8s-node1 和 k8s-node2 上执行
kubeadm join 192.168.1.100:6443 --token abc123.def456 \
  --discovery-token-ca-cert-hash sha256:xxxxx
```

### 7.2 Token 过期处理

如果 token 过期（默认 24 小时），在 Master 节点重新生成：

```bash
# 生成新 token
kubeadm token create --print-join-command

# 或查看当前 token
kubeadm token list

# 如果 ca-cert-hash 也过期，重新获取
openssl x509 -pubkey -in /etc/kubernetes/pki/ca.crt | openssl rsa -pubin -outform der 2>/dev/null | openssl dgst -sha256 -hex | sed 's/^.* //'
```

### 7.3 验证节点加入

```bash
# 在 Master 节点执行
kubectl get nodes
```

**输出示例：**
```
NAME         STATUS   ROLES           AGE   VERSION
k8s-master   Ready    control-plane   10m   v1.29.0
k8s-node1    Ready    <none>          2m    v1.29.0
k8s-node2    Ready    <none>          1m    v1.29.0
```

---

## 八、集群验证

### 8.1 核心组件状态

```bash
kubectl get pods -n kube-system
kubectl get svc -n kube-system
kubectl get cs
```

### 8.2 部署测试应用

```bash
# 创建测试 Deployment
kubectl create deployment nginx --image=nginx --replicas=3

# 查看 Pod 状态
kubectl get pods -o wide

# 创建 Service
kubectl expose deployment nginx --port=80 --type=NodePort

# 查看 Service
kubectl get svc nginx
```

### 8.3 访问测试

```bash
# 获取 NodePort
NODE_PORT=$(kubectl get svc nginx -o jsonpath='{.spec.ports[0].nodePort}')

# 访问测试
curl http://192.168.1.100:$NODE_PORT
curl http://192.168.1.101:$NODE_PORT
curl http://192.168.1.102:$NODE_PORT
```

### 8.4 DNS 测试

```bash
kubectl run -it --rm dns-test --image=busybox:1.28 --restart=Never -- nslookup kubernetes.default
```

---

## 九、常用管理命令

### 9.1 节点管理

```bash
# 查看节点
kubectl get nodes -o wide

# 标记节点不可调度
kubectl cordon k8s-node1

# 驱逐节点上的 Pod
kubectl drain k8s-node1 --ignore-daemonsets --delete-emptydir-data

# 恢复节点调度
kubectl uncordon k8s-node1

# 删除节点
kubectl delete node k8s-node1
```

### 9.2 重置节点

```bash
# 在需要重置的节点上执行
kubeadm reset -f
rm -rf /etc/kubernetes /var/lib/kubelet /var/lib/etcd
rm -rf ~/.kube
```

### 9.3 查看日志

```bash
# kubelet 日志
journalctl -u kubelet -f

# containerd 日志
journalctl -u containerd -f

# Pod 日志
kubectl logs -f <pod-name> -n <namespace>
```

---

## 十、常见问题排查

### 10.1 节点 NotReady

```bash
# 检查网络插件 Pod
kubectl get pods -n kube-system | grep -E 'calico|flannel'

# 检查 kubelet 状态
systemctl status kubelet
journalctl -u kubelet -n 50
```

### 10.2 Pod 无法启动

```bash
# 查看 Pod 事件
kubectl describe pod <pod-name>

# 查看节点资源
kubectl describe node <node-name>
```

### 10.3 镜像拉取失败

```bash
# 手动拉取镜像
ctr -n k8s.io images pull registry.aliyuncs.com/google_containers/pause:3.9

# 或配置镜像加速器
cat > /etc/containerd/config.toml << EOF
[plugins."io.containerd.grpc.v1.cri".registry]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
      endpoint = ["https://mirror.ccs.tencentyun.com"]
EOF
systemctl restart containerd
```

---

## 十一、高可用集群（可选）

### 11.1 多 Master 架构

```
        ┌─────────────┐
        │   Keepalived │
        │   + HAProxy  │
        │  VIP: 192.168.1.200
        └──────┬──────┘
               │
    ┌──────────┼──────────┐
    │          │          │
┌───┴───┐ ┌───┴───┐ ┌───┴───┐
│Master1│ │Master2│ │Master3│
└───────┘ └───────┘ └───────┘
```

### 11.2 初始化第一个 Master

```bash
kubeadm init \
  --control-plane-endpoint "192.168.1.200:6443" \
  --upload-certs \
  --image-repository registry.aliyuncs.com/google_containers \
  --pod-network-cidr=10.244.0.0/16
```

### 11.3 加入其他 Master 节点

```bash
kubeadm join 192.168.1.200:6443 \
  --control-plane \
  --token abc123.def456 \
  --discovery-token-ca-cert-hash sha256:xxxxx \
  --certificate-key xxxxx
```

---

## 十二、附录

### 12.1 端口要求

| 组件 | 端口 | 方向 |
|------|------|------|
| API Server | 6443 | 入站 |
| etcd | 2379-2380 | 入站 |
| kubelet | 10250 | 入站 |
| NodePort | 30000-32767 | 入站 |

### 12.2 推荐配置

| 组件 | 推荐值 |
|------|--------|
| Kubernetes 版本 | v1.29.x |
| containerd 版本 | 1.7.x |
| CNI 插件 | Calico 3.26.x |
| Pod CIDR | 10.244.0.0/16 |
| Service CIDR | 10.96.0.0/12 |

### 12.3 国内镜像加速

```bash
# kubeadm 初始化时使用
--image-repository registry.aliyuncs.com/google_containers

# 或使用其他镜像源
registry.cn-hangzhou.aliyuncs.com/google_containers
```
