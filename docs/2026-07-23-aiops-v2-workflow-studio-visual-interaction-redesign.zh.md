# aiops-v2 Workflow Studio 视觉与交互重设计方案

> 日期：2026-07-23
> 状态：设计提案
> 适用页面：`/runner/:workflowId`
> 关联文档：
> - `docs/2026-07-21-aiops-v2-workflow-canvas-mcp-control-plane-design.zh.md`
> - `docs/superpowers/plans/2026-07-21-aiops-v2-workflow-canvas-mcp-control-plane-implementation-todo.zh.md`

## 0. 结论

当前界面的问题不只是“颜色不好看”，而是操作所有权、信息架构和视觉优先级同时失控：

- 顶部同时出现保存状态、检查状态、试运行、保存、运行、运行详情、AI 和更多；
- 顶部“运行”、运行面板中的“试运行当前草稿 / 运行已发布版本”不是同一语义；
- `AI / 审阅 / 运行 / 历史` 四个等权 Tab 把不同性质的任务放在同一层；
- Run 页面重复展示门禁、运行目标、当前运行、变量、路径和日志；
- History 页面把事务历史和运行记录固定上下各占一半，即使没有运行记录也保留巨大空白；
- 更多菜单把文档、导入导出、校验、变量和发布混成一个无分组列表；
- 大量边框、圆角卡片、状态胶囊和深色选中块，让每个元素都在争抢注意力。

本方案选择：

1. 页面定位为“**以画布为中心、以版本门禁为主线、AI 为协作者的可审计运维工作台**”；
2. 顶部只保留一个状态驱动的生命周期主操作；
3. 自动保存成功时删除常驻“保存”按钮；
4. 删除含义不明确的“运行”，所有运行按钮必须明确写出“草稿”或“已发布版本”；
5. 工作流级浮层收敛为 `AI 协作 / 变更 / 运行` 三个任务模式；
6. 节点配置继续保留独立节点侧栏，不与工作流级工具混合；
7. 工作流浮层继续使用 Overlay，不改变 Canvas 容器几何；
8. 使用“**生命周期状态轨道**”作为页面唯一标志性视觉元素，其余区域保持安静、紧凑。

这份方案替代旧设计文档中关于统一四 Tab 工作台和顶部固定 Save/Run 操作的视觉与交互描述，不改变 Runner、Workflow Candidate、Approval、Receipt 和持久事务的事实源约束。

---

## 1. 产品对象、用户和页面唯一任务

### 1.1 产品对象

对象是一个可能影响真实基础设施的 Runner Workflow。它同时存在：

- 当前已保存草稿；
- 待确认 Candidate；
- 静态检查和试运行依据；
- 已发布不可变版本；
- 运行记录；
- 可撤销的持久事务。

### 1.2 目标用户

- SRE、运维开发、平台工程师；
- 需要快速判断工作流是否安全、当前操作针对哪个版本；
- 不应被迫理解 `sha256`、`transactionId`、MCP Tool 名或原始 JSON 才能完成主任务。

### 1.3 页面唯一任务

> 让用户安全地理解、修改、检查、试运行并交付一个 Workflow，同时始终知道当前看到的是草稿、Candidate 还是已发布版本。

设计不能把页面做成聊天产品旁边附带一张图，也不能把内部协议状态直接堆给用户。

---

## 2. 截图问题诊断

### 2.1 顶部栏

当前截图顶部同时存在：

- `已自动保存`
- `静态检查通过`
- `试运行当前草稿`
- `保存`
- `运行`
- `运行详情`
- `AI`
- `更多`

问题：

1. 八个控件几乎具有相同视觉重量；
2. 状态和动作都使用胶囊外形，用户难以区分“这是事实”还是“点击后会执行”；
3. 绿色“运行”像当前最安全的下一步，但 Run 面板又显示环境未绑定、草稿不能试运行；
4. 自动保存和手动保存同时存在，容易与“应用 Candidate”混淆；
5. 发布、检查、试运行同时散落在顶部、Run 面板和更多菜单。

### 2.2 工作流浮层

当前浮层连续出现三层标题：

1. `工作流工具`
2. `AI / 审阅 / 运行 / 历史`
3. `工作流 AI`、`运行`、`事务历史`

问题：

- 头部占用过多高度；
- 选中 Tab 使用整块近黑色，视觉压迫感过强；
- 四个 Tab 被做成等权大按钮，但业务优先级并不相同；
- 面板内部继续使用大面积卡片和边框，形成“框套框”；
- 抽屉宽度有限，却显示完整 SHA、版本和重复说明。

### 2.3 AI 空状态

当前 AI 空状态是一张巨大虚线框：

> 选择画布对象并描述目标。AI 会先展示待确认修改……

问题：

- 空间占用很大，但没有给出可立即执行的任务；
- 用户已经位于具体 Workflow，不需要再次选择 Workflow；
- 不应恢复此前已删除的 Workflow selector 或“本次发送范围”模块；
- 当前节点上下文可以被动展示，但不能变成另一个需要用户管理的作用域选择器。

### 2.4 Run 页面

当前 Run 页面同时出现：

- `运行`标题和说明；
- 一张完整检查状态卡；
- 草稿目标卡；
- 已发布版本卡；
- 当前运行详情；
- 变量、路径、日志调试器；
- 禁用的大型试运行按钮；
- 完整 SHA。

问题：

- “下一步是什么”被淹没；
- 检查通过但环境未绑定时，界面只展示禁用动作；但当前系统没有可信的环境绑定入口，因此应诚实显示阻断状态，在环境配置 capability 落地前不能制造一个实际无效的“选择测试环境”按钮；
- 草稿和已发布目标虽然在文案上区分，视觉上仍是两张等权大卡；
- 变量和日志在多个组件中重复；
- 固定上下 `1fr / 1fr` 产生双滚动和空白区域。

### 2.5 History 页面

当前 History 页面把：

- 服务端持久事务；
- Page 内存运行记录；

固定分为上下各一半。

