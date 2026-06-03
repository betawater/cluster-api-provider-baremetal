# ReleaseImage 完整设计方案

## 一、概述

### 1.1 设计目标

ReleaseImage 是 CAPBM 项目的核心 CRD，用于定义一个完整的 Kubernetes 发行版本，包括：

| 目标 | 说明 |
|------|------|
| **版本管理** | 定义集群所有组件的版本映射 |
| **升级编排** | 定义升级顺序、依赖关系和策略 |
| **高内聚配置** | 每个组件自带安装、升级、备份、回滚配置 |
| **多架构支持** | 支持 amd64、arm64 等多种架构 |
| **多 OS 支持** | 支持 Ubuntu、CentOS、Rocky 等操作系统 |
| **离线支持** | 支持 air-gapped 环境的离线安装 |

### 1.2 API 信息

| 属性 | 值 |
|------|-----|
| **API Group** | `cvo.capbm.io` |
| **Version** | `v1beta1` |
| **Kind** | `ReleaseImage` |
| **Scope** | Cluster (集群级别) |
| **Storage** | etcd |

---

## 二、CRD 结构

### 2.1 顶层结构

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
  labels:
    capbm.io/release-channel: stable
spec:
  version: v1.31.1
  image: registry.example.com/capbm/release:v1.31.1
  httpServer: {...}
  imageRegistry: {...}
  channels: [stable, fast]
  previousVersions: [v1.31.0, v1.30.0]
  components: {...}
  addons: [...]
  upgradeGraph: [...]
  contentHash: sha256:abc123...
status:
  verified: true
  manifestCount: 15
  imagesImported: true
  importJobName: release-image-import-v1.31.1
  importStatus: Completed
  importMessage: All images imported successfully
  importedImages: [...]
```

### 2.2 Spec 字段详解

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `version` | string | 是 | 发行版本号 (如 v1.31.1) |
| `image` | string | 是 | OCI 镜像地址 |
| `httpServer` | HTTPServerConfig | 否 | HTTP 服务器配置 |
| `imageRegistry` | ImageRegistryConfig | 否 | 镜像仓库配置 |
| `channels` | []string | 否 | 发布通道 (stable, fast, etc.) |
| `previousVersions` | []string | 否 | 可升级的前置版本列表 |
| `components` | ReleaseComponentVersions | 是 | 二进制组件定义 |
| `addons` | []AddonDefinition | 否 | Addon 定义列表 |
| `upgradeGraph` | []UpgradePhase | 是 | 升级阶段定义 |
| `contentHash` | string | 否 | 内容校验和 |

---

## 三、组件定义

### 3.1 二进制组件 (ReleaseComponentVersions)

```yaml
components:
  kubernetes:
    version: v1.31.1
    type: binary
    path: /opt/capbm/binaries/kubernetes
    platforms:
      ubuntu:
        architectures: [amd64, arm64]
        packages:
          kubeadm: kubeadm_1.31.1-00
          kubelet: kubelet_1.31.1-00
          kubectl: kubectl_1.31.1-00
      centos:
        architectures: [amd64]
        packages:
          kubeadm: kubeadm-1.31.1-0
          kubelet: kubelet-1.31.1-0
          kubectl: kubectl-1.31.1-0
    imageList:
      - registry.k8s.io/kube-apiserver:v1.31.1
      - registry.k8s.io/kube-controller-manager:v1.31.1
      - registry.k8s.io/kube-scheduler:v1.31.1
      - registry.k8s.io/kube-proxy:v1.31.1
      - registry.k8s.io/pause:3.9
      - registry.k8s.io/etcd:3.5.15-0
      - registry.k8s.io/coredns/coredns:v1.11.1
    installStrategy:
      timeout: 600s
      retryCount: 3
      method: package
      serviceName: kubelet
    upgradeStrategy:
      type: Rolling
      maxConcurrent: 1
      timeout: 900s
      retryCount: 3
      drain: true
    preHooks:
      - name: drain-node
        command: kubectl drain {{.NodeName}} --ignore-daemonsets --delete-emptydir-data
        timeout: 300s
        onFailure: Abort
    postHooks:
      - name: uncordon-node
        command: kubectl uncordon {{.NodeName}}
        timeout: 30s
        onFailure: Abort
    upgrade:
      backup:
        enabled: true
        config:
          - path: /etc/kubernetes
            type: directory
        etcdSnapshot: true
      rollback:
        script: scripts/rollback-kubernetes.sh
        timeout: 600s
      healthCheck:
        command: kubectl get nodes
        timeout: 60s
        retries: 3

  containerd:
    version: 1.7.24
    type: binary
    path: /opt/capbm/binaries/containerd
    architectures: [amd64, arm64]
    files:
      archive: containerd-1.7.24-linux-amd64.tar.gz
    installStrategy:
      timeout: 300s
      retryCount: 3
      method: archive
      serviceName: containerd
    upgradeStrategy:
      type: Rolling
      maxConcurrent: 1
      timeout: 600s
      retryCount: 3
      drain: false
    upgrade:
      backup:
        enabled: true
        config:
          - path: /etc/containerd/config.toml
            type: file
      rollback:
        script: scripts/rollback-containerd.sh
        timeout: 300s
      healthCheck:
        command: systemctl is-active containerd
        timeout: 30s
        retries: 3

  helm:
    version: 3.15.0
    type: binary
    path: /opt/capbm/binaries/helm
    architectures: [amd64, arm64]
    files:
      archive: helm-v3.15.0-linux-amd64.tar.gz

  cniPlugins:
    version: 1.5.0
    type: binary
    path: /opt/capbm/binaries/cni-plugins
    architectures: [amd64, arm64]
    files:
      archive: cni-plugins-linux-amd64-v1.5.0.tgz
