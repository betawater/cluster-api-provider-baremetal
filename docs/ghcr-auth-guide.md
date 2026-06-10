# GHCR 镜像拉取认证指南

## 问题描述

尝试从 GitHub Container Registry (GHCR) 拉取镜像时出现认证错误：

```bash
$ docker pull ghcr.io/betawater/capbm/release:v0.8.1
Error response from daemon: error from registry: unauthorized
unauthorized
```

**原因**：GHCR 默认要求认证才能拉取镜像，即使是公开镜像也可能需要基本的 GitHub 认证。

---

## 解决方案

### 方案一：使用 GitHub Personal Access Token (PAT) 认证

#### 1. 创建 Personal Access Token

1. 访问 https://github.com/settings/tokens
2. 点击 **Generate new token** → **Generate new token (classic)**
3. 填写 Token 描述（例如：`GHCR Image Pull`）
4. 勾选以下权限：
   - ✅ `read:packages` - 下载容器镜像和其他包
   - ✅ `repo` - （如果是私有镜像仓库需要此权限）
5. 点击 **Generate token**
6. **立即复制 Token**（离开页面后将无法再次查看）
   > - key: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
#### 2. 登录 GHCR

```bash
# 使用 GitHub 用户名和 Token 登录
echo <YOUR_GITHUB_TOKEN> | docker login ghcr.io -u <YOUR_GITHUB_USERNAME> --password-stdin
```

**示例：**
```bash
echo ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx | docker login ghcr.io -u myuser --password-stdin
Login Succeeded
```

#### 3. 拉取镜像

```bash
docker pull ghcr.io/betawater/capbm/release:v0.8.1
```

#### 4. 验证镜像

```bash
docker images | grep capbm/release
```

---

### 方案二：使用 GitHub CLI 认证

