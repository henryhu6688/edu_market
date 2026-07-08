# 前端重设计 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 edu_market 前端建立统一设计系统（Tailwind v4 + shadcn-vue + 设计 token），重写 Home/MaterialList/AgentChat 三个试点页，切换 Home 到 materials API。

**Architecture:** Tailwind CSS v4 通过 `@tailwindcss/vite` 插件接入，设计 token 通过 `@theme` 指令定义在全局 CSS 中。shadcn-vue 组件按需 copy 到 `src/components/ui/`，通过 `cn()` 工具函数合并 class。三个页面全部用 Tailwind class + 新组件重写，零 scoped CSS 残留。

**Tech Stack:** Vue3 + Vite5 + Tailwind CSS v4 + shadcn-vue (reka-ui) + lucide-vue-next + @fontsource/inter + Pinia + Axios

## Global Constraints

- 零 scoped `<style>` 块 — 所有样式走 Tailwind class
- 配色/字体/圆角/阴影全部来自 CSS 变量，禁止硬编码 hex
- 图标用 lucide-vue-next，禁止 emoji 图标（🎓📚🤖）
- AgentChat SSE 流式逻辑保持现有协议（thinking/delta/action/done/error），只换视觉层
- Home 数据源切到 `getMaterials` API，`getCourses` 相关引用全部移除
- Node 22，`npm run dev` + `npm run build` 无 error
- 每次任务结束 git commit

---

### Task 1: 基础设施 — 依赖安装 + Tailwind + 设计 token

**Files:**
- Modify: `web/package.json`
- Modify: `web/vite.config.js`
- Modify: `web/src/assets/style.css`
- Create: `web/src/lib/utils.js`

**Interfaces:**
- Consumes: 无（独立任务）
- Produces: Tailwind 可用、CSS token 全局定义、`cn()` 工具函数

- [ ] **Step 1: 安装依赖**

```bash
cd web
npm install tailwindcss @tailwindcss/vite reka-ui lucide-vue-next @fontsource/inter clsx tailwind-merge
```

- [ ] **Step 2: 修改 vite.config.js，加入 Tailwind 插件**

```js
// web/vite.config.js
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      '/uploads': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
})
```

- [ ] **Step 3: 替换 web/src/assets/style.css 为 Tailwind 入口 + 设计 token**

```css
/* web/src/assets/style.css */
@import "tailwindcss";
@import "@fontsource/inter";

@theme {
  --color-primary: #0D9488;
  --color-primary-fg: #FFFFFF;
  --color-accent: #F59E0B;
  --color-foreground: #0F172A;
  --color-muted: #475569;
  --color-border: #E2E8F0;
  --color-background: #F8FAFC;
  --color-card: #FFFFFF;
  --color-destructive: #EF4444;
  --color-thinking-bg: #FEF9C3;
  --color-thinking-fg: #92400E;

  --radius-card: 10px;
  --radius-btn: 8px;
  --radius-pill: 20px;
  --radius-input: 8px;

  --font-sans: 'Inter', -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Microsoft YaHei', sans-serif;
}

body {
  font-family: var(--font-sans);
  background: var(--color-background);
  color: var(--color-foreground);
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}

/* 滚动条美化 — 保留原有 */
::-webkit-scrollbar { width: 6px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: #ccc; border-radius: 3px; }
::-webkit-scrollbar-thumb:hover { background: #aaa; }
```

- [ ] **Step 4: 创建 cn() 工具函数（shadcn-vue 依赖）**

```js
// web/src/lib/utils.js
import { clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs) {
  return twMerge(clsx(inputs))
}
```

- [ ] **Step 5: 验证 Tailwind 编译正常**

