# Agent Tools 重设计

> 基于 2026-06-24 agent-redesign 的运行反馈，重构 Tool 体系、模式判定、状态管理。

## 一、问题诊断

### 1.1 翻车链路

```
用户: "我的资料里面哪些地方涉及到函数的概念"
  → LLM 选 query_orders（"我的" 匹配 "我的订单"）
  → ResolveMode → support（不可逆）
  → 7 个 Tool 被砍掉 → 永远无法搜文档内容
  → 回答: "您没有购买记录"
```

### 1.2 三个根因

| 问题 | 原因 |
|------|------|
| Tool 体系有缺口 | 没有表达"用户有哪些资料"的原语，LLM 被迫用 `query_orders` 近似 |
| 模式白名单是死锁 | `ResolveMode` 判定错了 → 砍 Tool → 无法纠正 → 永久困在错误模式 |
| 任务状态只展示不治理 | `SessionState` 拼进 Prompt 但不参与决策，LLM 可以无视它乱走 |

### 1.3 不做

| 不做 | 原因 |
|------|------|
| NLP / 关键词匹配 | 专一性，换个问法就失效 |
| Prompt 调参（加示例/加禁止项/加边界描述） | 针对已知 case 打补丁，下一个 case 继续翻 |
| 增加 Tool 数量来覆盖所有边缘情况 | 更多 Tool = 更大概率为选错 |

---

## 二、Tool 重设计

### 2.1 设计原则

```
工具粒度 = 一个清晰的业务动作
太细 → LLM 被迫做多步编排，中间任何一步出错就断链
太粗 → 黑盒，不可解释，不可调试

工具描述 = 使用说明书，不是注释
必须写清楚三件事：能做什么、不能做什么、何时优先使用

参数 Schema = 约束自由度
不丢一个裸 string 让 LLM 自己编。有来源的参数写死来源，有格式的写死格式
```

### 2.2 变化总览

| 当前 (9) | 新版 (7) | 处理 |
|----------|---------|------|
| `query_materials` | `search_materials` | 改名，参数收紧 |
| `get_material_detail` | `get_material_detail` | 合并 `get_reviews`，返回加 `access` 字段 |
| 无 | `my_materials` | **新增**。我发布 + 我购买的资料列表 |
| `search_documents` | `search_documents` | material_id 改为可选 |
| `query_orders` | `get_orders` | 合并 `get_order_detail`，order_no 可选区分列表/详情 |
| `get_order_detail` | — | 合并到 `get_orders` |
| `get_reviews` | — | 合并到 `get_material_detail` |
| `get_categories` | — | 砍掉。分类信息在 search_materials 返回中自带 |
| `search_faq` | `search_faq` | 保留 |
| `trigger_purchase_offer` | `purchase` | 改名 + 加 ALREADY_OWNED 拦截 |

### 2.3 逐 Tool 定义

---

#### 1. `search_materials` — 搜全平台资料

```
能做什么
  在平台已发布资料中按关键词、分类、价格范围搜索，返回匹配列表。

不能做什么
  不返回资料的具体章节内容或文档正文（用 search_documents）
  不返回用户的已购/已发布资料（用 my_materials）
  不返回评价详情（get_material_detail 已包含评价）
  不搜下架或草稿状态的资料

何时优先使用
  - 用户问「有没有XX方向的资料」「帮我找XX相关的课程」
  - 用户表达学习方向但未指定具体资料名
  - 搜不到时直接告知，不要反复换关键词重试

参数
  keyword     string  可选  1-50字，匹配标题和描述，不用完整句子
  category_id number  可选  只能来自 get_material_detail 返回的 category.id，不可编造
  min_price   number  可选  >=0
  max_price   number  可选  <=10000
  sort_by     string  可选  enum: newest | price_asc | price_desc | popular（默认 newest）
  page        number  可选  1-100，默认 1
  page_size   number  可选  1-10，默认 5

返回
  {
    success: true,
    materials: [{id, title, price, category_name, buy_count, description}],
    total, page
  }

错误
  DATABASE_ERROR → recoverable: false, action: "告知用户系统繁忙，稍后重试"
```

