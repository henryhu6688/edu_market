<template>
  <div class="material-editor">
    <h2>{{ isEdit ? '编辑资料' : '发布学习资料' }}</h2>
    <form @submit.prevent="save">
      <label>资料名称</label>
      <input v-model="form.title" placeholder="给你的资料取个名字" required maxlength="200" />

      <label>分类</label>
      <select v-model="form.category_id" required>
        <option value="">请选择分类</option>
        <option v-for="c in categories" :key="c.id" :value="c.id">{{ c.name }}</option>
      </select>

      <label>描述</label>
      <textarea v-model="form.description" placeholder="介绍一下这份资料的内容" rows="5"></textarea>

      <label>价格（元）</label>
      <input v-model.number="form.price" type="number" min="0" step="0.01" placeholder="0 表示免费" />

      <label>封面图 URL</label>
      <input v-model="form.cover_image" placeholder="图片链接（可选）" />

      <div class="actions">
        <button type="submit" :disabled="saving">{{ saving ? '发布中...' : '发布' }}</button>
        <router-link to="/materials" class="cancel">取消</router-link>
      </div>
    </form>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { createMaterial, getMaterial, updateMaterial } from '@/api/material'
import { getCategories } from '@/api/category'

const route = useRoute()
const router = useRouter()
const isEdit = ref(false)
const saving = ref(false)
const categories = ref([])

const form = ref({
  title: '', category_id: '', description: '', price: 0, cover_image: ''
})

onMounted(async () => {
  const res = await getCategories()
  categories.value = res.data || []
  if (route.params.id) {
    isEdit.value = true
    const m = await getMaterial(route.params.id)
    Object.assign(form.value, {
      title: m.data.title, category_id: m.data.category_id,
      description: m.data.description || '', price: m.data.price,
      cover_image: m.data.cover_image || ''
    })
  }
})

async function save() {
  saving.value = true
  try {
    let res
    if (isEdit.value) {
      res = await updateMaterial(route.params.id, form.value)
    } else {
      res = await createMaterial(form.value)
    }
    router.push(`/materials/${res.data.id}`)
  } catch (e) {
    alert('保存失败：' + (e.message || '未知错误'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.material-editor { max-width: 600px; margin: 20px auto; padding: 20px; }
h2 { margin-bottom: 24px; }
label { display: block; margin: 12px 0 4px; font-size: 14px; font-weight: 500; color: #374151; }
input, select, textarea { width: 100%; padding: 8px 12px; border: 1px solid #d1d5db; border-radius: 6px; font-size: 14px; outline: none; box-sizing: border-box; }
input:focus, select:focus, textarea:focus { border-color: #3b82f6; }
.actions { margin-top: 20px; display: flex; gap: 12px; align-items: center; }
.actions button { padding: 10px 24px; background: #3b82f6; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-size: 14px; }
.actions button:disabled { background: #9ca3af; }
.cancel { color: #6b7280; font-size: 14px; }
</style>
