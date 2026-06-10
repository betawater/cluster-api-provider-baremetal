# GHCR 镜像拉取认证指南

## 问题现象

Pod 处于 `ImagePullBackOff` 状态，kubelet 无法拉取 GHCR 私有镜像：

```
Events:
  Type     Reason     Age   From     Message
  ----     ------     ----  ----     -------
  Normal   Pulling    29s   kubelet  Pulling image "ghcr.io/betawater/capbm-manager:v0.8.1"
  Warning  Failed     26s   kubelet  Failed to pull image: 401 Unauthorized
  Warning  Failed     26s   kubelet  Error: ErrImagePull
  Normal   BackOff    14s   kubelet  Back-off pulling image
  Warning  Failed     14s   kubelet  Error: ImagePullBackOff
```

但是 `docker pull` 可以成功拉取（因为 Docker CLI 已登录）：

```bash
$ docker pull ghcr.io/betawater/capbm-manager:v0.8.1
Status: Image is up to date for ghcr.io/betawater/capbm-manager:v0.8.1
```

## 问题根因

**kubelet 不会使用 Docker CLI 的认证信息**。

- `docker pull` 使用 `~/.docker/config.json` 中的认证信息
- `kubelet`（使用 containerd）有独立的认证机制
- 即使 Docker 已登录 GHCR，kubelet 仍然无法拉取私有镜像

## 解决方案：创建 imagePullSecret

### 前置条件

需要 GitHub Personal Access Token，权限要求：
- `read:packages` - 读取 GHCR 包

### 步骤 1：创建 GitHub Personal Access Token

1. 访问 https://github.com/settings/tokens
2. 点击 "Generate new token" → "Generate new token (classic)"
3. 设置 Token 名称（如 `ghcr-image-pull`）
4. 勾选以下权限：
   - ✅ `read:packages`
   - ✅ `repo`（如果镜像在私有仓库中）
5. 点击 "Generate token" 并**立即复制 token 值**（只显示一次）

### 步骤 2：在 Kubernetes 集群中创建 Secret

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

### 步骤 3：将 Secret 绑定到 ServiceAccount

```bash
kubectl patch serviceaccount capbm-controller-manager \
  -n capbm-system \
  -p '{"imagePullSecrets": [{"name": "ghcr-secret"}]}'
```

### 步骤 4：为 CVO 命名空间创建 Secret（如果需要）

```bash
# 创建 Secret
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=<YOUR_GITHUB_USERNAME> \
  --docker-password=<YOUR_GITHUB_TOKEN> \
  --docker-email=<YOUR_EMAIL> \
  -n cvo-system

# 绑定到 ServiceAccount
kubectl patch serviceaccount cvo-controller-manager \
  -n cvo-system \
  -p '{"imagePullSecrets": [{"name": "ghcr-secret"}]}'
```

### 步骤 5：重启 Pod

```bash
kubectl rollout restart deployment/capbm-controller-manager -n capbm-system
kubectl rollout restart deployment/cvo-controller-manager -n cvo-system
```

### 步骤 6：验证

```bash
# 检查 Pod 状态
kubectl get pods -n capbm-system
kubectl get pods -n cvo-system

# 检查 Pod 事件
kubectl describe pod -n capbm-system -l control-plane=capbm-controller-manager
```

**预期成功输出**：

```
Normal  Pulling    kubelet  Pulling image "ghcr.io/betawater/capbm-manager:v0.8.1"
Normal  Pulled     kubelet  Successfully pulled image "ghcr.io/betawater/capbm-manager:v0.8.1"
Normal  Created    kubelet  Created container manager
Normal  Started    kubelet  Started container manager
```

## 验证配置

### 检查 Secret 是否已创建

```bash
kubectl get secret ghcr-secret -n capbm-system
```

**预期输出**：

```
NAME         TYPE                             DATA   AGE
ghcr-secret  kubernetes.io/dockerconfigjson   1      2m
```

### 检查 ServiceAccount 是否已绑定 Secret

```bash
kubectl get serviceaccount capbm-controller-manager -n capbm-system -o yaml
```

**预期输出**：

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: capbm-controller-manager
  namespace: capbm-system
