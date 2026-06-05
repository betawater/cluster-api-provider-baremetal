# 非 Root 用户 + Sudo 安装支持设计文档

## 1. 概述

### 1.1 设计目标

支持使用非 root 用户（拥有 sudo 权限）在裸金属节点上安装组件，满足企业安全合规要求。

### 1.2 核心原则

| 原则 | 说明 |
|------|------|
| **向后兼容** | 默认行为不变，root 用户直接执行 |
| **最小改动** | 在 SSH 客户端层包装脚本，不修改嵌入式脚本 |
| **灵活配置** | 支持密码相同/不同、NOPASSWD 等场景 |

### 1.3 实现方案

**方案 A：脚本级 sudo 包装**

在 SSH 客户端的 `ExecuteScript` 方法中，根据配置将脚本包装为 `sudo bash -c '<script>'` 执行。

```go
// 修改前
command := fmt.Sprintf("bash -c '%s'", escapedScript)

// 修改后
if useSudo {
    command := fmt.Sprintf("sudo bash -c '%s'", escapedScript)
} else {
    command := fmt.Sprintf("bash -c '%s'", escapedScript)
}
```

---

## 2. 架构设计

### 2.1 数据流

```
┌─────────────────────────────────────────────────────────────────────┐
│  BareMetalMachine Spec                                              │
│                                                                     │
│  spec:                                                              │
│    sshPort: 22                                                      │
│    useSudo: true                    ◄── 新增字段                    │
│    credentialsRef:                                                    │
│      name: node-credentials                                           │
└───────────────────────────┬─────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Secret (node-credentials)                                          │
│                                                                     │
│  data:                                                              │
│    username: YWRtaW4=            (admin)                             │
│    password: cGFzc3dvcmQ=        (password)                          │
│    sudoPassword: c3Vkb3Bhc3M=    (sudopass) ◄── 可选，新增           │
└───────────────────────────┬─────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│  BareMetalMachine Controller                                        │
│                                                                     │
│  1. 读取 Secret 凭据                                                 │
│  2. 构建 SSH Credentials                                             │
│     - Username: admin                                                │
│     - Password: password                                             │
│     - SudoPassword: sudopass (可选)                                  │
│     - UseSudo: true                                                  │
│  3. 调用 Installer.Install()                                         │
└───────────────────────────┬─────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│  SSH Client (ExecuteScript)                                         │
│                                                                     │
│  if useSudo:                                                        │
│      command = "sudo bash -c '<script>'"                             │
│  else:                                                              │
│      command = "bash -c '<script>'"                                  │
│                                                                     │
│  执行: ssh admin@node "sudo bash -c '...'"                           │
└───────────────────────────┬─────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│  BareMetal Node                                                     │
│                                                                     │
│  $ sudo bash -c 'apt-get install -y containerd ...'                  │
│  [sudo] password for admin: ********                                 │
│  ✓ containerd installed                                              │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 3. 详细设计

### 3.1 Credentials 结构扩展

```go
// modules/cvo/pkg/ssh/manager.go

type Credentials struct {
    Username     string
    Password     string
    SudoPassword string // 可选，sudo 密码（如果与登录密码不同）
    UseSudo      bool   // 是否使用 sudo 执行脚本
}
```

### 3.2 SSH Client 修改

```go
// modules/cvo/pkg/ssh/client.go

func (c *SSHConnection) ExecuteScript(ctx context.Context, script string) (*CommandResult, error) {
    escapedScript := bytes.ReplaceAll([]byte(script), []byte("'"), []byte("'\\''"))
    
    var command string
    if c.credentials.UseSudo {
        // 使用 sudo 包装整个脚本
        command = fmt.Sprintf("sudo bash -c '%s'", string(escapedScript))
    } else {
        command = fmt.Sprintf("bash -c '%s'", string(escapedScript))
    }
    
    return c.ExecuteCommand(ctx, command)
}
```

### 3.3 BareMetalMachine CRD 扩展

```go
// modules/capbm/api/v1beta1/baremetalmachine_types.go

type BareMetalMachineSpec struct {
    // ... 现有字段 ...
    
    // UseSudo indicates whether to use sudo for privileged operations.
    // When true, all installation scripts will be executed with sudo.
    // +optional
    UseSudo bool `json:"useSudo,omitempty"`
}
```

### 3.4 Secret 字段扩展

| 字段 | 必填 | 说明 |
|------|------|------|
| `username` | ✅ | SSH 登录用户名 |
| `password` | ✅ | SSH 登录密码 |
| `sudoPassword` | ❌ | sudo 密码（如果与登录密码不同） |

### 3.5 控制器修改

```go
// modules/capbm/internal/controllers/baremetalmachine_controller.go

