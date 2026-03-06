## 任务：将 GoCode 前端 UI 右对齐到 OpenCode 设计

### 目标
将 gocode (https://github.com/nolouch/gocode) 的 TUI 右状态栏设计和实现对齐到 OpenCode (https://github.com/anomalyco/opencode) 的设计。

### 参考项目
- **gocode**: ~/clawprogram/projects/gocode (你的项目)
- **opencode**: ~/clawprogram/projects/opencode (参考设计)

### OpenCode 右状态栏 (SessionSidePanel) 关键特性
参考文件: `opencode/packages/app/src/pages/session/session-side-panel.tsx`

1. **布局结构**
   - 可调整大小的右侧面板 (ResizeHandle)
   - 多个标签页: Files, Review, Context 等
   - 文件树组件 (FileTree)
   - 差异/审核面板 (review panel)
   - 终端面板 (terminal panel)

2. **组件**
   - Tabs 组件 (@opencode-ai/ui/tabs)
   - IconButton 组件
   - Tooltip 组件
   - ResizeHandle 组件
   - FileTree 文件树
   - Dialog 选择器

3. **功能**
   - 拖拽排序文件标签
   - 上下文使用显示 (SessionContextUsage)
   - 文件差异对比
   - 终端集成

### GoCode 当前 TUI 结构
- 位置: `gocode/internal/cli/tui/`
- 主文件: `app.go`, `run.go`, `styles.go`
- 使用 Bubble Tea 框架

### 任务要求

1. **分析** OpenCode 的右面板设计 (session-side-panel.tsx)
2. **设计** gocode 对应的右侧状态栏，包含:
   - 文件浏览器/树形结构
   - 会话上下文使用量
   - 差异审核视图
   - 可调整大小的面板
3. **实现** 使用 Bubble Tea (gocode 现有框架) 实现设计

### 实现原则
- 视觉设计尽量对齐 OpenCode
- 保持 GoCode 的代码风格
- 可以复用 OpenCode 的交互模式

请先分析两个项目的 UI 实现，然后给出具体的修改方案和代码。