```

### 3.2 Addon 定义

```yaml
addons:
  - name: calico
    type: helm
    version: v3.28.1
    contentPath: charts/calico-v3.28.1.tgz
    namespace: kube-system
    dependencies: []
    variables:
      - name: podCIDR
        type: string
        description: Pod network CIDR
        required: true
      - name: ipam
        type: string
        description: IPAM mode (calico-ipam, host-local)
        required: false
        default: calico-ipam
        enum: [calico-ipam, host-local]
    defaultValues:
      ipam: calico-ipam
      typhaReplicas: 1
    installStrategy:
      timeout: 300s
      retryCount: 3
      createNamespace: true
      wait: true
    upgradeStrategy:
      type: Rolling
      maxUnavailable: 0
      timeout: 300s
      retryCount: 3
    preHooks:
      - name: backup-calico-config
        command: kubectl get configmap calico-config -n kube-system -o yaml > /tmp/calico-config-backup.yaml
        timeout: 30s
        onFailure: Abort
    postHooks:
      - name: verify-calico-pods
        command: kubectl wait --for=condition=Ready pods -n kube-system -l k8s-app=calico-node --timeout=120s
        timeout: 120s
        onFailure: Abort
    upgrade:
      backup:
        enabled: true
        config:
          - path: /etc/cni/net.d
            type: directory
      rollback:
        script: scripts/rollback-calico.sh
        timeout: 300s
      healthCheck:
        command: kubectl get pods -n kube-system -l k8s-app=calico-node
        timeout: 60s
        retries: 3

  - name: ceph-csi
    type: helm
    version: v3.11.0
    contentPath: charts/ceph-csi-rbd-v3.11.0.tgz
    namespace: ceph-csi
    dependencies: [calico]
    variables:
      - name: clusterID
        type: string
        description: Ceph cluster ID
        required: true
      - name: monitors
        type: string
        description: Ceph monitor addresses
        required: true
    installStrategy:
      timeout: 300s
      retryCount: 3
      createNamespace: true
      wait: true
    upgrade:
      backup:
        enabled: true
        config:
          - path: /etc/csi
            type: directory
      rollback:
        script: scripts/rollback-ceph-csi.sh
        timeout: 300s
      healthCheck:
        command: kubectl get pods -n ceph-csi -l app=ceph-csi-rbdplugin
        timeout: 60s
        retries: 3

  - name: capi-core-controller
    type: helm
    version: v1.7.0
    contentPath: charts/capi-core-controller-v1.7.0.tgz
    namespace: capi-system
    dependencies: []
    installStrategy:
      timeout: 300s
      retryCount: 3
      createNamespace: true
      wait: true
    upgrade:
      backup:
        enabled: true
        config:
          - path: /etc/capi-core-controller
            type: directory
      rollback:
        script: scripts/rollback-capi-core.sh
        timeout: 300s
      healthCheck:
        command: kubectl get deployment capi-controller-manager -n capi-system
        timeout: 60s
        retries: 3
```

### 3.3 升级图 (UpgradeGraph)

```yaml
upgradeGraph:
  - name: phase-1-runtime
    order: 1
    blocking: true
    rollingUpdate:
      maxUnavailable: 1
    components:
      - name: containerd
        blocking: true
        dependsOn: []
        manifests: []
        scripts:
          - scripts/upgrade-containerd.sh
        healthCheck:
          type: ServiceRunning
          name: containerd
          timeout: 30s

  - name: phase-2-kubernetes
    order: 2
    blocking: true
    rollingUpdate:
      maxUnavailable: 1
    components:
      - name: kubernetes
        blocking: true
        dependsOn: [containerd]
        manifests: []
        scripts:
          - scripts/upgrade-kubernetes.sh
        healthCheck:
          type: DeploymentReady
          namespace: kube-system
          name: kube-apiserver
          timeout: 60s

  - name: phase-3-addons
    order: 3
    blocking: false
    components:
      - name: calico
        blocking: true
        dependsOn: [kubernetes]
        manifests: []
        scripts: []
        healthCheck:
          type: DaemonSetReady
          namespace: kube-system
          name: calico-node
          timeout: 120s

      - name: ceph-csi
        blocking: false
        dependsOn: [calico]
        manifests: []
        scripts: []
        healthCheck:
          type: DeploymentReady
          namespace: ceph-csi
          name: ceph-csi-rbdplugin-provisioner
          timeout: 120s

      - name: capi-core-controller
        blocking: true
        dependsOn: [kubernetes]
        manifests: []
        scripts: []
        healthCheck:
          type: DeploymentReady
          namespace: capi-system
          name: capi-controller-manager
          timeout: 60s
