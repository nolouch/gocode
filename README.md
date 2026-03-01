# gocode

**gocode** 是 [opencode](https://github.com/anthropics/opencode) 的 Go 语言实现。

## 简介

gocode 是一个 AI 编程助手，提供交互式 TUI 界面和 HTTP 服务器模式，支持多种 LLM 提供商。

## 特性

- 🚀 **交互式 TUI** - 基于 Bubble Tea 的终端用户界面
- 🔧 **丰富的工具集** - 文件操作、代码编辑、Bash 执行等
- 🔌 **MCP 支持** - 完整的 Model Context Protocol 实现
  - 本地和远程 MCP 服务器
  - OAuth 2.0 认证（PKCE + CSRF）
  - Prompts 和 Resources 支持
- 🎯 **Skill 系统** - 按需加载专业化工作流
- 🌐 **HTTP 服务器** - RESTful API + SSE 事件流
- 📦 **纯 Go 实现** - 无 CGO 依赖（使用 modernc.org/sqlite）

## 快速开始

### 安装

```bash
go build ./cmd/gocode
```

### 配置

创建配置文件 `~/.gocode/config.yaml`：

```yaml
provider:
  base_url: https://api.openai.com/v1
  api_key: your-api-key
  model: gpt-4o

default_agent: build
```

### 使用

```bash
# 启动交互式 TUI
./gocode tui

# 运行单次命令
./gocode run -p "帮我创建一个 HTTP 服务器"

# 启动 HTTP 服务器
./gocode serve

# 管理 MCP 服务器
./gocode mcp list
./gocode mcp auth <server-name>
```

## 架构

```
gocode/
├── cmd/gocode/       # 主程序入口
├── internal/
│   ├── agent/           # Agent 定义和管理
│   ├── bus/             # 事件总线
│   ├── cli/             # CLI 和 TUI 实现
│   ├── config/          # 配置管理
│   ├── llm/             # LLM 提供商集成
│   ├── loop/            # Agent 执行循环
│   ├── mcp/             # MCP 协议实现
│   ├── processor/       # 消息处理器
│   ├── server/          # HTTP 服务器
│   ├── session/         # 会话管理
│   ├── skill/           # Skill 加载器
│   ├── storage/         # SQLite 存储
│   ├── systemprompt/    # 系统提示生成
│   └── tool/            # 工具实现
└── pkg/sdk/             # Go SDK

```

## 与 opencode 的对齐

gocode 完全对齐 opencode 的设计理念：

- ✅ **Skill 系统** - 按需加载，不在启动时注入
- ✅ **MCP 协议** - 完整支持 Tools、Prompts、Resources
- ✅ **OAuth 认证** - PKCE + CSRF + 动态客户端注册
- ✅ **模块化架构** - 清晰的职责分离

## 开发

```bash
# 运行测试
go test ./...

# 构建
go build ./cmd/gocode

# 查看配置
./gocode config
```

## License

MIT