```bash
cd web && npm run dev
```
打开浏览器确认无编译错误。观察 Navbar/Home 页面 — 全局字体应该已切换到 Inter（中文字体不变），背景色变成 `#F8FAFC`。Ctrl+C 停止 dev。

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/package-lock.json web/vite.config.js web/src/assets/style.css web/src/lib/utils.js
git commit -m "feat(frontend): tailwind v4 + 设计 token 基础设施"
```

---

### Task 2: shadcn-vue 组件初始化

**Files:**
- Create: `web/components.json`
- Create: `web/src/components/ui/button/*` (shadcn 生成的 Button)
- Create: `web/src/components/ui/card/*` (shadcn 生成的 Card)
- Create: `web/src/components/ui/input/*` (shadcn 生成的 Input)
- Create: `web/src/components/ui/badge/*` (shadcn 生成的 Badge)
- Create: `web/src/components/ui/dialog/*` (shadcn 生成的 Dialog)
- Create: `web/src/components/ui/select/*` (shadcn 生成的 Select)

**Interfaces:**
- Consumes: Task 1（Tailwind + CSS token + cn()）
- Produces: 6 个 UI 组件可用，`@/components/ui/*` import 路径就绪

- [ ] **Step 1: 创建 components.json（shadcn-vue 配置）**

```json
{
  "$schema": "https://shadcn-vue.com/schema.json",
  "style": "default",
  "typescript": false,
  "tailwind": {
    "config": "",
    "css": "src/assets/style.css",
    "baseColor": "slate",
    "cssVariables": true
  },
  "framework": "vite",
  "aliases": {
    "components": "@/components",
    "utils": "@/lib/utils"
  }
}
```
保存到 `web/components.json`。

- [ ] **Step 2: 逐个添加 shadcn-vue 组件**

```bash
cd web
npx shadcn-vue@latest add button
npx shadcn-vue@latest add card
npx shadcn-vue@latest add input
npx shadcn-vue@latest add badge
npx shadcn-vue@latest add dialog
npx shadcn-vue@latest add select
```

每一条命令会生成组件文件到 `src/components/ui/` 并自动安装所需依赖。如果某条命令提示覆盖 `cn()` 函数，选 **No**（我们的 `src/lib/utils.js` 已就绪）。

- [ ] **Step 3: 验证组件文件结构**

```bash
ls web/src/components/ui/
```
预期有 `button/`、`card/`、`input/`、`badge/`、`dialog/`、`select/` 每个目录下有 `.vue` 文件。

- [ ] **Step 4: 验证 dev 不报错**

```bash
cd web && npm run dev
```

无 console error。Ctrl+C 停止。

- [ ] **Step 5: Commit**

```bash
git add web/components.json web/src/components/ui/
git commit -m "feat(frontend): shadcn-vue 组件初始化 (button/card/input/badge/dialog/select)"
```

---

### Task 3: Navbar — 配色迁移 + lucide 图标

**Files:**
- Modify: `web/src/components/Navbar.vue`

**Interfaces:**
- Consumes: Task 1（CSS token）、Task 2（lucide-vue-next 已安装）
- Produces: Navbar 使用 teal 配色 + lucide 图标，后续页面共享

- [ ] **Step 1: 重写 Navbar.vue template**

```vue
<template>
  <nav class="sticky top-0 z-50 bg-white border-b border-[var(--color-border)]">
    <div class="max-w-[1200px] mx-auto flex items-center justify-between px-5 h-[60px]">
      <router-link to="/" class="text-xl font-bold text-[var(--color-primary)] no-underline flex items-center gap-2">
        <GraduationCap :size="24" />
        EduMarket
      </router-link>
      <div class="flex items-center gap-2">
        <router-link to="/" class="nav-link">首页</router-link>
        <router-link to="/materials" class="nav-link">
          <BookOpen :size="16" class="inline mr-0.5" />学习资料
        </router-link>
        <router-link v-if="userStore.isLoggedIn" to="/agent" class="nav-link">
          <Sparkles :size="16" class="inline mr-0.5" />AI 助手
        </router-link>
        <template v-if="userStore.isLoggedIn">
          <router-link to="/orders" class="nav-link">我的订单</router-link>
          <router-link to="/profile" class="nav-link">个人中心</router-link>
          <router-link v-if="userStore.isAdmin" to="/admin" class="nav-link">管理后台</router-link>
          <a href="#" @click.prevent="handleLogout" class="nav-link">退出</a>
        </template>
        <template v-else>
          <router-link to="/login" class="px-4 py-2 bg-[var(--color-primary)] text-white rounded-[var(--radius-btn)] text-sm font-medium no-underline hover:brightness-90 transition-all">登录 / 注册</router-link>
        </template>
      </div>
    </div>
  </nav>
</template>
```

- [ ] **Step 2: 重写 script 部分（保留逻辑，加 icon import）**

```vue
<script setup>
import { useUserStore } from '@/stores/user'
import { useRouter } from 'vue-router'
import { GraduationCap, BookOpen, Sparkles } from 'lucide-vue-next'

const userStore = useUserStore()
const router = useRouter()

function handleLogout() {
  userStore.logout()
  router.push('/')
}
</script>
```

- [ ] **Step 3: 重写 style 部分（Tailwind class 覆盖，删掉所有 scoped CSS）**

```vue
<style scoped>
.nav-link {
  color: var(--color-foreground);
  text-decoration: none;
  padding: 8px 14px;
  border-radius: var(--radius-btn);
  font-size: 14px;
  transition: all 0.2s;
}
.nav-link:hover,
.nav-link.router-link-exact-active {
  background: #ECFDF5;
  color: var(--color-primary);
}
</style>
```

只需保留 `.nav-link` 的 scoped style（因为 `router-link-exact-active` 是 Vue Router 注入的 class，纯 Tailwind 无法处理 hover+active 组合）。登录/注册按钮样式已在 template 中走 Tailwind inline class。

- [ ] **Step 4: 验证**

```bash
cd web && npm run dev
```
打开浏览器确认：
- Navbar 图标从 emoji 🎓📚🤖 变成 lucide 图标
- 主色从 `#4f46e5`(indigo) 变成 `#0D9488`(teal)
- 登录按钮 teal 实心
- 所有路由链接 hover + active 变色正常

- [ ] **Step 5: Commit**

```bash
git add web/src/components/Navbar.vue
git commit -m "feat(frontend): Navbar teal配色 + lucide图标替换"
```

---

### Task 4: MaterialCard 共用组件

**Files:**
- Create: `web/src/components/MaterialCard.vue`

**Interfaces:**
- Consumes: Task 2（Badge 组件）、Task 1（CSS token）
- Produces:
  ```
  <MaterialCard :material="MaterialObject" />
  // MaterialObject = { id, title, cover_image, description, price, buy_count, category_name }
  ```
  MaterialList 和 Home 共用此组件

- [ ] **Step 1: 创建 MaterialCard.vue**

```vue
<!-- web/src/components/MaterialCard.vue -->
<template>
  <div
    class="group cursor-pointer rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-card)] overflow-hidden transition-all duration-200 hover:-translate-y-0.5 hover:shadow-lg"
    @click="$router.push(`/materials/${material.id}`)"
  >
    <!-- 封面 -->
    <div v-if="material.cover_image" class="h-[140px]">
      <img :src="material.cover_image" :alt="material.title" class="w-full h-full object-cover" />
    </div>
    <div v-else class="h-[140px] bg-gradient-to-br from-teal-600 to-teal-400 flex items-center justify-center">
      <span class="text-white text-3xl font-bold">{{ (material.title || '?').charAt(0) }}</span>
    </div>

    <!-- 信息 -->
    <div class="p-3.5">
      <span class="inline-block px-2 py-0.5 text-[11px] rounded-[var(--radius-pill)] bg-teal-50 text-[var(--color-primary)] font-medium mb-1.5">
        {{ material.category_name || '未分类' }}
      </span>
      <h3 class="font-semibold text-[15px] text-[var(--color-foreground)] truncate mb-0.5">{{ material.title }}</h3>
      <p class="text-[13px] text-[var(--color-muted)] leading-relaxed line-clamp-2 mb-2.5">
        {{ material.description?.substring(0, 80) || '' }}
      </p>
      <div class="flex items-center justify-between">
        <span class="text-base font-bold text-[var(--color-accent)]">¥{{ material.price }}</span>
        <span class="text-xs text-[var(--color-muted)]">{{ material.buy_count || 0 }}人购买</span>
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  material: { type: Object, required: true }
})
</script>
```

不需要 `<style>` 块，纯 Tailwind class。

- [ ] **Step 2: 验证组件可 import**

```bash
cd web && npm run dev
```
在浏览器 console 中无 "Failed to resolve component" 错误。Ctrl+C 停止。

- [ ] **Step 3: Commit**

```bash
git add web/src/components/MaterialCard.vue
git commit -m "feat(frontend): MaterialCard 共用资料卡片组件"
```

---

### Task 5: Home.vue 重写 + 数据源切换

**Files:**
- Modify: `web/src/views/Home.vue`

**Interfaces:**
- Consumes: Task 3（Navbar）、Task 4（MaterialCard）、`getMaterials` from `@/api/material`、`getCategories` from `@/api/category`
- Produces: 首页展示材料网格（非课程），旧 CourseCard 引用移除

- [ ] **Step 1: 重写 template**

```vue
<template>
  <div class="home">
    <!-- Hero -->
    <section class="rounded-2xl bg-gradient-to-br from-teal-600 to-teal-500 text-white mb-8 overflow-hidden">
      <div class="flex flex-col md:flex-row items-center gap-8 px-8 py-12 md:py-14">
        <div class="flex-1 text-center md:text-left">
          <h1 class="text-[32px] md:text-[40px] font-extrabold leading-tight mb-3">
            找到对的资料，<br class="hidden md:block" /><span class="text-[var(--color-accent)]">问对的问题</span>
          </h1>
          <p class="text-teal-50 text-base md:text-lg mb-6 opacity-90">精选学习资料 + AI 智能答疑，一站式学习体验</p>
          <div class="flex gap-3 justify-center md:justify-start">
            <router-link to="/materials" class="inline-flex items-center px-6 py-3 bg-white text-[var(--color-primary)] rounded-[var(--radius-btn)] font-semibold text-sm hover:bg-teal-50 transition-all">
              <BookOpen :size="18" class="mr-1.5" />浏览资料
            </router-link>
            <router-link to="/agent" class="inline-flex items-center px-6 py-3 border-2 border-white/30 text-white rounded-[var(--radius-btn)] font-semibold text-sm hover:bg-white/10 transition-all">
              <Sparkles :size="18" class="mr-1.5" />试用 AI 答疑
            </router-link>
          </div>
        </div>
        <!-- AI 预览卡 (signature) -->
        <div class="flex-1 max-w-[340px] w-full">
          <div class="bg-white/10 backdrop-blur rounded-xl p-4 border border-white/20">
            <div class="flex items-center gap-2 mb-3 text-white/70 text-xs">
              <Sparkles :size="14" /> AI 助手
            </div>
            <div class="bg-white/15 rounded-lg px-3 py-2 text-xs text-white/80 mb-3">想学 Python 数据分析，有什么推荐？</div>
            <div class="bg-white rounded-lg px-3 py-2.5 text-xs text-[var(--color-foreground)]">
              <div class="text-[var(--color-muted)] mb-1.5">为你找到 2 份资料：</div>
              <div class="flex flex-col gap-1">
                <span class="text-[var(--color-primary)] font-medium">📄 Python 零基础入门 ¥49</span>
                <span class="text-[var(--color-primary)] font-medium">📄 数据分析实战 ¥79</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 搜索 + 分类 chips -->
    <div class="flex flex-col sm:flex-row gap-3 mb-6">
      <div class="relative flex-1">
        <Search :size="18" class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
        <input
          v-model="keyword"
          @keyup.enter="search"
          placeholder="搜索学习资料..."
          class="w-full pl-10 pr-4 py-2.5 border border-[var(--color-border)] rounded-[var(--radius-input)] text-sm outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-teal-100 transition-all"
        />
      </div>
      <div class="flex gap-1.5 flex-wrap items-center">
        <button
          v-for="cat in categoryChips"
          :key="cat.id"
          @click="categoryId = cat.id; search()"
          :class="[
            'px-4 py-2 text-[13px] rounded-[var(--radius-pill)] font-medium transition-all whitespace-nowrap',
            categoryId === cat.id
              ? 'bg-[var(--color-primary)] text-white'
              : 'bg-white border border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]'
          ]"
        >{{ cat.name }}</button>
      </div>
    </div>

    <!-- 资料网格 -->
    <div v-if="materials.length" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5 mb-6">
      <MaterialCard v-for="m in materials" :key="m.id" :material="m" />
    </div>
    <div v-else-if="!loading" class="text-center py-20 text-[var(--color-muted)]">
      <BookOpen :size="48" class="mx-auto mb-4 opacity-30" />
      <p class="text-base">暂无资料数据</p>
      <p class="text-sm text-[#bbb] mt-1">请先启动后端服务并确保数据库已连接</p>
    </div>
    <div v-else class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5 mb-6">
      <div v-for="n in 4" :key="n" class="h-[260px] rounded-[var(--radius-card)] bg-[var(--color-border)] animate-pulse" />
    </div>

    <!-- 更多链接 -->
    <div v-if="total > materials.length" class="text-center pb-8">
      <router-link to="/materials" class="inline-flex items-center text-[var(--color-primary)] font-medium text-sm hover:underline">
        查看更多资料 <ChevronRight :size="16" />
      </router-link>
    </div>
  </div>
</template>
```

- [ ] **Step 2: 重写 script（切换数据源）**

```vue
<script setup>
import { ref, onMounted } from 'vue'
import { getMaterials } from '@/api/material'
import { getCategories } from '@/api/category'
import MaterialCard from '@/components/MaterialCard.vue'
import { BookOpen, Sparkles, Search, ChevronRight } from 'lucide-vue-next'

const materials = ref([])
const categories = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(8)
const keyword = ref('')
const categoryId = ref(0)
const loading = ref(true)

const categoryChips = ref([{ id: 0, name: '全部分类' }])

function search() {
  page.value = 1
  fetchMaterials()
}

async function fetchMaterials() {
  loading.value = true
  try {
    const params = { page: page.value, page_size: pageSize.value }
    if (keyword.value) params.keyword = keyword.value
    if (categoryId.value) params.category_id = categoryId.value
    const res = await getMaterials(params)
    if (res.code === 200) {
      materials.value = res.data.list || []
      total.value = res.data.total || 0
    }
  } catch (e) { /* 后端未启动时静默 */ }
  finally { loading.value = false }
}

