# 基于 KubeadmControlPlane 的控制平面原地升级设计

## 概述

本文档描述基于 `KubeadmControlPlane` (KCP) 的控制平面原地升级（In-Place Upgrade）设计方案。KCP 本身已提供控制平面节点的升级能力，本方案重点在于如何与 CAPBM 的组件升级（containerd、CNI、CSI 等）协调，实现完整的控制平面原地升级。

## 核心设计原则

| 原则 | 说明 |
|------|------|
| **KCP 主导** | KCP 负责 kubeadm/kubelet/kubectl 和静态 Pod 升级 |
| **CAPBM 协调** | CAPBM 负责 containerd、CNI、CSI 等组件升级 |
| **安全性优先** | etcd 备份是升级前的必要步骤 |
| **逐节点升级** | HA 集群每次只升级一个控制面节点 |
| **健康检查** | 每个节点升级后必须通过健康检查才能继续 |

## 架构设计

### 现有 KCP 升级机制

```
KubeadmControlPlane 升级流程 (现有):
    │
    ├── 1. 更新 KCP.spec.version
    │
    ├── 2. KCP Controller 检测版本变更
    │
    ├── 3. 逐节点升级 (滚动)
    │   ├── 节点 1: kubeadm upgrade node → 重启 kubelet → 健康检查
    │   ├── 节点 2: kubeadm upgrade node → 重启 kubelet → 健康检查
    │   └── 节点 3: kubeadm upgrade node → 重启 kubelet → 健康检查
    │
    └── 4. 更新 KCP.status.version
```

### 完整控制平面原地升级架构

```
┌─────────────────────────────────────────────────────────────┐
│ 完整控制平面原地升级流程                                     │
│                                                             │
│  ClusterVersion Controller (升级编排器)                      │
│  ├── 1. 检测版本变更 (desiredVersion)                        │
│  ├── 2. 验证升级路径 (UpgradePath)                           │
│  ├── 3. 前置检查                                            │
│  │   ├── 检查集群健康状态                                    │
│  │   ├── 检查所有控制面节点 Ready                            │
│  │   └── 检查 etcd 集群健康                                 │
│  ├── 4. 执行升级前备份                                       │
│  │   ├── 备份 etcd (所有控制面节点)                          │
│  │   └── 备份组件配置                                        │
│  ├── 5. 逐节点升级 (协调 KCP + CAPBM)                        │
│  │   │                                                       │
│  │   ├── 节点 1:                                            │
│  │   │   ├── 5.1 CAPBM: 升级 containerd                      │
│  │   │   ├── 5.2 CAPBM: 升级 CNI 组件                        │
│  │   │   ├── 5.3 CAPBM: 升级 CSI 组件                        │
│  │   │   ├── 5.4 KCP: kubeadm upgrade node                   │
│  │   │   └── 5.5 健康检查                                    │
│  │   │                                                       │
│  │   ├── 节点 2:                                            │
│  │   │   ├── 5.1 CAPBM: 升级 containerd                      │
│  │   │   ├── 5.2 CAPBM: 升级 CNI 组件                        │
│  │   │   ├── 5.3 CAPBM: 升级 CSI 组件                        │
│  │   │   ├── 5.4 KCP: kubeadm upgrade node                   │
│  │   │   └── 5.5 健康检查                                    │
│  │   │                                                       │
│  │   └── 节点 3:                                            │
│  │       ├── 5.1 CAPBM: 升级 containerd                      │
│  │       ├── 5.2 CAPBM: 升级 CNI 组件                        │
│  │       ├── 5.3 CAPBM: 升级 CSI 组件                        │
│  │       ├── 5.4 KCP: kubeadm upgrade node                   │
│  │       └── 5.5 健康检查                                    │
│  │                                                           │
│  ├── 6. 验证升级结果                                         │
│  └── 7. 失败时自动回滚                                       │
└─────────────────────────────────────────────────────────────┘
```