```

---

## 四、策略配置详解

### 4.1 安装策略 (InstallStrategy)

#### 二进制组件安装策略

```yaml
installStrategy:
  timeout: 600s              # 安装超时时间
  retryCount: 3              # 失败重试次数
  method: package            # 安装方法: package, archive, manual
  serviceName: kubelet       # 安装后需要重启的服务名
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `timeout` | Duration | 300s | 安装超时时间 |
| `retryCount` | int | 3 | 失败重试次数 |
| `method` | enum | package | 安装方法: package (apt/yum), archive (tar.gz), manual (自定义脚本) |
| `serviceName` | string | - | 安装后需要重启的系统服务名 |

#### Addon 安装策略

```yaml
installStrategy:
  timeout: 300s              # 安装超时时间
  retryCount: 3              # 失败重试次数
  createNamespace: true      # 是否自动创建 namespace
  wait: true                 # 是否等待资源就绪
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `timeout` | Duration | 300s | 安装超时时间 |
| `retryCount` | int | 3 | 失败重试次数 |
| `createNamespace` | bool | true | 是否自动创建 namespace |
| `wait` | bool | true | 是否等待资源就绪 (Helm --wait) |

### 4.2 升级策略 (UpgradeStrategy)

#### 二进制组件升级策略

```yaml
upgradeStrategy:
  type: Rolling              # 升级类型: Rolling, DrainAndUpgrade, Parallel
  maxConcurrent: 1           # 最大并发升级节点数
  timeout: 900s              # 单节点升级超时
  retryCount: 3              # 失败重试次数
  force: false               # 是否强制升级 (忽略版本路径检查)
  drain: true                # 升级前是否驱逐 Pod
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | enum | Rolling | 升级类型: Rolling (滚动), DrainAndUpgrade (驱逐后升级), Parallel (并行) |
| `maxConcurrent` | int | 1 | 最大并发升级节点数 |
| `timeout` | Duration | 600s | 单节点升级超时时间 |
| `retryCount` | int | 3 | 失败重试次数 |
| `force` | bool | false | 是否强制升级 (忽略版本路径检查) |
| `drain` | bool | false | 升级前是否驱逐 Pod |

#### Addon 升级策略

```yaml
upgradeStrategy:
  type: Rolling              # 升级类型: Rolling, Recreate, BlueGreen
  rollingUpdate:
    maxSurge: 1              # 最大可超出期望的 Pod 数
    partition: 0             # 滚动更新起始序号 (用于 StatefulSet)
  maxUnavailable: 1          # 最大不可用 Pod 数
  timeout: 300s              # 升级超时时间
  retryCount: 3              # 失败重试次数
  force: false               # 是否强制升级
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | enum | Rolling | 升级类型: Rolling (滚动), Recreate (重建), BlueGreen (蓝绿) |
| `rollingUpdate.maxSurge` | int | 1 | 最大可超出期望的 Pod 数 |
| `rollingUpdate.partition` | int | 0 | 滚动更新起始序号 (用于 StatefulSet) |
| `maxUnavailable` | int | 1 | 最大不可用 Pod 数 |
| `timeout` | Duration | 300s | 升级超时时间 |
| `retryCount` | int | 3 | 失败重试次数 |
| `force` | bool | false | 是否强制升级 |

### 4.3 升级配置 (UpgradeConfig)

```yaml
upgrade:
  backup:
    enabled: true
    config:
      - path: /etc/kubernetes
        type: directory
      - path: /etc/containerd/config.toml
        type: file
    etcdSnapshot: true
  rollback:
    script: scripts/rollback-kubernetes.sh
    timeout: 600s
  healthCheck:
    command: kubectl get nodes
    timeout: 60s
    retries: 3
```

#### 备份配置 (Backup)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | true | 是否启用备份 |
| `config` | []BackupItem | - | 需要备份的配置项列表 |
| `config[].path` | string | - | 文件或目录路径 |
| `config[].type` | enum | - | 类型: file, directory |
| `etcdSnapshot` | bool | false | 是否需要 etcd 快照 (控制面组件) |

#### 回滚配置 (Rollback)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `script` | string | - | 回滚脚本路径 (相对于 ReleaseImage scripts 目录) |
| `timeout` | Duration | 300s | 回滚超时时间 |

#### 健康检查配置 (HealthCheck)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `command` | string | - | 健康检查命令 |
| `timeout` | Duration | 60s | 单次检查超时时间 |
| `retries` | int | 3 | 重试次数 |

### 4.4 Hooks 配置

```yaml
preHooks:
  - name: drain-node
    command: kubectl drain {{.NodeName}} --ignore-daemonsets --delete-emptydir-data
    timeout: 300s
    onFailure: Abort