onMounted(async () => {
  fetchMaterials()
  try {
    const res = await getCategories()
    if (res.code === 200) {
      categories.value = res.data || []
      categoryChips.value = [{ id: 0, name: '全部分类' }, ...categories.value]
    }
  } catch (e) { /* 静默 */ }
})
</script>
```

- [ ] **Step 3: 确认无 scoped style 块残留，删掉旧的 `<style scoped>` 块**

- [ ] **Step 4: 验证**

```bash
cd web && npm run dev
```
打开浏览器：
- Hero 应该不再是紫色渐变，变成 teal 渐变 `from-teal-600 to-teal-500`
- 右侧有 AI 预览卡（静态展示）
- 分类 chips 点击切换
- 资料卡片用 MaterialCard 渲染
- 搜索框回车过滤
- 空数据时展示空态

- [ ] **Step 5: Commit**

```bash
git add web/src/views/Home.vue
git commit -m "feat(frontend): Home 重写 — teal Hero + AI预览卡 + materials API"
```

---

### Task 6: MaterialList.vue 重写

**Files:**
- Modify: `web/src/views/MaterialList.vue`

**Interfaces:**
- Consumes: Task 3（Navbar）、Task 4（MaterialCard）、Task 2（Select 组件）、`getMaterials`、`getCategories`、Pagination 组件
- Produces: 资料商城页顶部 chips 筛选 + 网格 + 发布资料按钮

- [ ] **Step 1: 重写 template**

```vue
<template>
  <div class="max-w-[1200px] mx-auto px-5 py-6">
    <!-- Header -->
    <div class="flex justify-between items-center mb-5">
      <h2 class="text-xl font-extrabold text-[var(--color-foreground)] m-0">学习资料</h2>
      <router-link
        v-if="userStore.isLoggedIn"
        to="/materials/new"
        class="inline-flex items-center px-4 py-2 bg-[var(--color-primary)] text-white rounded-[var(--radius-btn)] text-sm font-medium no-underline hover:brightness-90 transition-all"
      >
        <Plus :size="16" class="mr-1" />发布资料
      </router-link>
    </div>

    <!-- 筛选 bar -->
    <div class="flex flex-col sm:flex-row gap-3 mb-5">
      <div class="relative flex-1">
        <Search :size="18" class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
        <input
          v-model="keyword"
          @keyup.enter="search"
          placeholder="搜索资料..."
          class="w-full pl-10 pr-4 py-2.5 border border-[var(--color-border)] rounded-[var(--radius-input)] text-sm outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-teal-100 transition-all"
        />
      </div>
      <div class="flex gap-1.5 flex-wrap items-center">
        <button
          v-for="cat in categoryChips"
          :key="cat.id"
          @click="categoryId = cat.id; search()"
          :class="[
            'px-4 py-2 text-[13px] rounded-[var(--radius-pill)] font-medium transition-all whitespace-nowrap',
            categoryId === cat.id
              ? 'bg-[var(--color-primary)] text-white'
              : 'bg-white border border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]'
          ]"
        >{{ cat.name }}</button>
      </div>
      <select
        v-model="sortBy"
        @change="search()"
        class="px-3 py-2.5 border border-[var(--color-border)] rounded-[var(--radius-input)] text-sm bg-white text-[var(--color-foreground)] outline-none focus:border-[var(--color-primary)]"
      >
        <option value="latest">最新</option>
        <option value="price_asc">价格从低到高</option>
        <option value="price_desc">价格从高到低</option>
        <option value="popular">最多购买</option>
      </select>
    </div>

    <!-- 卡片网格 -->
    <div v-if="materials.length" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5">
      <MaterialCard v-for="m in materials" :key="m.id" :material="m" />
    </div>
    <div v-else class="text-center py-20 text-[var(--color-muted)]">
      <Search :size="48" class="mx-auto mb-4 opacity-30" />
      <p>没有找到匹配的资料</p>
    </div>

    <!-- 分页 -->
    <div v-if="total > pageSize" class="mt-8">
      <Pagination :total="total" :page="page" :page-size="pageSize" @change="onPage" />
    </div>
  </div>