#### 2. `get_material_detail` — 资料详情 + 目录 + 评价

```
能做什么
  查看单份资料的完整信息：基本信息、发布者、文档目录（含试读标记）、
  用户评价、当前用户对该资料的访问权限。

不能做什么
  不返回文档正文内容（用 search_documents）
  不搜全平台资料（用 search_materials）
  不修改资料信息

何时优先使用
  - 用户提到具体资料名或资料ID
  - 用户问「这个资料多少钱」「有哪些章节」「评价怎么样」
  - 用户对 search_materials 返回的某项结果感兴趣
  - 与 search_documents 配合：先本工具确认资料存在，再用 search_documents 搜内容

参数
  material_id  number  必填  >0，来自 search_materials 或 my_materials 返回的ID，不可编造

返回
  {
    success: true,
    material: {
      id, title, price, description, cover_image,
      category: {id, name},
      publisher: {id, username},
      stats: {view_count, buy_count, review_count, avg_rating},
      access: {is_owner: bool, has_purchased: bool},
      outline: [{title, is_free_preview, document_id, sort_order}],
      reviews: [{rating, content, username, created_at}]
    }
  }

错误
  NOT_FOUND → recoverable: true, action: "确认资料ID是否正确，或用 search_materials 重新搜索"
```

`access` 字段是核心——LLM 据此判断是导购还是助教，不再依赖模式系统告诉它。

#### 3. `my_materials` — 我的资料 **[新增]**

```
能做什么
  列出当前登录用户可访问的全部资料：自己发布的 + 已购买并支付成功的。

不能做什么
  不搜全平台资料（用 search_materials）
  不返回文档内容（用 search_documents）
  不查订单详情如支付时间、物流状态（用 get_orders）

何时优先使用
  - 用户说「我的资料」「我买的资料」「我发布的资料」
  - 用户想在自己拥有的资料范围内搜索具体内容时，先调此工具拿到列表，
    再调 search_documents
  - 这是唯一能告诉 LLM「用户到底有哪些资料」的工具。遇到含「我的」的查询应优先考虑

参数
  无

返回
  {
    success: true,
    materials: {
      published: [{id, title, price, buy_count, status}],
      purchased: [{id, title, price, order_no, paid_at}]
    },
    total,
    hint: "共 N 份资料可访问（发布 X 份，购买 Y 份）"
  }

错误
  DATABASE_ERROR → recoverable: false, action: "告知用户查询失败，稍后重试"
```

#### 4. `search_documents` — 搜文档内容

```
能做什么
  在资料文档中进行语义检索，返回与用户问题相关的文本片段。
  不传 material_id 时自动搜索用户全部可访问资料。

不能做什么
  不搜资料基本信息如标题、价格、目录（用 get_material_detail）
  不搜全平台资料（用 search_materials）
  不搜 FAQ（用 search_faq）
  不保证搜到结果——语义检索可能存在召回盲区

何时优先使用
  - 用户指定了一份资料并问具体知识点（如「第三章讲了什么」「关于闭包的章节有哪些」）
  - 用户没指定资料但可通过我的资料推断范围时，先调 my_materials 获取 material_id，再调本工具
  - 搜不到时诚实告知「资料中未涉及该内容」，不要反复换 query 重试
  - 来源为 "preview" 的片段只能用于介绍和引导购买，不能作为完整答案的依据

参数
  query        string  必填  1-200字。具体知识点或章节号，如「闭包的原理」「第三章重点」
  material_id  number  可选  >=1。来自 my_materials 或 get_material_detail 返回的真实ID。
                            留空时系统自动搜索全部可访问资料。
                            不可编造——如果你不知道 material_id，先调 my_materials。

返回
  {
    success: true,
    results: [{
      chunk_text,
      document_title,
      material_title,
      material_id,
      section_path,
      score,
      source           "full" | "preview"
    }],
    total,
    searched_materials
  }

错误
  NOT_FOUND       → recoverable: true,  action: "告知用户资料中未涉及该内容"
  INVALID_ARGUMENT → recoverable: true,  action: "修正 query 参数后重试"
  SEARCH_ERROR     → recoverable: false, action: "告知用户检索服务暂不可用"
```

