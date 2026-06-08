<template>
  <div class="admin-dashboard">
    <h1>管理后台</h1>
    <div class="admin-nav">
      <router-link to="/admin/dashboard">概览</router-link>
      <router-link to="/admin/courses">课程管理</router-link>
      <router-link to="/admin/categories">分类管理</router-link>
    </div>
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{{ stats.courses }}</div>
        <div class="stat-label">课程总数</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ stats.orders }}</div>
        <div class="stat-label">订单总数</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ stats.users }}</div>
        <div class="stat-label">用户总数</div>
      </div>
    </div>
    <p class="hint">连接后端数据库后可查看实时统计数据</p>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const stats = ref({ courses: '-', orders: '-', users: '-' })
</script>

<style scoped>
.admin-dashboard { max-width: 900px; margin: 0 auto; }
.admin-dashboard h1 { margin-bottom: 16px; }
.admin-nav { display: flex; gap: 8px; margin-bottom: 30px; }
.admin-nav a {
  padding: 8px 18px; background: #f4f6fb; color: #555;
  border-radius: 6px; text-decoration: none; font-size: 14px;
}
.admin-nav a.router-link-exact-active { background: #4f46e5; color: #fff; }
.stats-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; }
.stat-card {
  background: #fff; padding: 30px; border-radius: 12px; text-align: center;
  box-shadow: 0 2px 8px rgba(0,0,0,0.06);
}
.stat-value { font-size: 36px; font-weight: 700; color: #4f46e5; }
.stat-label { font-size: 14px; color: #888; margin-top: 4px; }
.hint { text-align: center; color: #bbb; margin-top: 30px; font-size: 13px; }
</style>
