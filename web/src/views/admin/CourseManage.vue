<template>
  <div class="course-manage">
    <div class="page-header">
      <h1>课程管理</h1>
      <button class="btn-add" @click="showForm = true">{{ editing ? '取消编辑' : '新增课程' }}</button>
    </div>
    <div class="admin-nav">
      <router-link to="/admin/dashboard">概览</router-link>
      <router-link to="/admin/courses">课程管理</router-link>
      <router-link to="/admin/categories">分类管理</router-link>
    </div>

    <!-- 表单 -->
    <div v-if="showForm" class="form-card">
      <h3>{{ editing ? '编辑课程' : '新增课程' }}</h3>
      <input v-model="form.title" placeholder="课程标题" />
      <input v-model="form.price" type="number" step="0.01" placeholder="价格" />
      <select v-model="form.category_id">
        <option :value="0">选择分类</option>
        <option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option>
      </select>
      <textarea v-model="form.description" placeholder="课程描述" rows="4"></textarea>
      <div class="form-actions">
        <button class="btn-submit" @click="editing ? handleUpdate() : handleCreate()">{{ editing ? '更新' : '创建' }}</button>
        <button class="btn-cancel" @click="showForm = false; editing = false">取消</button>
      </div>
    </div>

    <!-- 列表 -->
    <div class="course-table">
      <table>
        <thead>
          <tr><th>ID</th><th>标题</th><th>分类</th><th>价格</th><th>状态</th><th>浏览/购买</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr v-for="c in courses" :key="c.id">
            <td>{{ c.id }}</td>
            <td>{{ c.title }}</td>
            <td>{{ c.category?.name }}</td>
            <td>¥{{ c.price }}</td>
            <td><span class="status" :class="c.status">{{ c.status }}</span></td>
            <td>{{ c.view_count }} / {{ c.buy_count }}</td>
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
import { getCourses, createCourse, updateCourse, deleteCourse } from '@/api/course'
import { getCategories } from '@/api/category'

const courses = ref([])
const categories = ref([])
const showForm = ref(false)
const editing = ref(false)
const form = reactive({ title: '', price: 0, category_id: 0, description: '' })

async function fetchCourses() {
  const res = await getCourses({ page: 1, page_size: 100 })
  if (res.code === 200) courses.value = res.data.list || []
}

async function handleCreate() {
  await createCourse(form)
  showForm.value = false
  resetForm()
  fetchCourses()
}

function startEdit(c) {
  editing.value = c.id
  showForm.value = true
  Object.assign(form, { title: c.title, price: c.price, category_id: c.category_id, description: c.description })
}

async function handleUpdate() {
  await updateCourse(editing.value, form)
  showForm.value = false
  editing.value = false
  resetForm()
  fetchCourses()
}

async function handleDelete(id) {
  if (confirm('确定删除？')) {
    await deleteCourse(id)
    fetchCourses()
  }
}

function resetForm() {
  Object.assign(form, { title: '', price: 0, category_id: 0, description: '' })
}

onMounted(async () => {
  fetchCourses()
  const res = await getCategories()
  if (res.code === 200) categories.value = res.data || []
})
</script>

<style scoped>
.course-manage { max-width: 1000px; margin: 0 auto; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-header h1 { margin: 0; }
.admin-nav { display: flex; gap: 8px; margin-bottom: 24px; }
.admin-nav a { padding: 6px 16px; background: #f4f6fb; color: #555; border-radius: 6px; text-decoration: none; font-size: 13px; }
.admin-nav a.router-link-exact-active { background: #4f46e5; color: #fff; }
.btn-add { padding: 8px 18px; background: #4f46e5; color: #fff; border: none; border-radius: 6px; cursor: pointer; }
.form-card {
  background: #fff; padding: 24px; border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.06); margin-bottom: 20px;
  display: flex; flex-direction: column; gap: 12px;
}
.form-card input, .form-card select, .form-card textarea {
  padding: 10px 14px; border: 1px solid #ddd; border-radius: 8px; font-size: 14px; outline: none; font-family: inherit;
}
.form-card input:focus, .form-card select:focus, .form-card textarea:focus { border-color: #4f46e5; }
.form-actions { display: flex; gap: 10px; }
.btn-submit, .btn-cancel { padding: 8px 20px; border: none; border-radius: 6px; cursor: pointer; }
.btn-submit { background: #4f46e5; color: #fff; }
.btn-cancel { background: #f1f5f9; color: #555; }
.course-table { background: #fff; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.06); overflow: hidden; }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 12px 16px; text-align: left; border-bottom: 1px solid #f0f0f0; font-size: 13px; }
th { background: #fafbfc; font-weight: 600; }
.status { padding: 2px 6px; border-radius: 4px; font-size: 11px; }
.status.published { background: #d1fae5; color: #059669; }
.status.draft { background: #fef3c7; color: #d97706; }
.status.off { background: #f1f5f9; color: #94a3b8; }
.btn-sm { padding: 4px 10px; border: 1px solid #ddd; background: #fff; border-radius: 4px; cursor: pointer; font-size: 12px; margin-right: 4px; }
.btn-sm:hover { border-color: #4f46e5; color: #4f46e5; }
.btn-del { color: #e74c3c; border-color: #fecaca; }
.btn-del:hover { background: #fef2f2; color: #e74c3c; }
</style>
