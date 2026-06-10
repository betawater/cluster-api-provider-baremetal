# GHCR 镜像拉取认证问题排查指南

## 问题现象

Pod 处于 `ImagePullBackOff` 状态，kubelet 无法拉取 GHCR 镜像：

```
Events:
  Type     Reason     Age   From     Message
  ----     ------     ----  ----     -------
  Normal   Scheduled  51s   default-scheduler  Successfully assigned capbm-system/capbm-controller-manager-xxx to node-0004
  Normal   Pulling    29s   kubelet  Pulling image "ghcr.io/betawater/capbm-manager:v0.8.1"
  Warning  Failed     26s   kubelet  Failed to pull image "ghcr.io/betawater/capbm-manager:v0.8.1": 
           failed to authorize: failed to fetch anonymous token: 
           unexpected status from GET request to https://ghcr.io/token: 401 Unauthorized
  Warning  Failed     26s   kubelet  Error: ErrImagePull
  Normal   BackOff    14s   kubelet  Back-off pulling image "ghcr.io/betawater/capbm-manager:v0.8.1"
  Warning  Failed     14s   kubelet  Error: ImagePullBackOff
```

但是 `docker pull` 可以成功拉取：

```bash
$ docker pull ghcr.io/betawater/capbm-manager:v0.8.1
v0.8.1: Pulling from betawater/capbm-manager
Digest: sha256:9a9656057ec06c31347a1fb50db9b526ca12472dfc0e9f1a8e5422b4243135e0
Status: Image is up to date for ghcr.io/betawater/capbm-manager:v0.8.1
```

## 问题根因

**kubelet 不会使用 Docker CLI 的认证信息**。

- `docker pull` 使用 `~/.docker/config.json` 中的认证信息
- `kubelet`（使用 containerd）有独立的认证机制
- 即使 Docker 已登录 GHCR，kubelet 仍然无法拉取私有镜像

## 解决方案

### 方案 A：创建 imagePullSecret（推荐）

适用于生产环境，镜像保持私有。

#### 步骤 1：创建 GitHub Personal Access Token

1. 访问 https://github.com/settings/tokens
2. 点击 "Generate new token" -> "Generate new token (classic)"
3. 设置 Token 名称（如 `ghcr-image-pull`）
4. 勾选以下权限：
   - `read:packages` - 读取包
   - `repo` - （如果镜像在私有仓库中）
5. 点击 "Generate token" 并复制 token 值

#### 步骤 2：在 Kubernetes 集群中创建 Secret

```bash
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=<YOUR_GITHUB_USERNAME> \
  --docker-password=<YOUR_GITHUB_TOKEN> \
  --docker-email=<YOUR_EMAIL> \
  -n capbm-system
```

**参数说明**：
| 参数 | 说明 | 示例 |
|------|------|------|
| `--docker-server` | 镜像仓库地址 | `ghcr.io` |
| `--docker-username` | GitHub 用户名 | `betawater` |
| `--docker-password` | GitHub Personal Access Token | `ghp_xxxx...` |
| `--docker-email` | 邮箱地址（可选） | `user@example.com` |
| `-n` | 命名空间 | `capbm-system` |

#### 步骤 3：将 Secret 绑定到 ServiceAccount

```bash
kubectl patch serviceaccount capbm-controller-manager \
  -n capbm-system \
  -p '{"imagePullSecrets": [{"name": "ghcr-secret"}]}'
```

#### 步骤 4：验证 Secret 已绑定

```bash
kubectl get serviceaccount capbm-controller-manager -n capbm-system -o yaml
```

预期输出应包含：
```yaml
imagePullSecrets:
- name: ghcr-secret
```

#### 步骤 5：重启 Pod

```bash
kubectl rollout restart deployment/capbm-controller-manager -n capbm-system
```

#### 步骤 6：验证 Pod 状态

```bash
kubectl get pods -n capbm-system
kubectl describe pod -n capbm-system -l control-plane=capbm-controller-manager
```

预期输出应包含：
```
Normal  Pulling    kubelet  Pulling image "ghcr.io/betawater/capbm-manager:v0.8.1"
Normal  Pulled     kubelet  Successfully pulled image "ghcr.io/betawater/capbm-manager:v0.8.1"
Normal  Created    kubelet  Created container manager
Normal  Started    kubelet  Started container manager
```

---

### 方案 B：将镜像设为公开

适用于开源项目，简化部署流程。

