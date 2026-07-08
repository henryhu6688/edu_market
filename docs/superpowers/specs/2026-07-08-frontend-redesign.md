# 前端重设计 — 设计文档

## 1. 背景

当前前端（Vue3 + Vite）无设计系统，全局样式仅 30 行 reset，14 个页面各写各的 scoped CSS，颜色/间距/圆角/阴影散落各处，视觉平淡（系统默认字体、emoji 图标、弱阴影、`!important` 打补丁）。本次重设计的目标是建立统一的视觉语言，先改造 3 个门面页试点，风格确认后铺开到全站。

## 2. 技术选型

| 层 | 选型 | 说明 |
|---|---|---|
| 样式基础 | Tailwind CSS v4 | 原子化 CSS，token 集中在 `theme`，消除样式冲突 |
| 组件底座 | shadcn-vue（基于 reka-ui） | 组件代码 copy 进仓库、完全可定制、内置 a11y，不是黑盒依赖 |
| 图标 | lucide-vue-next | 替换现有 emoji（🎓📚🤖），统一线性图标风格 |
| 字体 | @fontsource/inter | 西文/数字部分，中文走系统字体栈 |

新增依赖：

```
tailwindcss @tailwindcss/vite
reka-ui (shadcn-vue 运行依赖)
lucide-vue-next
@fontsource/inter
```

## 3. 设计 Token

### 3.1 配色

```
  --primary:    #0D9488  (teal-600)   主色，按钮/链接/选中的分类 chip/强调元素
  --primary-fg: #FFFFFF               主色上的文字
  --accent:     #F59E0B  (amber-500)  价格/评分星/强调数字/促销元素
  --foreground: #0F172A  (slate-900)  正文/标题
  --muted:      #475569  (slate-600)  次要文字/简介/灰色说明
  --border:     #E2E8F0  (slate-200)  卡片边框/分割线/输入框边框
  --background: #F8FAFC  (slate-50)   页面背景
  --card:       #FFFFFF              卡片/模态框/气泡背景
  --destructive:#EF4444  (red-500)    删除/危险操作
```

映射到 Tailwind theme → shadcn-vue CSS 变量体系。不涉及暗色模式。

### 3.2 字体

```css
font-family: 'Inter', -apple-system, BlinkMacSystemFont,
             'PingFang SC', 'Microsoft YaHei', sans-serif;
```

- 标题：Inter 700-800，letter-spacing -0.3px
- 正文：Inter 400-500，行高 1.6-1.7
- 代码/数字：Inter（Tabular Numbers，价格/购买数对齐）
- 中文：系统字体栈，零额外体积

### 3.3 间距 & 圆角 & 阴影

| Token | 值 | 用途 |
|---|---|---|
| 卡片圆角 | 10px | MaterialList 卡片、AI 消息气泡 |
| 按钮圆角 | 8px | 所有按钮 |
| 标签/Pill 圆角 | 20px | 分类 chip、状态 Badge |
| 输入框圆角 | 8px | 搜索框、输入栏 |
| 网格间距 | 20px | 卡片 grid gap |
| 卡片阴影(默认) | 无 | 扁平设计，靠边框区分 |
| 卡片阴影(hover) | 0 4px 16px rgba(0,0,0,0.08) | hover 上浮 +4px translateY(-2px) |
| 模态框阴影 | 0 8px 30px rgba(0,0,0,0.12) | Dialog |
| Navbar 阴影 | 0 1px 3px rgba(0,0,0,0.06) | sticky navbar |
| 主按钮阴影 | 0 2px 8px rgba(13,148,136,0.25) | teal 主按钮微发光 |

## 4. 组件库（shadcn-vue，按需 copy）

本次需要的组件：

| 组件 | 用途 | 关键配置 |
|---|---|---|
| Button | 主/次/危险/ghost 4 个 variant | primary=teal 实心，secondary=灰边框，destructive=红，ghost=透明+hover |
| Card | 资料卡片、AI 推荐卡、Hero 内嵌卡 | 无默认阴影，hover 上浮 |
| Input | 搜索框、聊天输入栏 | focus: ring-teal |
| Badge | 分类标签、模式标签、状态指示 | teal/amber/gray 3 色 |
| Dialog | 替换 AgentChat 删除确认弹窗 | 居中模态 + 遮罩 |
| Select | 排序下拉（替换原生 select） | Popover 触发 |

## 5. 页面设计

### 5.1 Home（首页）

**数据源变更：** 从旧的 `getCourses` API 切换到 `getMaterials` API。CourseCard 组件不再被引用后可删除。

**布局（从上到下）：**

