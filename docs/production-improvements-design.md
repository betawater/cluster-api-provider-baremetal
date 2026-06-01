# CAPBM 生产环境改进设计

## 概述

本文档描述 CAPBM 控制平面原地升级在生产环境的关键改进点，包括错误处理、状态管理、并发控制、监控、安全性和优雅降级。

## 核心改进点

### 1. 错误处理与重试机制

#### 设计原则
- 区分可重试错误和不可重试错误
- 指数退避重试
- 最大重试次数限制
- 重试上下文传递

#### 实现

```go
// RetryConfig defines retry configuration
type RetryConfig struct {
    MaxRetries      int
    Backoff         BackoffConfig
    RetryableErrors []string
}

type BackoffConfig struct {
    Initial time.Duration
    Max     time.Duration
    Factor  float64
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error, retryableErrors []string) bool {
    errMsg := strings.ToLower(err.Error())
    for _, pattern := range retryableErrors {
        if strings.Contains(errMsg, strings.ToLower(pattern)) {
            return true
        }
    }
    return false
}

// RetryWithBackoff executes a function with exponential backoff
func RetryWithBackoff(ctx context.Context, config RetryConfig, fn func(ctx context.Context) error) error {
    backoff := config.Backoff.Initial
    
    for attempt := 0; attempt <= config.MaxRetries; attempt++ {
        err := fn(ctx)
        if err == nil {
            return nil
        }
        
        if !IsRetryableError(err, config.RetryableErrors) {
            return err
        }
        
        if attempt == config.MaxRetries {
            return fmt.Errorf("max retries (%d) exceeded: %w", config.MaxRetries, err)
        }
        
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(backoff):
            backoff = time.Duration(float64(backoff) * config.Backoff.Factor)
            if backoff > config.Backoff.Max {
                backoff = config.Backoff.Max
            }
        }
    }
    
    return nil
}
```

### 2. 状态管理与持久化

#### UpgradeSession CRD

```yaml
apiVersion: upgrade.capbm.io/v1alpha1
kind: UpgradeSession
metadata:
  name: my-cluster-upgrade-20240115
  namespace: cluster-my-cluster
spec:
  clusterRef:
    name: my-cluster
    namespace: cluster-my-cluster
  targetVersion: v1.31.1
  sourceVersion: v1.31.0
  upgradeStrategy:
    type: InPlace
    rollingUpdate:
      maxUnavailable: 1
    etcdBackup:
      enabled: true
    rollback:
      enabled: true
status:
  phase: Upgrading
  currentNode: node-2
  completedNodes:
    - node-1
  failedNodes: []
  startTime: "2024-01-15T10:30:00Z"
  backupRef:
    name: etcd-backup-20240115
    namespace: cluster-my-cluster
  conditions:
    - type: Ready
      status: "True"
      reason: UpgradeInProgress
```

#### Go 类型定义

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="TargetVersion",type="string",JSONPath=".spec.targetVersion"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type UpgradeSession struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    
    Spec   UpgradeSessionSpec   `json:"spec,omitempty"`
    Status UpgradeSessionStatus `json:"status,omitempty"`
}

type UpgradeSessionSpec struct {
    ClusterRef      corev1.ObjectReference    `json:"clusterRef"`
    TargetVersion   string                    `json:"targetVersion"`
    SourceVersion   string                    `json:"sourceVersion"`
    UpgradeStrategy UpgradeStrategyConfig     `json:"upgradeStrategy"`
}

type UpgradeSessionStatus struct {
    Phase            UpgradePhase            `json:"phase"`
    CurrentNode      string                  `json:"currentNode,omitempty"`
    CompletedNodes   []string                `json:"completedNodes,omitempty"`
    FailedNodes      []NodeFailureInfo       `json:"failedNodes,omitempty"`
    StartTime        metav1.Time             `json:"startTime"`
    CompletionTime   *metav1.Time            `json:"completionTime,omitempty"`
    BackupRef        *corev1.ObjectReference `json:"backupRef,omitempty"`
    Conditions       []metav1.Condition      `json:"conditions,omitempty"`
}

type UpgradePhase string

const (
    UpgradePhasePending     UpgradePhase = "Pending"
    UpgradePhaseBackingUp   UpgradePhase = "BackingUp"
    UpgradePhaseUpgrading   UpgradePhase = "Upgrading"
    UpgradePhaseVerifying   UpgradePhase = "Verifying"
    UpgradePhaseCompleted   UpgradePhase = "Completed"
    UpgradePhaseFailed      UpgradePhase = "Failed"
    UpgradePhaseRollingBack UpgradePhase = "RollingBack"
)