## CRD 设计

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
  
  # 升级策略
  upgradeStrategy:
    type: InPlace    # InPlace | Replace
    
    # 原地升级配置
    inPlaceConfig:
      # 控制平面升级配置
      controlPlane:
        # 与 KCP 协调
        kubeadmControlPlane:
          enabled: true          # 使用 KCP 进行 kubeadm 升级
          waitForKCP: true       # 等待 KCP 完成升级后再升级其他组件
        
        # 滚动升级配置
        rollingUpdate:
          maxUnavailable: 1      # 每次只升级一个节点
          drain:
            enabled: true
            timeout: 300s
            ignoreDaemonSets: true
          timeout: 600s          # 单节点升级超时
        
        # etcd 备份配置
        etcdBackup:
          enabled: true
          timeout: 300s
          storage:
            type: Secret
            retention: 3
        
        # 回滚配置
        rollback:
          enabled: true
          onTimeout: true
          onFailure: true
```

### ReleaseImage - 组件定义 (高内聚)

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ReleaseImage
metadata:
  name: v1.31.1
spec:
  version: v1.31.1
  
  # 组件定义 (包含升级配置)
  components:
    kubernetes:
      version: v1.31.1
      type: binary
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
      version: 1.7.2
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
    
    cni:
      version: 3.28.0
      upgrade:
        backup:
          enabled: true
          config:
            - path: /etc/cni/net.d
              type: directory
        rollback:
          script: scripts/rollback-cni.sh
          timeout: 300s
        healthCheck:
          command: kubectl get pods -n kube-system -l k8s-app=calico-node
          timeout: 60s
```

## 升级流程详细设计

### 1. 升级触发

```yaml
# 更新 DesiredVersion 触发升级
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: ClusterVersion
spec:
  desiredVersion: v1.31.1   # 从 v1.31.0 升级到 v1.31.1
```

### 2. 前置检查

```
前置检查
    │
    ├── 1. 检查集群健康状态
    │   └── kubectl get nodes - 所有节点 Ready
    │
    ├── 2. 检查控制面节点数量
    │   └── 至少 3 个控制面节点 (HA)
    │
    ├── 3. 检查 etcd 集群健康
    │   ├── etcdctl endpoint health
    │   └── etcdctl member list
    │
    └── 4. 检查版本兼容性
        ├── 当前版本 vs 目标版本
        └── 验证升级路径允许
```

### 3. 单节点升级流程 (与 KCP 协调)

```
单节点升级 (与 KCP 协调)
    │
    ├── 1. 驱逐 Pod (可选)
    │   └── kubectl drain <node> --ignore-daemonsets
    │
    ├── 2. CAPBM: 升级 containerd
    │   ├── 备份 /etc/containerd/config.toml
    │   ├── 升级 containerd 包
    │   ├── 恢复配置
    │   └── systemctl restart containerd
    │
    ├── 3. CAPBM: 升级 CNI 组件
    │   ├── 备份 /etc/cni/net.d
    │   ├── 更新 CNI DaemonSet 镜像
    │   └── 等待 CNI Pod Ready
    │
    ├── 4. CAPBM: 升级 CSI 组件
    │   ├── 备份 CSI 配置
    │   ├── 更新 CSI Controller/Node 镜像
    │   └── 等待 CSI Pod Ready
    │
    ├── 5. KCP: kubeadm upgrade node
    │   ├── kubeadm upgrade node
    │   ├── systemctl restart kubelet
    │   └── 验证 kubelet 版本
    │
    ├── 6. 取消节点调度
    │   └── kubectl uncordon <node>
    │
    └── 7. 健康检查
        ├── 检查节点 Ready
        ├── 检查控制面 Pod 运行正常
        └── 检查 etcd 集群健康
```

### 4. 与 KCP 协调机制