```
┌─────────────────────────────────────────────────────┐
│ Navbar (sticky，teal 主色 + lucide 图标)              │
├─────────────────────────────────────────────────────┤
│ Hero (左右两栏)                                      │
│ ┌──────────────────────┐ ┌────────────────────────┐ │
│ │ 大标题 + 副标题        │ │ AI 预览卡 (signature)    │ │
│ │ "找到对的资料，        │ │ ┌────────────────────┐ │ │
│ │  问对的问题"           │ │ │ 🤖 想学 Python？     │ │ │
│ │                       │ │ │ 推荐 3 份资料...     │ │ │
│ │ [浏览资料] [试用AI答疑] │ │ └────────────────────┘ │ │
│ └──────────────────────┘ └────────────────────────┘ │
├─────────────────────────────────────────────────────┤
│ 搜索 + 分类 chips                                    │
│ [🔍 搜索资料...]  [全部] [编程] [设计] [AI] [考研]    │
├─────────────────────────────────────────────────────┤
│ 精选资料网格 (3-4 列，取前 N 条)                      │
│ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐               │
│ │ Card │ │ Card │ │ Card │ │ Card │               │
│ └──────┘ └──────┘ └──────┘ └──────┘               │
├─────────────────────────────────────────────────────┤
│ [查看更多资料 →] (link 到 /materials)                 │
└─────────────────────────────────────────────────────┘
```

**状态处理：**
- 资料为空：展示空态插画 + "暂无资料" 文案
- 加载中：卡片骨架屏（shadcn Skeleton，灰度占位块闪烁）
- AI 预览卡：静态示意，不做实时对话（不在本次实现范围）

**交互：**
- 搜索框回车 → 过滤资料网格
- 分类 chip 点击 → 切换选中状态（teal 实心变灰边框）+ 过滤
- 卡片点击 → 跳转 MaterialDetail `/materials/:id`
- "试用 AI 答疑"按钮 → 跳转 AgentChat `/agent`

### 5.2 MaterialList（资料商城）

**布局：**

```
┌─────────────────────────────────────────────────────┐
│ 学习资料                        [+ 发布资料] (登录可见) │
├─────────────────────────────────────────────────────┤
│ [🔍 搜索...]                    [最新 ▾] (排序下拉)    │
│ [全部] [编程] [设计] [AI] [更多分类...]               │
├─────────────────────────────────────────────────────┤
│ 卡片网格 (3-4 列自动填充)                             │
│ ┌──────────┐ ┌──────────┐ ┌──────────┐              │
│ │ 封面渐变   │ │ 封面渐变   │ │ 封面渐变   │              │
│ │ [编程]     │ │ [设计]     │ │ [AI]      │ ← Badge   │
│ │ Python入门 │ │ UI 设计    │ │ 大模型应用  │              │
│ │ 从零学...   │ │ 规范与...   │ │ 实战项目... │              │
│ │ ¥49 · 1.2k │ │ ¥69 · 856  │ │ ¥99 · 432  │ ← amber价格│
│ └──────────┘ └──────────┘ └──────────┘              │
├─────────────────────────────────────────────────────┤
│ Pagination                                          │
└─────────────────────────────────────────────────────┘
```

**卡片结构（每张卡片）：**

```
┌─────────────────┐
│ 封面区域          │ ← 有图显示图，无图用 teal 渐变 + 资料标题首字
│ (120px 高)       │
├─────────────────┤
│ [分类名]          │ ← Badge，teal/amber 按分类切换
│ 资料标题          │ ← font-semibold，单行截断
│ 简介(1-2行)       │ ← text-muted text-sm，60字截断
│ ¥49 · 1200人 · 4.8★  │ ← amber价格+muted购买数+amber评分
└─────────────────┘
```

**交互：**
- 分类 chip 和排序联动过滤
- 卡片 hover 上浮（translateY(-2px) + 阴影出现）
- 卡片点击 → MaterialDetail
- "发布资料"按钮 → MaterialEditor（仅登录用户可见，保持现有逻辑）

### 5.3 AgentChat（AI 答疑）

**布局：**

