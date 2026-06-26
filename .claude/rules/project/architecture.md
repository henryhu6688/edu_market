# 项目架构

## 分层架构

```
请求 → router (Gin) → middleware (CORS/Logger/JWT) → controller → service → model (GORM) → MySQL
                                                                          ↘ service/rag → Qdrant (向量库)
                                                                          ↘ utils/captcha → Redis
```

| 层 | 包路径 | 职责 |
|---|---|---|
| Router | `router/` | 路由注册，公开、JWT、Admin 三组 |
| Middleware | `middleware/` | CORS、请求日志、JWT（注入 user_id/username/role） |
| Controller | `controller/` | 绑定参数 + 调 service + `utils` 统一响应 |
| Service | `service/` | 业务逻辑，操作 `database.DB`，不碰 `gin.Context` |
| Service/RAG | `service/rag/` | RAG 向量检索（Qdrant + Embedding + Rerank + 切片 + 缓存） |
| Service/Agent | `service/agent/` | Agent 引擎（Tool Calling 循环 + System Prompt + 熔断器） |
| Model | `model/` | GORM 模型 + `TableName()` |
| DTO | `dto/request/` `dto/response/` | 请求 binding 校验 + 响应结构 |
| Config | `config/` | Viper 读 `app.yml`，环境变量覆盖敏感字段 |
| Database | `database/` | GORM + MySQL（`database.DB`），Redis 客户端 |
| Utils | `utils/` | JWT、统一响应、日志(slog+lumberjack)、验证码 |

## 数据模型

| 模型 | 表名 | 关键字段 |
|------|------|---------|
| User | users | username, phone, role(user/admin), password_hash |
| Material | materials | title, price, category_id, user_id, status(draft/published/off) |
| Document | documents | material_id, parent_id, title, content(markdown), is_free_preview |
| Course | courses | 旧模型，保留兼容 |
| Order | orders | order_no, user_id, course_id, status(pending/paid/cancelled)，软删除 |
| Review | reviews | user_id, course_id, rating(1-5) |
| Category | categories | name, parent_id |
| Session | sessions | user_id, agent_type, title, status, mode(shopping/tutoring/support), state(JSON) |
| Message | messages | session_id, role, content, tool_calls(JSON), tokens_used, reasoning_content |
| DocumentChunk | document_chunks | course_id, content, embedding, chunk_index, document_id, section_path |
| FAQ | faqs | question, answer, category |
| UserMemory | user_memories | user_id, mem_key, mem_value, source, confidence, status |

## 路由表

| 组 | 中间件 | 示例 |
|----|--------|------|
| 公开 `/api/*` | CORS + Logger | `/api/courses`, `/api/materials`, `/api/login` |
| JWT `/api/*` | CORS + Logger + JWT | `/api/agent/chat`, `/api/orders`, `/api/user/profile` |
| Admin `/api/admin/*` | CORS + Logger + JWT + AdminOnly | `/api/admin/courses` |

## 前端 (`web/`)

- Vue3 + Vite + Pinia + Vue Router + Axios
- `web/src/api/` — 各模块 API 封装
- `web/src/views/` — 页面组件（AgentChat, MaterialList, DocumentEditor 等）
- `web/src/components/` — 公共组件（Navbar, Pagination）
- `web/src/stores/` — Pinia store（user）
- `web/src/router/` — 路由守卫（auth 要求登录，admin 要求管理员）
- Vite 代理：`/api` → `localhost:8080`，`/uploads` → `localhost:8080`
