# Docker 沙箱隔离

## 概述

Shell 工具支持基于 Docker 的沙箱隔离。启用后，所有 shell 命令都在隔离的 Docker 容器中执行，而非直接在主机系统上运行。

## 配置

### 启用沙箱

在 `~/.anyclaw/config.json` 中添加以下配置：

```json
{
  "tools": {
    "shell": {
      "enabled": true,
      "sandbox": {
        "enabled": true
      }
    }
  }
}
```

### 完整配置选项

```json
{
  "tools": {
    "shell": {
      "enabled": true,
      "allowed_cmds": [],
      "denied_cmds": ["rm -rf", "dd", "mkfs", "format"],
      "timeout": 30,
      "working_dir": "",
      "sandbox": {
        "enabled": true,
        "image": "anyclaw/sandbox:latest",
        "workdir": "/workspace",
        "remove": true,
        "network": "none",
        "privileged": false,
        "memory": "512m",
        "cpu_quota": 50000
      }
    }
  }
}
```

### 配置字段

| 字段 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `enabled` | bool | `false` | 启用/禁用 Docker 沙箱 |
| `image` | string | `anyclaw/sandbox:latest` | 容器使用的 Docker 镜像 |
| `workdir` | string | `/workspace` | 容器内的工作目录 |
| `remove` | bool | `true` | 执行后自动删除容器 |
| `network` | string | `none` | 网络模式 (`none`, `bridge`, `host`) |
| `privileged` | bool | `false` | 以特权模式运行容器 |
| `memory` | string | `""` | 内存限制（如 `512m`, `1g`） |
| `cpu_quota` | int | `0` | CPU 配额（微秒，50000 = 50%） |

## 构建沙箱镜像

### Dockerfile

创建一个 `Dockerfile`：

```dockerfile
FROM alpine:latest

# 安装常用工具
RUN apk add --no-cache \
    bash \
    curl \
    wget \
    python3 \
    py3-pip \
    nodejs \
    npm \
    git \
    jq \
    coreutils \
    grep \
    sed \
    awk \
    ca-certificates \
    openssh-client \
    rsync

# 设置工作目录
WORKDIR /workspace

CMD ["/bin/sh"]
```

### 构建镜像

```bash
# 构建镜像
docker build -t anyclaw/sandbox:latest .

# 推送到 Docker Hub 以便远程使用
docker tag anyclaw/sandbox:latest <username>/anyclaw-sandbox:latest
docker push <username>/anyclaw-sandbox:latest
```

### 使用预构建镜像

如果使用 Docker Hub 上的自定义镜像，更新配置：

```json
{
  "tools": {
    "shell": {
      "sandbox": {
        "enabled": true,
        "image": "<username>/anyclaw-sandbox:latest"
      }
    }
  }
}
```

## 工作原理

### 执行流程

```
用户命令请求
      │
      ▼
┌─────────────────────────────────────────────────────────────┐
│                      Shell Tool                              │
│                                                              │
│  检查 sandbox.enabled                                        │
│      │                                                       │
│      ├── false ──► 直接在主机执行                            │
│      │                                                       │
│      └── true ──► Docker 沙箱执行                            │
│                     │                                        │
│                     ▼                                        │
│            ┌────────────────────────────────────┐            │
│            │     Docker 容器创建                 │            │
│            │  - 挂载工作区目录                   │            │
│            │  - 设置网络隔离                     │            │
│            │  - 配置资源限制                     │            │
│            │  - 注入环境变量                     │            │
│            └──────────────┬─────────────────────┘            │
│                           │                                  │
│                           ▼                                  │
│            ┌────────────────────────────────────┐            │
│            │     容器内执行命令                  │            │
│            │  - 超时控制                         │            │
│            │  - 输出捕获                         │            │
│            └──────────────┬─────────────────────┘            │
│                           │                                  │
│                           ▼                                  │
│            ┌────────────────────────────────────┐            │
│            │     清理容器                        │            │
│            │  - 移除容器（如果 remove=true）      │            │
│            └────────────────────────────────────┘            │
│                                                              │
└─────────────────────────────────────────────────────────────┘
      │
      ▼
返回结果给 Agent
```

### 工作区挂载

工作区目录被挂载到容器中指定的 `workdir` 路径：

| 主机路径 | 容器路径 |
|----------|----------|
| `~/.anyclaw/workspace` | `/workspace` |
| 或配置中的 workspace | |

在容器中创建的文件会保存到主机的工作区目录。

### 网络模式

| 模式 | 描述 | 使用场景 |
|------|------|----------|
| `none` | 无网络访问（默认） | 最大安全性，离线操作 |
| `bridge` | 隔离的桥接网络 | 允许出站网络访问 |
| `host` | 主机网络 | 完整网络访问（不推荐） |

## 安全考虑

### 推荐设置