type NodeFailureInfo struct {
    Node      string    `json:"node"`
    Error     string    `json:"error"`
    Timestamp metav1.Time `json:"timestamp"`
}
```

### 3. 并发控制

#### Leader Election

```go
type UpgradeCoordinator struct {
    client   client.Client
    recorder record.EventRecorder
    scheme   *runtime.Scheme
}

func (c *UpgradeCoordinator) Start(ctx context.Context) error {
    id, err := os.Hostname()
    if err != nil {
        return err
    }
    
    lock := &resourcelock.LeaseLock{
        LeaseMeta: metav1.ObjectMeta{
            Name:      "capbm-upgrade-lock",
            Namespace: "capbm-system",
        },
        Client: c.client.CoordinationV1(),
        LockConfig: resourcelock.ResourceLockConfig{
            Identity: id,
        },
    }
    
    le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
        Lock:            lock,
        LeaseDuration:   15 * time.Second,
        RenewDeadline:   10 * time.Second,
        RetryPeriod:     2 * time.Second,
        ReleaseOnCancel: true,
        Callbacks: leaderelection.LeaderCallbacks{
            OnStartedLeading: func(ctx context.Context) {
                c.runUpgradeLoop(ctx)
            },
            OnStoppedLeading: func() {
                log.Info("Lost leadership, stopping upgrade coordinator")
            },
            OnNewLeader: func(identity string) {
                if identity != id {
                    log.Info("New leader elected", "leader", identity)
                }
            },
        },
    })
    
    le.Run(ctx)
    return nil
}
```

### 4. 监控与可观测性

#### Prometheus Metrics

```go
var (
    upgradeDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "capbm_upgrade_duration_seconds",
            Help:    "Duration of control plane upgrade in seconds",
            Buckets: prometheus.ExponentialBuckets(60, 2, 10),
        },
        []string{"cluster", "source_version", "target_version", "status"},
    )
    
    upgradeInProgress = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "capbm_upgrade_in_progress",
            Help: "Whether an upgrade is currently in progress",
        },
        []string{"cluster"},
    )
    
    nodeUpgradeDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "capbm_node_upgrade_duration_seconds",
            Help:    "Duration of single node upgrade in seconds",
            Buckets: prometheus.ExponentialBuckets(30, 2, 8),
        },
        []string{"cluster", "node", "component", "status"},
    )
    
    etcdBackupDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "capbm_etcd_backup_duration_seconds",
            Help:    "Duration of etcd backup in seconds",
            Buckets: prometheus.ExponentialBuckets(10, 2, 6),
        },
        []string{"cluster", "node", "status"},
    )
)

func init() {
    prometheus.MustRegister(upgradeDuration, upgradeInProgress, nodeUpgradeDuration, etcdBackupDuration)
}
```

#### Kubernetes Events

```go
func (u *ControlPlaneUpgrader) recordEvent(obj runtime.Object, eventType, reason, message string) {
    u.recorder.Eventf(obj, eventType, reason, message)
}

// Usage
u.recordEvent(node, corev1.EventTypeNormal, "UpgradeStarted", 
    "Starting upgrade of node %s from %s to %s", node.Name, sourceVersion, targetVersion)