</template>
```

- [ ] **Step 2: 重写 script**

```vue
<script setup>
import { ref, onMounted } from 'vue'
import { useUserStore } from '@/stores/user'
import { getMaterials } from '@/api/material'
import { getCategories } from '@/api/category'
import MaterialCard from '@/components/MaterialCard.vue'
import Pagination from '@/components/Pagination.vue'
import { Plus, Search } from 'lucide-vue-next'

const userStore = useUserStore()

const materials = ref([])
const keyword = ref('')
const categoryId = ref(0)
const sortBy = ref('latest')
const page = ref(1)
const pageSize = ref(12)
const total = ref(0)
const categories = ref([])
const categoryChips = ref([{ id: 0, name: '全部' }])

function search() { page.value = 1; load() }
function onPage(p) { page.value = p; load() }

async function load() {
  const params = { page: page.value, page_size: pageSize.value }
  if (keyword.value) params.keyword = keyword.value
  if (categoryId.value) params.category_id = categoryId.value
  if (sortBy.value !== 'latest') params.sort = sortBy.value
  const res = await getMaterials(params)
  materials.value = res.data.list
  total.value = res.data.total
}

onMounted(async () => {
  load()
  try {
    const res = await getCategories()
    if (res.code === 200) {
      categories.value = res.data || []
      categoryChips.value = [{ id: 0, name: '全部' }, ...categories.value]
    }
  } catch (e) { /* 静默 */ }
})
</script>
```

- [ ] **Step 3: 确认无 scoped style 块残留**

- [ ] **Step 4: 验证**

```bash
cd web && npm run dev
```
打开 `/materials`：
- 分类 chips 点击切换，选中 teal 实心
- 卡片 hover 上浮
- 排序下拉工作
- "发布资料"按钮登录后可见
- 分页正常

- [ ] **Step 5: Commit**

```bash
git add web/src/views/MaterialList.vue
git commit -m "feat(frontend): MaterialList 重写 — chips筛选+卡片网格+排序"
```

---

### Task 7: AgentChat.vue 重写（Part 1 — 布局 + 侧边栏折叠）

**Files:**
- Modify: `web/src/views/AgentChat.vue`

**Interfaces:**
- Consumes: Task 2（Dialog 组件）、Task 3（Navbar）、lucide-vue-next、`@/api/agent`（agentChat/getSessions/deleteSession/getMessages）、userStore
- Produces: 新 AgentChat 布局就绪（侧边栏可折叠 + 对话区骨架 + 输入栏 + Dialog 删除确认）
- 注意：Part 1 先做 layout + sidebar + input + dialog，Part 2 做 messages + SSE + 资料推荐卡

- [ ] **Step 1: 重写 template（layout + sidebar + input + dialog）**

```vue
<template>
  <div class="flex h-[calc(100vh-60px)] max-w-[1200px] mx-auto">
    <!-- 侧边栏 -->
    <aside
      :class="[
        'flex flex-col bg-[var(--color-background)] border-r border-[var(--color-border)] flex-shrink-0 transition-all duration-300',
        sidebarCollapsed ? 'w-[48px]' : 'w-[260px]'
      ]"
    >
      <div v-if="!sidebarCollapsed" class="p-4 border-b border-[var(--color-border)] flex justify-between items-center">
        <h3 class="m-0 text-base font-semibold">AI 对话</h3>
        <div class="flex gap-1">
          <button @click="newChat" class="px-3 py-1 text-xs border border-[var(--color-primary)] bg-white text-[var(--color-primary)] rounded-[var(--radius-btn)] cursor-pointer hover:bg-teal-50 transition-all">
            + 新对话
          </button>
          <button @click="sidebarCollapsed = true" class="p-1 border-none bg-transparent cursor-pointer text-[var(--color-muted)] hover:text-[var(--color-foreground)]">
            <PanelLeftClose :size="16" />
          </button>
        </div>
      </div>
      <button
        v-else
        @click="sidebarCollapsed = false"
        class="p-3 border-none bg-transparent cursor-pointer text-[var(--color-muted)] hover:text-[var(--color-primary)] transition-colors"
      >
        <PanelLeft :size="20" />
      </button>

      <div v-if="!sidebarCollapsed" class="flex-1 overflow-y-auto">
        <div v-if="sessions.length === 0" class="p-4 text-[13px] text-[var(--color-muted)] text-center">
          暂无对话，点击上方按钮开始
        </div>
        <div
          v-for="s in sessions"
          :key="s.id"
          @click="switchSession(s.id)"
          :class="[
            'flex items-center gap-2 px-4 py-3 border-b border-[var(--color-border)] cursor-pointer text-sm hover:bg-teal-50/50 transition-colors',
            s.id === currentSessionId ? 'bg-teal-50 border-l-[3px] border-l-[var(--color-primary)]' : 'border-l-[3px] border-l-transparent'
          ]"
        >
          <span class="flex-1 truncate">{{ s.title || '新对话' }}</span>
          <button @click.stop="removeSession(s.id)" class="border-none bg-transparent text-lg text-[var(--color-muted)] cursor-pointer p-0 leading-none hover:text-[var(--color-destructive)] transition-colors">&times;</button>
        </div>
      </div>
    </aside>

    <!-- 对话区 -->
    <main class="flex-1 flex flex-col min-w-0">
      <!-- 顶栏 -->
      <div class="flex items-center gap-2 px-4 py-2.5 border-b border-[var(--color-border)] text-sm">
        <button v-if="sidebarCollapsed" @click="sidebarCollapsed = false" class="border-none bg-transparent cursor-pointer text-[var(--color-muted)] p-1">
          <PanelLeft :size="16" />
        </button>
        <span class="font-semibold text-[var(--color-foreground)]">{{ currentSessionTitle || '新对话' }}</span>
        <span v-if="currentAgentType" class="ml-auto px-2.5 py-0.5 text-[11px] rounded-[var(--radius-pill)] bg-teal-50 text-[var(--color-primary)] font-medium">
          {{ currentAgentType === 'shopping' ? '购物模式' : currentAgentType === 'tutoring' ? '辅导模式' : currentAgentType }}
        </span>
      </div>

      <!-- 消息区（Part 2 填充） -->
      <div class="flex-1 overflow-y-auto p-5" ref="msgContainer">
        <div v-if="messages.length === 0 && !currentSessionId" class="text-center pt-20 text-[var(--color-muted)]">
          <MessageCircle :size="48" class="mx-auto mb-4 opacity-30" />
          <h2 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">有什么可以帮你的？</h2>
          <p class="text-sm">我可以帮你解答课程问题、推荐资料、处理订单咨询</p>
        </div>
        <!-- 消息气泡由 Part 2 实现，这里留插槽 -->
      </div>

      <!-- 输入区 -->
      <div class="p-4 border-t border-[var(--color-border)] flex gap-3">
        <input
          v-model="input"
          @keyup.enter="send"
          placeholder="输入你的问题..."
          :disabled="loading"
          ref="inputBox"
          class="flex-1 px-4 py-2.5 border border-[var(--color-border)] rounded-[var(--radius-input)] text-sm outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-teal-100 transition-all disabled:opacity-50"
        />
        <button
          @click="send"
          :disabled="loading || !input.trim()"
          class="px-6 py-2.5 bg-[var(--color-primary)] text-white border-none rounded-[var(--radius-btn)] text-sm font-medium cursor-pointer hover:brightness-90 disabled:opacity-40 disabled:cursor-not-allowed transition-all"
        >
          发送
        </button>
      </div>
    </main>

    <!-- 删除确认 Dialog -->
    <Dialog :open="showDelDialog" @update:open="showDelDialog = $event">
      <DialogContent class="sm:max-w-[380px]">
        <DialogHeader>
          <DialogTitle>删除对话</DialogTitle>
          <DialogDescription>
            确定要删除这个对话吗？<br />
            <span class="text-[var(--color-destructive)] text-sm">删除后无法恢复。</span>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter class="flex justify-end gap-3 mt-4">
          <Button variant="outline" @click="showDelDialog = false">取消</Button>
          <Button variant="destructive" @click="confirmDelete">删除</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
