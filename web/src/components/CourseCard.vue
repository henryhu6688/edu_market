<template>
  <div class="course-card" @click="goDetail">
    <div class="card-cover">
      <img v-if="course.cover_image" :src="course.cover_image" alt="" />
      <div v-else class="cover-placeholder">📚</div>
    </div>
    <div class="card-body">
      <span class="category-tag">{{ course.category?.name || '未分类' }}</span>
      <h3 class="card-title">{{ course.title }}</h3>
      <p class="card-desc">{{ course.description?.substring(0, 80) || '暂无简介' }}</p>
      <div class="card-footer">
        <span class="price">¥{{ course.price }}</span>
        <span class="stats">
          👁️ {{ course.view_count }} | 🛒 {{ course.buy_count }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router'

const props = defineProps({ course: { type: Object, required: true } })
const router = useRouter()

function goDetail() {
  router.push(`/course/${props.course.id}`)
}
</script>

<style scoped>
.course-card {
  background: #fff;
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 2px 8px rgba(0,0,0,0.06);
  cursor: pointer;
  transition: all 0.3s;
}
.course-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 8px 24px rgba(0,0,0,0.12);
}
.card-cover {
  height: 160px;
  background: #f0f0ff;
  display: flex;
  align-items: center;
  justify-content: center;
}
.card-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cover-placeholder {
  font-size: 48px;
}
.card-body {
  padding: 16px;
}
.category-tag {
  font-size: 12px;
  color: #4f46e5;
  background: #eef2ff;
  padding: 2px 8px;
  border-radius: 4px;
}
.card-title {
  margin: 8px 0;
  font-size: 16px;
  color: #1a1a2e;
}
.card-desc {
  font-size: 13px;
  color: #888;
  margin-bottom: 12px;
}
.card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.price {
  font-size: 20px;
  font-weight: 700;
  color: #e74c3c;
}
.stats {
  font-size: 12px;
  color: #aaa;
}
</style>
