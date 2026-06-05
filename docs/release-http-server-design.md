# ReleaseImage HTTP Server 设计文档

## 1. 概述

### 1.1 设计目标

在引导集群（Management Cluster）中部署 Nginx HTTP Server，为裸金属节点提供 ReleaseImage 内容下载服务，支持离线/air-gapped 环境安装。

### 1.2 核心功能

| 功能 | 说明 |
|------|------|
| **HTTP 服务** | 提供 release-image 目录的 HTTP 访问 |
| **自动加载** | InitContainer 从 OCI 镜像提取内容到共享卷 |
| **零配置** | 部署后自动可用，无需手动上传文件 |
| **集群内访问** | 通过 Service 提供集群内 DNS 访问 |

### 1.3 架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Management Cluster                           │
│                                                                     │
│  ┌─────────────────┐    ┌──────────────────────────────────────┐   │
│  │  ReleaseImage   │    │  release-http-server Deployment      │   │
│  │  (CRD)          │    │                                      │   │
│  │  httpServer:    │    │  ┌────────────────────────────────┐  │   │
│  │    baseUrl:     │───▶│  │ InitContainer: content-loader  │  │   │
│  │    http://...   │    │  │ image: capbm/release:v1.31.1   │  │   │
│  └─────────────────┘    │  │ cp -r /release/* /content/     │  │   │
│                         │  └────────────┬───────────────────┘  │   │
│                         │               │                      │   │
│                         │  ┌────────────▼───────────────────┐  │   │
│                         │  │ Container: nginx               │  │   │
│                         │  │ image: nginx:1.25-alpine       │  │   │
│                         │  │ port: 8080                     │  │   │
│                         │  │ root: /usr/share/nginx/html    │  │   │
│                         │  └────────────────────────────────┘  │   │
│                         │               │                      │   │
│                         │  ┌────────────▼───────────────────┐  │   │
│                         │  │ Volume: PVC                    │  │   │
│                         │  │ claimName: release-content-pvc │  │   │
│                         │  └────────────────────────────────┘  │   │
│                         └──────────────────────────────────────┘   │
│                                         │                          │
│                              ┌──────────▼──────────┐               │
│                              │ Service             │               │
│                              │ release-http-server │               │
│                              │ :8080               │               │
│                              └──────────┬──────────┘               │
└─────────────────────────────────────────┼──────────────────────────┘
                                          │
                                          │ HTTP 下载
                                          ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        BareMetal Nodes                              │
│                                                                     │
│  CAPBM Installer ──▶ curl http://release-http-server:8080/release/  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 2. 组件设计

### 2.1 PersistentVolumeClaim

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
```

| 字段 | 值 | 说明 |
|------|-----|------|
| `accessModes` | `ReadWriteOnce` | 单节点读写，足够单副本 HTTP 服务器 |
| `storage` | `10Gi` | 根据 release-image 内容大小调整 |

### 2.2 Deployment

#### 2.2.1 InitContainer: content-loader

```yaml
initContainers:
  - name: content-loader
    image: registry.example.com/capbm/release:v1.31.1
    command: ["sh", "-c", "cp -r /release/* /content/"]
    volumeMounts:
      - name: content
        mountPath: /content
```

| 字段 | 值 | 说明 |
|------|-----|------|
| `image` | `capbm/release:v{version}` | ReleaseImage OCI 镜像 |
| `command` | `cp -r /release/* /content/` | 从镜像提取内容到共享卷 |
| `volumeMounts` | `/content` | 共享卷挂载点 |

#### 2.2.2 Container: nginx

```yaml
containers:
  - name: nginx
    image: nginx:1.25-alpine
    ports:
      - containerPort: 8080
    volumeMounts:
      - name: content
        mountPath: /usr/share/nginx/html/release
        readOnly: true
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
      limits:
        cpu: 200m
        memory: 128Mi
```

| 字段 | 值 | 说明 |
|------|-----|------|
| `image` | `nginx:1.25-alpine` | 轻量级 Nginx 镜像 |
| `ports` | `8080` | HTTP 服务端口 |
| `volumeMounts` | `/usr/share/nginx/html/release` | 只读挂载内容卷 |
| `resources` | `50m-200m CPU, 64Mi-128Mi memory` | 资源限制 |

### 2.3 Service

```yaml
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

| 字段 | 值 | 说明 |
|------|-----|------|
| `type` | `ClusterIP` | 集群内访问 |
| `port` | `8080` | 服务端口 |
| `selector` | `app: release-http-server` | 选择 Deployment Pod |

---

## 3. 部署流程

```
用户执行 make deploy-release-http-server
                │
                ▼
┌─────────────────────────────────────────────────┐
│ Step 1: 创建 PVC                                │
│                                                 │
│ kubectl apply -f release-content-pvc.yaml       │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Step 2: 部署 HTTP Server                        │
│                                                 │
│ kubectl apply -f release-http-server.yaml       │
│                                                 │
│ InitContainer 启动:                             │
│   - 拉取 ReleaseImage OCI 镜像                   │
│   - 提取 /release/* 到 /content/                 │
│   - 完成后退出                                   │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Step 3: Nginx 启动                              │
│                                                 │
│ - 挂载 /content/ 到 /usr/share/nginx/html/release│
│ - 监听 8080 端口                                 │
│ - 提供 HTTP 服务                                 │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│ Step 4: 验证                                    │
│                                                 │
│ curl http://release-http-server:8080/release/   │
└─────────────────────────────────────────────────┘
```

---

## 4. 配置示例

### 4.1 ReleaseImage 配置

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1-31-1
  namespace: capbm-system
spec:
  version: v1.31.1
  image: registry.example.com/capbm/release:v1.31.1
  httpServer:
    enabled: true
    port: 8080
    basePath: /release/v1.31.1
    baseUrl: http://release-http-server.capbm-system.svc.cluster.local:8080/release/v1.31.1
```

### 4.2 BareMetalMachine 配置

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachine
metadata:
  name: worker-1
spec:
  installSource:
    type: HTTPServer
    httpServerConfig:
      baseUrl: http://release-http-server.capbm-system.svc.cluster.local:8080/release/v1.31.1
```

---

## 5. 实现计划

### 5.1 文件清单

| 文件 | 说明 |
|------|------|
| `templates/release-http-server.yaml` | Nginx HTTP Server 完整部署清单 |
| `templates/release-content-pvc.yaml` | PVC 清单（可选，已包含在上方） |
| `Makefile` | 新增 `deploy-release-http-server` 目标 |

### 5.2 Makefile 目标

```makefile
.PHONY: deploy-release-http-server
deploy-release-http-server: ## Deploy ReleaseImage HTTP Server
	kubectl apply -f templates/release-http-server.yaml

.PHONY: undeploy-release-http-server
undeploy-release-http-server: ## Undeploy ReleaseImage HTTP Server
	kubectl delete -f templates/release-http-server.yaml
```

---

## 6. 故障排查

### 6.1 InitContainer 失败

```bash
# 查看 InitContainer 日志
kubectl logs -n capbm-system deployment/release-http-server -c content-loader

# 常见问题:
# - 镜像拉取失败: 检查 image 和 imagePullSecrets
# - 权限错误: 检查 volume 挂载权限
# - 空间不足: 检查 PVC 大小
```

### 6.2 Nginx 启动失败

```bash
# 查看 Nginx 日志
kubectl logs -n capbm-system deployment/release-http-server -c nginx

# 常见问题:
# - 目录不存在: InitContainer 未完成
# - 权限错误: 检查 readOnly 挂载
```

### 6.3 验证 HTTP 服务

```bash
# 从集群内测试
kubectl run -it --rm test-curl --image=curlimages/curl --restart=Never -- \
  curl -s http://release-http-server.capbm-system.svc.cluster.local:8080/release/v1.31.1/binaries/

# 查看目录结构
kubectl run -it --rm test-curl --image=curlimages/curl --restart=Never -- \
  curl -s http://release-http-server.capbm-system.svc.cluster.local:8080/release/v1.31.1/
```

---

## 7. 扩展设计

### 7.1 高可用部署

```yaml
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
```

需要 ReadWriteMany PV 支持多副本共享内容。

### 7.2 TLS 加密

添加 TLS Secret 和 Nginx SSL 配置，提供 HTTPS 服务。

### 7.3 基本认证

添加 Nginx auth_basic 配置，保护 HTTP 服务器访问。

---

## 8. 参考文档

| 文档 | 说明 |
|------|------|
| [ReleaseImage 目录规范](./release-image-directory-spec.md) | OCI 镜像目录结构和文件命名规范 |
| [ReleaseImage 安装指南](./release-image-install-guide.md) | 使用 ReleaseImage 安装集群 |
| [HTTP 服务器部署指南](./release-http-server-guide.md) | HTTP 服务器部署详细指南 |
