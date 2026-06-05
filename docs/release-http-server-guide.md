# ReleaseImage HTTP 服务器部署指南

## 1. 概述

ReleaseImage HTTP 服务器用于在离线/air-gapped 环境中为裸金属节点提供组件下载服务。节点安装过程中，CAPBM Installer 会从该服务器下载 Kubernetes 二进制包、containerd、CNI 插件等组件。

### 1.1 工作原理

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Management Cluster                           │
│                                                                     │
│  ┌─────────────────┐    ┌─────────────────┐                        │
│  │  ReleaseImage   │    │  HTTP Server    │                        │
│  │  (CRD)          │    │  (nginx/caddy)  │                        │
│  │  httpServer:    │    │  :8080          │                        │
│  │    baseUrl:     │───▶│  /release/      │                        │
│  │    http://...   │    │  ├── binaries/  │                        │
│  └─────────────────┘    │  ├── images/    │                        │
│                         │  ├── charts/    │                        │
│                         │  └── scripts/   │                        │
│                         └─────────────────┘                        │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  │ HTTP 下载
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        BareMetal Nodes                              │
│                                                                     │
│  CAPBM Installer ──▶ curl http://release-server:8080/release/...    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 1.2 使用场景

| 场景 | 是否需要 HTTP 服务器 | 说明 |
|------|---------------------|------|
| **在线环境** | ❌ 不需要 | 节点直接从官方仓库下载（apt/yum/zypper） |
| **离线环境** | ✅ 需要 | 节点从内部 HTTP 服务器下载二进制包 |
| **混合环境** | 可选 | 部分组件从 HTTP 服务器下载，部分从官方仓库 |

---

## 2. HTTP 服务器配置

### 2.1 ReleaseImage 中的配置

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-31-1
  namespace: capbm-system
spec:
  version: v1.31.1
  httpServer:
    enabled: true
    port: 8080
    basePath: /release/v1.31.1
    baseUrl: http://release-server.capbm-system.svc.cluster.local:8080/release/v1.31.1
    tlsSecretRef: ""
    insecureSkipVerify: false
```

### 2.2 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 HTTP 服务器模式 |
| `port` | int | `8080` | HTTP 服务器端口 |
| `basePath` | string | `/release/v{version}` | 内容基础路径 |
| `baseUrl` | string | - | 完整的 HTTP 服务器 URL |
| `tlsSecretRef` | string | - | TLS 证书 Secret 名称 |
| `insecureSkipVerify` | bool | `false` | 跳过 TLS 验证（内部 CA 时使用） |

### 2.3 BareMetalMachine 中的配置

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachine
metadata:
  name: worker-1
spec:
  installSource:
    type: HTTPServer
    httpServerConfig:
      baseUrl: http://release-server.capbm-system.svc.cluster.local:8080/release/v1.31.1
```

---

## 3. 部署方案

### 3.1 方案一：Nginx（推荐）

#### 3.1.1 使用 ConfigMap 存储内容

适用于内容较小的场景：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: release-image-content
  namespace: capbm-system
data:
  # 内容通过 kubectl create configmap 导入
  # kubectl create configmap release-image-content \
  #   --from-file=release-image/ \
  #   -n capbm-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-http-server
  namespace: capbm-system
  labels:
    app: release-http-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: release-http-server
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        app: release-http-server
    spec:
      containers:
        - name: nginx
          image: nginx:1.25-alpine
          ports:
            - containerPort: 8080
              name: http
              protocol: TCP
          volumeMounts:
            - name: release-content
              mountPath: /usr/share/nginx/html/release
              readOnly: true
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi
      volumes:
        - name: release-content
          configMap:
            name: release-image-content
---
apiVersion: v1
kind: Service
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
      protocol: TCP
      name: http
  selector:
    app: release-http-server
```

#### 3.1.2 使用 PersistentVolume 存储内容

适用于内容较大的场景（推荐生产环境）：

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: release-content-pvc
  namespace: capbm-system
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: release-http-server
  template:
    spec:
      containers:
        - name: nginx
          image: nginx:1.25-alpine
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: release-content
              mountPath: /usr/share/nginx/html/release
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi
      volumes:
        - name: release-content
          persistentVolumeClaim:
            claimName: release-content-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
  selector:
    app: release-http-server
```