postHooks:
  - name: uncordon-node
    command: kubectl uncordon {{.NodeName}}
    timeout: 30s
    onFailure: Abort
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `name` | string | - | Hook 名称 |
| `command` | string | - | 执行的命令 (支持模板变量) |
| `timeout` | Duration | 60s | 执行超时时间 |
| `onFailure` | enum | Abort | 失败处理: Continue (继续), Abort (中止), Ignore (忽略) |

---

## 五、OCI 镜像结构

### 5.1 镜像目录结构

```
release:v1.31.1/
├── release.json                  # ReleaseImage spec JSON
├── catalog.json                  # 版本目录 (可选)
├── upgrade-path.json             # 升级路径 (可选)
│
├── binaries/
│   ├── kubernetes/
│   │   ├── ubuntu/
│   │   │   ├── amd64/
│   │   │   │   ├── kubeadm_1.31.1-00_amd64.deb
│   │   │   │   ├── kubelet_1.31.1-00_amd64.deb
│   │   │   │   └── kubectl_1.31.1-00_amd64.deb
│   │   │   └── arm64/
│   │   │       ├── kubeadm_1.31.1-00_arm64.deb
│   │   │       ├── kubelet_1.31.1-00_arm64.deb
│   │   │       └── kubectl_1.31.1-00_arm64.deb
│   │   └── centos/
│   │       └── amd64/
│   │           ├── kubeadm-1.31.1-0.x86_64.rpm
│   │           ├── kubelet-1.31.1-0.x86_64.rpm
│   │           └── kubectl-1.31.1-0.x86_64.rpm
│   │
│   ├── containerd/
│   │   ├── containerd-1.7.24-linux-amd64.tar.gz
│   │   └── containerd-1.7.24-linux-arm64.tar.gz
│   │
│   ├── helm/
│   │   └── helm-v3.15.0-linux-amd64.tar.gz
│   │
│   └── cni-plugins/
│       └── cni-plugins-linux-amd64-v1.5.0.tgz
│
├── images/
│   ├── kube-apiserver_v1.31.1.tar
│   ├── kube-controller-manager_v1.31.1.tar
│   ├── kube-scheduler_v1.31.1.tar
│   ├── kube-proxy_v1.31.1.tar
│   ├── pause_3.9.tar
│   ├── etcd_3.5.15-0.tar
│   └── coredns_v1.11.1.tar
│
├── charts/
│   ├── calico-v3.28.1.tgz
│   ├── ceph-csi-rbd-v3.11.0.tgz
│   └── capi-core-controller-v1.7.0.tgz
│
├── manifests/
│   ├── calico-manifests-v3.28.1/
│   │   ├── tigera-operator.yaml
│   │   └── custom-resources.yaml
│   └── csi-manifests/
│       └── ceph-csi-rbd.yaml
│
├── scripts/
│   ├── upgrade-containerd.sh
│   ├── upgrade-kubernetes.sh
│   ├── rollback-containerd.sh
│   ├── rollback-kubernetes.sh
│   ├── rollback-calico.sh
│   └── rollback-ceph-csi.sh
│
└── checksums/
    ├── sha256sums.txt
    └── sha256sums.txt.sig
```

### 5.2 release.json 示例

