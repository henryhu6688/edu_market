<template>
  <div class="material-list">
    <h2>学习资料</h2>
    <div class="filters">
      <input v-model="keyword" placeholder="搜索资料..." @keyup.enter="search" />
      <button @click="search">搜索</button>
    </div>
    <div class="grid">
      <div v-for="m in materials" :key="m.id" class="card" @click="$router.push(`/materials/${m.id}`)">
        <img :src="m.cover_image || '/placeholder.jpg'" :alt="m.title" class="cover" />
        <div class="info">
          <h3>{{ m.title }}</h3>
          <p class="desc">{{ m.description?.substring(0, 60) || '' }}</p>
          <div class="meta">
            <span class="price">¥{{ m.price }}</span>
            <span>{{ m.buy_count }} 人购买</span>
          </div>
        </div>
      </div>
    </div>
    <Pagination v-if="total > pageSize" :total="total" :page="page" :page-size="pageSize" @change="onPage" />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getMaterials } from '@/api/material'
import Pagination from '@/components/Pagination.vue'

const materials = ref([])
const keyword = ref('')
const page = ref(1)
const pageSize = ref(12)
const total = ref(0)

async function load() {
  const res = await getMaterials({ page: page.value, page_size: pageSize.value, keyword: keyword.value })
  materials.value = res.data.list
  total.value = res.data.total
}
function search() { page.value = 1; load() }
function onPage(p) { page.value = p; load() }
onMounted(load)
</script>

<style scoped>
.material-list { max-width: 1200px; margin: 0 auto; padding: 20px; }
h2 { margin-bottom: 20px; }
.filters { display: flex; gap: 10px; margin-bottom: 20px; }
.filters input { flex: 1; padding: 8px 12px; border: 1px solid #d1d5db; border-radius: 6px; font-size: 14px; }
.filters button { padding: 8px 20px; background: #3b82f6; color: #fff; border: none; border-radius: 6px; cursor: pointer; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 20px; }
.card { border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden; cursor: pointer; transition: box-shadow 0.2s; }
.card:hover { box-shadow: 0 4px 12px rgba(0,0,0,0.1); }
.cover { width: 100%; height: 150px; object-fit: cover; background: #f3f4f6; }
.info { padding: 12px; }
.info h3 { margin: 0 0 6px; font-size: 16px; }
.desc { font-size: 13px; color: #6b7280; margin-bottom: 8px; }
.meta { display: flex; justify-content: space-between; font-size: 13px; color: #6b7280; }
.price { color: #ef4444; font-weight: 600; font-size: 16px; }
</style>