问题：

- 没有运行记录时，下半区仍占一半高度；
- 每条事务都用大圆角卡片，信息密度过低；
- “已完成，有提醒”和“撤销此操作”比业务动作名称更抢眼；
- 持久事务与运行记录的数据可靠性不同，不能被误认为同一事实源。

### 2.6 更多菜单

当前菜单为一个平铺列表：

- 生成运维手册；
- 导入；
- 导出；
- 校验；
- 发布前检查；
- 变量；
- 发布。

问题：

- 高频生命周期动作与低频文件动作混在一起；
- 校验、试运行和发布与顶部主操作重复；
- 发布被埋在列表末尾，但又是高风险动作；
- 手写 Menu 缺少完整方向键导航、碰撞处理和关闭后焦点恢复；
- 菜单层级高于 Drawer，截图中会直接覆盖 Drawer 内容。

---

## 3. 设计目标与非目标

### 3.1 设计目标

1. 用户在 5 秒内回答：
   - 当前是否已保存；
   - 当前看到的是草稿还是已发布版本；
   - 是否已检查、是否已试运行；
   - 当前是否存在安全下一步；若不存在，阻断原因是什么。
2. 页面普通状态下最多只有一个实心主按钮；没有真实恢复能力的阻断状态允许为零。
3. 所有“运行”动作必须明确目标版本。
4. Canvas 始终是视觉主角。
5. 工作流浮层可自由关闭，打开和切换时 Canvas 容器几何差值为 `0px`。
6. 节点操作和工作流操作继续保持独立 owner。
7. 默认 UI 不暴露 Hash、Tool 名、transactionId、Raw JSON。
8. 空状态必须告诉用户下一步，而不是只说明系统原理。
9. 桌面、平板、手机使用同一语义，不通过隐藏关键状态来适配。

### 3.2 非目标

- 不改变 Workflow 的后端生命周期规则；
- 不把草稿试运行与已发布版本运行合并；
- 不把服务端持久事务和 Page 内存运行记录合并成同一数据模型；
- 不恢复 Workflow selector、“本次发送范围”或结构大纲；
- 不用新的视觉层掩盖尚未完成的运行/发布能力；
- 不全局修改 shadcn `--primary`，避免影响其他页面。

---

## 4. 方案比较与选择

| 方案 | 内容 | 优点 | 问题 | 结论 |
|---|---|---|---|---|
| A. 只做视觉润色 | 换颜色、圆角和阴影 | 成本低 | 不解决重复入口和错误语义 | 不采用 |
| B. 固定右侧工作台 | 工作流工具始终占据 420px | 内容稳定 | 再次压缩 Canvas，与最新用户反馈冲突 | 不采用 |
| C. 状态驱动顶部栏 + Overlay 工作流面板 | 顶部唯一主操作，工作流面板可收起 | 同时解决层级、语义和 Canvas 空间 | 需要重构 action owner 和 Run/History 内容 | **采用** |

---

## 5. 设计方向：“安静的控制室”

### 5.1 视觉性格

- 专业、克制、准确；
- 接近运维控制台，而不是营销后台或通用聊天应用；
- 结构和状态承担视觉表达，不靠渐变、玻璃效果或大量彩色卡片；
- 状态色不与操作色混用。

### 5.2 标志性元素：生命周期状态轨道

页面唯一有辨识度的元素是一条真实编码状态的轨道：

```text
已保存 ✓ ─ 静态检查 ✓ ─ 草稿试运行 ○ ─ 发布 ○
```

规则：

- 每个节点绑定当前精确 `resourceVersion / semanticHash` 的服务端事实；
- 已完成使用小型绿色确认点；
- 当前待办使用蓝色实心点和短标签；
- 过期或阻断使用琥珀/红色图标及明确文案；
- 不用持续流动动画；
- `<1180px` 时收敛为“检查通过 · 尚未试运行”，点击后展开完整轨道；
- 只在这一处表达完整生命周期，其他区域不再重复一套彩色状态胶囊。

这是本方案唯一的视觉风险点。它是业务事实的可视化，不是装饰性 Stepper。

### 5.3 Workflow 局部 Token

先以 `--workflow-*` 形式局部落地，不修改全站基础主题。

| Token | 值 | 用途 |
|---|---:|---|
| `--workflow-canvas` | `#F8FAFC` | Canvas 与页面浅底 |
| `--workflow-panel` | `#FFFFFF` | 顶部栏、Drawer、节点侧栏 |
| `--workflow-ink` | `#172033` | 主标题和关键事实 |
| `--workflow-muted` | `#667085` | 辅助信息 |
| `--workflow-line` | `#D7DEE8` | 分隔线与边框 |
| `--workflow-command` | `#2563EB` | 当前焦点和主操作 |
| `--workflow-command-hover` | `#1D4ED8` | 主操作 Hover |
| `--workflow-command-soft` | `#EFF6FF` | 当前 Tab、信息提示 |
| `--workflow-success` | `#15803D` | 已确认成功事实 |
| `--workflow-warning` | `#B45309` | 待配置、过期 |
| `--workflow-danger` | `#B91C1C` | 阻断、删除、失败 |

原则：

- 蓝色表示“可操作 / 当前选择”；
- 绿色只表示“已确认成功”，不再作为普通 Run 按钮颜色；
- 琥珀只表示需要处理；
- 红色只表示阻断或破坏性动作；
- 内部列表优先用分隔线和留白，不再每一行套卡片。

### 5.4 字体与字号

保留现有系统字体，避免额外字体加载：

```css
-apple-system, BlinkMacSystemFont, "SF Pro Text", "PingFang SC",
"Segoe UI", "Helvetica Neue", Arial, sans-serif
```

| 角色 | 规格 |
|---|---|
| Workflow 名称 | `18px / 600` |
| Drawer 标题 | `16px / 600` |
| 区块标题 | `14px / 600` |
| 正文与按钮 | `13–14px / 500` |
| 元信息 | `12px / 400` |
| Hash、表达式、日志 | `12px ui-monospace` |

