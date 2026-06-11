<template>
  <div class="material-detail" v-if="material">
    <div class="header">
      <img :src="material.cover_image || '/placeholder.jpg'" class="cover" />
      <div class="meta">
        <h1>{{ material.title }}</h1>
        <p class="author">发布者：{{ material.user?.username }}</p>
        <p class="price">¥{{ material.price }}</p>
        <button v-if="canEdit" @click="$router.push(`/materials/${material.id}/docs`)">📝 编辑文档</button>
        <button v-else-if="!purchased" @click="buy">🛒 购买</button>
        <span v-else class="owned">✅ 已购买</span>
      </div>
    </div>
    <div class="desc">{{ material.description }}</div>
    <div class="doc-tree" v-if="docs.length">
      <h3>文档目录</h3>
      <div v-for="d in docs" :key="d.id" class="doc-item" @click="openDoc(d)">
        {{ d.parent_id ? '📄' : '📁' }} {{ d.title }}
        <span v-if="d.is_free_preview" class="free-badge">免费试读</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { getMaterial, deleteMaterial } from '@/api/material'
import { getDocTree } from '@/api/document'
import { createOrder } from '@/api/order'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const material = ref(null)
const docs = ref([])
const purchased = ref(false)

const canEdit = computed(() => material.value?.user_id === userStore.user?.id || userStore.isAdmin)

async function load() {
  const res = await getMaterial(route.params.id)
  material.value = res.data
  const tree = await getDocTree(route.params.id)
  docs.value = tree.data || []
}

function openDoc(doc) {
  if (doc.is_free_preview || purchased.value || canEdit.value) {
    router.push(`/materials/${material.value.id}/docs/${doc.id}`)
  } else {
    alert('请先购买后再查看')
  }
}

async function buy() {
  try {
    await createOrder({ course_id: material.value.id, price: material.value.price })
    await router.push('/orders')
  } catch (e) {
    alert('创建订单失败')
  }
}

onMounted(load)
</script>

<style scoped>
.material-detail { max-width: 900px; margin: 0 auto; padding: 20px; }
.header { display: flex; gap: 20px; margin-bottom: 20px; }
.cover { width: 200px; height: 150px; object-fit: cover; border-radius: 8px; background: #f3f4f6; }
.meta h1 { margin: 0 0 8px; font-size: 24px; }
.author { color: #6b7280; font-size: 14px; margin-bottom: 8px; }
.price { font-size: 28px; color: #ef4444; font-weight: 700; margin-bottom: 12px; }
.meta button { padding: 8px 20px; background: #3b82f6; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-size: 14px; }
.owned { color: #10b981; font-weight: 600; }
.desc { background: #f9fafb; padding: 16px; border-radius: 8px; margin-bottom: 20px; white-space: pre-wrap; }
.doc-tree h3 { margin-bottom: 10px; }
.doc-item { padding: 8px 12px; cursor: pointer; border-radius: 4px; display: flex; align-items: center; gap: 8px; }
.doc-item:hover { background: #f3f4f6; }
.free-badge { font-size: 11px; background: #dbeafe; color: #1d4ed8; padding: 1px 6px; border-radius: 4px; }
</style>