#### 5. `get_orders` — 订单查询

```
能做什么
  查询当前用户的订单。不传 order_no 返回最近订单列表，传了返回该笔订单完整详情。

不能做什么
  不查其他用户的订单
  不能发起退款、取消订单或修改订单——这是只读工具
  不查已购资料的内容（用 search_documents 或 get_material_detail）

何时优先使用
  - 用户问「我的订单」「买了什么」「查看订单」且未提供订单号 → 不传 order_no，返回列表
  - 用户提供了具体订单号 → 传入 order_no 获取详情
  - 用户问「这笔订单怎么样了」「付了没」「什么时候买的」→ 传 order_no
  - 用户说「我的资料」不是问订单，用 my_materials

参数
  order_no  string  可选  格式: 20位数字字符串。只能来自本工具列表返回的 order_no，
                         或用户直接提供的订单号。不可编造、不可推测、不可拼接。
  status    string  可选  enum: pending | paid | cancelled（仅列表模式有效）
  page      number  可选  1-100，默认 1
  page_size number  可选  1-20，默认 10

返回
  列表模式 (order_no 为空):
    {success: true, orders: [{order_no, material_title, amount, status, created_at}], total}
  详情模式 (order_no 非空):
    {success: true, order: {order_no, material_id, material_title, amount, status, paid_at, created_at}}

错误
  NOT_FOUND → recoverable: true,  action: "告知用户订单不存在，可用列表模式查看所有订单"
  EMPTY     → recoverable: false, action: "告知用户暂无订单记录"（不是错误，是正常状态）
```

#### 6. `search_faq` — 搜索 FAQ

```
能做什么
  在平台 FAQ 知识库中搜索匹配的问答对，覆盖退款政策、支付方式、使用指南等平台规则。

不能做什么
  不能搜资料内容（用 search_documents）
  不能查订单信息（用 get_orders）
  不能编造 FAQ 中没有的答案——搜不到就说需要联系客服

何时优先使用
  - 用户问平台规则：「怎么退款」「支持哪些支付方式」「多久发货」
  - 用户遇到售后问题需要政策依据
  - 搜不到时直接告知「需要联系人工客服确认」，不要反复调用

参数
  query  string  必填  1-100字。简洁关键词如「退款」「支付方式」，不用完整问句。

返回
  {success: true, faqs: [{question, answer}], total}

错误
  NOT_FOUND → recoverable: true, action: "告知用户FAQ中暂无相关内容，建议联系人工客服"
```

#### 7. `purchase` — 发起购买

```
能做什么
  向用户发送购买卡片。这是让用户看到购买入口的唯一方式——不调用此工具，用户无法下单。

不能做什么
  不能替代文字回复——调用后仍需简要说明
  不能替用户做购买决定
  不能在用户未明确表达购买意向时调用

何时优先使用
  - 用户明确表达购买意向：「买」「下单」「就这个」「来一份」「怎么买」
  - 用户已在多次对话中持续关注某份资料，最后确认想购买
  - 调用前应确认用户尚未拥有该资料（通过 get_material_detail.access.has_purchased）
  - 调用后等用户决策，不要再推其他资料

参数
  material_id  number  必填  >0。只能来自 search_materials 或 get_material_detail 返回的真实ID。

返回
  {
    success: true,
    __action: "purchase_offer",
    material: {id, title, price, cover_image},
    requires_user_action: true,
    hint: "购买卡片已发送，用户需点击卡片完成下单。在用户完成下单前，不要声称已购买。"
  }

错误
  NOT_FOUND      → recoverable: true,  action: "确认资料ID，用 search_materials 重新查找"
  ALREADY_OWNED  → recoverable: false, action: "告知用户已拥有该资料，无需重复购买"
```