```go
// ControlPlaneUpgrader 协调控制平面升级
type ControlPlaneUpgrader struct {
    client     client.Client
    sshManager *ssh.SSHManager
    config     ControlPlaneUpgradeConfig
}

// ExecuteUpgrade 执行控制平面滚动升级
func (u *ControlPlaneUpgrader) ExecuteUpgrade(ctx context.Context, cv *infrav1.ClusterVersion, releaseImage *infrav1.ReleaseImage) error {
    // 1. 前置检查
    if err := u.preUpgradeChecks(ctx, cv); err != nil {
        return fmt.Errorf("pre-upgrade checks failed: %w", err)
    }
    
    // 2. 执行升级前备份
    if err := u.backupBeforeUpgrade(ctx, cv, releaseImage); err != nil {
        return fmt.Errorf("backup failed: %w", err)
    }
    
    // 3. 获取控制面节点列表
    nodes, err := u.getControlPlaneNodes(ctx, cv)
    if err != nil {
        return err
    }
    
    // 4. 逐节点升级
    for _, node := range nodes {
        if err := u.upgradeNode(ctx, cv, node, releaseImage); err != nil {
            // 升级失败时尝试回滚
            if u.config.Rollback.Enabled {
                if rollbackErr := u.rollback(ctx, cv, releaseImage); rollbackErr != nil {
                    return fmt.Errorf("upgrade failed on node %s and rollback also failed: %w, rollback error: %v", node.Name, err, rollbackErr)
                }
            }
            return fmt.Errorf("upgrade failed on node %s: %w", node.Name, err)
        }
    }
    
    // 5. 等待 KCP 完成升级 (如果启用)
    if u.config.ControlPlane.KubeadmControlPlane.WaitForKCP {
        if err := u.waitForKCPUpgrade(ctx, cv); err != nil {
            return fmt.Errorf("KCP upgrade failed: %w", err)
        }
    }
    
    // 6. 验证升级结果
    return u.postUpgradeVerification(ctx, cv, releaseImage)
}

// upgradeNode 升级单个控制面节点
func (u *ControlPlaneUpgrader) upgradeNode(ctx context.Context, cv *infrav1.ClusterVersion, node *corev1.Node, releaseImage *infrav1.ReleaseImage) error {
    // 1. 驱逐 Pod
    if u.config.ControlPlane.RollingUpdate.Drain.Enabled {
        if err := u.drainNode(ctx, node); err != nil {
            return err
        }
    }
    
    // 2. CAPBM: 升级 containerd
    if err := u.upgradeContainerd(ctx, node, releaseImage); err != nil {
        return err
    }
    
    // 3. CAPBM: 升级 CNI 组件
    if err := u.upgradeCNI(ctx, node, releaseImage); err != nil {
        return err
    }
    
    // 4. CAPBM: 升级 CSI 组件
    if err := u.upgradeCSI(ctx, node, releaseImage); err != nil {
        return err
    }
    
    // 5. KCP: kubeadm upgrade node (由 KCP Controller 处理)
    // KCP 会自动检测版本变更并执行 kubeadm upgrade node
    
    // 6. 验证节点升级
    if err := u.verifyNodeUpgrade(ctx, node, releaseImage); err != nil {
        return err
    }
    
    // 7. 取消节点调度
    if u.config.ControlPlane.RollingUpdate.Drain.Enabled {
        return u.uncordonNode(ctx, node)
    }
    
    return nil
}

// waitForKCPUpgrade 等待 KCP 完成升级
func (u *ControlPlaneUpgrader) waitForKCPUpgrade(ctx context.Context, cv *infrav1.ClusterVersion) error {
    // 等待 KCP.status.version == desiredVersion
    // 等待 KCP.status.conditions[Ready] == True
    return wait.PollUntilContextTimeout(ctx, 10*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
        kcp := &controlplanev1.KubeadmControlPlane{}
        if err := u.client.Get(ctx, types.NamespacedName{
            Namespace: cv.Namespace,
            Name:      cv.Spec.ClusterRef.Name + "-control-plane",
        }, kcp); err != nil {
            return false, err
        }
        
        return kcp.Status.UpdatedReplicas == kcp.Status.Replicas &&
               kcp.Status.ReadyReplicas == kcp.Status.Replicas, nil
    })
}
```

### 5. etcd 备份与恢复

#### 5.1 etcd 备份脚本

