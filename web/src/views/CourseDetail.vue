<template>
  <div class="course-detail" v-if="course">
    <div class="detail-header">
      <div class="detail-info">
        <span class="cat-tag">{{ course.category?.name || '未分类' }}</span>
        <h1>{{ course.title }}</h1>
        <p class="meta">👁️ {{ course.view_count }} 次浏览 | 🛒 {{ course.buy_count }} 人已购买</p>
        <p class="desc">{{ course.description || '暂无简介' }}</p>
        <div class="price-row">
          <span class="price">¥{{ course.price }}</span>
          <button v-if="userStore.isLoggedIn" class="btn-buy" @click="handleBuy">立即购买</button>
          <router-link v-else to="/login" class="btn-buy">登录后购买</router-link>
        </div>
      </div>
    </div>

    <!-- 评论区 -->
    <div class="reviews-section">
      <h3>用户评价</h3>
      <div v-if="userStore.isLoggedIn" class="review-form">
        <StarRating v-model="newRating" :interactive="true" />
        <textarea v-model="newContent" placeholder="写下你的评价..." rows="3"></textarea>
        <button @click="submitReview" class="btn-submit-review">发表评价</button>
      </div>
      <div v-if="reviews.length" class="review-list">
        <div v-for="r in reviews" :key="r.id" class="review-item">
          <div class="review-top">
            <strong>{{ r.user?.username }}</strong>
            <StarRating :modelValue="r.rating" />
            <span class="review-time">{{ new Date(r.created_at).toLocaleDateString() }}</span>
          </div>
          <p>{{ r.content }}</p>
        </div>
      </div>
      <p v-else class="no-reviews">暂无评价</p>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { getCourseDetail } from '@/api/course'
import { createOrder } from '@/api/order'
import { getReviews, createReview } from '@/api/review'
import { useUserStore } from '@/stores/user'
import StarRating from '@/components/StarRating.vue'

const route = useRoute()
const userStore = useUserStore()
const course = ref(null)
const reviews = ref([])
const newRating = ref(5)
const newContent = ref('')

async function fetchCourse() {
  try {
    const res = await getCourseDetail(route.params.id)
    if (res.code === 200) course.value = res.data
  } catch (e) { /* 静默 */ }
}

async function fetchReviews() {
  try {
    const res = await getReviews(route.params.id, { page: 1, page_size: 50 })
    if (res.code === 200) reviews.value = res.data.list || []
  } catch (e) { /* 静默 */ }
}

async function handleBuy() {
  try {
    const res = await createOrder({ course_id: course.value.id })
    if (res.code === 201) alert('订单已创建，请前往订单页完成支付')
  } catch (e) { alert(e?.message || '购买失败') }
}

async function submitReview() {
  try {
    const res = await createReview({
      course_id: course.value.id,
      rating: newRating.value,
      content: newContent.value
    })
    if (res.code === 201) {
      newContent.value = ''
      newRating.value = 5
      fetchReviews()
    }
  } catch (e) { alert(e?.message || '评论失败') }
}

onMounted(() => { fetchCourse(); fetchReviews() })
</script>

<style scoped>
.course-detail { max-width: 800px; margin: 0 auto; }
.detail-header { background: #fff; padding: 32px; border-radius: 16px; box-shadow: 0 2px 12px rgba(0,0,0,0.06); margin-bottom: 24px; }
.cat-tag { font-size: 12px; color: #4f46e5; background: #eef2ff; padding: 2px 10px; border-radius: 4px; }
.detail-info h1 { margin: 12px 0; font-size: 24px; }
.meta { color: #888; font-size: 14px; }
.desc { color: #555; margin: 16px 0; line-height: 1.7; }
.price-row { display: flex; align-items: center; gap: 20px; margin-top: 20px; }
.price { font-size: 32px; font-weight: 700; color: #e74c3c; }
.btn-buy {
  padding: 12px 32px; background: #4f46e5; color: #fff;
  border: none; border-radius: 8px; font-size: 16px; cursor: pointer; text-decoration: none;
}
.btn-buy:hover { background: #4338ca; }
.reviews-section {
  background: #fff; padding: 24px 32px; border-radius: 16px; box-shadow: 0 2px 12px rgba(0,0,0,0.06);
}
.reviews-section h3 { margin-bottom: 16px; }
.review-form { margin-bottom: 20px; display: flex; flex-direction: column; gap: 10px; }
.review-form textarea {
  width: 100%; padding: 10px; border: 1px solid #ddd;
  border-radius: 8px; font-size: 14px; outline: none; resize: vertical; box-sizing: border-box;
}
.review-form textarea:focus { border-color: #4f46e5; }
.btn-submit-review {
  align-self: flex-end; padding: 8px 20px; background: #4f46e5;
  color: #fff; border: none; border-radius: 6px; cursor: pointer;
}
.review-item { padding: 14px 0; border-bottom: 1px solid #f0f0f0; }
.review-top { display: flex; align-items: center; gap: 10px; margin-bottom: 6px; }
.review-time { color: #aaa; font-size: 12px; margin-left: auto; }
.no-reviews { color: #999; text-align: center; padding: 20px; }
</style>