```json
{
  "version": "v1.31.1",
  "image": "registry.example.com/capbm/release:v1.31.1",
  "httpServer": {
    "enabled": true,
    "port": 8080,
    "basePath": "/release/v1.31.1"
  },
  "imageRegistry": {
    "enabled": true,
    "registry": "registry.example.com",
    "repository": "capbm",
    "imagePrefix": "release"
  },
  "channels": ["stable", "fast"],
  "previousVersions": ["v1.31.0", "v1.30.0"],
  "components": {
    "kubernetes": {
      "version": "v1.31.1",
      "type": "binary",
      "path": "/opt/capbm/binaries/kubernetes",
      "platforms": {
        "ubuntu": {
          "architectures": ["amd64", "arm64"],
          "packages": {
            "kubeadm": "kubeadm_1.31.1-00",
            "kubelet": "kubelet_1.31.1-00",
            "kubectl": "kubectl_1.31.1-00"
          }
        }
      },
      "imageList": [
        "registry.k8s.io/kube-apiserver:v1.31.1",
        "registry.k8s.io/kube-controller-manager:v1.31.1",
        "registry.k8s.io/kube-scheduler:v1.31.1",
        "registry.k8s.io/kube-proxy:v1.31.1",
        "registry.k8s.io/pause:3.9",
        "registry.k8s.io/etcd:3.5.15-0",
        "registry.k8s.io/coredns/coredns:v1.11.1"
      ],
      "installStrategy": {
        "timeout": "600s",
        "retryCount": 3,
        "method": "package",
        "serviceName": "kubelet"
      },
      "upgradeStrategy": {
        "type": "Rolling",
        "maxConcurrent": 1,
        "timeout": "900s",
        "retryCount": 3,
        "drain": true
      },
      "upgrade": {
        "backup": {
          "enabled": true,
          "config": [
            {"path": "/etc/kubernetes", "type": "directory"}
          ],
          "etcdSnapshot": true
        },
        "rollback": {
          "script": "scripts/rollback-kubernetes.sh",
          "timeout": "600s"
        },
        "healthCheck": {
          "command": "kubectl get nodes",
          "timeout": "60s",
          "retries": 3
        }
      }
    },
    "containerd": {
      "version": "1.7.24",
      "type": "binary",
      "path": "/opt/capbm/binaries/containerd",
      "architectures": ["amd64", "arm64"],
      "files": {
        "archive": "containerd-1.7.24-linux-amd64.tar.gz"
      },
      "installStrategy": {
        "timeout": "300s",
        "retryCount": 3,
        "method": "archive",
        "serviceName": "containerd"
      },
      "upgradeStrategy": {
        "type": "Rolling",
        "maxConcurrent": 1,
        "timeout": "600s",
        "retryCount": 3,
        "drain": false
      },
      "upgrade": {
        "backup": {
          "enabled": true,
          "config": [
            {"path": "/etc/containerd/config.toml", "type": "file"}
          ]
        },
        "rollback": {
          "script": "scripts/rollback-containerd.sh",
          "timeout": "300s"
        },
        "healthCheck": {
          "command": "systemctl is-active containerd",
          "timeout": "30s",
          "retries": 3
        }
      }
    }
  },
  "addons": [
    {
      "name": "calico",
      "type": "helm",
      "version": "v3.28.1",
      "contentPath": "charts/calico-v3.28.1.tgz",
      "namespace": "kube-system",
      "dependencies": [],
      "installStrategy": {
        "timeout": "300s",
        "retryCount": 3,
        "createNamespace": true,
        "wait": true
      },
      "upgrade": {
        "backup": {
          "enabled": true,
          "config": [
            {"path": "/etc/cni/net.d", "type": "directory"}
          ]
        },
        "rollback": {
          "script": "scripts/rollback-calico.sh",
          "timeout": "300s"
        },
        "healthCheck": {
          "command": "kubectl get pods -n kube-system -l k8s-app=calico-node",
          "timeout": "60s",
          "retries": 3
        }
      }
    }
  ],
  "upgradeGraph": [
    {
      "name": "phase-1-runtime",
      "order": 1,
      "blocking": true,
      "rollingUpdate": {
        "maxUnavailable": 1
      },
      "components": [
        {
          "name": "containerd",
          "blocking": true,
          "dependsOn": [],
          "scripts": ["scripts/upgrade-containerd.sh"],
          "healthCheck": {
            "type": "ServiceRunning",
            "name": "containerd",
            "timeout": "30s"
          }
        }
      ]
    },
    {
      "name": "phase-2-kubernetes",
      "order": 2,
      "blocking": true,
      "rollingUpdate": {
        "maxUnavailable": 1
      },
      "components": [
        {
          "name": "kubernetes",
          "blocking": true,
          "dependsOn": ["containerd"],
          "scripts": ["scripts/upgrade-kubernetes.sh"],
          "healthCheck": {
            "type": "DeploymentReady",
            "namespace": "kube-system",
            "name": "kube-apiserver",
            "timeout": "60s"
          }
        }
      ]
    },
    {
      "name": "phase-3-addons",
      "order": 3,
      "blocking": false,
      "components": [
        {
          "name": "calico",
          "blocking": true,
          "dependsOn": ["kubernetes"],
          "healthCheck": {
            "type": "DaemonSetReady",
            "namespace": "kube-system",
            "name": "calico-node",
            "timeout": "120s"
          }
        }
      ]
    }
  ],
  "contentHash": "sha256:abc123def456..."
}
```

---

## 六、完整 YAML 示例

### 6.1 标准 ReleaseImage