```bash
#!/bin/bash
# etcd 备份脚本

BACKUP_DIR="/backup/etcd"
TIMESTAMP=$(date +%s)
SNAPSHOT_FILE="$BACKUP_DIR/etcd-snapshot-${TIMESTAMP}.db"

# 创建备份目录
mkdir -p $BACKUP_DIR

# 执行 etcd 快照
ETCDCTL_API=3 etcdctl snapshot save $SNAPSHOT_FILE \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key

# 验证快照
ETCDCTL_API=3 etcdctl snapshot status $SNAPSHOT_FILE --write-out=table

# 存储到 Secret
kubectl create secret generic etcd-backup-${TIMESTAMP} \
  --from-file=snapshot.db=$SNAPSHOT_FILE \
  --namespace=cluster-my-cluster

# 清理旧备份 (保留最新 3 个)
ls -t $BACKUP_DIR/etcd-snapshot-*.db | tail -n +4 | xargs rm -f
```

#### 5.2 etcd 恢复脚本

```bash
#!/bin/bash
# etcd 恢复脚本

SNAPSHOT_FILE=$1

# 停止 kubelet 和 etcd
systemctl stop kubelet
systemctl stop kube-apiserver
systemctl stop kube-controller-manager
systemctl stop kube-scheduler

# 恢复 etcd 快照
ETCDCTL_API=3 etcdctl snapshot restore $SNAPSHOT_FILE \
  --data-dir=/var/lib/etcd \
  --name=$(hostname) \
  --initial-cluster=$(hostname)=https://$(hostname -i):2380 \
  --initial-advertise-peer-urls=https://$(hostname -i):2380

# 修复权限
chown -R etcd:etcd /var/lib/etcd

# 重启服务
systemctl start etcd
systemctl start kube-apiserver
systemctl start kube-controller-manager
systemctl start kube-scheduler
systemctl start kubelet

# 验证
kubectl get nodes
```

### 6. 健康检查设计

```go
// ControlPlaneHealthChecker 控制平面健康检查器
type ControlPlaneHealthChecker struct {
    client  client.Client
    timeout time.Duration
    retries int
}

// CheckControlPlaneHealth 检查控制平面健康
func (h *ControlPlaneHealthChecker) CheckControlPlaneHealth(ctx context.Context, cv *infrav1.ClusterVersion) error {
    // 1. 检查所有控制面节点 Ready
    if err := h.checkControlPlaneNodesReady(ctx, cv); err != nil {
        return err
    }
    
    // 2. 检查 etcd 集群健康
    if err := h.checkEtcdHealth(ctx, cv); err != nil {
        return err
    }
    
    // 3. 检查控制面 Pod 运行正常
    if err := h.checkControlPlanePods(ctx, cv); err != nil {
        return err
    }
    
    // 4. 检查 API Server 健康
    if err := h.checkAPIServerHealth(ctx, cv); err != nil {
        return err
    }
    
    return nil
}
```

## 设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **升级策略** | InPlace vs Replace | InPlace | 最小化中断，节省资源 |
| **KCP 协调** | 独立 vs 协调 | 协调 | 利用 KCP 现有能力 |
| **节点升级顺序** | 并行 vs 串行 | 串行 (每次一个节点) | 安全性优先 |
| **etcd 备份** | 每次升级前 | 是 | 数据安全 |
| **Pod 驱逐** | 驱逐 vs 不驱逐 | 驱逐 | 减少 Pod 中断 |
| **回滚触发** | 自动 vs 手动 | 两者都支持 | 灵活性 |
| **重试次数** | 多次 vs 一次 | 一次 (控制面) | 控制面升级失败应谨慎 |

## 实施步骤

1. **扩展 CRD**: 添加控制平面升级配置到 ClusterVersion
2. **实现控制平面升级协调器**: 协调 KCP 和 CAPBM 升级
3. **实现 etcd 备份/恢复**: 安全的 etcd 快照管理
4. **实现健康检查**: 升级前后验证
5. **实现回滚机制**: 自动和手动回滚
6. **添加监控指标**: 升级进度和状态
7. **编写文档**: 用户指南和最佳实践
