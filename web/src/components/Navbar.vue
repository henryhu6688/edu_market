<template>
  <nav class="navbar">
    <div class="navbar-inner">
      <router-link to="/" class="logo">🎓 EduMarket</router-link>
      <div class="nav-links">
        <router-link to="/">首页</router-link>
        <router-link v-if="userStore.isLoggedIn" to="/agent">🤖 AI 助手</router-link>
        <template v-if="userStore.isLoggedIn">
          <router-link to="/orders">我的订单</router-link>
          <router-link to="/profile">个人中心</router-link>
          <router-link v-if="userStore.isAdmin" to="/admin">管理后台</router-link>
          <a href="#" @click.prevent="handleLogout">退出</a>
        </template>
        <template v-else>
          <router-link to="/login" class="btn-register">登录 / 注册</router-link>
        </template>
      </div>
    </div>
  </nav>
</template>

<script setup>
import { useUserStore } from '@/stores/user'
import { useRouter } from 'vue-router'

const userStore = useUserStore()
const router = useRouter()

function handleLogout() {
  userStore.logout()
  router.push('/')
}
</script>

<style scoped>
.navbar {
  background: #fff;
  box-shadow: 0 2px 12px rgba(0,0,0,0.08);
  position: sticky;
  top: 0;
  z-index: 100;
}
.navbar-inner {
  max-width: 1200px;
  margin: 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 60px;
}
.logo {
  font-size: 20px;
  font-weight: 700;
  color: #4f46e5;
  text-decoration: none;
}
.nav-links {
  display: flex;
  align-items: center;
  gap: 8px;
}
.nav-links a {
  color: #333;
  text-decoration: none;
  padding: 8px 14px;
  border-radius: 6px;
  font-size: 14px;
  transition: all 0.2s;
}
.nav-links a:hover,
.nav-links a.router-link-exact-active {
  background: #eef2ff;
  color: #4f46e5;
}
.btn-register {
  background: #4f46e5 !important;
  color: #fff !important;
}
.btn-register:hover {
  background: #4338ca !important;
}
</style>