```yaml
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
  labels:
    capbm.io/release-channel: stable
    capbm.io/kubernetes-version: "1.31"
spec:
  version: v1.31.1
  image: registry.example.com/capbm/release:v1.31.1
  
  httpServer:
    enabled: true
    port: 8080
    basePath: /release/v1.31.1
  
  imageRegistry:
    enabled: true
    registry: registry.example.com
    repository: capbm
    imagePrefix: release
    credentialsSecret: registry-credentials
    insecureSkipVerify: false
  
  channels:
    - stable
    - fast
  
  previousVersions:
    - v1.31.0
    - v1.30.0
  
  components:
    kubernetes:
      version: v1.31.1
      type: binary
      path: /opt/capbm/binaries/kubernetes
      platforms:
        ubuntu:
          architectures: [amd64, arm64]
          packages:
            kubeadm: kubeadm_1.31.1-00
            kubelet: kubelet_1.31.1-00
            kubectl: kubectl_1.31.1-00
        centos:
          architectures: [amd64]
          packages:
            kubeadm: kubeadm-1.31.1-0
            kubelet: kubelet-1.31.1-0
            kubectl: kubectl-1.31.1-0
      imageList:
        - registry.k8s.io/kube-apiserver:v1.31.1
        - registry.k8s.io/kube-controller-manager:v1.31.1
        - registry.k8s.io/kube-scheduler:v1.31.1
        - registry.k8s.io/kube-proxy:v1.31.1
        - registry.k8s.io/pause:3.9
        - registry.k8s.io/etcd:3.5.15-0
        - registry.k8s.io/coredns/coredns:v1.11.1
      installStrategy:
        timeout: 600s
        retryCount: 3
        method: package
        serviceName: kubelet
      upgradeStrategy:
        type: Rolling
        maxConcurrent: 1
        timeout: 900s
        retryCount: 3
        drain: true
      preHooks:
        - name: drain-node
          command: kubectl drain {{.NodeName}} --ignore-daemonsets --delete-emptydir-data
          timeout: 300s
          onFailure: Abort
      postHooks:
        - name: uncordon-node
          command: kubectl uncordon {{.NodeName}}
          timeout: 30s
          onFailure: Abort
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/kubernetes
              type: directory
          etcdSnapshot: true
        rollback:
          script: scripts/rollback-kubernetes.sh
          timeout: 600s
        healthCheck:
          command: kubectl get nodes
          timeout: 60s
          retries: 3

    containerd:
      version: 1.7.24
      type: binary
      path: /opt/capbm/binaries/containerd
      architectures: [amd64, arm64]
      files:
        archive: containerd-1.7.24-linux-amd64.tar.gz
      installStrategy:
        timeout: 300s
        retryCount: 3
        method: archive
        serviceName: containerd
      upgradeStrategy:
        type: Rolling
        maxConcurrent: 1
        timeout: 600s
        retryCount: 3
        drain: false
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/containerd/config.toml
              type: file
        rollback:
          script: scripts/rollback-containerd.sh
          timeout: 300s
        healthCheck:
          command: systemctl is-active containerd
          timeout: 30s
          retries: 3

    helm:
      version: 3.15.0
      type: binary
      path: /opt/capbm/binaries/helm
      architectures: [amd64, arm64]
      files:
        archive: helm-v3.15.0-linux-amd64.tar.gz

    cniPlugins:
      version: 1.5.0
      type: binary
      path: /opt/capbm/binaries/cni-plugins
      architectures: [amd64, arm64]
      files:
        archive: cni-plugins-linux-amd64-v1.5.0.tgz

  addons:
    - name: calico
      type: helm
      version: v3.28.1
      contentPath: charts/calico-v3.28.1.tgz
      namespace: kube-system
      dependencies: []
      variables:
        - name: podCIDR
          type: string
          description: Pod network CIDR
          required: true
        - name: ipam
          type: string
          description: IPAM mode
          required: false
          default: calico-ipam
          enum: [calico-ipam, host-local]
      defaultValues:
        ipam: calico-ipam
        typhaReplicas: 1
      installStrategy:
        timeout: 300s
        retryCount: 3
        createNamespace: true
        wait: true
      upgradeStrategy:
        type: Rolling
        maxUnavailable: 0
        timeout: 300s
        retryCount: 3
      preHooks:
        - name: backup-calico-config
          command: kubectl get configmap calico-config -n kube-system -o yaml > /tmp/calico-config-backup.yaml
          timeout: 30s
          onFailure: Abort
      postHooks:
        - name: verify-calico-pods
          command: kubectl wait --for=condition=Ready pods -n kube-system -l k8s-app=calico-node --timeout=120s
          timeout: 120s
          onFailure: Abort
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/cni/net.d
              type: directory
        rollback:
          script: scripts/rollback-calico.sh
          timeout: 300s
        healthCheck:
          command: kubectl get pods -n kube-system -l k8s-app=calico-node
          timeout: 60s
          retries: 3

    - name: ceph-csi
      type: helm
      version: v3.11.0
      contentPath: charts/ceph-csi-rbd-v3.11.0.tgz
      namespace: ceph-csi
      dependencies: [calico]
      variables:
        - name: clusterID
          type: string
          description: Ceph cluster ID
          required: true
        - name: monitors
          type: string
          description: Ceph monitor addresses
          required: true
      installStrategy:
        timeout: 300s
        retryCount: 3
        createNamespace: true
        wait: true
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/csi
              type: directory
        rollback:
          script: scripts/rollback-ceph-csi.sh
          timeout: 300s
        healthCheck:
          command: kubectl get pods -n ceph-csi -l app=ceph-csi-rbdplugin
          timeout: 60s
          retries: 3

    - name: capi-core-controller
      type: helm
      version: v1.7.0
      contentPath: charts/capi-core-controller-v1.7.0.tgz
      namespace: capi-system
      dependencies: []
      installStrategy:
        timeout: 300s
        retryCount: 3
        createNamespace: true
        wait: true
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/capi-core-controller
              type: directory
        rollback:
          script: scripts/rollback-capi-core.sh
          timeout: 300s
        healthCheck:
          command: kubectl get deployment capi-controller-manager -n capi-system
          timeout: 60s
          retries: 3

  upgradeGraph:
    - name: phase-1-runtime
      order: 1
      blocking: true
      rollingUpdate:
        maxUnavailable: 1
      components:
        - name: containerd
          blocking: true
          dependsOn: []
          scripts:
            - scripts/upgrade-containerd.sh
          healthCheck:
            type: ServiceRunning
            name: containerd
            timeout: 30s

    - name: phase-2-kubernetes
      order: 2
      blocking: true
      rollingUpdate:
        maxUnavailable: 1
      components:
        - name: kubernetes
          blocking: true
          dependsOn: [containerd]
          scripts:
            - scripts/upgrade-kubernetes.sh
          healthCheck:
            type: DeploymentReady
            namespace: kube-system
            name: kube-apiserver
            timeout: 60s

    - name: phase-3-addons
      order: 3
      blocking: false
      components:
        - name: calico
          blocking: true
          dependsOn: [kubernetes]
          healthCheck:
            type: DaemonSetReady
            namespace: kube-system
            name: calico-node
            timeout: 120s

        - name: ceph-csi
          blocking: false
          dependsOn: [calico]
          healthCheck:
            type: DeploymentReady
            namespace: ceph-csi
            name: ceph-csi-rbdplugin-provisioner
            timeout: 120s

        - name: capi-core-controller
          blocking: true
          dependsOn: [kubernetes]
          healthCheck:
            type: DeploymentReady
            namespace: capi-system
            name: capi-controller-manager
            timeout: 60s

  contentHash: sha256:abc123def456789...
```