```

- [ ] **Step 2: 重写 script（Part 1 部分：session 管理 + 基础状态 + Dialog）**

```vue
<script setup>
import { ref, nextTick, onMounted, computed } from 'vue'
import { useUserStore } from '@/stores/user'
import { agentChat, getSessions, deleteSession, getMessages } from '@/api/agent'
import {
  PanelLeft, PanelLeftClose, MessageCircle
} from 'lucide-vue-next'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

const userStore = useUserStore()

const sessions = ref([])
const currentSessionId = ref(null)
const currentAgentType = ref('')
const messages = ref([])
const input = ref('')
const loading = ref(false)
const msgContainer = ref(null)
const inputBox = ref(null)
const showDelDialog = ref(false)
const pendingDeleteId = ref(null)
const sidebarCollapsed = ref(false)   // ★ 新增：侧边栏折叠状态

const currentSessionTitle = computed(() => {
  const s = sessions.value.find(s => s.id === currentSessionId.value)
  return s?.title || ''
})

// ===== 会话管理（保持不变）=====
async function loadSessions() {
  try {
    const res = await getSessions({ page: 1, page_size: 50 })
    sessions.value = res.data.list || []
  } catch (e) { console.error('加载会话列表失败', e) }
}

function parseHistoryMsg(m) {
  if (m.role === 'assistant' && m.content) {
    try {
      const parsed = JSON.parse(m.content)
      if (parsed.type === 'purchase_offer' && parsed.payload) {
        return { role: 'action', action: { type: 'purchase', ...parsed.payload } }
      }
    } catch {}
  }
  return { role: m.role, content: m.content }
}