如果您已安装 [GitHub CLI](https://cli.github.com/)，可以使用它进行认证：

```bash
# 登录 GitHub
gh auth login

# 使用 gh 的 token 登录 GHCR
gh auth token | docker login ghcr.io -u <YOUR_GITHUB_USERNAME> --password-stdin
```

---

### 方案三：使用 Docker Credential Helper

#### Linux

```bash
# 安装 ghcr-docker-credential-helper
curl -sSL https://github.com/containerd/nerdctl/releases/download/v1.7.0/nerdctl-1.7.0-linux-amd64.tar.gz | tar xz
sudo mv nerdctl /usr/local/bin/

# 配置 Docker 使用 credential helper
mkdir -p ~/.docker
cat > ~/.docker/config.json <<EOF
{
  "credHelpers": {
    "ghcr.io": "ecr-login"
  }
}
EOF
```

#### macOS

```bash
# 使用 Homebrew 安装
brew install docker-credential-helper

# 配置 Docker
cat > ~/.docker/config.json <<EOF
{
  "credHelpers": {
    "ghcr.io": "osxkeychain"
  }
}
EOF
```

---

## CI/CD 环境配置

### GitHub Actions

在 GitHub Actions 工作流中添加登录步骤：

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Login to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Pull release image
        run: docker pull ghcr.io/betawater/capbm/release:v0.8.1

      - name: Deploy
        run: |
          # 使用镜像进行部署...
```

### GitLab CI

```yaml
deploy:
  image: docker:latest
  services:
    - docker:dind
  script:
    - echo $GHCR_TOKEN | docker login ghcr.io -u $GHCR_USERNAME --password-stdin
    - docker pull ghcr.io/betawater/capbm/release:v0.8.1
    # 部署步骤...
  variables:
    GHCR_TOKEN:
      value: $GHCR_PERSONAL_ACCESS_TOKEN
      masked: true
    GHCR_USERNAME:
      value: $GHCR_USER
```

### Jenkins

```groovy
pipeline {
    agent any
    stages {
        stage('Pull Image') {
            steps {
                withCredentials([usernamePassword(credentialsId: 'ghcr-credentials', usernameVariable: 'USERNAME', passwordVariable: 'PASSWORD')]) {
                    sh 'echo $PASSWORD | docker login ghcr.io -u $USERNAME --password-stdin'
                    sh 'docker pull ghcr.io/betawater/capbm/release:v0.8.1'
                }
            }
        }
    }
}
```

---

## 自行构建镜像

如果您无法访问 GHCR 上的镜像，可以自行构建：

### 1. 克隆仓库

```bash
git clone https://github.com/betawater/cluster-api-provider-baremetal.git
cd cluster-api-provider-baremetal
```

### 2. 构建镜像

```bash
# 设置镜像标签
export RELEASE_VERSION=v0.8.1
export REGISTRY=your-registry.example.com

# 构建 release image
make docker-build-release RELEASE_IMG=${REGISTRY}/capbm/release:${RELEASE_VERSION}
```

### 3. 推送到自己的镜像仓库

```bash
docker push ${REGISTRY}/capbm/release:${RELEASE_VERSION}
```

### 4. 使用自定义镜像

在 ReleaseImage CR 中指定您的镜像地址：

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v0-8-1
spec:
  version: v0.8.1
  image: your-registry.example.com/capbm/release:v0.8.1
  # ... 其他配置
```

---

## 离线环境镜像传输

### 1. 在有网络的环境导出镜像

```bash
docker save ghcr.io/betawater/capbm/release:v0.8.1 -o capbm-release-v0.8.1.tar
```

### 2. 传输到离线环境

```bash
# 使用 SCP
scp capbm-release-v0.8.1.tar user@offline-host:/tmp/

# 或使用 USB 驱动器
```

### 3. 在离线环境导入镜像

```bash
docker load -i capbm-release-v0.8.1.tar
```

### 4. 推送到离线镜像仓库（可选）

```bash
docker tag ghcr.io/betawater/capbm/release:v0.8.1 offline-registry.example.com/capbm/release:v0.8.1
docker push offline-registry.example.com/capbm/release:v0.8.1
```

---

## 常见问题

### Q1: Token 权限不足

**错误信息：**
```
Error response from daemon: unauthorized: authentication required
```

**解决方案：**
- 确保 Token 包含 `read:packages` 权限
- 如果是私有镜像，还需要 `repo` 权限
- 重新生成 Token 并检查权限设置

### Q2: Token 已过期

**错误信息：**
```
Error response from daemon: unauthorized: token has expired
```

**解决方案：**
```bash
# 重新登录
docker logout ghcr.io
echo <NEW_TOKEN> | docker login ghcr.io -u <USERNAME> --password-stdin
```

### Q3: 用户名错误

**错误信息：**
```
Error response from daemon: unauthorized: incorrect username or password
```

**解决方案：**
- 确保使用 GitHub 用户名（不是邮箱）
- 检查 Token 是否正确复制（无多余空格）

### Q4: Docker 版本过旧

**错误信息：**
```
Error response from daemon: Get https://ghcr.io/v2/: unauthorized
```

**解决方案：**
```bash
# 升级 Docker 到最新版本
sudo apt-get update
sudo apt-get install docker-ce docker-ce-cli containerd.io
```

### Q5: 网络代理问题

**错误信息：**
```
Error response from daemon: Get https://ghcr.io/v2/: proxyconnect tcp: dial tcp: lookup proxy.example.com: no such host
```

**解决方案：**

配置 Docker 代理：

```bash
# 创建或编辑 Docker 代理配置
sudo mkdir -p /etc/systemd/system/docker.service.d
cat > /etc/systemd/system/docker.service.d/http-proxy.conf <<EOF
[Service]
Environment="HTTP_PROXY=http://proxy.example.com:8080"
Environment="HTTPS_PROXY=http://proxy.example.com:8080"
Environment="NO_PROXY=localhost,127.0.0.1,.example.com"
EOF

# 重启 Docker
sudo systemctl daemon-reload
sudo systemctl restart docker
```

---

## 参考链接

| 资源 | 链接 |
|------|------|
| GitHub Packages 文档 | https://docs.github.com/en/packages |
| GHCR 认证指南 | https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry |
| Personal Access Token | https://github.com/settings/tokens |
| Docker 登录文档 | https://docs.docker.com/engine/reference/commandline/login/ |