```

### 5. 安全性改进

#### etcd 备份加密

```go
func (u *ControlPlaneUpgrader) backupEtcd(ctx context.Context, node *corev1.Node, session *upgraderv1alpha1.UpgradeSession) error {
    // Create backup directory
    backupDir := fmt.Sprintf("/tmp/etcd-backup-%d", time.Now().Unix())
    if err := os.MkdirAll(backupDir, 0700); err != nil {
        return err
    }
    defer os.RemoveAll(backupDir)
    
    // Run etcdctl snapshot save
    snapshotFile := filepath.Join(backupDir, "snapshot.db")
    cmd := exec.CommandContext(ctx, "etcdctl", "snapshot", "save", snapshotFile,
        "--endpoints=https://127.0.0.1:2379",
        "--cacert=/etc/kubernetes/pki/etcd/ca.crt",
        "--cert=/etc/kubernetes/pki/etcd/server.crt",
        "--key=/etc/kubernetes/pki/etcd/server.key",
    )
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("etcd snapshot failed: %w", err)
    }
    
    // Encrypt backup
    encryptedFile := filepath.Join(backupDir, "snapshot.db.enc")
    if err := encryptFile(snapshotFile, encryptedFile); err != nil {
        return err
    }
    
    // Store in Secret
    encryptedData, err := os.ReadFile(encryptedFile)
    if err != nil {
        return err
    }
    
    secret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("etcd-backup-%s-%d", node.Name, time.Now().Unix()),
            Namespace: session.Namespace,
            Labels: map[string]string{
                "capbm.io/upgrade-session": session.Name,
                "capbm.io/backup-type":     "etcd",
            },
        },
        Data: map[string][]byte{
            "snapshot.db.enc": encryptedData,
        },
    }
    
    if err := u.client.Create(ctx, secret); err != nil {
        return err
    }
    
    // Update session status
    session.Status.BackupRef = &corev1.ObjectReference{
        Name:      secret.Name,
        Namespace: secret.Namespace,
    }
    
    return u.client.Status().Update(ctx, session)
}
```

### 6. 优雅降级

```go
type GracefulDegradationConfig struct {
    MaxConsecutiveFailures int           `json:"maxConsecutiveFailures"`
    PauseOnFailure         bool          `json:"pauseOnFailure"`
    AutoResumeAfter        time.Duration `json:"autoResumeAfter"`
    FallbackVersion        string        `json:"fallbackVersion,omitempty"`
}

func (u *ControlPlaneUpgrader) executeWithDegradation(ctx context.Context, session *upgraderv1alpha1.UpgradeSession, nodes []*corev1.Node, releaseImage *infrav1.ReleaseImage) error {
    consecutiveFailures := 0
    
    for _, node := range nodes {
        if err := u.upgradeNode(ctx, session, node, releaseImage); err != nil {
            consecutiveFailures++
            
            // Record failure
            session.Status.FailedNodes = append(session.Status.FailedNodes, upgraderv1alpha1.NodeFailureInfo{
                Node:      node.Name,
                Error:     err.Error(),
                Timestamp: metav1.Now(),
            })
            
            if consecutiveFailures >= u.config.MaxConsecutiveFailures {
                if u.config.PauseOnFailure {
                    // Pause upgrade
                    session.Status.Phase = upgraderv1alpha1.UpgradePhaseFailed
                    if err := u.client.Status().Update(ctx, session); err != nil {
                        return err
                    }
                    
                    u.recorder.Eventf(session, corev1.EventTypeWarning, "UpgradePaused",
                        "Upgrade paused after %d consecutive failures", consecutiveFailures)
                    
                    return fmt.Errorf("upgrade paused after %d consecutive failures", consecutiveFailures)
                }
                
                // Rollback to fallback version
                if u.config.FallbackVersion != "" {
                    if err := u.rollbackToVersion(ctx, session, u.config.FallbackVersion); err != nil {
                        return fmt.Errorf("upgrade failed and rollback also failed: %w", err)
                    }
                    return fmt.Errorf("upgrade failed, rolled back to %s", u.config.FallbackVersion)
                }
                
                return err
            }
        } else {
            consecutiveFailures = 0
            session.Status.CompletedNodes = append(session.Status.CompletedNodes, node.Name)
        }
    }
    
    return nil
}
```

## 实施步骤

1. **创建 UpgradeSession CRD** - 状态管理基础
2. **实现重试机制** - 错误处理改进
3. **实现 Leader Election** - 并发控制
4. **添加 Prometheus Metrics** - 监控
5. **添加 Kubernetes Events** - 可观测性
6. **实现 etcd 备份加密** - 安全性
7. **实现优雅降级** - 韧性
8. **编写单元测试** - 质量保证
9. **编写集成测试** - 集成验证
10. **编写文档** - 运维指南

## 设计决策

| 决策点 | 选项 | 推荐 | 理由 |
|--------|------|------|------|
| **状态存储** | ConfigMap vs CRD | CRD | 结构化，支持 status subresource |
| **并发控制** | Leader Election vs 分布式锁 | Leader Election | Kubernetes 原生支持 |
| **重试策略** | 固定间隔 vs 指数退避 | 指数退避 | 更好的资源利用 |
| **备份加密** | 不加密 vs 加密 | 加密 | 安全性 |
| **监控** | 日志 vs Metrics | 两者都需要 | 互补 |