async function loadMessages(sessionId) {
  try {
    const res = await getMessages(sessionId, { page: 1, page_size: 100 })
    messages.value = (res.data.list || [])
      .filter(m => m.role !== 'tool')
      .map(parseHistoryMsg)
    scrollToBottom()
  } catch (e) { console.error('加载消息失败', e) }
}

function newChat() {
  currentSessionId.value = null
  currentAgentType.value = ''
  messages.value = []
  input.value = ''
  nextTick(() => inputBox.value?.focus())
}

function switchSession(id) {
  currentSessionId.value = id
  const s = sessions.value.find(s => s.id === id)
  if (s) currentAgentType.value = s.agent_type
  loadMessages(id)
}

function removeSession(id) {
  pendingDeleteId.value = id
  showDelDialog.value = true
}

async function confirmDelete() {
  const id = pendingDeleteId.value
  showDelDialog.value = false
  pendingDeleteId.value = null
  if (!id) return
  try {
    await deleteSession(id)
    sessions.value = sessions.value.filter(s => s.id !== id)
    if (currentSessionId.value === id) newChat()
  } catch (e) { console.error('删除会话失败', e) }
}

function scrollToBottom() {
  nextTick(() => {
    if (msgContainer.value) {
      msgContainer.value.scrollTop = msgContainer.value.scrollHeight
    }
  })
}

