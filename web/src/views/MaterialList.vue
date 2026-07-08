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
  try {
    const params = { page: page.value, page_size: pageSize.value }
    if (keyword.value) params.keyword = keyword.value
    if (categoryId.value) params.category_id = categoryId.value
    if (sortBy.value !== 'latest') params.sort = sortBy.value
    const res = await getMaterials(params)
    if (res.code === 200) {
      materials.value = res.data.list || []
      total.value = res.data.total || 0
    }
  } catch (e) { /* 后端未启动时静默 */ }
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
