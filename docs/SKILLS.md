# AnyClaw 技能系统

## 概述

AnyClaw 技能系统允许用户通过可插拔的技能扩展 Agent 的能力。每个技能是一个独立的模块，包含配置文件、文档和工具绑定。

## 技能结构

每个技能目录包含以下文件：

```
skills/
  my-skill/
    SKILL.md          # 技能文档
    skill.json        # 技能元数据
    config.schema.json  # 配置schema (可选)
    tools/            # 工具定义 (可选)
      *.json
    examples/         # 使用示例 (可选)
      *.md
```

## skill.json 格式

```json
{
  "name": "skill-name",
  "description": "技能描述",
  "version": "1.0.0",
  "author": "作者名",
  "source": "local|skillhub|git",
  "repository": "https://github.com/...",
  "license": "MIT",
  "tags": ["coding", "productivity"],
  "permissions": ["read", "write", "exec"],
  "required_tools": ["read_file", "write_file"],
  "provided_tools": ["custom_tool"],
  "configuration": {
    "enabled": true,
    "option1": "value1"
  },
  "dependencies": {
    "other-skill": ">=1.0.0"
  },
  "health_check": "skill-name-health",
  "documentation": "SKILL.md"
}
```

### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 技能唯一标识 |
| description | string | 是 | 技能简短描述 |
| version | string | 是 | 语义化版本号 |
| author | string | 否 | 作者名称 |
| source | string | 否 | 来源: local/skillhub/git |
| repository | string | 否 | Git 仓库地址 |
| license | string | 否 | 开源许可证 |
| tags | string[] | 否 | 技能标签 |
| permissions | string[] | 否 | 所需权限列表 |
| required_tools | string[] | 否 | 依赖的工具 |
| provided_tools | string[] | 否 | 提供的工具 |
| configuration | object | 否 | 默认配置 |
| dependencies | object | 否 | 依赖的其他技能 |
| health_check | string | 否 | 健康检查工具名 |
| documentation | string | 否 | 文档文件名 |

## 安装技能

### 本地安装

```bash
# 从目录安装
./anyclaw skills install ./skills/my-skill

# 从 Git 安装
./anyclaw skills install https://github.com/user/skill-repo
```

### SkillHub 安装

```bash
# 搜索技能
./anyclaw skills search coding

# 安装技能
./anyclaw skills install coder --source skillhub
```

## 配置技能

### 启用/禁用

```bash
# 启用技能
./anyclaw skills enable coder

# 禁用技能
./anyclaw skills disable coder
```

### 配置

```bash
# 查看当前配置
./anyclaw skills config coder

# 设置配置项
./anyclaw skills config coder set option1 value1
```

## 查看技能状态

```bash
# 列出所有技能
./anyclaw skills list

# 查看技能详情
./anyclaw skills info coder

# 查看技能文档
./anyclaw skills doc coder
```

## 权限系统

技能权限定义在 `skill.json` 的 `permissions` 字段：

| 权限 | 说明 |
|------|------|
| read | 读取文件和数据 |
| write | 写入文件和数据 |
| exec | 执行命令 |
| network | 访问网络 |
| admin | 管理员权限 |

### 权限检查

运行时，Agent 会检查技能是否有足够权限执行操作。

## 健康检查

技能可以定义健康检查工具：

```json
{
  "health_check": "skill-health"
}
```

运行健康检查：

```bash
./anyclaw skills health coder
```

## 故障排查

### 技能加载失败

```bash
# 验证技能配置
./anyclaw doctor --check-skills

# 查看详细日志
./anyclaw logs --skill coder
```

### 权限不足

```bash
# 检查权限配置
./anyclaw skills permissions coder
```

## 示例：创建自定义技能

1. 创建技能目录

```bash
mkdir -p skills/my-custom-skill
```

2. 创建 skill.json

```json
{
  "name": "my-custom-skill",
  "description": "My custom skill for XYZ",
  "version": "1.0.0",
  "author": "Your Name",
  "permissions": ["read", "write"],
  "required_tools": ["read_file", "write_file"]
}
```

3. 创建 SKILL.md

```markdown
# My Custom Skill

## 简介
这个技能提供 XYZ 功能。

## 使用方法
使用工具 `custom_tool` 来执行操作。

## 示例
示例用法...
```

4. 安装技能

```bash
./anyclaw skills install ./skills/my-custom-skill
```

## 下一步

- 阅读[快速开始指南](QUICKSTART.md)
- 查看[架构文档](ARCHITECTURE.md)
- 探索更多[示例技能](../skills/)