onMounted(() => loadSessions())
</script>
```

- [ ] **Step 3: 确认无 scoped style 块残留**

- [ ] **Step 4: Commit**

```bash
git add web/src/views/AgentChat.vue
git commit -m "feat(frontend): AgentChat Part1 — 可折叠侧边栏 + Dialog删除确认"
```

---

### Task 8: AgentChat.vue 重写（Part 2 — 消息气泡 + SSE + 资料推荐卡）

**Files:**
- Modify: `web/src/views/AgentChat.vue`（在 Task 7 基础上添加消息区 + SSE 逻辑）

**Interfaces:**
- Consumes: Task 7（AgentChat 骨架）、Task 2（Badge）、Task 4（MaterialCard 样式参考）
- Produces: 完整的 AgentChat 聊天功能（消息气泡 + SSE 流式 + 资料推荐卡）

- [ ] **Step 1: 在 template 的消息区（`<!-- 消息气泡由 Part 2 实现 -->` 替换为）**

```vue
      <!-- 消息区 -->
      <div class="flex-1 overflow-y-auto p-5" ref="msgContainer">
        <!-- 欢迎 -->
        <div v-if="messages.length === 0 && !currentSessionId" class="text-center pt-20 text-[var(--color-muted)]">
          <MessageCircle :size="48" class="mx-auto mb-4 opacity-30" />
          <h2 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">有什么可以帮你的？</h2>
          <p class="text-sm">我可以帮你解答课程问题、推荐资料、处理订单咨询</p>
        </div>

        <!-- 消息列表 -->
        <div v-for="(msg, i) in messages" :key="i" :class="['flex mb-4', msg.role === 'user' ? 'justify-end' : 'justify-start']">
          <!-- thinking 气泡 -->
          <div v-if="msg.role === 'thinking'" class="bg-[var(--color-thinking-bg)] text-[var(--color-thinking-fg)] px-3 py-2 rounded-lg text-[13px] animate-[fadeIn_0.3s_ease] max-w-[75%]">
            &#128269; {{ msg.content }}
          </div>

          <!-- action 购买卡片 -->
          <div v-else-if="msg.role === 'action' && msg.action?.type === 'purchase'" class="bg-[var(--color-card)] border border-[var(--color-border)] rounded-xl px-5 py-4 max-w-[75%] text-center shadow-sm">
            <p class="text-[15px] font-semibold text-[var(--color-foreground)] mb-1.5">🛒 {{ msg.action.title }}</p>
            <p class="text-xl font-bold text-[var(--color-destructive)] mb-3">¥{{ msg.action.price }}</p>
            <router-link to="/orders" class="inline-block px-5 py-2 bg-[var(--color-primary)] text-white rounded-[var(--radius-btn)] text-sm font-medium no-underline hover:brightness-90 transition-all">立即购买</router-link>
          </div>

          <!-- 用户气泡 -->
          <div
            v-else-if="msg.role === 'user'"
            class="bg-[var(--color-primary)] text-white px-4 py-2.5 rounded-[12px_12px_2px_12px] text-sm leading-relaxed whitespace-pre-wrap break-words max-w-[75%]"
          >{{ msg.content }}</div>

          <!-- AI 气泡 -->
          <div
            v-else-if="msg.role === 'assistant'"
            class="bg-[var(--color-card)] border border-[var(--color-border)] px-4 py-2.5 rounded-[12px_12px_12px_2px] text-sm leading-relaxed whitespace-pre-wrap break-words max-w-[75%] text-[var(--color-foreground)]"
          >
            {{ msg.content }}

            <!-- 资料推荐卡组 (signature) -->
            <div v-if="msg.recommendations?.length" class="mt-3 pt-3 border-t border-[var(--color-border)]">
              <div class="text-xs text-[var(--color-muted)] mb-2">为你找到 {{ msg.recommendations.length }} 份资料：</div>
              <div class="flex gap-2 flex-wrap">
                <div
                  v-for="rec in msg.recommendations"
                  :key="rec.id"
                  @click.stop="$router.push(`/materials/${rec.id}`)"
                  class="flex items-center gap-2.5 bg-[var(--color-background)] rounded-lg p-2 cursor-pointer hover:bg-teal-50 transition-colors border border-[var(--color-border)] min-w-0"
                >
                  <div class="w-9 h-9 rounded-md bg-gradient-to-br from-teal-600 to-teal-400 flex-shrink-0 flex items-center justify-center">
                    <span class="text-white text-xs font-bold">{{ (rec.title || '?').charAt(0) }}</span>
                  </div>
                  <div class="min-w-0">
                    <div class="text-[13px] font-medium text-[var(--color-foreground)] truncate">{{ rec.title }}</div>
                    <div class="text-xs font-bold text-[var(--color-accent)]">¥{{ rec.price }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 加载中 dots -->
        <div v-if="loading" class="flex mb-4">
          <div class="bg-[var(--color-card)] border border-[var(--color-border)] px-4 py-3 rounded-[12px_12px_12px_2px] flex gap-1.5">
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--color-muted)] animate-bounce"></span>
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--color-muted)] animate-bounce" style="animation-delay:0.15s"></span>
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--color-muted)] animate-bounce" style="animation-delay:0.3s"></span>
          </div>
        </div>
      </div>
