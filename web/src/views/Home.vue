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
