# CAPBM 原地升级与备份回滚完整设计

## 概述

本文档描述 CAPBM 的原地升级（In-Place Upgrade）完整设计方案，包括升级流程、备份策略、回滚机制，所有配置与组件定义高内聚聚合。

## 核心设计原则

| 原则 | 说明 |
|------|------|
| **高内聚** | 组件定义 + 备份策略 + 回滚策略聚合在一起 |
| **单一职责** | 每个组件负责自己的备份和回滚 |
| **可扩展** | 添加新组件只需添加组件块，无需修改核心逻辑 |
| **安全性** | 升级前自动备份，失败自动回滚 |

## 架构设计

### 升级架构

```
┌─────────────────────────────────────────────────────────────┐
│ 原地升级完整架构                                             │
│                                                             │
│  ClusterVersion Controller (升级编排器)                      │
│  ├── 1. 检测版本变更 (desiredVersion)                        │
│  ├── 2. 验证升级路径 (UpgradePath)                           │
│  ├── 3. 获取目标 ReleaseImage                                │
│  ├── 4. 前置检查 (集群健康/节点 Ready)                       │
│  ├── 5. 执行升级前备份                                       │
│  │   ├── 备份组件配置 (高内聚配置)                           │
│  │   └── 创建 etcd 快照 (控制面)                             │
│  ├── 6. 执行升级 (分阶段、滚动)                              │
│  │   ├── Phase 1: containerd (基础运行时)                    │
│  │   ├── Phase 2: kubernetes (kubeadm/kubelet/kubectl)       │
│  │   ├── Phase 3: cni (网络插件)                             │
│  │   └── Phase 4: csi (存储插件)                             │
│  ├── 7. 验证升级结果 (健康检查)                              │
│  └── 8. 失败时自动回滚                                       │
│      ├── 恢复组件配置                                        │
│      ├── 恢复组件版本                                        │
│      └── 验证回滚成功                                        │
└─────────────────────────────────────────────────────────────┘
```

### 替换升级 vs 原地升级

| 维度 | 替换升级 (Replace) | 原地升级 (InPlace) |
|------|-------------------|-------------------|
| **实现方式** | 创建新节点 → 迁移 → 删除旧节点 | 直接在现有节点上升级组件 |
| **中断时间** | 较长 (节点重建时间) | 较短 (仅组件重启时间) |
| **资源消耗** | 高 (需要额外节点资源) | 低 (使用现有节点) |
| **适用场景** | 节点级变更 (OS/内核/硬件) | 组件级变更 (CNI/CSI/运行时) |
| **回滚复杂度** | 高 (需要恢复旧节点) | 低 (重新安装旧版本) |

## CRD 设计

### ReleaseImage - 组件定义 (高内聚)

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
spec:
  version: v1.31.1
  image: registry.example.com/capbm/release:v1.31.1
  
  # 组件定义 (包含升级配置 - 高内聚)
  components:
    containerd:
      version: 1.7.2
      type: binary
      
      # 升级配置 (高内聚)
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/containerd/config.toml
              type: file
          etcdSnapshot: false
        rollback:
          script: scripts/rollback-containerd.sh
          timeout: 300s
        healthCheck:
          command: systemctl is-active containerd
          timeout: 30s
          retries: 3
    
    kubernetes:
      version: v1.31.1
      type: binary
      
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/kubernetes
              type: directory
          etcdSnapshot: true  # 控制面需要 etcd 备份
        rollback:
          script: scripts/rollback-kubernetes.sh
          timeout: 600s
        healthCheck:
          command: kubectl get nodes
          timeout: 60s
          retries: 3
    
    cni:
      version: 3.28.0
      type: addon
      
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/cni/net.d
              type: directory
          etcdSnapshot: false
        rollback:
          script: scripts/rollback-cni.sh
          timeout: 300s
        healthCheck:
          command: kubectl get pods -n kube-system -l k8s-app=calico-node
          timeout: 60s
          retries: 3
    
    csi:
      version: 3.12.0
      type: addon
      
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/csi
              type: directory
          etcdSnapshot: false
        rollback:
          script: scripts/rollback-csi.sh
          timeout: 300s
        healthCheck:
          command: kubectl get pods -n ceph-csi
          timeout: 60s
          retries: 3
  
  # 升级图只定义执行顺序
  upgradeGraph:
    - name: phase-1-runtime
      order: 1
      blocking: true
      components: [containerd]
    - name: phase-2-kubernetes
      order: 2
      blocking: true
      components: [kubernetes]
    - name: phase-3-addons
      order: 3
      blocking: false
      components: [cni, csi]
