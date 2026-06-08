<template>
  <div class="category-manage">
    <div class="page-header">
      <h1>分类管理</h1>
      <button class="btn-add" @click="showForm = true">{{ editing ? '取消编辑' : '新增分类' }}</button>
    </div>
    <div class="admin-nav">
      <router-link to="/admin/dashboard">概览</router-link>
      <router-link to="/admin/courses">课程管理</router-link>
      <router-link to="/admin/categories">分类管理</router-link>
    </div>

    <!-- 表单 -->
    <div v-if="showForm" class="form-card">
      <h3>{{ editing ? '编辑分类' : '新增分类' }}</h3>
      <input v-model="form.name" placeholder="分类名称" />
      <input v-model="form.description" placeholder="分类描述" />
      <div class="form-actions">
        <button class="btn-submit" @click="editing ? handleUpdate() : handleCreate()">{{ editing ? '更新' : '创建' }}</button>
        <button class="btn-cancel" @click="showForm = false; editing = false">取消</button>
      </div>
    </div>

    <!-- 列表 -->
    <div class="table-wrap">
      <table>
        <thead><tr><th>ID</th><th>名称</th><th>描述</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="c in categories" :key="c.id">
            <td>{{ c.id }}</td>
            <td>{{ c.name }}</td>
            <td>{{ c.description }}</td>
            <td>
              <button class="btn-sm" @click="startEdit(c)">编辑</button>
              <button class="btn-sm btn-del" @click="handleDelete(c.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { getCategories, createCategory, updateCategory, deleteCategory } from '@/api/category'

const categories = ref([])
const showForm = ref(false)
const editing = ref(false)
const form = reactive({ name: '', description: '' })

async function fetch() {
  const res = await getCategories()
  if (res.code === 200) categories.value = res.data || []
}

async function handleCreate() {
  await createCategory(form)
  showForm.value = false
  form.name = ''; form.description = ''
  fetch()
}

function startEdit(c) {
  editing.value = c.id
  showForm.value = true
  form.name = c.name
  form.description = c.description
}

async function handleUpdate() {
  await updateCategory(editing.value, form)
  showForm.value = false; editing.value = false
  form.name = ''; form.description = ''
  fetch()
}

async function handleDelete(id) {
  if (confirm('确定删除？')) {
    await deleteCategory(id)
    fetch()
  }
}

onMounted(fetch)
</script>

<style scoped>
.category-manage { max-width: 800px; margin: 0 auto; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-header h1 { margin: 0; }
.admin-nav { display: flex; gap: 8px; margin-bottom: 24px; }
.admin-nav a { padding: 6px 16px; background: #f4f6fb; color: #555; border-radius: 6px; text-decoration: none; font-size: 13px; }
.admin-nav a.router-link-exact-active { background: #4f46e5; color: #fff; }
.btn-add { padding: 8px 18px; background: #4f46e5; color: #fff; border: none; border-radius: 6px; cursor: pointer; }
.form-card { background: #fff; padding: 24px; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.06); margin-bottom: 20px; display: flex; flex-direction: column; gap: 12px; }
.form-card input { padding: 10px 14px; border: 1px solid #ddd; border-radius: 8px; font-size: 14px; outline: none; }
.form-card input:focus { border-color: #4f46e5; }
.form-actions { display: flex; gap: 10px; }
.btn-submit, .btn-cancel { padding: 8px 20px; border: none; border-radius: 6px; cursor: pointer; }
.btn-submit { background: #4f46e5; color: #fff; }
.btn-cancel { background: #f1f5f9; color: #555; }
.table-wrap { background: #fff; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.06); overflow: hidden; }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 12px 16px; text-align: left; border-bottom: 1px solid #f0f0f0; font-size: 13px; }
th { background: #fafbfc; font-weight: 600; }
.btn-sm { padding: 4px 10px; border: 1px solid #ddd; background: #fff; border-radius: 4px; cursor: pointer; font-size: 12px; margin-right: 4px; }
.btn-sm:hover { border-color: #4f46e5; color: #4f46e5; }
.btn-del { color: #e74c3c; border-color: #fecaca; }
.btn-del:hover { background: #fef2f2; }
</style>