**上传内容到 PV：**

```bash
# 获取 Pod 名称
POD=$(kubectl get pods -n capbm-system -l app=release-http-server -o jsonpath='{.items[0].metadata.name}')

# 上传 release-image 目录内容
kubectl cp release-image/ capbm-system/${POD}:/usr/share/nginx/html/release/v1.31.1
```

### 3.2 方案二：Caddy（自动 HTTPS）

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: caddy-config
  namespace: capbm-system
data:
  Caddyfile: |
    :8080 {
      root * /srv/release
      file_server
      encode gzip
      log {
        output stdout
        format console
      }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: release-http-server
  template:
    spec:
      containers:
        - name: caddy
          image: caddy:2.7-alpine
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: caddy-config
              mountPath: /etc/caddy/Caddyfile
              subPath: Caddyfile
            - name: release-content
              mountPath: /srv/release
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi
      volumes:
        - name: caddy-config
          configMap:
            name: caddy-config
        - name: release-content
          persistentVolumeClaim:
            claimName: release-content-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
  selector:
    app: release-http-server
```

### 3.3 方案三：Python http.server（测试用）

仅适用于开发/测试环境：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: release-http-server
  template:
    spec:
      containers:
        - name: python-http
          image: python:3.11-alpine
          command: ["python3", "-m", "http.server", "8080"]
          workingDir: /srv
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: release-content
              mountPath: /srv/release
      volumes:
        - name: release-content
          persistentVolumeClaim:
            claimName: release-content-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
  selector:
    app: release-http-server
```

### 3.4 方案四：外部 HTTP 服务器

如果已有内部 HTTP 服务器（如 Nexus、Artifactory、Nginx 独立部署），只需在 ReleaseImage 中配置 `baseUrl`：

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-31-1
spec:
  version: v1.31.1
  httpServer:
    enabled: true
    baseUrl: http://nexus.internal.com:8081/repository/capbm-release/v1.31.1
```

---

## 4. 离线环境部署

### 4.1 构建 ReleaseImage 内容

```bash
# 在可访问外网的环境中构建
export RELEASE_VERSION=v1.31.1
export OUTPUT_DIR=release-image

# 使用构建脚本
./scripts/build-release-image-no-docker.sh

# 验证目录结构
ls -la ${OUTPUT_DIR}/
```

### 4.2 传输到离线环境

```bash
# 打包
tar -czf release-image-${RELEASE_VERSION}.tar.gz release-image/

# 传输（使用 USB、SCP 等）
scp release-image-${RELEASE_VERSION}.tar.gz offline-user@offline-host:/tmp/

# 在离线环境解压
tar -xzf release-image-${RELEASE_VERSION}.tar.gz -C /srv/
```

### 4.3 部署 HTTP 服务器

```bash
# 在离线环境部署 Nginx
kubectl apply -f release-http-server.yaml

# 上传内容
POD=$(kubectl get pods -n capbm-system -l app=release-http-server -o jsonpath='{.items[0].metadata.name}')
kubectl cp release-image/ capbm-system/${POD}:/usr/share/nginx/html/release/v1.31.1
```

### 4.4 配置 ReleaseImage

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-31-1-offline
spec:
  version: v1.31.1
  httpServer:
    enabled: true
    baseUrl: http://release-server.capbm-system.svc.cluster.local:8080/release/v1.31.1
  imageRegistry:
    enabled: true
    registry: offline-registry.internal.com
    repository: capbm
```

---

## 5. 安全配置

### 5.1 TLS 加密

#### 5.1.1 创建 TLS Secret

```bash
kubectl create secret tls release-server-tls \
  --cert=tls.crt \
  --key=tls.key \
  -n capbm-system
```

#### 5.1.2 配置 Nginx 使用 TLS

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-tls-config
  namespace: capbm-system
data:
  nginx.conf: |
    server {
      listen 8443 ssl;
      server_name release-server.capbm-system.svc.cluster.local;

      ssl_certificate /etc/tls/tls.crt;
      ssl_certificate_key /etc/tls/tls.key;

      root /usr/share/nginx/html;

      location /release/ {
        autoindex on;
      }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  template:
    spec:
      containers:
        - name: nginx
          image: nginx:1.25-alpine
          ports:
            - containerPort: 8443
          volumeMounts:
            - name: nginx-config
              mountPath: /etc/nginx/conf.d/default.conf
              subPath: nginx.conf
            - name: tls-cert
              mountPath: /etc/tls
              readOnly: true
            - name: release-content
              mountPath: /usr/share/nginx/html/release
      volumes:
        - name: nginx-config
          configMap:
            name: nginx-tls-config
        - name: tls-cert
          secret:
            secretName: release-server-tls
        - name: release-content
          persistentVolumeClaim:
            claimName: release-content-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  type: ClusterIP
  ports:
    - port: 8443
      targetPort: 8443
  selector:
    app: release-http-server
```

#### 5.1.3 配置 ReleaseImage 使用 HTTPS

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-31-1
spec:
  httpServer:
    enabled: true
    baseUrl: https://release-server.capbm-system.svc.cluster.local:8443/release/v1.31.1
    tlsSecretRef: release-server-tls
    insecureSkipVerify: false
```

### 5.2 基本认证

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: release-server-auth
  namespace: capbm-system
type: Opaque
data:
  # echo -n "admin:password" | base64
  htpasswd: YWRtaW46JDJ5JDA1JHhYWVhYWVhYWVhYWVhYWVhYWVhYWVhYWVhYWVhYWVhYWVhYWVhYWVh
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-auth-config
  namespace: capbm-system
data:
  nginx.conf: |
    server {
      listen 8080;

      root /usr/share/nginx/html;

      location /release/ {
        auth_basic "Release Server";
        auth_basic_user_file /etc/nginx/.htpasswd;
        autoindex on;
      }
    }
```

---

## 6. 高可用部署

### 6.1 多副本部署

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-http-server
  namespace: capbm-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: release-http-server
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
  template:
    spec:
      containers:
        - name: nginx
          image: nginx:1.25-alpine
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: release-content
              mountPath: /usr/share/nginx/html/release
              readOnly: true
      volumes:
        - name: release-content
          persistentVolumeClaim:
            claimName: release-content-pvc
```

### 6.2 使用 ReadWriteMany PV

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: release-content-pvc
  namespace: capbm-system
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 10Gi
  storageClassName: nfs
```

---

## 7. 监控和日志

### 7.1 启用 Nginx 访问日志

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-logging-config
  namespace: capbm-system
data:
  nginx.conf: |
    log_format release '$remote_addr - $remote_user [$time_local] '
                       '"$request" $status $body_bytes_sent '
                       '"$http_referer" "$http_user_agent"';

    server {
      listen 8080;
      access_log /var/log/nginx/release-access.log release;

      root /usr/share/nginx/html;

      location /release/ {
        autoindex on;
      }
    }
```

### 7.2 查看下载日志

```bash
# 获取 Pod 名称
POD=$(kubectl get pods -n capbm-system -l app=release-http-server -o jsonpath='{.items[0].metadata.name}')

# 查看访问日志
kubectl logs -n capbm-system $POD | grep "GET /release/"

# 统计下载次数
kubectl logs -n capbm-system $POD | grep "GET /release/" | wc -l
```

---

## 8. 故障排查

### 8.1 验证 HTTP 服务器可达

```bash
# 从管理集群内测试
kubectl run -it --rm test-curl --image=curlimages/curl --restart=Never \
  -- curl -v http://release-server.capbm-system.svc.cluster.local:8080/release/v1.31.1/binaries/

# 从裸金属节点测试
curl -v http://release-server.capbm-system.svc.cluster.local:8080/release/v1.31.1/binaries/
```

### 8.2 常见问题

| 问题 | 可能原因 | 解决方案 |
|------|----------|----------|
| 404 Not Found | 路径配置错误 | 检查 `baseUrl` 和内容目录结构 |
| 403 Forbidden | 权限问题 | 检查 Nginx 配置和文件权限 |
| 连接超时 | 网络不通 | 检查 Service 和 NetworkPolicy |
| 下载失败 | 内容不完整 | 重新上传 release-image 内容 |
| TLS 错误 | 证书不匹配 | 检查证书域名或使用 `insecureSkipVerify: true` |

### 8.3 检查 HTTP 服务器状态

```bash
# 检查 Pod 状态
kubectl get pods -n capbm-system -l app=release-http-server

# 检查 Service
kubectl get svc -n capbm-system release-http-server

# 检查 Endpoints
kubectl get endpoints -n capbm-system release-http-server

# 查看 Pod 日志
kubectl logs -n capbm-system -l app=release-http-server
```

---

## 9. 完整部署示例

### 9.1 一键部署脚本

```bash
#!/bin/bash
set -e

NAMESPACE="capbm-system"
RELEASE_VERSION="v1.31.1"
RELEASE_DIR="release-image"

echo "=== ReleaseImage HTTP Server 部署脚本 ==="

# 1. 检查 release-image 目录
if [ ! -d "${RELEASE_DIR}" ]; then
    echo "错误: ${RELEASE_DIR} 目录不存在"
    echo "请先运行: ./scripts/build-release-image-no-docker.sh"
    exit 1
fi

# 2. 创建 PVC
echo "[1/5] 创建 PersistentVolumeClaim..."
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: release-content-pvc
  namespace: ${NAMESPACE}
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
EOF

# 3. 部署 HTTP 服务器
echo "[2/5] 部署 HTTP 服务器..."
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-http-server
  namespace: ${NAMESPACE}
  labels:
    app: release-http-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: release-http-server
  template:
    metadata:
      labels:
        app: release-http-server
    spec:
      containers:
        - name: nginx
          image: nginx:1.25-alpine
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: release-content
              mountPath: /usr/share/nginx/html/release
      volumes:
        - name: release-content
          persistentVolumeClaim:
            claimName: release-content-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: release-http-server
  namespace: ${NAMESPACE}
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
  selector:
    app: release-http-server
EOF

# 4. 等待 Pod 就绪
echo "[3/5] 等待 Pod 就绪..."
kubectl wait --for=condition=Ready pod -n ${NAMESPACE} -l app=release-http-server --timeout=120s

# 5. 上传内容
echo "[4/5] 上传 release-image 内容..."
POD=$(kubectl get pods -n ${NAMESPACE} -l app=release-http-server -o jsonpath='{.items[0].metadata.name}')
kubectl cp ${RELEASE_DIR}/ ${NAMESPACE}/${POD}:/usr/share/nginx/html/release/${RELEASE_VERSION}

# 6. 验证
echo "[5/5] 验证部署..."
kubectl run -it --rm test-curl --image=curlimages/curl --restart=Never -- \
  curl -s http://release-http-server.${NAMESPACE}.svc.cluster.local:8080/release/${RELEASE_VERSION}/binaries/

echo ""
echo "=== 部署完成 ==="
echo ""
echo "ReleaseImage 配置示例："
echo ""
cat <<EOF
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-31-1
  namespace: ${NAMESPACE}
spec:
  version: ${RELEASE_VERSION}
  httpServer:
    enabled: true
    baseUrl: http://release-http-server.${NAMESPACE}.svc.cluster.local:8080/release/${RELEASE_VERSION}
EOF
```

### 9.2 使用脚本

```bash
chmod +x deploy-release-http-server.sh
./deploy-release-http-server.sh
```

---

## 10. 参考文档

| 文档 | 说明 |
|------|------|
| [ReleaseImage 目录规范](./release-image-directory-spec.md) | OCI 镜像目录结构和文件命名规范 |
| [ReleaseImage 安装指南](./release-image-install-guide.md) | 使用 ReleaseImage 安装集群 |
| [GHCR 认证指南](./ghcr-auth-guide.md) | 镜像拉取认证 |
| [自升级方案设计](./self-upgrade-design.md) | 管理集群自升级设计 |
