# GoReleaser 使用指南

GoReleaser 是一个用于 Go 项目的自动化发布工具，它可以自动构建多平台的二进制文件、生成 Docker 镜像、创建发布说明等。

## 配置文件

项目根目录下的 `.goreleaser.yml` 是 GoReleaser 的配置文件。

## 本地测试

### 安装 GoReleaser

```bash
# macOS
brew install goreleaser

# Linux
curl -sL https://git.io/goreleaser | bash

# Go 安装
go install github.com/goreleaser/goreleaser@latest
```

### 验证配置

```bash
goreleaser check
```

### 构建 Snapshot（不发布）

```bash
goreleaser build --snapshot --clean
```

构建产物将存放在 `dist/` 目录下。

### 模拟发布（不实际发布）

```bash
goreleaser release --snapshot --clean
```

## 自动发布

### 当前发布方式

当前仓库已关闭远程 Release 自动发布。你可以根据自己的托管平台或本地发布流程调整 GoReleaser 配置。

#### 生成 GPG 密钥

```bash
# 生成 GPG 密钥
gpg --full-generate-key

# 列出密钥
gpg --list-keys --keyid-format=long

# 导出私钥
gpg --armor --export-secret-keys YOUR_KEY_ID > private.key

# 将私钥内容保存到你的密钥管理系统
# cat private.key | pbcopy  # macOS
```

## 发布产物

GoReleaser 会生成以下内容：

1. **多平台二进制文件**
   - Linux (amd64, arm64, 386)
   - macOS (amd64, arm64)
   - Windows (amd64, arm64, 386)

2. **压缩包**
   - Linux/macOS: tar.gz
   - Windows: zip

3. **校验和文件**
   - checksums.txt

4. **GPG 签名**
   - checksums.txt.asc

## 语义化版本

推荐使用语义化版本号格式：

- `v1.0.0` - 主版本.次版本.修订号
- `v1.0.1` - Bug 修复
- `v1.1.0` - 新功能（向后兼容）
- `v2.0.0` - 破坏性更改

## 提交消息规范

为了生成更好的 changelog，建议使用以下提交消息前缀：

- `feat:` - 新功能
- `fix:` - Bug 修复
- `docs:` - 文档更新
- `test:` - 测试相关
- `ci:` - CI/CD 相关
- `chore:` - 其他杂项

## Makefile 命令

Makefile 中添加了相关命令：

```bash
make release-test    # 本地测试 goreleaser
make release-snapshot # 构建 snapshot
make release-check   # 验证配置
```