```

### ClusterVersion - 升级策略

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ClusterVersion
metadata:
  name: my-cluster
  namespace: cluster-my-cluster
spec:
  clusterRef:
    name: my-cluster
    namespace: cluster-my-cluster
  
  # 期望版本 (触发升级)
  desiredVersion: v1.31.1
  
  # 升级策略 (如何升级，不是升级顺序)
  upgradeStrategy:
    type: InPlace    # InPlace | Replace
    
    # 原地升级配置
    inPlaceConfig:
      # 滚动升级配置
      rollingUpdate:
        maxUnavailable: 1    # 同时升级的最大节点数
        drain:
          enabled: true      # 升级前驱逐 Pod
          timeout: 300s      # 驱逐超时
          ignoreDaemonSets: true
        timeout: 600s        # 单节点升级超时
      
      # 回滚配置
      rollback:
        enabled: true        # 自动回滚
        onTimeout: true      # 超时时回滚
        onFailure: true      # 失败时回滚
        maxRetries: 3        # 最大重试次数
```

### Go 类型定义

```go
// ComponentUpgradeConfig 定义组件级升级配置 (高内聚)
type ComponentUpgradeConfig struct {
    // 备份配置
    Backup ComponentBackupConfig `json:"backup"`
    
    // 回滚配置
    Rollback ComponentRollbackConfig `json:"rollback"`
    
    // 健康检查配置
    HealthCheck ComponentHealthCheckConfig `json:"healthCheck"`
}

// ComponentBackupConfig 定义组件备份配置
type ComponentBackupConfig struct {
    // 是否启用备份
    Enabled bool `json:"enabled"`
    
    // 需要备份的配置
    Config []BackupItem `json:"config"`
    
    // 是否需要 etcd 快照
    EtcdSnapshot bool `json:"etcdSnapshot"`
}

// BackupItem 定义备份项
type BackupItem struct {
    // 路径
    Path string `json:"path"`
    
    // 类型: file | directory
    Type string `json:"type"`
}

// ComponentRollbackConfig 定义组件回滚配置
type ComponentRollbackConfig struct {
    // 回滚脚本路径 (相对于 ReleaseImage 脚本目录)
    Script string `json:"script"`
    
    // 回滚超时
    Timeout *metav1.Duration `json:"timeout"`
}

// ComponentHealthCheckConfig 定义健康检查配置
type ComponentHealthCheckConfig struct {
    // 健康检查命令
    Command string `json:"command"`
    
    // 超时
    Timeout *metav1.Duration `json:"timeout"`
    
    // 重试次数
    Retries int `json:"retries"`
}
```

## 升级流程详细设计

### 1. 升级触发

```yaml
# 方式一：更新 DesiredVersion
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ClusterVersion
spec:
  desiredVersion: v1.31.1   # 从 v1.31.0 升级到 v1.31.1

# 方式二：更新 ReleaseImage 引用
spec:
  releaseImageRef:
    name: v1.31.1   # 指向包含新版本的 ReleaseImage
```

### 2. 升级计划生成

```
ClusterVersion Controller
    │
    ├── 1. 检测版本变更
    │   ├── 对比 currentVersion vs desiredVersion
    │   ├── 获取目标 ReleaseImage
    │   └── 计算组件版本差异
    │
    ├── 2. 前置检查
    │   ├── 检查集群健康状态
    │   ├── 检查节点 Ready 状态
    │   ├── 检查组件当前状态
    │   ├── 验证版本兼容性 (skew policy)
    │   └── 检查 etcd 健康 (控制面)
    │
    ├── 3. 执行升级前备份
    │   ├── 遍历组件升级配置
    │   ├── 备份组件配置 (高内聚配置)
    │   └── 创建 etcd 快照 (如需要)
    │
    └── 4. 开始执行升级
        └── 触发 Phase 1: containerd
```

### 3. 原地升级执行

#### 3.1 Containerd 原地升级

