<template>
  <div class="home">
    <div class="hero">
      <h1>📚 在线学习资料售卖平台</h1>
      <p>精选优质学习资料，AI 智能答疑，助力你的学习之路</p>
    </div>

    <!-- 搜索 + 分类筛选 -->
    <div class="filters">
      <input v-model="keyword" @keyup.enter="search" placeholder="搜索课程..." class="search-input" />
      <select v-model="categoryId" @change="search" class="category-select">
        <option :value="0">全部分类</option>
        <option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option>
      </select>
      <button @click="search" class="btn-search">搜索</button>
    </div>

    <!-- 课程列表 -->
    <div class="course-grid" v-if="courses.length">
      <CourseCard v-for="course in courses" :key="course.id" :course="course" />
    </div>
    <div v-else class="empty">
      <p>暂无课程数据</p>
      <p class="hint">请先启动后端服务并确保数据库已连接</p>
    </div>

    <Pagination
      :page="page" :pageSize="pageSize" :total="total"
      @change="p => { page = p; fetchCourses() }"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getCourses } from '@/api/course'
import { getCategories } from '@/api/category'
import CourseCard from '@/components/CourseCard.vue'
import Pagination from '@/components/Pagination.vue'

const courses = ref([])
const categories = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(12)
const keyword = ref('')
const categoryId = ref(0)

function search() {
  page.value = 1
  fetchCourses()
}

async function fetchCourses() {
  try {
    const res = await getCourses({ page: page.value, page_size: pageSize.value, keyword: keyword.value, category_id: categoryId.value })
    if (res.code === 200) {
      courses.value = res.data.list || []
      total.value = res.data.total || 0
    }
  } catch (e) { /* 后端未启动时静默 */ }
}

onMounted(async () => {
  fetchCourses()
  try {
    const res = await getCategories()
    if (res.code === 200) categories.value = res.data || []
  } catch (e) { /* 静默 */ }
})
</script>

<style scoped>
.hero {
  text-align: center;
  padding: 40px 0;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 16px;
  color: #fff;
  margin-bottom: 24px;
}
.hero h1 { margin: 0 0 8px; font-size: 28px; }
.hero p { opacity: 0.9; margin: 0; }
.filters {
  display: flex;
  gap: 10px;
  margin-bottom: 24px;
  flex-wrap: wrap;
}
.search-input {
  flex: 1;
  min-width: 200px;
  padding: 10px 16px;
  border: 1px solid #ddd;
  border-radius: 8px;
  font-size: 14px;
  outline: none;
}
.search-input:focus { border-color: #4f46e5; }
.category-select {
  padding: 10px 16px;
  border: 1px solid #ddd;
  border-radius: 8px;
  font-size: 14px;
  outline: none;
}
.btn-search {
  padding: 10px 24px;
  background: #4f46e5;
  color: #fff;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
}
.btn-search:hover { background: #4338ca; }
.course-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 20px;
}
.empty { text-align: center; padding: 60px 0; color: #999; }
.empty .hint { font-size: 13px; color: #bbb; margin-top: 8px; }
</style>