```

- [ ] **Step 2: 在 script 中添加 `send()` 函数（SSE 流式，从原来 AgentChat.vue 移植，调整样式相关部分）**

在现有 `</script>` 前插入：

```js
// ===== 发送消息 + SSE 流式（逻辑与原版相同，调整了气泡样式）=====
async function send() {
  if (!input.value.trim() || loading.value) return
  const question = input.value.trim()
  input.value = ''
  loading.value = true

  messages.value.push({ role: 'user', content: question })
  scrollToBottom()

  let currentAssistantIdx = -1

  try {
    const resp = await agentChat({
      session_id: currentSessionId.value || undefined,
      question
    }, userStore.accessToken)

    if (!resp.ok) {
      messages.value.push({ role: 'assistant', content: `请求失败 (${resp.status})` })
      loading.value = false
      return
    }

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let currentEvent = ''
    let streamEnded = false

    while (!streamEnded) {
      let readResult
      try {
        readResult = await Promise.race([
          reader.read(),
          new Promise((_, reject) => setTimeout(() => reject(new Error('STREAM_TIMEOUT')), 30000))
        ])
      } catch (raceErr) {
        if (raceErr.message === 'STREAM_TIMEOUT') console.log('SSE stream timeout, forcing end')
        break
      }
      const { done, value } = readResult
      if (done) { console.log('SSE stream closed by server'); break }

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i]
        if (line.startsWith('event: ')) {
          currentEvent = line.slice(7).trim()
          continue
        }
        if (line.startsWith('data: ')) {
          const payload = line.slice(6)

          if (currentEvent === 'delta') {
            try {
              const d = JSON.parse(payload)
              if (currentAssistantIdx === -1) {
                messages.value.push({ role: 'assistant', content: '', recommendations: [] })
                currentAssistantIdx = messages.value.length - 1
              }
              messages.value[currentAssistantIdx].content += d.content
            } catch {}
          } else if (currentEvent === 'action') {
            try {
              const d = JSON.parse(payload)
              if (d.type === 'purchase_offer') {
                messages.value.push({
                  role: 'action',
                  action: { type: 'purchase', ...d.payload }
                })
              }
              // 资料推荐 action（后端返回 material_recommendations）
              if (d.type === 'material_recommendations' && d.materials?.length) {
                if (currentAssistantIdx === -1) {
                  messages.value.push({ role: 'assistant', content: '', recommendations: [] })
                  currentAssistantIdx = messages.value.length - 1
                }
                messages.value[currentAssistantIdx].recommendations = d.materials
              }
            } catch {}
          } else if (currentEvent === 'done') {
            streamEnded = true
            try {
              const d = JSON.parse(payload)
              if (d.session_id && !currentSessionId.value) {
                currentSessionId.value = d.session_id
                currentAgentType.value = d.agent_type
                loadSessions()
              }
            } catch {}
          } else if (currentEvent === 'error') {
            streamEnded = true
            try {
              const d = JSON.parse(payload)
              messages.value.push({ role: 'assistant', content: d.message || '发生错误，请重试' })
            } catch {
              messages.value.push({ role: 'assistant', content: '发生错误，请重试' })
            }
          }
          scrollToBottom()
        }
      }
      if (streamEnded) break
    }
  } catch (e) {
    messages.value.push({ role: 'assistant', content: `连接失败: ${e.message}` })
  } finally {
    messages.value = messages.value.filter(m => m.role !== 'thinking')
    currentAssistantIdx = -1
    loading.value = false
  }
}
```

- [ ] **Step 3: 确保 `MessageCircle` 也在 import 中**

```js
import { PanelLeft, PanelLeftClose, MessageCircle } from 'lucide-vue-next'
```

- [ ] **Step 4: 验证**

```bash
cd web && npm run dev
```
打开 `/agent`：
- 侧边栏可折叠/展开（点击 `PanelLeftClose` 收起，点击 `PanelLeft` 展开）
- 新建对话、切换会话
- 发送消息 → SSE 流式显示
- teal 用户气泡 + 白底 AI 气泡
- thinking 气泡 amber 色
- purchase action 卡片
- 删除确认用 shadcn Dialog
- 加载中跳动的三个灰点

- [ ] **Step 5: Commit**

```bash
git add web/src/views/AgentChat.vue
git commit -m "feat(frontend): AgentChat Part2 — 消息气泡 + SSE流式 + 资料推荐卡"
```

---

### Task 9: 清理 + 构建验证

**Files:**
- Delete: `web/src/components/CourseCard.vue`
- Modify: `web/src/App.vue`（如有需要）

**Interfaces:**
- Consumes: Task 5（Home 已切换数据源，CourseCard 无引用）
- Produces: 旧组件删除，构建通过

- [ ] **Step 1: 确认 CourseCard 不再被任何文件引用**

```bash
cd web && grep -r "CourseCard" src/ --include="*.vue" --include="*.js"
```

预期无输出。如果有残留引用（例如 admin/CourseManage.vue 用了同名的但不同组件），那条是正常的、不删。

- [ ] **Step 2: 删除 CourseCard.vue**

```bash
rm web/src/components/CourseCard.vue
```

- [ ] **Step 3: 更新 App.vue 的 main-content padding（适配新 Navbar 高度）**

```vue
<!-- web/src/App.vue — 只需替换 style -->
<style scoped>
.main-content {
  min-height: calc(100vh - 60px);
}
</style>
```

去掉原来的 `padding: 20px` 和 `max-width: 1200px; margin: 0 auto`——各个页面的容器由各自的 Tailwind class 控制（Home 全宽 Hero，MaterialList 有 `max-w-[1200px]`，AgentChat 有 `flex h-[calc(100vh-60px)]`）。

- [ ] **Step 4: 构建验证**

```bash
cd web && npm run build
```

预期：无 error，无 warning。如果 shadcn-vue 组件的某些 prop 产生 deprecation warning（不是 error），忽略。

- [ ] **Step 5: 完整功能冒烟**

```bash
cd web && npm run dev
```

逐页检查：
- `/` Home — Hero teal 渐变 + AI 预览卡 + 搜索 + 资料网格
- `/materials` MaterialList — 筛选 + 卡片 + 分页 + 发布按钮
- `/agent` AgentChat — 侧边栏折叠 + 会话切换 + 发送消息 + SSE 流式 + 删除 Dialog
- 所有页面无 scoped style 残留（仅在编辑器中搜索 `<style scoped>`）
- 全站无 emoji 图标（🎓📚🤖）

- [ ] **Step 6: Commit**

```bash
git add web/src/components/CourseCard.vue web/src/App.vue
git commit -m "chore(frontend): 删除 CourseCard + App.vue 布局适配"
```

---

## Plan Self-Review

**1. Spec coverage:** 逐条对照 spec：
- §2 技术选型 → Task 1（依赖+Tailwind）+ Task 2（shadcn-vue） ✓
- §3 设计 Token → Task 1 Step 3（CSS 变量定义） ✓
- §4 组件库 → Task 2（6 个 shadcn-vue 组件） ✓
- §5.1 Home → Task 5 ✓
- §5.2 MaterialList → Task 6 ✓
- §5.3 AgentChat → Task 7 + 8 ✓
- §6 数据迁移 → Task 5（Home 切 materials）+ Task 9（删 CourseCard） ✓
- §8 非目标 → 无后续页面任务，暗色模式未涉及 ✓
- §9 成功标准 → Task 9 构建+冒烟验证 ✓

**2. Placeholder scan:** 所有步骤都有实际代码。无 TBD/TODO/"add validation" 类占位。

**3. Type consistency:**
- MaterialCard props: `material: { type: Object, required: true }` — 一致
- AgentChat `messages[].recommendations[]` 对象格式 `{ id, title, price }` — 在 Task 8 的 template 和 SSE 逻辑中一致
- CSS token 变量名 `--color-*` / `--radius-*` — 全计划一致
- `cn()` 导入路径 `@/lib/utils` — Task 1 定义，全计划一致
