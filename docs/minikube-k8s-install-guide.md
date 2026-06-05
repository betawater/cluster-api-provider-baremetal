# Minikube 安装 Kubernetes 集群完整指南

## 一、环境准备

### 1.1 系统要求

| 项目 | 最低要求 |
|------|---------|
| CPU | 2 核+ |
| 内存 | 4GB+ |
| 磁盘 | 20GB+ 可用空间 |
| 操作系统 | Linux / macOS / Windows |
| 虚拟化 | 支持 VT-x/AMD-v |

### 1.2 支持的驱动

| 驱动 | 平台 | 说明 |
|------|------|------|
| docker | Linux/macOS/Windows | 推荐，最轻量 |
| podman | Linux | 无守护进程 |
| kvm2 | Linux | 性能好 |
| hyperkit | macOS | macOS 原生 |
| hyperv | Windows | Windows 原生 |
| virtualbox | 跨平台 | 通用但较重 |

---

## 二、安装 Minikube

### 2.1 Linux 安装

```bash
# 下载最新稳定版
curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64

# 安装
sudo install minikube-linux-amd64 /usr/local/bin/minikube

# 验证
minikube version
```

### 2.2 macOS 安装

```bash
# 使用 Homebrew（推荐）
brew install minikube

# 或手动下载
curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-darwin-amd64
sudo install minikube-darwin-amd64 /usr/local/bin/minikube

# 验证
minikube version
```

### 2.3 Windows 安装

```powershell
# 使用 Chocolatey
choco install minikube

# 或使用 Winget
winget install Kubernetes.minikube

# 或手动下载
# 访问 https://github.com/kubernetes/minikube/releases/latest
# 下载 minikube-windows-amd64.exe 并添加到 PATH

# 验证
minikube version
```

---

## 三、安装 kubectl

### 3.1 Linux 安装

```bash
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
kubectl version --client
```

### 3.2 macOS 安装

```bash
# 使用 Homebrew
brew install kubectl

# 或手动下载
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/darwin/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/
kubectl version --client
```

### 3.3 Windows 安装

```powershell
# 使用 Chocolatey
choco install kubernetes-cli

# 或使用 Winget
winget install Kubernetes.kubectl

# 验证
kubectl version --client
```

---

## 四、启动集群

### 4.1 基础启动

```bash
# 使用默认驱动启动
minikube start

# 指定驱动
minikube start --driver=docker

# 指定 Kubernetes 版本
minikube start --kubernetes-version=v1.29.0

# 指定资源
minikube start --cpus=4 --memory=8192

# 指定容器运行时
minikube start --container-runtime=containerd
```

### 4.2 国内环境优化

```bash
# 使用阿里云镜像
minikube start \
  --image-mirror-country=cn \
  --image-repository=registry.cn-hangzhou.aliyuncs.com/google_containers

# 或使用代理
minikube start \
  --docker-env HTTP_PROXY=http://proxy.example.com:8080 \
  --docker-env HTTPS_PROXY=http://proxy.example.com:8080 \
  --docker-env NO_PROXY=localhost,127.0.0.1,10.96.0.0/12,192.168.0.0/16
```

### 4.3 多节点集群

```bash
# 创建多节点集群
minikube start --nodes=3

# 查看节点
kubectl get nodes
```

**输出示例：**
```
NAME           STATUS   ROLES           AGE   VERSION
minikube       Ready    control-plane   2m    v1.29.0
minikube-m02   Ready    <none>          1m    v1.29.0
minikube-m03   Ready    <none>          1m    v1.29.0
```

---

## 五、集群管理

### 5.1 查看状态

```bash
# 集群状态
minikube status

# 节点状态
kubectl get nodes

# 系统 Pod
kubectl get pods -n kube-system
```

### 5.2 停止/启动

```bash
# 停止集群
minikube stop

# 启动集群
minikube start

# 暂停集群（保留资源）
minikube pause

# 恢复集群
minikube unpause
```

### 5.3 删除集群

```bash
# 删除当前集群
minikube delete

# 删除所有配置文件
minikube delete --all --purge
```

---

## 六、常用操作

### 6.1 Dashboard

```bash
# 打开 Dashboard
minikube dashboard

# 仅打印 URL
minikube dashboard --url
```

### 6.2 镜像管理

```bash
# 构建镜像到 Minikube
eval $(minikube docker-env)
docker build -t myapp:latest .

# 或直接加载本地镜像
minikube image load myapp:latest

# 查看 Minikube 中的镜像
minikube image ls
```

### 6.3 文件共享

```bash
# 挂载本地目录
minikube mount /path/to/local/dir:/path/in/minikube

# 在 Pod 中使用
kubectl run test --image=nginx -v /path/in/minikube:/usr/share/nginx/html
```

### 6.4 插件管理

```bash
# 查看可用插件
minikube addons list

# 启用插件
minikube addons enable ingress
minikube addons enable metrics-server
minikube addons enable dashboard

# 禁用插件
minikube addons disable metrics-server
```