---

## 七、使用场景

### 7.1 场景一：全新集群安装

```yaml
# 1. 创建 ReleaseImage
kubectl apply -f release-image-v1.31.1.yaml

# 2. 创建 ClusterVersion 触发安装
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
spec:
  clusterRef:
    name: my-cluster
    namespace: default
  desiredUpdate:
    version: v1.31.1
    image: registry.example.com/capbm/release:v1.31.1
```

### 7.2 场景二：集群升级 (K8S + Addon)

```yaml
# 用户修改 ClusterVersion 触发升级
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
spec:
  clusterRef:
    name: my-cluster
    namespace: default
  desiredUpdate:
    version: v1.31.1  # 从 v1.31.0 升级到 v1.31.1
```

**升级流程**:
1. CVO Controller 检测到版本变更
2. 验证升级路径 (UpgradePath)
3. 获取目标 ReleaseImage
4. Phase 1: K8S 升级 (containerd → kubernetes)
5. Phase 2: Addon 升级 (calico → ceph-csi → capi-core-controller)
6. 更新状态

### 7.3 场景三：仅 Addon 升级 (K8S 版本不变)

```yaml
# 用户修改 ClusterVersion (K8S 版本不变，但指向新 ReleaseImage)
apiVersion: cvo.capbm.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
spec:
  clusterRef:
    name: my-cluster
    namespace: default
  desiredUpdate:
    version: v1.31.0     # K8S 版本不变
    image: registry.example.com/capbm/release:v1.31.0-patch1  # 新 ReleaseImage
```

**升级流程**:
1. CVO Controller 检测到 image 变更
2. K8S 版本未变，跳过 K8S 升级
3. 比较 ReleaseImage.Addons 版本
4. 执行 Addon 升级

### 7.4 场景四：离线环境安装

```yaml
# ReleaseImage 配置离线 HTTP 服务器
apiVersion: cvo.capbm.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
spec:
  version: v1.31.1
  image: registry.example.com/capbm/release:v1.31.1
  
  httpServer:
    enabled: true
    port: 8080
    basePath: /release/v1.31.1
    baseURl: http://192.168.1.100:8080
    insecureSkipVerify: true
  
  imageRegistry:
    enabled: true
    registry: 192.168.1.100:5000
    repository: capbm
    insecureSkipVerify: true
```

---

## 八、验证规则

### 8.1 必填字段验证

| 字段 | 验证规则 |
|------|---------|
| `spec.version` | 非空，语义化版本格式 (vX.Y.Z) |
| `spec.image` | 非空，有效的 OCI 镜像地址 |
| `spec.components.kubernetes` | 非空，必须定义 |
| `spec.upgradeGraph` | 非空，至少一个阶段 |

### 8.2 组件验证