```bash
#!/bin/bash
# 原地升级 containerd - 单节点

NODE=$1
TARGET_VERSION=$2
BACKUP_DIR="/tmp/capbm-backup-$(date +%s)"

# 1. 备份当前配置
ssh $NODE "mkdir -p $BACKUP_DIR"
ssh $NODE "cp /etc/containerd/config.toml $BACKUP_DIR/ 2>/dev/null || true"
ssh $NODE "containerd --version > $BACKUP_DIR/version.txt 2>/dev/null || true"

# 2. 安装新版本
ssh $NODE << EOF
  # 安装新版本
  apt-get update
  apt-get install -y containerd=${TARGET_VERSION}
  
  # 恢复配置 (保留用户自定义)
  if [ -f "$BACKUP_DIR/config.toml" ]; then
    cp "$BACKUP_DIR/config.toml" /etc/containerd/config.toml
  fi
  
  # 重启服务
  systemctl restart containerd
  
  # 验证
  containerd --version
  systemctl is-active containerd
EOF

# 3. 健康检查
ssh $NODE "systemctl is-active containerd"
```

#### 3.2 Kubernetes 原地升级

```bash
#!/bin/bash
# 原地升级 Kubernetes 组件 - 单节点

NODE=$1
TARGET_VERSION=$2
ROLE=$3  # control-plane | worker

# 1. 备份当前配置
ssh $NODE "mkdir -p /tmp/capbm-backup-$(date +%s)"
ssh $NODE "cp -r /etc/kubernetes /tmp/capbm-backup-$(date +%s)/ 2>/dev/null || true"

# 2. 升级组件
ssh $NODE << EOF
  # 升级 kubeadm/kubelet/kubectl
  apt-get update
  apt-get install -y kubeadm=${TARGET_VERSION#v} kubelet=${TARGET_VERSION#v} kubectl=${TARGET_VERSION#v}
  
  # 如果是控制面节点，执行 kubeadm upgrade
  if [ "$ROLE" = "control-plane" ]; then
    kubeadm upgrade node
  fi
  
  # 重启 kubelet
  systemctl daemon-reload
  systemctl restart kubelet
  
  # 验证
  kubelet --version
  systemctl is-active kubelet
EOF

# 3. 健康检查 (控制面额外检查)
if [ "$ROLE" = "control-plane" ]; then
  # 检查 API Server 健康
  curl -k https://$NODE:6443/healthz
fi
```

#### 3.3 CNI 原地升级

```bash
#!/bin/bash
# 原地升级 CNI (Calico 示例)

TARGET_VERSION=$1

# 1. 检查当前版本
CURRENT_VERSION=$(kubectl get daemonset calico-node -n kube-system -o jsonpath='{.spec.template.spec.containers[0].image}' | grep -o 'v[0-9.]*')
if [ "$CURRENT_VERSION" = "v${TARGET_VERSION}" ]; then
  echo "Calico already at target version"
  exit 0
fi

# 2. 更新 DaemonSet 镜像
kubectl set image daemonset/calico-node -n kube-system \
  calico-node=docker.io/calico/node:v${TARGET_VERSION} \
  calico-cni=docker.io/calico/cni:v${TARGET_VERSION}

# 3. 等待滚动更新完成
kubectl rollout status daemonset/calico-node -n kube-system --timeout=300s

# 4. 验证网络连通性
kubectl run network-test --image=busybox --restart=Never --rm -it -- wget -q --timeout=5 kubernetes.default
```

#### 3.4 CSI 原地升级

```bash
#!/bin/bash
# 原地升级 CSI (Ceph-CSI 示例)

TARGET_VERSION=$1

# 1. 更新 Controller Deployment
kubectl set image deployment/ceph-csi-rbdplugin-provisioner -n ceph-csi \
  csi-rbdplugin-provisioner=quay.io/cephcsi/cephcsi:v${TARGET_VERSION}

# 2. 更新 Node DaemonSet
kubectl set image daemonset/csi-rbdplugin -n ceph-csi \
  csi-rbdplugin=quay.io/cephcsi/cephcsi:v${TARGET_VERSION}

# 3. 等待滚动更新完成
kubectl rollout status deployment/ceph-csi-rbdplugin-provisioner -n ceph-csi --timeout=300s
kubectl rollout status daemonset/csi-rbdplugin -n ceph-csi --timeout=300s

# 4. 验证存储操作
kubectl create -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: upgrade-test-pvc
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
EOF
kubectl wait --for=condition=Bound pvc/upgrade-test-pvc --timeout=60s
kubectl delete pvc upgrade-test-pvc
```