---

### 2.4 错误码规范

错误信息是给 Agent 看的，不是只给人看。每个 Tool 的失败返回必须带三个字段：
`error_code`（机器可读）、`recoverable`（能不能重试）、`recommended_action`（下一步做什么）。

Agent 根据这三个字段自主决策：换参数、换 Tool、追问用户、还是放弃。

#### 统一错误码表

| 错误码 | 含义 | 可恢复 | 建议动作 |
|--------|------|:---:|------|
| `NOT_FOUND` | 资源不存在（资料/订单/FAQ） | ✅ | 确认ID后重试，或换工具搜 |
| `EMPTY` | 查询成功但无数据 | ❌ | 告知用户当前无数据，不是错误 |
| `INVALID_ARGUMENT` | 参数不符合约束 | ✅ | 修正参数后重试 |
| `MISSING_PARAMETER` | 缺少必填参数 | ✅ | 从上下文补全或用其他工具获取 |
| `ALREADY_OWNED` | 用户已拥有该资源 | ❌ | 告知用户无需重复操作 |
| `SEARCH_ERROR` | 检索服务异常（Qdrant/timeout） | ✅ | 稍后重试，或建议缩小查询范围 |
| `DATABASE_ERROR` | 数据库异常 | ❌ | 告知用户系统繁忙，稍后重试 |
| `SERVICE_UNAVAILABLE` | 依赖服务不可用 | ❌ | 告知用户功能暂不可用 |
| `BUDGET_EXCEEDED` | 调用配额耗尽 | ❌ | 基于已有信息回答 |
| `TOOL_BLOCKED` | 被熔断器拦截（重复调用/回路） | ❌ | 换策略，基于已有信息回答 |

#### 各 Tool 错误分支

```
search_materials:
  DATABASE_ERROR → "搜索失败，请稍后重试"

get_material_detail:
  NOT_FOUND        → "资料 #N 不存在或已下架，请确认ID或用 search_materials 重新搜索"
  DATABASE_ERROR   → "查询失败，请稍后重试"

my_materials:
  DATABASE_ERROR   → "查询失败，请稍后重试"

search_documents:
  INVALID_ARGUMENT  → "query 不能为空或超过200字，请修正后重试"
  MISSING_PARAMETER → "查询内容不能为空，请提供具体问题"
  NOT_FOUND         → "资料中未找到相关内容，建议换个问法"
  SEARCH_ERROR      → "检索服务暂不可用，请稍后重试"

get_orders:
  NOT_FOUND   → "订单 #XXX 不存在，可用列表模式查看所有订单"
  EMPTY       → "暂无订单记录"（success=true，不是错误）

search_faq:
  NOT_FOUND   → "FAQ 中未找到相关内容，建议联系人工客服"

purchase:
  NOT_FOUND       → "资料 #N 不存在或已下架，请用 search_materials 重新查找"
  ALREADY_OWNED   → "您已拥有该资料，无需重复购买"
  INVALID_ARGUMENT → "material_id 无效，请提供有效的资料ID"
```

#### 原则

```
1. 每条错误都告诉 Agent "做什么"——不要扔一个 "系统异常"
2. recoverable=true → Agent 应该尝试恢复
3. recoverable=false → Agent 应该告诉用户发生了什么，不要重试
4. EMPTY 不是错误，是正常状态——success=true，hint 里写清楚
```

---

## 三、模式判定改造

### 3.1 当前问题

```
ResolveMode 做两件事：
  ① 返回模式标签 → 影响 System Prompt
  ② 配合 checkToolMode → 砍 Tool（不可逆）

问题：② 是死锁。第一轮判定错了 → Tool 被砍 → 无法纠正。
```

### 3.2 去掉白名单