```
┌────────┬────────────────────────────────────────────┐
│ 侧边栏  │ 对话区                                      │
│        │                                            │
│ [+ 新]  │ Python 入门              [购物模式] Badge   │
│ ──────  │ ────────────────────────────────────────── │
│ Python  │                         ┌────────────────┐│
│ 入门 ✓  │                         │ 想学 Python      ││← 用户气泡(右,teal)
│ ──────  │                         └────────────────┘│
│ 设计    │ ┌──────────────────────────────────────────┐│
│ 资源    │ │ 为你找到 2 份资料：                        ││← AI 气泡(左,白)
│ ──────  │ │ ┌──────┐ ┌──────┐                       ││
│ 面试    │ │ │Python │ │21天  │ ← 资料推荐卡组(signature)│
│ 准备    │ │ │¥49   │ │Python │                       ││
│        │ │ └──────┘ └──────┘                       ││
│        │ └──────────────────────────────────────────┘│
│        │ 🔍 正在检索资料...  ← thinking 气泡(amber)     │
│        │                                            │
│ [×删除]│ ──────────────────────────────────────────  │
│        │ [输入问题...]                    [↑发送]     │
└────────┴────────────────────────────────────────────┘
```

**侧边栏（保留现有功能）：**
- 折叠/展开切换：点 ☰ 收起为窄竖条（仅图标），再点展开
- 会话列表：每个 item 显示 `s.title || '新对话'`
- 激活态：teal 浅底色 + 左边框 teal 线条
- "新对话"按钮：teal 实心小按钮
- 删除按钮(×)：hover 变红
- 删除确认：用 shadcn-vue Dialog 替换现有的自定义 modal

**消息气泡（用新 token 重写样式）：**

| 消息类型 | 样式 |
|---|---|
| 用户消息 | 右对齐，teal 背景 `#0D9488`，白色文字，圆角 12px/12px/2px/12px |
| AI 文字回复 | 左对齐，白色背景 `#FFFFFF`，灰色边框 `#E2E8F0`，圆角 12px/12px/12px/2px |
| AI 资料推荐卡组 | 左对齐，白色 Card(s) 水平排列，封面彩条+标题+价格，可点击跳转详情 |
| thinking 气泡 | 左对齐，amber 浅底色 `#FEF9C3`，amber 深色文字 `#92400E`，🔍 图标 |
| purchase action 卡片 | 左对齐，白 Card，标题+价格+"立即购买"按钮 → 跳转订单页 |
| 加载中(等待回复) | 左对齐，三个灰点跳动动画 |

**输入区：**
- 输入框 `flex-1`，focus ring-teal
- 发送按钮：teal 实心，loading 时 disabled

**SSE 流式：**
- `delta` 事件：逐字追加到当前 AI 消息的 content，自动 scrollToBottom
- `thinking` 事件：插入 thinking 气泡
- `action` 事件（purchase_offer）：插入 purchase action 卡片
- `done` 事件：首次对话自动关联 session_id 到侧边栏
- `error` 事件：插入错误消息气泡

## 6. 数据源迁移

| 变更 | 说明 |
|---|---|
| Home 切换数据源 | `getCourses` → `getMaterials`，`getCategories` 保留（已跨表通用） |
| Home 组件替换 | `CourseCard` → 新的资料 Card（或跟 MaterialList 共用） |
| CourseCard 清理 | Home 切完后，CourseCard 不再被任何页引用 → 删除 `web/src/components/CourseCard.vue` |

## 7. 文件变更预估

| 操作 | 文件 |
|---|---|
| 新增 | `web/src/assets/tokens.css`（CSS 变量定义） |
| 新增 | `web/src/components/ui/*`（shadcn-vue 组件：Button/Card/Input/Badge/Dialog/Select） |
| 重写 | `web/src/assets/style.css`（Tailwind 入口，替换当前 35 行） |
| 重写 | `web/src/views/Home.vue` |
| 重写 | `web/src/views/MaterialList.vue` |
| 重写 | `web/src/views/AgentChat.vue` |
| 修改 | `web/src/components/Navbar.vue`（配色+图标替换） |
| 修改 | `web/src/App.vue`（如有需要适配新布局） |
| 修改 | `web/package.json`（新增依赖） |
| 修改 | `web/vite.config.js`（Tailwind 插件） |
| 删除 | `web/src/components/CourseCard.vue`（Home 切换数据源后无引用） |
| 不动 | 其余 11 页 + admin 后台 + stores/router/api 层 |

## 8. 非目标（本次不做）

- 暗色模式
- 其余 11 页改造 + admin 后台
- 首页 AI 实时对话动效（静态示意即可）
- 资料详情页、文档编辑器/阅读器

## 9. 成功标准

1. 3 页试点视觉统一，符合"现代教育 SaaS 克制/专业/清晰"预期
2. 无 `!important` 或样式冲突
3. 配色/字体/间距/圆角/阴影全部来自设计 token，不再散落各页面
4. AgentChat 侧边栏功能完整保留（折叠/展开/新建/切换/删除）
5. AI 资料推荐卡可点击跳转资料详情
6. 构建成功，Node 22 无 deprecation warning
7. 移动端（320px 宽度）无破损