func (r *BareMetalMachineReconciler) getSSHCredentials(ctx context.Context, bmm *capbmv1.BareMetalMachine) (*ssh.Credentials, error) {
    secret := &corev1.Secret{}
    // ... 获取 Secret ...
    
    creds := &ssh.Credentials{
        Username: string(secret.Data["username"]),
        Password: string(secret.Data["password"]),
        UseSudo:  bmm.Spec.UseSudo,
    }
    
    // 如果提供了 sudoPassword 且与登录密码不同
    if sudoPass, ok := secret.Data["sudoPassword"]; ok && string(sudoPass) != creds.Password {
        creds.SudoPassword = string(sudoPass)
    }
    
    return creds, nil
}
```

### 3.6 sudo 可用性预检

```go
// modules/cvo/pkg/ssh/preflight.go

func CheckSudoAvailable(ctx context.Context, conn *SSHConnection) error {
    result, err := conn.ExecuteCommand(ctx, "sudo -n true 2>/dev/null || sudo -v")
    if err != nil {
        return fmt.Errorf("sudo is not available: %w", err)
    }
    if result.ExitCode != 0 {
        return fmt.Errorf("user does not have sudo privileges")
    }
    return nil
}
```

---

## 4. 使用场景

### 4.1 场景一：root 用户（默认行为）

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachine
metadata:
  name: worker-1
spec:
  sshPort: 22
  useSudo: false  # 或不设置，默认 false
  credentialsRef:
    name: node-credentials
---
apiVersion: v1
kind: Secret
metadata:
  name: node-credentials
type: Opaque
stringData:
  username: root
  password: rootpassword
```

执行命令：`bash -c 'apt-get install -y containerd'`

### 4.2 场景二：非 root 用户 + NOPASSWD sudo

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachine
metadata:
  name: worker-1
spec:
  sshPort: 22
  useSudo: true
  credentialsRef:
    name: node-credentials
---
apiVersion: v1
kind: Secret
metadata:
  name: node-credentials
type: Opaque
stringData:
  username: admin
  password: adminpassword
  # 不需要 sudoPassword，因为配置了 NOPASSWD
```

执行命令：`sudo bash -c 'apt-get install -y containerd'`

节点 sudoers 配置：
```
admin ALL=(ALL) NOPASSWD: ALL
```

### 4.3 场景三：非 root 用户 + 需要 sudo 密码

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: BareMetalMachine
metadata:
  name: worker-1
spec:
  sshPort: 22
  useSudo: true
  credentialsRef:
    name: node-credentials
---
apiVersion: v1
kind: Secret
metadata:
  name: node-credentials
type: Opaque
stringData:
  username: admin
  password: adminpassword
  sudoPassword: sudopassword  # 与登录密码不同
```

执行命令：`sudo -S bash -c 'apt-get install -y containerd'`（通过 stdin 传递密码）

---

## 5. 修改文件清单

| 文件 | 修改内容 | 优先级 |
|------|----------|--------|
| `modules/cvo/pkg/ssh/manager.go` | Credentials 结构添加 SudoPassword、UseSudo | P0 |
| `modules/cvo/pkg/ssh/client.go` | ExecuteScript 添加 sudo 包装 | P0 |
| `modules/capbm/api/v1beta1/baremetalmachine_types.go` | 添加 UseSudo 字段 | P0 |
| `modules/capbm/internal/controllers/baremetalmachine_controller.go` | 传递 sudo 配置 | P0 |
| `modules/cvo/pkg/ssh/preflight.go` | 添加 sudo 可用性检查 | P1 |
| `modules/capbm/api/v1beta1/zz_generated.deepcopy.go` | 重新生成 DeepCopy | P0 |
| `modules/capbm/config/crd/bases/` | 重新生成 CRD YAML | P0 |

---

## 6. 实现计划

### Phase 1: 核心功能

| 任务 | 说明 | 预估时间 |
|------|------|----------|
| 扩展 Credentials 结构 | 添加 SudoPassword、UseSudo 字段 | 0.5 天 |
| 修改 SSH Client | ExecuteScript 添加 sudo 包装 | 0.5 天 |
| 扩展 BareMetalMachine CRD | 添加 UseSudo 字段 | 0.5 天 |
| 修改控制器 | 传递 sudo 配置到 SSH 管理器 | 0.5 天 |
| 重新生成 CRD 和 DeepCopy | make generate manifests | 0.5 天 |

### Phase 2: 增强功能

| 任务 | 说明 | 预估时间 |
|------|------|----------|
| sudo 可用性预检 | 安装前验证 sudo 可用 | 0.5 天 |
| sudo 密码传递 | 支持 -S 标志通过 stdin 传递密码 | 0.5 天 |
| 单元测试 | 测试 sudo 包装逻辑 | 1 天 |

---

## 7. 风险缓解

| 风险 | 缓解措施 |
|------|----------|
| sudo 不可用 | 预检阶段检查，失败时返回明确错误 |
| sudo 密码错误 | 重试机制，最多 3 次 |
| sudo 超时 | 设置合理的 sudo timeout 配置 |
| 向后兼容 | UseSudo 默认 false，不影响现有行为 |

---

## 8. 参考文档

| 文档 | 说明 |
|------|------|
| [ReleaseImage 安装指南](./release-image-install-guide.md) | 使用 ReleaseImage 安装集群 |
| [HTTP 服务器设计](./release-http-server-design.md) | HTTP Server 部署设计 |