不再砍 Tool。安全靠 Tool 自身内置校验和返回字段保障：

| 安全边界 | 实现位置 |
|---------|---------|
| 未购买搜文档只返 preview | `search_documents.Execute` 内 hasAccess 分支 |
| 已拥有拦截重复购买 | `purchase.Execute` → ALREADY_OWNED |
| 订单只查自己 | `get_orders.Execute` → WHERE user_id |
| 资料 ID 不可编造 | 每个 Tool 的 description 写死来源 |

### 3.3 新模式判定规则

```
ResolveMode 只返回模式标签，只影响 System Prompt 选哪个 modeBlock。
7 个 Tool 始终全量可用。
```

| 触发条件 | 模式 | 原因 |
|---------|------|------|
| `get_orders` 被调用 | `support` | 查订单 → 客服场景 |
| `purchase` 被调用 | `shopping` | 购买卡片 → 导购场景 |
| `search_documents` / `get_material_detail` + hasAccess=true | `tutoring` | 有访问权 → 助教场景 |
| `search_documents` / `get_material_detail` + hasAccess=false | `shopping` | 无访问权 → 导购场景 |
| `my_materials` / `search_materials` / `search_faq` | 不改变 | 中立查询 |

**模式可随数据变化更新。** 这轮是 shopping，下轮通过 `my_materials` 发现 is_owner=true → hasAccess 变 true → 切 tutoring。

### 3.4 代码变更

删掉：
- `checkToolMode()` — 不再需要
- `countModeTools()` — 不再需要
- `Tool.AllowedModes()` — 不再需要
- `tools_usable` / `tools_total` 日志字段

保留：
- `ResolveMode()` — 仍判断，但仅用于 prompt 导向
- `buildPrompt()` 中 mode block 拼接 — 不变

---

## 四、State 从显示器升级为任务板

### 4.1 当前问题

`SessionState` 有结构，但只被拼成 Prompt 展示给 LLM。不参与决策，不校验完整性。
LLM 可以无视它走偏——实际上就是这么翻的车。

### 4.2 改造目标

State 从"展示当前进度"变为"标注缺口 + 交付条件"。

引擎不替代 LLM 做决策。引擎告诉 LLM 当前位置离目标还有多远。
LLM 仍自主规划，但每次行动后都会被回注位置信号。

```
每轮 Tool 执行完后：

updateTaskState()  → 更新 completed / context
  ↓
assessState()      → 检查 5 个维度
  ↓
buildStateBlock()  → 注入治理信号
  ↓
buildPrompt()      → 与 modeBlock 一起拼为 System Prompt
```

### 4.3 校验维度

| 维度 | 检查逻辑 | 信号示例 |
|------|---------|---------|
| 目标满足 | completed 是否覆盖 task | `可以交付: 是` / `可以交付: 否` |
| 失败步骤 | 本轮有无 recoverable error | `⚠️ search_documents 无结果 → 建议换关键词` |
| 信息缺口 | 目标所需信息是否缺失 | `⚠️ 缺少目标资料 → 先调用 my_materials` |
| 重复调用 | 是否同一 Tool 同一参数 | `⛔ 该查询已执行过，请用已有结果回答` |
| 交付就绪 | 所有缺口已闭合 | `立即回答用户，不要继续调用工具` |

### 4.4 Prompt 中的呈现

```
/* ── 任务状态 ── */
当前目标: 回答用户在自有资料中关于「函数」的问题
已完成:
  ✅ my_materials → 可访问 1 份资料（《JavaScript 教程》）
已失败:
  ❌ search_documents(query="函数", material_id=5) → 未找到内容
缺口:
  ⚠️ 尝试更具体的关键词如「函数定义」「闭包」
可以交付: 否
```

### 4.5 State 新增字段