### 4. 滚动升级协调

```go
// RollingUpgradeCoordinator 协调滚动升级
type RollingUpgradeCoordinator struct {
    client        client.Client
    sshManager    *ssh.SSHManager
    maxUnavailable int
    drainConfig   DrainConfig
}

// ExecuteRollingUpgrade 执行滚动升级
func (c *RollingUpgradeCoordinator) ExecuteRollingUpgrade(ctx context.Context, cluster *infrav1.ClusterVersion, phase UpgradePhase) error {
    // 1. 获取需要升级的节点列表
    nodes, err := c.getNodesForUpgrade(ctx, cluster, phase)
    if err != nil {
        return err
    }
    
    // 2. 分批升级
    batches := c.createBatches(nodes, c.maxUnavailable)
    
    for _, batch := range batches {
        // 3. 并行升级当前批次
        errChan := make(chan error, len(batch))
        for _, node := range batch {
            go func(n *corev1.Node) {
                errChan <- c.upgradeNode(ctx, cluster, n, phase)
            }(node)
        }
        
        // 4. 等待批次完成
        for range batch {
            if err := <-errChan; err != nil {
                return err
            }
        }
        
        // 5. 批次间健康检查
        if err := c.checkClusterHealth(ctx, cluster); err != nil {
            return err
        }
    }
    
    return nil
}

// upgradeNode 升级单个节点
func (c *RollingUpgradeCoordinator) upgradeNode(ctx context.Context, cluster *infrav1.ClusterVersion, node *corev1.Node, phase UpgradePhase) error {
    // 1. 驱逐 Pod (如果是 worker 节点)
    if !isControlPlane(node) && c.drainConfig.Enabled {
        if err := c.drainNode(ctx, node); err != nil {
            return err
        }
    }
    
    // 2. 执行升级脚本
    if err := c.executeUpgradeScript(ctx, node, phase); err != nil {
        // 3. 升级失败时回滚
        if c.drainConfig.Enabled {
            c.uncordonNode(ctx, node)
        }
        return err
    }
    
    // 4. 验证节点健康
    if err := c.verifyNodeHealth(ctx, node, phase); err != nil {
        return err
    }
    
    // 5. 恢复节点调度
    if !isControlPlane(node) && c.drainConfig.Enabled {
        return c.uncordonNode(ctx, node)
    }
    
    return nil
}
```

### 5. 备份与回滚设计 (高内聚)

#### 5.1 备份流程

```
升级前备份
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. 遍历组件升级配置 (ReleaseImage components[*].upgrade)     │
│    ├── 读取需要备份的配置路径                                 │
│    └── 读取 etcd 快照配置                                    │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. 执行备份                                                  │
│    ├── 备份组件配置 (文件/目录)                               │
│    │   ├── containerd: /etc/containerd/config.toml           │
│    │   ├── kubernetes: /etc/kubernetes                       │
│    │   ├── cni: /etc/cni/net.d                              │
│    │   └── csi: /etc/csi                                    │
│    └── 创建 etcd 快照 (控制面)                               │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. 存储备份                                                  │
│    ├── 配置备份 → ConfigMap/Secret                           │
│    └── etcd 快照 → Secret                                   │
└─────────────────────────────────────────────────────────────┘
```

#### 5.2 回滚流程

```
升级失败触发回滚
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. 获取组件回滚配置 (ReleaseImage components[*].upgrade)      │
│    ├── 读取回滚脚本路径                                      │
│    ├── 读取回滚超时                                          │
│    └── 读取健康检查配置                                      │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. 执行回滚 (逐组件)                                         │
│    ├── 获取回滚脚本                                          │
│    ├── 执行回滚脚本                                          │
│    └── 等待超时                                              │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. 验证回滚                                                  │
│    ├── 执行健康检查                                          │
│    └── 验证组件版本                                          │
└─────────────────┬───────────────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. 更新状态                                                  │
│    ├── 标记回滚成功/失败                                     │
│    └── 记录回滚时间                                          │
└─────────────────────────────────────────────────────────────┘
```

#### 5.3 回滚脚本示例