imagePullSecrets:
- name: ghcr-secret
```

## 自动化部署（可选）

### 方式 1：在 Makefile 中添加

在 `Makefile` 中添加以下目标：

```makefile
.PHONY: create-ghcr-secret
create-ghcr-secret:
	@echo "Creating GHCR imagePullSecret..."
	kubectl create secret docker-registry ghcr-secret \
		--docker-server=ghcr.io \
		--docker-username=$(GHCR_USERNAME) \
		--docker-password=$(GHCR_TOKEN) \
		-n capbm-system \
		--dry-run=client -o yaml | kubectl apply -f -
	kubectl patch serviceaccount capbm-controller-manager \
		-n capbm-system \
		-p '{"imagePullSecrets": [{"name": "ghcr-secret"}]}'
```

使用方式：

```bash
make create-ghcr-secret GHCR_USERNAME=betawater GHCR_TOKEN=ghp_xxxx...
```

### 方式 2：在 GitHub Actions 中自动创建

在部署 workflow 中添加：

```yaml
- name: Create imagePullSecret
  run: |
    kubectl create secret docker-registry ghcr-secret \
      --docker-server=ghcr.io \
      --docker-username=${{ github.actor }} \
      --docker-password=${{ secrets.GITHUB_TOKEN }} \
      -n capbm-system \
      --dry-run=client -o yaml | kubectl apply -f -
    
    kubectl patch serviceaccount capbm-controller-manager \
      -n capbm-system \
      -p '{"imagePullSecrets": [{"name": "ghcr-secret"}]}'
```

## Token 过期处理

### 症状

Pod 再次出现 `ImagePullBackOff` 错误，日志显示 `401 Unauthorized`。

### 解决步骤

1. **创建新的 GitHub Personal Access Token**
   - 访问 https://github.com/settings/tokens
   - 创建新 token（权限同上）

2. **删除旧 Secret**

   ```bash
   kubectl delete secret ghcr-secret -n capbm-system
   kubectl delete secret ghcr-secret -n cvo-system
   ```

3. **创建新 Secret**

   ```bash
   kubectl create secret docker-registry ghcr-secret \
     --docker-server=ghcr.io \
     --docker-username=<YOUR_GITHUB_USERNAME> \
     --docker-password=<NEW_GITHUB_TOKEN> \
     -n capbm-system
   ```

4. **重启 Pod**

   ```bash
   kubectl rollout restart deployment/capbm-controller-manager -n capbm-system
   kubectl rollout restart deployment/cvo-controller-manager -n cvo-system
   ```

## 常见问题

### Q1: 如何检查 Secret 内容？

```bash
kubectl get secret ghcr-secret -n capbm-system -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d
```

### Q2: 多个命名空间都需要拉取镜像怎么办？

在每个命名空间中创建 Secret：

```bash
for ns in capbm-system cvo-system; do
  kubectl create secret docker-registry ghcr-secret \
    --docker-server=ghcr.io \
    --docker-username=<USERNAME> \
    --docker-password=<TOKEN> \
    -n $ns
done
```

### Q3: 如何验证 Secret 是否生效？

```bash
# 创建一个测试 Pod 使用该 Secret
kubectl run test-pull --image=ghcr.io/betawater/capbm-manager:v0.8.1 \
  --image-pull-policy=Always \
  --restart=Never \
  --overrides='{"spec": {"imagePullSecrets": [{"name": "ghcr-secret"}]}}' \
  -n capbm-system

# 检查 Pod 状态
kubectl get pod test-pull -n capbm-system

# 清理
kubectl delete pod test-pull -n capbm-system
```

### Q4: 可以将镜像设为公开吗？

如果你希望镜像公开访问（无需认证）：

1. 访问 https://github.com/orgs/betawater/packages/container/capbm-manager
2. 点击 "Package settings"
3. 在 "Danger zone" 区域，点击 "Change visibility"
4. 选择 "Public" 并确认

公开后，kubelet 可以直接拉取镜像，无需创建 imagePullSecret。

## 参考链接

- [GitHub Packages 文档](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Kubernetes imagePullSecrets 文档](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)
- [containerd 认证配置](https://github.com/containerd/containerd/blob/main/docs/hosts.md)