---

## 七、网络与暴露服务

### 7.1 NodePort 服务

```bash
# 创建 NodePort 服务
kubectl expose deployment nginx --type=NodePort --port=80

# 获取访问 URL
minikube service nginx --url

# 或直接打开浏览器
minikube service nginx
```

### 7.2 Ingress 控制器

```bash
# 启用 Ingress 插件
minikube addons enable ingress

# 创建 Ingress 资源
cat << EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: example-ingress
spec:
  rules:
  - host: example.local
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: nginx
            port:
              number: 80
EOF

# 获取 Ingress IP
minikube ip
```

### 7.3 端口转发

```bash
# 转发本地端口到 Pod
kubectl port-forward svc/nginx 8080:80

# 访问
curl http://localhost:8080
```

---

## 八、存储配置

### 8.1 持久卷

```bash
# 创建 PV
cat << EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolume
metadata:
  name: local-pv
spec:
  capacity:
    storage: 1Gi
  accessModes:
  - ReadWriteOnce
  hostPath:
    path: /data/local-pv
EOF

# 创建 PVC
cat << EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: local-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
EOF
```

### 8.2 StorageClass

```bash
# Minikube 默认提供 standard StorageClass
kubectl get storageclass

# 启用 hostpath-provisioner 插件
minikube addons enable storage-provisioner
```

---

## 九、多集群管理

### 9.1 创建多个集群

```bash
# 创建第一个集群
minikube start -p cluster1

# 创建第二个集群
minikube start -p cluster2 --driver=docker

# 查看所有集群
minikube profile list

# 切换集群
minikube profile cluster2

# 删除指定集群
minikube delete -p cluster2
```

### 9.2 kubectl 配置

```bash
# 查看 kubeconfig
kubectl config view

# 切换上下文
kubectl config use-context minikube

# 查看当前上下文
kubectl config current-context
```

---

## 十、调试与排查

### 10.1 查看日志

```bash
# Minikube 日志
minikube logs

# 组件日志
minikube logs --components=kubelet,containerd

# Pod 日志
kubectl logs <pod-name> -n <namespace>
```

### 10.2 SSH 到节点

```bash
# SSH 到 Minikube 节点
minikube ssh

# 执行命令
minikube ssh -- ls /var/lib/kubelet
```

### 10.3 常见问题

#### 问题 1：启动失败

```bash
# 清理后重试
minikube delete
minikube start --force

# 指定驱动
minikube start --driver=docker
```

#### 问题 2：镜像拉取失败

```bash
# 使用国内镜像源
minikube start --image-mirror-country=cn

# 或手动加载镜像
minikube image load myimage.tar
```

#### 问题 3：资源不足

```bash
# 增加资源
minikube config set cpus 4
minikube config set memory 8192
minikube delete
minikube start
```

#### 问题 4：DNS 解析失败

```bash
# 重启 CoreDNS
kubectl rollout restart deployment coredns -n kube-system

# 检查 CoreDNS
kubectl get pods -n kube-system -l k8s-app=kube-dns
```

---

## 十一、高级配置

### 11.1 自定义配置

```bash
# 使用配置文件启动
cat << EOF > minikube-config.yaml
apiVersion: k8s.io/v1
kind: ConfigMap
metadata:
  name: minikube-config
data:
  cpus: "4"
  memory: "8192"
  driver: "docker"
  container-runtime: "containerd"
  kubernetes-version: "v1.29.0"
EOF

minikube start --config=minikube-config.yaml
```

### 11.2 离线安装

```bash
# 下载离线包
minikube start --download-only

# 使用本地缓存
minikube start --cache-images
```

### 11.3 自定义网络

```bash
# 指定 Pod CIDR
minikube start --pod-network-cidr=10.244.0.0/16

# 指定 Service CIDR
minikube start --service-cluster-ip-range=10.96.0.0/12
```

---

## 十二、附录

### 12.1 常用命令速查

| 命令 | 说明 |
|------|------|
| `minikube start` | 启动集群 |
| `minikube stop` | 停止集群 |
| `minikube delete` | 删除集群 |
| `minikube status` | 查看状态 |
| `minikube dashboard` | 打开 Dashboard |
| `minikube ip` | 获取集群 IP |
| `minikube ssh` | SSH 到节点 |
| `minikube logs` | 查看日志 |
| `minikube addons list` | 列出插件 |

### 12.2 推荐配置

| 场景 | CPU | 内存 | 驱动 |
|------|-----|------|------|
| 开发测试 | 2 | 4GB | docker |
| 中等负载 | 4 | 8GB | docker/kvm2 |
| 多节点 | 4+ | 8GB+ | docker |

### 12.3 国内镜像加速

```bash
# 启动时指定
minikube start \
  --image-mirror-country=cn \
  --image-repository=registry.cn-hangzhou.aliyuncs.com/google_containers

# 或使用其他镜像源
registry.aliyuncs.com/google_containers
```