```bash
#!/bin/bash
# 回滚脚本 - 单节点

NODE=$1
BACKUP_VERSION=$2
BACKUP_DIR="/tmp/capbm-backup"

# 1. 停止当前升级
echo "Stopping current upgrade..."
systemctl stop kubelet 2>/dev/null || true

# 2. 恢复组件配置
echo "Restoring component configurations..."

# 恢复 containerd 配置
if [ -f "$BACKUP_DIR/containerd/config.toml" ]; then
  cp "$BACKUP_DIR/containerd/config.toml" /etc/containerd/config.toml
  systemctl restart containerd
fi

# 恢复 kubelet 配置
if [ -f "$BACKUP_DIR/kubernetes/kubelet-config.yaml" ]; then
  cp "$BACKUP_DIR/kubernetes/kubelet-config.yaml" /var/lib/kubelet/config.yaml
fi

# 恢复 CNI 配置
if [ -d "$BACKUP_DIR/cni/net.d" ]; then
  rm -rf /etc/cni/net.d
  cp -r "$BACKUP_DIR/cni/net.d" /etc/cni/
fi

# 3. 恢复组件版本
echo "Restoring component versions..."
apt-get update
apt-get install -y \
  containerd=$(cat "$BACKUP_DIR/containerd/version.txt") \
  kubelet=$(cat "$BACKUP_DIR/kubernetes/version.txt")

# 4. 重启服务
systemctl daemon-reload
systemctl restart containerd
systemctl restart kubelet

# 5. 验证回滚
echo "Verifying rollback..."
containerd --version
kubelet --version
systemctl is-active containerd
systemctl is-active kubelet
```

### 6. 健康检查设计

```go
// UpgradeHealthChecker 升级健康检查器
type UpgradeHealthChecker struct {
    client  client.Client
    timeout time.Duration
    retries int
}

// CheckClusterHealth 检查集群健康
func (h *UpgradeHealthChecker) CheckClusterHealth(ctx context.Context, cluster *infrav1.ClusterVersion) error {
    // 1. 检查所有节点 Ready
    if err := h.checkNodesReady(ctx, cluster); err != nil {
        return err
    }
    
    // 2. 检查核心组件健康
    if err := h.checkCoreComponents(ctx, cluster); err != nil {
        return err
    }
    
    // 3. 检查网络连通性
    if err := h.checkNetworkConnectivity(ctx, cluster); err != nil {
        return err
    }
    
    // 4. 检查存储操作
    if err := h.checkStorageOperations(ctx, cluster); err != nil {
        return err
    }
    
    return nil
}
```

### 7. 版本兼容性控制

```yaml
# ReleaseImage 中的版本兼容性定义
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
spec:
  version: v1.31.1
  
  # 版本兼容性
  compatibility:
    kubernetes:
      minVersion: v1.30.0    # 最低兼容版本
      maxSkew: 1             # 最大版本倾斜 (minor)
    containerd:
      minVersion: 1.6.0
    cni:
      calico:
        minVersion: 3.26.0
        maxVersion: 3.28.0
      cilium:
        minVersion: 1.14.0
        maxVersion: 1.16.0
```

## 设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **升级顺序定义** | ClusterVersion vs ReleaseImage | ReleaseImage | 升级顺序是版本特性，不是集群策略 |
| **备份/回滚位置** | 独立脚本 vs 组件内聚 | 组件内聚 | 高内聚，易维护 |
| **脚本存储** | 嵌入 CRD vs OCI/ConfigMap | OCI/ConfigMap | CRD 大小限制，脚本可独立更新 |
| **回滚触发** | 自动 vs 手动 | 两者都支持 | 灵活性 |
| **备份时机** | 每次升级前 | 是 | 数据安全 |
| **升级策略** | InPlace vs Replace | InPlace (组件级) | 最小化中断，节省资源 |
| **并发控制** | 串行 vs 并行 | 可配置 (默认串行) | 安全性优先 |
| **节点驱逐** | 驱逐 vs 不驱逐 | 可配置 (worker 驱逐) | 减少 Pod 中断 |

## 实施步骤

1. **扩展 CRD**: 添加升级策略和状态字段到 ClusterVersion 和 ReleaseImage
2. **实现升级控制器**: 处理升级逻辑和滚动协调
3. **实现备份/回滚执行器**: 高内聚备份和回滚
4. **实现健康检查**: 升级前后验证
5. **实现回滚机制**: 自动和手动回滚
6. **添加监控指标**: 升级进度和状态
7. **编写文档**: 用户指南和最佳实践