当前大量 `800/900` 字重统一下降到 `500/600`，通过层级和间距建立重点。

### 5.5 间距、圆角和阴影

- 间距序列：`4 / 8 / 12 / 16 / 20 / 24px`；
- 普通控件圆角：`8px`；
- 面板圆角：`12px`；
- Dialog / Bottom Sheet：`16px`；
- 顶部按钮高度：`36px`；
- Drawer 普通按钮：`40px`；
- 移动端触控目标：至少 `44px`；
- 只有 Menu、Drawer、Dialog 使用 Shadow；
- 内部列表和卡片不使用 Shadow。

---

## 6. 新信息架构

### 6.1 页面区域

```text
App Sidebar
└── Workflow Studio
    ├── Workflow Command Bar
    ├── Canvas Workspace
    │   ├── Canvas
    │   └── Node Sidebar（选中节点时出现）
    ├── Workflow Panel（可收起 Overlay）
    │   ├── AI 协作
    │   ├── 变更
    │   └── 运行
    └── Candidate Review Bar（仅存在 Candidate 时出现）
```

### 6.2 动作唯一所有权

| 动作 | 唯一 owner | 不再出现的位置 |
|---|---|---|
| 自动保存状态 | Command Bar 状态区 | 常驻 Save 按钮 |
| 检查 / 试运行 / 发布 / 运行已发布版本 | 状态驱动 Primary Action Slot | 固定 Run 按钮、More 重复项 |
| 应用 / 放弃 Candidate | `WorkflowCandidateWorkspaceActionBar`（唯一） | 顶部生命周期按钮、变更面板重复 Footer |
| 节点运行 / 节点配置 | Node Sidebar | Workflow Panel |
| AI 修改 | AI 协作 Composer | Canvas 空白主按钮之外的重复入口 |
| Undo / Redo | Command Bar 图标 + 变更记录上下文 | 每张历史大卡的固定大按钮 |
| 导入 / 导出 / 生成运维手册 | More 分组菜单 | 生命周期区域 |
| 变量、路径、日志 | 运行面板中的运行详情 | More 菜单、重复 Debug Dock |

### 6.3 Primary Action 固定所有权

同一时刻最多渲染一个实心主操作，owner 由明确的状态机固定决定，不能由当前打开的 Panel 决定：

1. 普通生命周期状态：`WorkflowCommandBar` 是唯一 owner；
2. Candidate Ready / Conflict：`WorkflowCandidateWorkspaceActionBar` 是唯一 owner，Command Bar 不渲染 Primary；
3. 确认 Dialog 打开：Dialog 是最终确认动作的唯一 owner，背景 owner 必须隐藏或 `inert`；
4. Run Panel 永远不接管生命周期 Primary，只展示事实、技术详情，以及确实可用的辅助配置动作；
5. 移动端底部按钮是当前固定 owner 的响应式呈现，不是第二份动作实例。

所有呈现复用同一个 `WorkflowPrimaryActionViewModel`，不能在 Command Bar、Candidate Bar、Dialog 或移动端各自重新推导业务状态。

---

## 7. 顶部 Command Bar

### 7.1 桌面布局

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ ← 工作流  8-redis  [草稿]   已保存 ✓ ─ 检查 ✓ ─ 未试运行 ○                 │
│                                     [↶] [↷] [工作台] [试运行草稿] [···]     │
└──────────────────────────────────────────────────────────────────────────────┘
```

实际保持一行：

- 左侧：返回、名称、生命周期小标签；
- 中间：生命周期状态轨道；
- 右侧：Undo、Redo、工作台、唯一主操作、More；
- Undo/Redo 可用通用图标，但必须带 Tooltip 和可访问名称；
- More 使用通用省略号图标，不再占用“更多”文字宽度；
- `工作台` 打开上次使用的工作流面板 Tab；
- `<1180px` 时轨道收敛为短状态；
- `<860px` 时工作台改为图标按钮，主操作保留文字。

### 7.2 删除的顶部动作

- 常驻“保存”；
- 含义不明确的“运行”；
- “运行详情”；
- 独立的“AI”和“工作流工具”重复入口；
- 与动态主操作重复的检查、试运行和发布。

自动保存失败、离线草稿或冲突时，可以显示明确恢复动作：

- `重新同步`
- `保存失败，重试`
- `解决冲突`

但恢复动作仍遵守单一 Primary 规则。

### 7.3 状态驱动主按钮

| 当前状态 | 唯一主按钮 | 目标 |
|---|---|---|
| 未检查 / 检查过期 | 检查工作流 | 当前已保存草稿 |
| 检查阻断 | 修复 N 个问题 | 当前草稿问题 |
| 检查通过、环境未绑定且存在可信配置 capability | 配置测试环境 | 草稿试运行配置 |
| 检查通过、环境未绑定且无可信配置 capability | —（阻断） | 明确说明环境绑定能力暂不可用 |
| 检查通过且环境已绑定 | 试运行草稿 | 当前草稿，非生产 |
| 试运行通过、尚未发布 | 发布工作流 | 当前草稿 |
| 已发布且无草稿变化 | 运行已发布版本 | 不可变已发布 revision |
| 已发布但草稿有变化 | 发布更改 | 当前草稿 |
| Candidate 待审阅 | 应用 N 项修改 | 当前 Candidate |
| Candidate 冲突 | 审阅冲突 | 三方 Diff |

禁止：

- 只写“运行”；
- 检查通过但环境未绑定时仍强调禁用试运行；
- 顶部“运行”走旧路径，Run Panel 再走另一条权威路径。

---

## 8. Workflow Panel

### 8.1 外壳

- 桌面宽度：`clamp(420px, 38vw, 480px)`；
- 距 Workspace 上、右、下各 `12–16px`；
- 使用 absolute Overlay；
- 打开、关闭、切换 Tab 时 Canvas 容器 `x / width` 差值为 `0px`；
- 不保留 Canvas 内的重复悬浮“工作流工具”按钮；
- 入口统一放在 Command Bar；
- 关闭后焦点返回 Command Bar 的“工作台”按钮；
- 节点侧栏保持 mounted，在 Overlay 打开时使用 `inert + aria-hidden`；
- 关闭 Overlay 后恢复节点选区和未保存字段。

### 8.2 头部

当前三级头部改成一层：

```text
┌──────────────────────────────────────────────┐
│ AI 协作        变更 2        运行         [×]│
├──────────────────────────────────────────────┤
│ Active Panel                                 │
└──────────────────────────────────────────────┘
```

规则：

- Tab 使用 `line` 风格；
- Active 为蓝色文字 + `2px` 下划线或浅蓝底；
- 不使用整块黑色胶囊；
- 变更数量使用小 Badge；
- 当前 Workflow 由路由唯一确定，不在 Panel 内再次显示或选择；
- 节点选区作为内部上下文传递，仅在具体 Candidate、Diff 或运行结果确有必要时随内容展示；
- 不出现常驻作用域栏、Workflow 下拉或“本次发送范围”模块；
- 无选区时默认作用于当前 Workflow，不要求用户先选节点。

### 8.3 一级入口

从四个收敛为三个：

1. **AI 协作**
   - 提问、解释、生成修改、计划和进度。
2. **变更**
   - 待审阅 Candidate；
   - 检查问题；
   - 已保存修改；
   - Undo / Redo。
3. **运行**
   - 运行门禁；
   - 草稿试运行；
   - 已发布版本运行；
   - 运行记录、变量、路径和日志。

`历史` 不再作为一级 Tab：

- 持久事务归入“变更 → 已保存修改”；
- 试运行和生产运行记录归入“运行 → 运行记录”；
- 两者保持不同的数据来源和持久性标识。

---

## 9. AI 协作面板

### 9.1 空状态

删除巨大虚线框，改成任务导向空态：

```text
想修改什么？