```json
{
  "tools": {
    "shell": {
      "enabled": true,
      "denied_cmds": ["rm -rf", "dd", "mkfs", "format", ":(){ :|:& };:"],
      "sandbox": {
        "enabled": true,
        "network": "none",
        "privileged": false,
        "remove": true,
        "memory": "512m",
        "cpu_quota": 50000
      }
    }
  }
}
```

### 安全优势

- **隔离性**：命令在隔离容器中运行
- **网络隔离**：默认 `none` 模式阻止网络访问
- **无权限提升**：默认非特权模式
- **自动清理**：执行后自动删除容器
- **资源限制**：Docker 强制执行资源约束
- **命令过滤**：`denied_cmds` 额外保护

### 潜在风险

- **文件访问**：工作区以完全读写权限挂载
- **特权模式**：生产环境中绝不要启用 `privileged: true`
- **主机网络**：除非必要，避免使用 `network: "host"`

## 使用示例

### 基本命令执行

```bash
# 启用沙箱后
anyclaw agent --message "列出当前目录文件"

# Agent 执行:
# Tool: run_shell
# Command: ls -la
# [在容器中执行]
# Result: 文件列表...
```

### Python 脚本执行

```bash
anyclaw agent --message "运行 Python 计算 2 的 10 次方"

# Agent 执行:
# Tool: run_shell
# Command: python3 -c "print(2 ** 10)"
# [Result: 1024]
```

### 网络限制

使用 `network: "none"` 时：

```bash
anyclaw agent --message "访问 https://example.com"

# Agent 执行:
# Tool: run_shell
# Command: curl https://example.com
# [Result: curl: (6) Could not resolve host: example.com]
```

如需网络访问，使用 `network: "bridge"`。

## 故障排查

### Docker 未运行

```
Failed to initialize Docker client, sandbox disabled
```

**解决方案**：启动 Docker Desktop 或 Docker 守护进程。

### 镜像未找到

```
failed to create container: Error response from daemon: pull access denied for anyclaw/sandbox
```

**解决方案**：构建或拉取所需镜像。

### 权限被拒绝

```
failed to start container: permission denied
```

**解决方案**：确保用户有 Docker 权限（将用户添加到 docker 组）。

### 容器超时

```
tool execution timed out after 3m0s
```

**解决方案**：增加工具超时时间或优化命令。

## 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                        anyclaw Agent                          │
│                                                              │
│  ┌──────────────┐         ┌──────────────────────────────┐ │
│  │  Shell Tool  │ ──────► │  Docker API                  │ │
│  │              │         │                              │ │
│  │  run_shell() │         │  ContainerCreate()           │ │
│  │  ┌────────┐  │         │  ContainerStart()            │ │
│  │  │Direct  │  │         │  ContainerWait()             │ │
│  │  │Exec    │  │         │  ContainerLogs()             │ │
│  │  └────────┘  │         │  ContainerRemove()           │ │
│  │  ┌────────┐  │         │                              │ │
│  │  │Sandbox │  │         │  ┌────────────────────────┐  │ │
│  │  │Mode    │  │         │  │ anyclaw/sandbox:latest │  │ │
│  │  └────────┘  │         │  │  - bash                │  │ │
│  └──────────────┘         │  │  - python3             │  │ │
│                            │  │  - nodejs              │  │ │
│                            │  │  - git                 │  │ │
│                            │  └────────────────────────┘  │ │
│                            └──────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## API 参考

### ShellTool 结构

```go
type ShellTool struct {
    enabled       bool
    allowedCmds   []string
    deniedCmds    []string
    timeout       time.Duration
    workingDir    string
    sandboxConfig SandboxConfig
    dockerClient  *client.Client
}
```

### SandboxConfig 结构

```go
type SandboxConfig struct {
    Enabled    bool   `mapstructure:"enabled" json:"enabled"`
    Image      string `mapstructure:"image" json:"image"`
    Workdir    string `mapstructure:"workdir" json:"workdir"`
    Remove     bool   `mapstructure:"remove" json:"remove"`
    Network    string `mapstructure:"network" json:"network"`
    Privileged bool   `mapstructure:"privileged" json:"privileged"`
    Memory     string `mapstructure:"memory" json:"memory"`
    CPUQuota   int    `mapstructure:"cpu_quota" json:"cpu_quota"`
}
```

## 与技能系统配合

当使用技能系统时，如果技能需要特定的二进制或包：

1. **技能 SKILL.md 配置依赖**：
   ```yaml
   metadata:
     openclaw:
       requires:
         bins: ["python3", "pip"]
         pythonPkgs: ["requests"]
       install:
         - kind: pip
           package: requests
   ```

2. **沙箱镜像预装依赖**：
   在 Dockerfile 中预装技能所需工具：
   ```dockerfile
   RUN pip3 install requests
   ```

3. **或者动态安装**：
   Agent 会在检测到缺失依赖时尝试安装（如果 `ANYCLAW_SKILL_AUTO_INSTALL=true`）。

## 相关文档

- [配置指南](./cli.md)
- [技能系统](./skills.md)
- [Agent 架构](./agent-design.md)
