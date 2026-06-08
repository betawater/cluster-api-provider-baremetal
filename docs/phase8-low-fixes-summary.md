# Phase 8: LOW 优先级修复总结

## 已修复的 6 项 LOW 问题

| # | 文件 | 修复前 | 修复后 |
|---|------|--------|--------|
| 24 | `ssh/manager.go` | `ssh.InsecureIgnoreHostKey()` 硬编码 | 添加 `WithKnownHosts()` 方法支持 known_hosts 文件验证 |
| 25 | `lb/f5.go` | `InsecureSkipVerify: true` 硬编码 | 添加 `InsecureSkipVerify` 配置字段 |
| 26 | `upgrader/oci_puller.go:36-37` | 硬编码示例 registry URL | 移除硬编码值，设为空字符串 |
| 27 | `clusterclass/baremetal-cluster-template.yaml:10` | `host: "PLACEHOLDER"` | 改为 `host: "${CONTROL_PLANE_ENDPOINT_HOST}"` |
| 28 | 测试文件 | 仅 4 个测试文件 | 添加 SSH manager 单元测试（6 个测试用例） |
| 29 | 文档 | 缺少 Phase 5-8 实施记录 | 更新设计文档 |

## 1. SSH 主机密钥验证

### 修复前
```go
config := &ssh.ClientConfig{
    User: creds.Username,
    Auth: []ssh.AuthMethod{
        ssh.Password(creds.Password),
    },
    HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 不安全
    Timeout:         10 * time.Second,
}
```

### 修复后
```go
type SSHManager struct {
    connections   map[string]*SSHConnection
    mu            sync.RWMutex
    idleTimeout   time.Duration
    knownHostsFile string // 新增
}

func (m *SSHManager) WithKnownHosts(path string) *SSHManager {
    m.knownHostsFile = path
    return m
}

func (m *SSHManager) Connect(host string, port int, creds Credentials) (*SSHConnection, error) {
    config := &ssh.ClientConfig{
        User: creds.Username,
        Auth: []ssh.AuthMethod{
            ssh.Password(creds.Password),
        },
        Timeout: 10 * time.Second,
    }

    // 配置主机密钥验证
    if m.knownHostsFile != "" {
        callback, err := knownhosts.New(m.knownHostsFile)
        if err != nil {
            return nil, fmt.Errorf("failed to load known hosts file %s: %w", m.knownHostsFile, err)
        }
        config.HostKeyCallback = callback
    } else {
        // 向后兼容（不推荐用于生产环境）
        config.HostKeyCallback = ssh.InsecureIgnoreHostKey()
    }
    // ...
}

// 添加主机密钥到 known_hosts 文件
func AddHostKey(host string, port int, keyType string, keyData []byte, knownHostsFile string) error {
    f, err := os.OpenFile(knownHostsFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
    if err != nil {
        return fmt.Errorf("failed to open known_hosts file: %w", err)
    }
    defer f.Close()

    addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
    _, err = f.WriteString(fmt.Sprintf("%s %s %s\n", addr, keyType, string(keyData)))
    return err
}
```

### 使用方式
```go
// 生产环境 - 启用主机密钥验证
sshManager := NewSSHManager(5 * time.Minute).
    WithKnownHosts("/etc/capbm/known_hosts")

// 开发环境 - 向后兼容（不推荐）
sshManager := NewSSHManager(5 * time.Minute)
```

## 2. F5 TLS 验证

### 修复前
```go
type F5Config struct {
    Host string `json:"host,omitempty"`
    Port int `json:"port,omitempty"`
    // ... 没有 InsecureSkipVerify 字段
}

// 硬编码跳过验证
httpClient: &http.Client{
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    },
},
```

### 修复后
```go
type F5Config struct {
    Host string `json:"host,omitempty"`
    Port int `json:"port,omitempty"`
    // ...
    InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"` // 新增
}

// 可配置 TLS 验证
tlsConfig := &tls.Config{
    InsecureSkipVerify: config.InsecureSkipVerify,
}