| 字段 | 验证规则 |
|------|---------|
| `components.kubernetes.platforms` | 至少一个 OS 平台 |
| `components.kubernetes.platforms[].packages` | 必须包含 kubeadm, kubelet, kubectl |
| `addons[].name` | 非空，唯一 |
| `addons[].type` | 必须是 manifest 或 helm |
| `addons[].contentPath` | 非空 |

### 8.3 升级图验证

| 字段 | 验证规则 |
|------|---------|
| `upgradeGraph[].order` | 正整数，唯一 |
| `upgradeGraph[].components[].name` | 必须在 components 或 addons 中定义 |
| `upgradeGraph[].components[].dependsOn` | 依赖的组件必须存在 |
| `upgradeGraph[].components[].dependsOn` | 不能有循环依赖 |

### 8.4 版本兼容性验证

| 规则 | 说明 |
|------|------|
| `previousVersions` | 列出的版本必须存在且可升级 |
| Kubernetes 版本倾斜 | 最大支持 ±1 minor 版本 |
| Addon 依赖顺序 | 按 dependencies 定义的顺序升级 |

---

## 九、状态管理

### 9.1 Status 字段

```yaml
status:
  verified: true                    # 是否已验证
  manifestCount: 15                 # Manifest 文件数量
  imagesImported: true              # 镜像是否已导入
  importJobName: release-image-import-v1.31.1  # 导入 Job 名称
  importStatus: Completed           # 导入状态: Pending, Running, Completed, Failed
  importMessage: All images imported successfully  # 导入消息
  importedImages:                   # 已导入镜像列表
    - component: kubernetes
      image: kube-apiserver
      targetRef: registry.example.com/capbm/kube-apiserver:v1.31.1
      status: imported
    - component: kubernetes
      image: kube-controller-manager
      targetRef: registry.example.com/capbm/kube-controller-manager:v1.31.1
      status: imported
```

### 9.2 导入状态流转

```
Pending → Running → Completed
                 ↘
                  Failed
```

---

## 十、与 ClusterVersion 的交互

### 10.1 升级触发机制

```
ClusterVersion.spec.desiredUpdate.version 变更
    │
    ▼
CVO Controller 检测版本变更
    │
    ├── 验证升级路径 (UpgradePath)
    │
    ├── 获取目标 ReleaseImage
    │
    ├── Phase 1: K8S 升级 (仅当 K8S 版本变更时)
    │   └── 按 upgradeGraph 顺序执行
    │
    └── Phase 2: Addon 升级 (总是执行)
        └── 按 dependencies 顺序执行
```

### 10.2 版本对比逻辑

```go
func needsUpgrade(current, target *ReleaseImage) bool {
    // K8S 版本变更
    if current.Spec.Components.Kubernetes.Version != target.Spec.Components.Kubernetes.Version {
        return true
    }
    
    // Addon 版本变更
    for _, targetAddon := range target.Spec.Addons {
        currentAddon := findAddon(current, targetAddon.Name)
        if currentAddon == nil || currentAddon.Version != targetAddon.Version {
            return true
        }
    }
    
    return false
}
```

---

## 十一、设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **组件定义位置** | 独立 CRD vs ReleaseImage 内 | ReleaseImage 内 | 高内聚，版本与组件绑定 |
| **升级图定义** | 独立 CRD vs ReleaseImage 内 | ReleaseImage 内 | 升级顺序是版本特性 |
| **备份/回滚配置** | 独立配置 vs 组件内聚 | 组件内聚 | 每个组件负责自己的备份回滚 |
| **多架构支持** | 多个 ReleaseImage vs 单个 | 单个 | 简化版本管理 |
| **多 OS 支持** | 多个 ReleaseImage vs 单个 | 单个 | 简化版本管理 |
| **Addon 依赖** | 隐式 vs 显式 | 显式 (dependencies 字段) | 清晰可控 |
| **升级触发** | 独立字段 vs DesiredUpdate | DesiredUpdate | 统一入口 |

---

## 十二、未来扩展

### 12.1 计划中的扩展

| 扩展 | 说明 | 优先级 |
|------|------|--------|
| **组件签名验证** | 支持 GPG 签名验证组件完整性 | 高 |
| **增量升级** | 支持只升级变更的组件 | 中 |
| **升级回滚点** | 支持创建升级回滚点 | 中 |
| **组件兼容性矩阵** | 定义组件间兼容性规则 | 低 |
| **自动回滚** | 升级失败自动回滚 | 低 |

### 12.2 已知的限制

| 限制 | 说明 |  workaround |
|------|------|------------|
| **单镜像限制** | 一个 ReleaseImage 对应一个 OCI 镜像 | 使用 imageRegistry 导入到本地仓库 |
| **无组件覆盖** | 无法在 ClusterVersion 中覆盖组件版本 | 创建新的 ReleaseImage |
| **无动态变量** | 不支持运行时动态变量 | 使用 ClusterAddon.Spec.Values |