#### 步骤 1：修改镜像可见性

1. 访问 GitHub Packages 页面：
   - CAPBM: https://github.com/orgs/betawater/packages/container/capbm-manager
   - CVO: https://github.com/orgs/betawater/packages/container/cvo-manager
   - Release: https://github.com/orgs/betawater/packages/container/capbm/release

2. 点击 "Package settings"

3. 在 "Danger zone" 区域，点击 "Change visibility"

4. 选择 "Public" 并确认

#### 步骤 2：验证公开访问

```bash
# 在未登录 Docker 的情况下尝试拉取
docker logout ghcr.io
docker pull ghcr.io/betawater/capbm-manager:v0.8.1
```

如果拉取成功，说明镜像已公开。

#### 步骤 3：重启 Pod

```bash
kubectl rollout restart deployment/capbm-controller-manager -n capbm-system
kubectl rollout restart deployment/cvo-controller-manager -n cvo-system
```

---

### 方案 C：配置 containerd 认证（不推荐）

适用于特殊场景，但管理复杂。

#### 步骤 1：在每个节点上创建认证配置

```bash
# 在每个 Kubernetes 节点上执行
sudo mkdir -p /etc/containerd/certs.d/ghcr.io/betawater

cat << EOF | sudo tee /etc/containerd/certs.d/ghcr.io/betawater/hosts.toml
[host."https://ghcr.io"]
  capabilities = ["pull", "resolve"]
  [host."https://ghcr.io".header]
    Authorization = "Bearer <YOUR_GITHUB_TOKEN>"
EOF
```

#### 步骤 2：重启 containerd

```bash
sudo systemctl restart containerd
```

#### 步骤 3：重启 Pod

```bash
kubectl rollout restart deployment/capbm-controller-manager -n capbm-system
```

---

## 方案对比

| 方案 | 优点 | 缺点 | 适用场景 |
|------|------|------|---------|
| **A: imagePullSecret** | 安全、标准做法 | 需要管理 Secret | 生产环境、私有镜像 |
| **B: 公开镜像** | 部署简单、无需认证 | 镜像公开可见 | 开源项目 |
| **C: containerd 认证** | 无需修改 K8s 配置 | 管理复杂、每个节点都要配置 | 特殊场景 |

## 推荐方案

- **生产环境**：方案 A（imagePullSecret）
- **开源项目**：方案 B（公开镜像）
- **开发测试**：方案 B 或 A

## 常见问题

### Q1: 如何检查 Secret 是否已正确创建？

```bash
kubectl get secret ghcr-secret -n capbm-system -o yaml
```

### Q2: 如何检查 ServiceAccount 是否已绑定 Secret？

```bash
kubectl get serviceaccount capbm-controller-manager -n capbm-system -o jsonpath='{.imagePullSecrets}'
```

### Q3: 多个命名空间都需要拉取镜像怎么办？

在每个命名空间中创建 Secret，或使用 `--all-namespaces` 标志：

```bash
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=<USERNAME> \
  --docker-password=<TOKEN> \
  -n capbm-system

kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=<USERNAME> \
  --docker-password=<TOKEN> \
  -n cvo-system
```

### Q4: Token 过期了怎么办？

1. 删除旧 Secret：
   ```bash
   kubectl delete secret ghcr-secret -n capbm-system
   ```

2. 创建新 Secret（使用新 Token）：
   ```bash
   kubectl create secret docker-registry ghcr-secret \
     --docker-server=ghcr.io \
     --docker-username=<USERNAME> \
     --docker-password=<NEW_TOKEN> \
     -n capbm-system
   ```

3. 重启 Pod：
   ```bash
   kubectl rollout restart deployment/capbm-controller-manager -n capbm-system
   ```

### Q5: 如何在 GitHub Actions 中自动创建 imagePullSecret？

可以在部署 workflow 中添加：

```yaml
- name: Create imagePullSecret
  run: |
    kubectl create secret docker-registry ghcr-secret \
      --docker-server=ghcr.io \
      --docker-username=${{ github.actor }} \
      --docker-password=${{ secrets.GITHUB_TOKEN }} \
      -n capbm-system \
      --dry-run=client -o yaml | kubectl apply -f -
```

---

## 参考链接

- [GitHub Packages 文档](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Kubernetes imagePullSecrets 文档](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)
- [containerd 认证配置](https://github.com/containerd/containerd/blob/main/docs/hosts.md)