httpClient: &http.Client{
    Transport: &http.Transport{
        TLSClientConfig: tlsConfig,
    },
},
```

### 使用方式
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalCluster
spec:
  loadBalancer:
    provider: f5
    f5:
      host: f5.example.com
      port: 443
      insecureSkipVerify: false  # 生产环境应设为 false
```

## 3. 移除硬编码示例 URL

### 修复前
```go
const (
    DefaultCatalogImage     = "registry.example.com/capbm/release-catalog:latest"
    DefaultUpgradePathImage = "registry.example.com/capbm/upgrade-path:latest"
)
```

### 修复后
```go
const (
    DefaultCatalogImage     = ""
    DefaultUpgradePathImage = ""
)
```

## 4. 移除 PLACEHOLDER

### 修复前
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalClusterTemplate
spec:
  template:
    spec:
      controlPlaneEndpoint:
        host: "PLACEHOLDER"
        port: 6443
```

### 修复后
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalClusterTemplate
spec:
  template:
    spec:
      controlPlaneEndpoint:
        host: "${CONTROL_PLANE_ENDPOINT_HOST}"
        port: 6443
```

## 5. 添加单元测试

### SSH Manager 测试
```go
func TestNewSSHManager(t *testing.T)
func TestSSHManagerWithKnownHosts(t *testing.T)
func TestSSHManagerConnectionCount(t *testing.T)
func TestSSHManagerCloseNil(t *testing.T)
func TestAddHostKey(t *testing.T)
func TestAddHostKeyAppend(t *testing.T)
```

### 测试结果
```
=== RUN   TestNewSSHManager
--- PASS: TestNewSSHManager (0.00s)
=== RUN   TestSSHManagerWithKnownHosts
--- PASS: TestSSHManagerWithKnownHosts (0.01s)
=== RUN   TestSSHManagerConnectionCount
--- PASS: TestSSHManagerConnectionCount (0.00s)
=== RUN   TestSSHManagerCloseNil
--- PASS: TestSSHManagerCloseNil (0.00s)
=== RUN   TestAddHostKey
--- PASS: TestAddHostKey (0.01s)
=== RUN   TestAddHostKeyAppend
--- PASS: TestAddHostKeyAppend (0.00s)
PASS
ok  	github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh	0.189s
```

## 编译验证

- `go vet ./modules/cvo/... ./modules/capbm/...` ✅ 通过
- `go build ./modules/cvo/... ./modules/capbm/...` ✅ 通过
- `go test ./modules/cvo/pkg/ssh/...` ✅ 通过（6/6 测试用例）

## 全部 Phase 实施总结

| Phase | 状态 | 修复数量 | 说明 |
|-------|------|----------|------|
| **Phase 1** (P0) | ✅ 完成 | 4 | OCI Puller、脚本执行、镜像加载、Gateway API 安装器 |
| **Phase 2** (P1) | ✅ 完成 | 14 | 控制平面升级器（etcd 备份、drain、containerd/CNI/CSI 升级等） |
| **Phase 3** (P2) | ✅ 完成 | 2 | MetalLB、Keepalived 负载均衡器实现 |
| **Phase 4** (P3) | ✅ 完成 | 5 | 滚动升级协调器、Worker 节点升级器、暂停/恢复、Metrics |
| **Phase 5** (CRITICAL) | ✅ 完成 | 4 | 钩子执行、端点健康检查、CRD 备份、回滚支持 |
| **Phase 6** (HIGH) | ✅ 完成 | 11 | toYAML、CNI/CSI 升级、MetalLB/EnvoyGateway 安装器、rollback 错误处理 |
| **Phase 7** (MEDIUM) | ✅ 完成 | 9 | 错误忽略修复（25+ 处） |
| **Phase 8** (LOW) | ✅ 完成 | 6 | SSH 主机密钥验证、F5 TLS 验证、硬编码值、PLACEHOLDER、测试覆盖 |
| **总计** | ✅ 完成 | **55** | 所有已识别问题已修复 |