```go
type SessionState struct {
    Task       string       `json:"task"`
    Completed  []StepRecord `json:"completed"`   // 改: 从 string 变为结构化记录
    Failed     []StepRecord `json:"failed"`      // 新增: 失败步骤
    Gaps       []string     `json:"gaps"`        // 新增: 当前信息缺口
    Deliverable bool        `json:"deliverable"`  // 新增: 是否可以交付
    Facts      []FactItem   `json:"facts"`
    Hypotheses []FactItem   `json:"hypotheses"`
    Discarded  []FactItem   `json:"discarded"`
    Context    ContextData  `json:"context"`
}

type StepRecord struct {
    Action  string `json:"action"`   // e.g. "搜索文档「函数」"
    Tool    string `json:"tool"`     // e.g. "search_documents"
    Args    string `json:"args"`     // JSON, for dedup check
    Error   string `json:"error,omitempty"`
    Success bool   `json:"success"`
}
```

`assessState()` 在每轮 Tool 执行后：

```go
func assessState(state *SessionState, executedTools []string) {
    // 1. 检查失败 → 写入 Failed，生成缺口建议
    // 2. 检查信息缺口 → 对比 task 和 completed 推断缺失什么
    // 3. 检查是否可以交付 → 有 completed 且无未覆盖缺口
    // 4. 写入 state.Deliverable
}
```

### 4.6 交付条件

以下条件全部满足时标记 `deliverable: true`：

- 至少有一个 completed 步骤
- 无未闭合的信息缺口
- 无 recoverable 失败步骤未处理
- 或者：已连续 2 轮无新增 completed（搜不到就是搜不到，不要死循环）

`deliverable: true` 的信号注入 Prompt 后，LLM 看到即停止工具调用，直接回答。

---

## 五、代码变更清单

| 文件 | 变动 |
|------|------|
| `service/agent/tools.go` | 9→7 Tool 重写：Definition / Execute / ValidateArgs / Describe。删 `AllowedModes()` 接口方法。`buildToolSet()` 更新。 |
| `service/agent/safety.go` | 删 `checkToolMode()`、`countModeTools()`。`ResolveMode` 更新判定表，不再返回 "" 以外的值用于砍工具。保留熔断器和预算。 |
| `service/agent/prompts.go` | 更新三个 modeBlock 中的 Tool 名称引用。`SessionState` 新增 `Failed`/`Gaps`/`Deliverable`。`StepRecord` 替代 plain string。`buildStateBlock()` 改输出格式。新增 `assessState()`。 |
| `service/agent/engine.go` | 删白名单检查逻辑。Round 结束后调 `assessState()`。`updateTaskState()` 改为写入 `StepRecord`。日志字段删 `tools_usable`/`tools_total`。 |
| `service/agent/service.go` | `buildToolSet()` 调用更新（删 `AllowedModes()` 约束）。 |
| `scripts/test_agent.go` | 适配新 Tool 名称和参数。 |
| `controller/agent_controller.go` | 无 Schema 变更（SSE 协议不变）。 |

---

## 六、验证标准

1. **发布会者问"我的资料里哪些涉及函数"** → LLM 调 `my_materials` → 获取列表 → 调 `search_documents(material_id=X)` → 返回文档片段 → 回答。全程不碰 `get_orders`，不触发 support。
2. **未购买用户搜资料** → `get_material_detail.access.has_purchased=false` → LLM 据此为导购模式 → 引导购买。
3. **客服模式仍正常工作** → 用户问"我的订单" → `get_orders` → ResolveMode → support → prompt 加载客服策略。
4. **第一轮选错 Tool 能恢复** → 即使 LLM 第一轮选了错误的 Tool，第二轮仍有全部 7 个 Tool 可用。State 信号标注缺口，LLM 据此纠正。

---

## 七、不做的

- **不做 NLP / 关键词匹配** — 专一性，换个问法就失效
- **不做固定工作流** — LLM 仍自主规划，State 是信息板不是流程控制
- **不做模式间 Tool 隔离** — 安全靠 Tool 自身校验，不靠砍 Tool
- **不新增 Tool 超过 7 个** — 更多 Tool = 更大概率选错