直接描述目标，AI 会先展示待确认修改。

[为失败添加重试]
[解释当前工作流]
[检查是否可以运行]
```

规则：

- 快捷任务最多 3 个；
- 必须来自服务端投影的 Workflow AI capability / prompt preset，并受 rollout 与权限门禁约束；
- 节点专属快捷任务只有在存在显式映射时才可引用 Action Catalog，不能直接从节点动作目录推断 AI Prompt；
- 有节点选区时更新为该节点相关任务；
- 无节点选区时默认针对当前 Workflow；
- 不增加 scope 选择器；
- 快捷任务是 Secondary / Ghost，不与主操作竞争。

### 9.2 对话状态

- Composer 固定底部；
- Transcript 是唯一主滚动区；
- Workflow 由路由确定，冻结的节点选区作为 Turn 内部上下文传递，不增加常驻 scope 模块；
- Tool 名、Raw JSON 和内部协议默认折叠；
- Candidate 到达时沿用现有交互合同，自动激活“变更”面板并聚焦待审阅内容；
- 自动切换不能清空 AI 草稿、Transcript 或冻结的 Turn 上下文，返回 AI 后状态必须完整保留；
- 生成期间允许停止和追加约束，但持久队列能力未完成前不能制造假排队体验。

### 9.3 文案

| 当前 | 建议 |
|---|---|
| 工作流 AI | AI 协作 |
| 可以继续提问或修改 | 描述目标，AI 会先展示待确认修改 |
| 选择画布对象并描述目标 | 想修改什么？ |
| 此区域暂无内容 | 说明为什么为空，并给一个可执行动作 |

---

## 10. 变更面板

### 10.1 二级结构

```text
[待审阅 2] [已保存修改]
```

- `待审阅`：Candidate、Diff、问题、冲突、Apply / Reject；
- `已保存修改`：服务端持久事务、Undo / Redo；
- 不显示运行记录。

### 10.2 已保存修改列表

改为日期分组的紧凑时间线：

```text
今天
21:05  调整工作流布局 · 3 个对象      已保存       [撤销]
21:04  更新工作流 · 4 个对象          已保存
20:58  修改 Shell Script 超时         1 条提醒      [查看]
```

规则：

- 左侧时间 + 细状态轨道；
- 中间业务动作名；
- 右侧紧凑状态；
- 最近一条可撤销记录可以直接显示“撤销”；
- 其他操作放行尾 Menu 或展开 Footer；
- 默认不显示 transactionId；
- 冲突项展开三方 Diff；
- 100 条记录仍保持单一滚动和可导航性。

### 10.3 Candidate 模式

- 顶部摘要：新增 / 修改 / 删除 / 问题数；
- 中间为紧凑 Diff 列表；
- 点击条目定位 Canvas；
- `WorkflowCandidateWorkspaceActionBar` 承载“应用 N 项修改”与“放弃”；
- 变更面板只负责呈现摘要与 Diff，不再复制 Apply / Reject Footer；
- Candidate 存在时，Workspace Action Bar 是页面唯一 Primary owner，Command Bar 不渲染生命周期 Primary。

---

## 11. 运行面板

### 11.1 新结构

```text
┌ 运行───────────────────────────────────────┐
│ 已保存 ✓ ─ 检查 ✓ ─ 试运行 ○ ─ 发布 ○      │
│                                             │
│ 当前草稿                                    │
│ 检查通过 · 尚未试运行                       │
│ 测试环境         未选择 [配置测试环境]*      │
│                                             │
│ 已发布版本                                  │
│ 尚无已发布版本                              │
│                                             │
│ 最近运行                                    │
│ 还没有运行记录。试运行后会显示结果和耗时。   │
│                                             │
│ 技术详情                             [展开] │
└─────────────────────────────────────────────┘
```

`*` 仅在服务端声明真实环境配置 capability 且存在可信配置入口时显示；否则该行只呈现阻断状态和原因。

### 11.2 门禁摘要

- 使用一条紧凑状态轨道，不再使用整块彩色状态卡；
- 检查通过但环境未绑定时，仅在可信环境配置 capability 和真实配置入口都存在时显示“配置测试环境”；
- capability 不存在时只显示明确阻断状态，不渲染假 CTA，也不能用重新打开 Run Panel 冒充恢复动作；
- 不显示一个灰掉的大型试运行按钮；
- 检查失败时显示首个阻断问题和“查看全部问题”；
- 状态事实必须绑定当前草稿版本。

### 11.3 运行目标

两个目标仍必须严格分开，但不再做成两张等权大卡：

1. `当前草稿 · 非生产`
2. `已发布版本 · vN`

采用紧凑 Target Row：

- 名称；
- 版本摘要；
- 环境；
- 当前可用性；
- 一项相关动作。

只有当前可执行目标展开详细信息。不存在已发布版本时只显示说明，不渲染禁用大按钮。

### 11.4 技术信息

默认隐藏：

- 完整 SHA；
- semantic hash；
- resourceVersion；
- input fixture hash；
- Raw payload。

在“运行依据 / 技术详情”中显示：

- 短 Hash；
- Copy；
- 完整 Tooltip 或详情行；
- 校验时间和环境。

### 11.5 运行记录与调试

- Run 页面只保留一个滚动 owner；
- 删除固定 `1fr / 1fr`；
- 当前运行、变量、路径、日志整合为 `RunInspector`；
- 运行完成后按需展开；
- `变量 / 路径 / 日志` 只在当前运行详情内部出现；
- 不再同时渲染 `RunnerRunPanel` 与另一套 `RunnerDebugDock`；
- 运行记录使用紧凑列表，点击后在原面板展开详情；
- Page 内存记录必须标识“当前会话”，不能伪装成服务端持久历史。

---

## 12. More 菜单

### 12.1 新分组

```text
文件
  导入工作流…
  导出工作流…

辅助
  生成运维手册
  查看快捷键

管理
  复制工作流
  删除工作流…        // 独立危险区
```

当前能力未提供“复制 / 删除”时不展示占位项。

### 12.2 移出的项目

- 校验 → 生命周期 Primary；
- 发布前检查 → “试运行草稿”；
- 变量 → 运行面板；
- 发布 → 生命周期 Primary。

### 12.3 交互规范

- 使用现有 Radix `DropdownMenu`；
- 打开后焦点进入第一项；
- 支持 ArrowUp / ArrowDown；
- Escape 关闭并返回 More Trigger；
- 菜单宽度 `220–240px`；
- 使用分组标题和 Separator；
- 发布、删除等高风险操作不能与普通导入导出混在一组；
- Menu 与 Drawer 遵守统一 Overlay 层级，不相互遮挡关键操作。

---

## 13. 按钮规范

### 13.1 四级按钮

| 级别 | 样式 | 用途 |
|---|---|---|
| Primary | Command Blue 实心 | 页面当前唯一安全下一步 |
| Secondary | 中性描边 / Soft | 工作台、查看、配置环境（capability 可用时） |
| Tertiary | Ghost / 图标 | Undo、Redo、关闭、展开 |
| Destructive | 红色 Soft / 确认后实心 | 删除、放弃、破坏性操作 |

### 13.2 规则

1. 页面普通状态实心主按钮数量不超过 `1`；没有真实恢复能力的阻断状态允许为 `0`；
2. 绿色不再用于普通操作按钮；
3. 禁用按钮旁必须显示原因和可恢复动作；
4. 不用 Tooltip 代替关键禁用原因；
5. 图标按钮仅用于用户普遍理解的 Undo、Redo、Close、More、Expand；
6. 不熟悉的工具保留图标 + 文字；
7. 同一动作在顶部、Drawer、Menu 中不能重复出现；
8. 移动端触控目标至少 `44×44px`。

---

## 14. 响应式设计

### 14.1 ≥1280px

- 完整 Command Bar；
- 完整生命周期轨道；
- Workflow Panel `420–480px`；
- Node Sidebar 保持独立 Split Pane；
- Overlay 打开后对 React Flow 设置可见安全 inset，避免关键节点被遮挡，但不改变 Canvas DOM 几何。

### 14.2 1024–1279px

- 生命周期轨道收敛为短状态；
- Undo / Redo 使用图标；
- Workbench 改为图标 + Tooltip 或短标签；
- Workflow Panel 固定约 `420px`；
- 顶部仍保持单行。

### 14.3 821–1023px

- Workflow Panel 使用全高 Overlay；
- 头部 Tab 保留；
- Command Bar 隐藏次要状态，只显示当前主状态；
- Node Sidebar 与 Workflow Panel 不同时交互。

### 14.4 ≤820px

保持四个全屏任务模式：

```text
画布 | AI | 变更 | 运行
```

移动端：

```text
┌──────────────────────────────┐
│ ‹ 8-redis [草稿]      已保存 ⋯│
│ 保存 ✓  检查 ✓  试运行 ○ 发布○│
├──────────────────────────────┤
│                              │
│        当前全屏模式           │
│                              │
├──────────────────────────────┤
│ [当前唯一主操作]              │
├──────────────────────────────┤
│ 画布       AI       变更   运行│
└──────────────────────────────┘
```

规则：

- Primary 固定在底部导航上方；
- More 使用 Bottom Sheet；
- 节点配置由选区打开全屏 Sheet；
- AI Composer 适配 safe area 和软键盘；
- 切换模式不丢 AI 草稿、选区、滚动位置和未保存节点字段；
- 日志使用全屏详情，不在窄屏堆叠多列 Tab；
- 关闭 Drawer / Sheet 后焦点返回原触发对象。

---

## 15. 空、加载、失败与完成状态

### 15.1 空状态

空状态必须包含：

1. 发生了什么；
2. 为什么为空；
3. 下一步可做什么。

示例：

| 场景 | 文案 |
|---|---|
| AI 无会话 | 想修改什么？直接描述目标，AI 会先展示待确认修改。 |
| 无 Candidate | 暂无待审阅修改。AI 或手工修改产生候选后会显示在这里。 |
| 无持久事务 | 还没有已保存修改。 |
| 无运行记录 | 还没有运行记录。试运行后会显示结果、耗时和日志。 |
| 无已发布版本 | 尚无已发布版本。完成检查和试运行后即可发布。 |

### 15.2 加载

- 骨架只用于数据列表和面板正文；
- 状态轨道显示“正在同步依据”；
- 不用不可解释的整页 Spinner；
- 长任务允许停止；
- 局部加载不锁死 Canvas。

### 15.3 失败

- 错误使用不影响 Canvas 布局的 Toast 或局部 Alert；
- 文案必须说明失败动作、数据是否已保存、恢复动作；
- 不显示泛化的 `workflow command could not be completed`；
- 不用红色大面积背景；
- 错误关闭后焦点返回触发对象。

---

## 16. 组件复用与代码结构

### 16.1 应复用的现有基础组件

- `components/ui/Button`
- `components/ui/Badge`
- `components/ui/DropdownMenu`
- `components/ui/Tabs` 的 `variant="line"`
- `components/ui/Alert`
- `components/ui/Tooltip`
- `components/ui/ScrollArea`
- `components/ui/Dialog`
- 移动端 `components/ui/Sheet`

桌面 Workflow Panel 继续保留 Workspace 内 absolute aside，不直接换成 fixed Sheet。

### 16.2 建议新增的业务组合组件

```text
web/src/runner/
├── WorkflowCommandBar.tsx
├── WorkflowLifecycleTrace.tsx
├── WorkflowPrimaryAction.tsx
├── WorkflowActionMenu.tsx
├── WorkflowToolPanel.tsx
├── WorkflowStateBanner.tsx
├── WorkflowTargetRow.tsx
├── WorkflowChangeList.tsx
├── WorkflowHistoryRow.tsx
└── WorkflowRunInspector.tsx
```

这些是业务组合组件，不再新增另一套基础 Button、Card 或 Tabs。

### 16.3 现有文件迁移

| 文件 | 调整 |
|---|---|
| `web/src/pages/RunnerStudioPage.tsx` | 移除固定主按钮数组和手写 More；保留页面数据和回调，Command Bar 由独立组件渲染 |
| `web/src/runner/WorkflowLifecycleHeader.tsx` | 保留 selector / view model，逐步改为 `WorkflowLifecycleTrace + WorkflowPrimaryAction` |
| `web/src/runner/WorkflowContextDock.tsx` | 保留 Overlay、焦点、事件和移动模式；一级 Tab 改为 AI / 变更 / 运行 |
| `web/src/runner/WorkflowEditorShell.tsx` | 重新装配三种工作流任务模式；节点 sidebar 保持独立 |
| `web/src/runner/WorkflowRunPanel.tsx` | 改为门禁摘要、Target Row、单滚动 RunInspector |
| `web/src/runner/WorkflowHistoryPanel.tsx` | 改为紧凑持久事务列表，归入变更面板 |
| `web/src/components/runner/runnerStudio.css` | 删除旧 Toolbar / More / Run / Debug 重复样式 |
| `web/src/runner/workflowWorkspace.css` | 保留 Overlay 和 Node Split Pane；新增局部 token，删除固定上下均分 |

### 16.4 必须保留的兼容契约

迁移阶段优先保留：

- Drawer open / close event；
- Candidate 完成后自动激活 Review / 变更面板；
- Escape 关闭和焦点回归；
- AI feature flag；
- Node Sidebar `inert / aria-hidden`；
- Canvas geometry 不变；
- 移动端 Node configuration Sheet；
- 现有稳定 test ID，完成迁移后再机械重命名。

---

## 17. 分阶段实施建议

### Phase A：动作所有权与 Topbar

1. 建立 `workflowStudioActionRegistry`；
2. 抽出 `WorkflowCommandBar`；
3. 接入生命周期状态轨道；
4. 保留唯一 Primary；
5. 删除常驻 Save、含义不明 Run、运行详情；
6. More 改为分组 Radix Menu。

退出条件：

- 顶部有文字的动作不超过 3 个；
- 普通状态只有一个实心按钮；
- 自动保存开启时 Save 数量为 0；
- 所有运行按钮名称明确版本目标。

### Phase B：Workflow Panel 外壳

1. 三级 Header 收敛为一层；
2. 四 Tab 收敛为三 Tab；
3. 改用 Line Tabs；
4. 移除 Canvas 内重复 Launcher；
5. 保留 Overlay 几何、焦点和事件合同。

退出条件：

- Drawer 切换期间 Canvas 几何差值为 `0px`；
- Header 总高度不超过 `96px`；
- 关闭后焦点返回 Workbench Trigger；
- 无黑色大面积 Active Tab。

### Phase C：Run Center

1. 合并运行门禁；
2. 引入 Target Row；
3. 环境未绑定时接入真实、可信的环境配置 capability；能力落地前保持诚实阻断，不放置假按钮；
4. 合并当前运行和 Debug；
5. 移除固定 `1fr / 1fr`；
6. 默认隐藏完整 Hash。

退出条件：

- 用户不展开技术详情即可判断运行目标；
- 不存在重复变量 / 日志区域；
- 无双滚动；
- 草稿和已发布目标不会被误认。

### Phase D：Changes 与 History

1. Candidate 和持久事务归入变更；
2. 历史改为时间线列表；
3. 运行记录归入 Run；
4. 空运行记录不占固定高度；
5. Undo / Redo 使用上下文操作。

退出条件：

- 持久事务和运行记录不会显示为同一可靠性；
- 100 条记录可稳定导航；
- 最近一次撤销最多 2 次点击可达。

### Phase E：AI、移动端与视觉收尾

1. 重做 AI 空态；
2. 统一 Button / Badge / Alert；
3. 完成移动模式和 Bottom Sheet；
4. 删除废弃 CSS；
5. 更新视觉基线和真实浏览器证据。

---

## 18. 测试方案

### 18.1 状态与按钮单元测试

| 编号 | 场景 | 断言 |
|---|---|---|
| UX-HDR-001 | 未检查 | 唯一 Primary 为“检查工作流” |
| UX-HDR-002 | 检查通过、环境未绑定 | capability 关闭时 Primary 为 0 且明确显示阻断原因；capability 开启且入口可信时唯一 Primary 为“配置测试环境” |
| UX-HDR-003 | 环境已绑定 | Primary 为“试运行草稿” |
| UX-HDR-004 | 试运行通过 | Primary 为“发布工作流” |
| UX-HDR-005 | 已发布、无草稿变化 | Primary 为“运行已发布版本” |
| UX-HDR-006 | 已发布、有草稿变化 | Primary 为“发布更改” |
| UX-HDR-007 | Candidate Ready | Topbar Primary 降级，Review 只有一个“应用 N 项修改” |
| UX-HDR-008 | 自动保存开启 | 常驻 Save 数量为 0 |
| UX-HDR-009 | 保存失败 | 显示“重新同步/重试”，不显示成功状态 |
| UX-HDR-010 | 任意状态 | 页面实心 Primary 数量不超过 1；无真实恢复能力的阻断状态允许为 0 |

### 18.2 信息架构测试

| 编号 | 断言 |
|---|---|
| UX-IA-001 | Workflow Panel 一级入口仅 AI 协作 / 变更 / 运行 |
| UX-IA-002 | 节点配置只存在于 Node Sidebar / Mobile Sheet |
| UX-IA-003 | 持久事务只存在于变更 → 已保存修改 |
| UX-IA-004 | 运行记录只存在于运行面板 |
| UX-IA-005 | More 按文件 / 辅助 / 管理分组 |
| UX-IA-006 | More 不出现检查、试运行、变量和发布的重复入口 |
| UX-IA-007 | 默认 DOM 不出现完整 sha256、transactionId、Tool 名或 Raw JSON |
| UX-IA-008 | AI 面板不出现 Workflow selector 和“本次发送范围” |

### 18.3 Run 测试

| 编号 | 场景 | 断言 |
|---|---|---|
| UX-RUN-001 | 草稿未绑定环境 | 无配置 capability 时显示阻断且没有 CTA；有可信配置 capability 时显示“配置测试环境”；两种情况都不显示可误触的禁用大按钮 |
| UX-RUN-002 | 草稿可试运行 | 请求明确绑定当前草稿 revision 和非生产环境 |
| UX-RUN-003 | 已发布版本可运行 | 请求明确绑定 immutable published revision |
| UX-RUN-004 | 已发布版本不存在 | 只显示说明，不显示禁用 CTA |
| UX-RUN-005 | 草稿存在新修改 | 运行旧版本必须标明版本，不能误运行草稿 |
| UX-RUN-006 | 运行成功 | 变量、路径、日志在同一 RunInspector 中 |
| UX-RUN-007 | 无运行记录 | 空状态不占固定半屏 |
| UX-RUN-008 | 长 Hash | 默认截断，可在技术详情复制完整值 |

### 18.4 Playwright 真实用户旅程

| 编号 | 场景 |
|---|---|
| UX-PW-001 | 桌面打开 / 切换 / 关闭三个 Workflow Tab，Canvas 几何差值为 0 |
| UX-PW-002 | 环境配置 capability 开启时：配置环境 → 试运行 → 查看结果和日志；关闭时验证诚实阻断且无假 CTA |
| UX-PW-003 | 草稿试运行不能误触发已发布版本运行 |
| UX-PW-004 | 已发布且有新草稿：Primary 为发布更改；旧版本运行目标明确 |
| UX-PW-005 | 从已保存修改撤销最近事务，Candidate 出现且 committed revision 不变 |
| UX-PW-006 | AI 空态快捷任务随节点选区更新，发送后 Turn scope 冻结 |
| UX-PW-007 | Workflow Panel 打开期间 Node Sidebar 状态保留，关闭后字段草稿仍在 |
| UX-PW-008 | 运行失败展示明确恢复动作，Canvas 和输入不丢失 |
| UX-PW-009 | 历史冲突展示三方差异和推荐动作 |
| UX-PW-010 | 默认页面无 Tool 名、Raw JSON、Secret、完整 Hash |
| UX-PW-011 | 390px 完成选节点、AI 提问、审阅、应用和试运行 |
| UX-PW-012 | 移动端 More 使用 Bottom Sheet，关闭后焦点回归 |
| UX-PW-013 | 纯键盘完成打开工作台、切 Tab、AI 输入、Apply、检查和 Undo |
| UX-PW-014 | 1440 / 1280 / 1095 / 1024 / 820 / 768 / 390 / 320 视觉截图 |
| UX-PW-015 | 200% zoom、reduced motion、axe 和焦点顺序 |
| UX-PW-016 | Drawer 打开后 React Flow camera 在 300ms 内让当前节点避开 Overlay |
| UX-PW-017 | More 使用 ArrowUp/ArrowDown/Escape，关闭后回焦 Trigger |
| UX-PW-018 | AI flag 关闭时导航和 Command Bar 无空位、无错误入口 |

### 18.5 视觉状态矩阵

每个视口至少覆盖：

- AI 空状态；
- AI 有对话；
- Candidate 待审阅；
- Candidate 冲突；
- Run 未检查；
- Run 检查通过但未绑定环境；
- Run 可试运行；
- Run 已发布且有草稿变化；
- 运行中；
- 运行失败；
- 0 / 1 / 100 条持久事务；
- More 打开；
- Node Sidebar + Workflow Panel 同时存在。

### 18.6 无障碍

- axe critical / serious = `0`；
- 文字和图标达到 WCAG AA；
- 状态不只靠颜色；
- Line Tab 有明确 `aria-selected`；
- Menu、Tab、Dialog、Sheet 使用正确键盘模型；
- Drawer / Sheet 关闭后焦点返回触发点；
- 200% zoom 无信息丢失；
- reduced-motion 下关闭非必要动画；
- 读屏不朗读 UUID、完整 Hash 或原始 JSON。

### 18.7 browser-in-app 验收

每次 Phase exit 使用当前真实部署，记录：

- URL、title、viewport；
- Workflow ID 和状态；
- Primary 按钮数量、名称；
- Drawer 几何；
- Canvas 几何和当前节点 bounding box；
- Node Sidebar 状态；
- console warn/error；
- network 失败；
- screenshot；
- 关闭后的焦点对象。

---

## 19. 可量化验收标准

1. 普通页面状态实心 Primary 数量不超过 `1`；无真实恢复能力的阻断状态允许为 `0`；
2. 顶部有文字的主要动作不超过 `3` 个：工作台、当前 Primary、More；
3. 自动保存正常时常驻 Save 按钮数量为 `0`；
4. 不出现只写“运行”的按钮；
5. More 不出现检查、试运行、变量和发布重复入口；
6. Workflow Panel 一级入口为 `3` 个；
7. 默认 DOM 中完整 SHA、Tool 名、transactionId 和 Raw JSON 数量为 `0`；
8. Drawer 开关前后 Canvas 容器几何差值为 `0px`；
9. Drawer 打开后当前关键节点不与 Overlay 相交；
10. 桌面 Drawer 宽度为 `420–480px`；
11. 环境配置 capability 可用时，检查通过但环境未绑定最多 2 次点击进入真实配置；不可用时不出现可点击的假入口；
12. 最近一次可撤销修改最多 2 次点击可达；
13. 页面无双滚动和固定半屏空状态；
14. `1440 / 1280 / 1095 / 1024 / 820 / 768 / 390 / 320` 无水平页面滚动；
15. Preview 被误认为 committed 的比例为 `0%`；
16. 草稿运行目标和已发布运行目标误判率为 `0%`；
17. 所有关键操作可用键盘完成；
18. axe serious / critical 为 `0`。

---

## 20. 外部用户验收

邀请 5–8 位未参与开发的目标用户完成：

1. 判断当前 Workflow 是否已保存；
2. 判断当前下一步应该检查、试运行、发布还是运行；
3. 在环境配置 capability 可用的测试环境中，为草稿配置环境并试运行；
4. 找到最近一次已保存修改并撤销；
5. 运行已发布版本，同时保留未发布草稿；
6. 让 AI 修改一个节点，并判断修改是否已经保存；
7. 找到一次运行失败的日志和恢复动作。

门槛：

- 无指导任务完成率 ≥ `80%`；
- 草稿 / 已发布运行目标误判率 `0%`；
- Preview / committed 误判率 `0%`；
- 首次有效操作中位时间 ≤ `15 秒`；
- 每项任务平均求助次数 ≤ `1`；
- 冲突恢复成功率 ≥ `90%`。

---

## 21. 风险与前置决策

### P0：旧顶部 Run 与权威 Run 语义

当前顶部 `runWorkflow()` 与 `WorkflowRunPanel` 的草稿 / 已发布运行目标并非同一路径。

实施视觉改造前必须确认：

- 普通“运行”是否只能运行已发布不可变版本；
- 草稿是否只能通过非生产试运行；
- `onRunPublishedWorkflow` 的生产 wiring 是否完整。

不能先把按钮改漂亮，再继续保留两套行为。

### P0：Primary Action 的事实源

所有 Primary 必须复用 `selectWorkflowRunView`、生命周期 selector 和 Candidate 状态，不能在 Page、Header、Run Panel 各自推导。

### P0：测试环境绑定能力

当前 `runGateFacts.environment` 是服务端投影事实，不是可编辑选择器；现有页面也没有可信的环境绑定 mutation 或配置路由。

因此：

- “配置测试环境”只有在服务端投影对应 capability、权限和可信入口后才能启用；
- Command Bar 与 Run Panel 必须消费同一 capability，不能用本地状态假装已完成绑定；
- capability 未落地时显示真实阻断状态，Primary 数量为 `0`；
- 测试必须分别覆盖 capability 开启与关闭，禁止用重新打开 Run Panel 作为“配置”成功。

### P1：历史可靠性

- Durable History 是服务端事实；
- 当前 `runRecords` 可能是 Page 会话事实；
- 可以统一视觉语言，不能统一持久性承诺。

### P1：交互合同

已有大量测试依赖：

- Dock Tab ID；
- open / close event；
- Candidate 完成后自动激活 Review / 变更面板；
- Escape 焦点恢复；
- AI flag；
- Node Sidebar inert；
- Mobile Sheet。

优先兼容迁移，最后再机械调整 test ID。

### P2：CSS 债务

`runnerStudio.css` 与 `workflowWorkspace.css` 已包含大量硬编码颜色和 legacy selector。新方案应：

1. 以实际加载的 `web/src/index.css` 为入口；当前未被 import 的 `web/src/style.css` 不能作为新 Token 基础；
2. 先引入 Workflow 局部 token；
3. 新业务组件复用现有 UI primitives；
4. 每完成一个区域立即删除旧 selector；
5. 禁止通过更多 `!important` 叠加视觉层。

---

## 22. Definition of Done

本次重设计只有同时满足以下条件才算完成：

- 动作所有权已收敛，不存在重复运行、检查、发布入口；
- 顶部状态轨道准确绑定当前版本事实；
- 三个 Workflow Panel 任务模式完成；
- 节点侧栏继续独立且状态不丢失；
- Run 无重复内容、无双滚动、无完整 Hash 默认暴露；
- History 不再固定上下 50/50；
- More 完成分组和完整键盘行为；
- 三浏览器单测、集成测试、Playwright、视觉基线通过；
- Codex 内置浏览器完成桌面和移动真实交互；
- 5–8 位外部用户验收达到门槛；
- 未完成的后端能力明确显示真实不可用状态，没有假成功或假排队。
- 环境配置动作只在可信 capability 与真实入口存在时出现；否则保持可解释的阻断状态。
