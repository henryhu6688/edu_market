<template>
  <nav class="sticky top-0 z-50 bg-white border-b border-[var(--color-border)]">
    <div class="max-w-[1200px] mx-auto flex items-center justify-between px-5 h-[60px]">
      <router-link to="/" class="text-xl font-bold text-[var(--color-primary)] no-underline flex items-center gap-2">
        <GraduationCap :size="24" />
        EduMarket
      </router-link>
      <div class="flex items-center gap-2">
        <router-link to="/" class="nav-link text-[var(--color-foreground)] no-underline px-3.5 py-2 rounded-[var(--radius-btn)] text-sm transition-all duration-200 hover:text-[var(--color-primary)] hover:bg-teal-50">首页</router-link>
        <router-link to="/materials" class="nav-link text-[var(--color-foreground)] no-underline px-3.5 py-2 rounded-[var(--radius-btn)] text-sm transition-all duration-200 hover:text-[var(--color-primary)] hover:bg-teal-50">
          <BookOpen :size="16" class="inline mr-0.5" />学习资料
        </router-link>
        <router-link v-if="userStore.isLoggedIn" to="/agent" class="nav-link text-[var(--color-foreground)] no-underline px-3.5 py-2 rounded-[var(--radius-btn)] text-sm transition-all duration-200 hover:text-[var(--color-primary)] hover:bg-teal-50">
          <Sparkles :size="16" class="inline mr-0.5" />AI 助手
        </router-link>
        <template v-if="userStore.isLoggedIn">
          <router-link to="/orders" class="nav-link text-[var(--color-foreground)] no-underline px-3.5 py-2 rounded-[var(--radius-btn)] text-sm transition-all duration-200 hover:text-[var(--color-primary)] hover:bg-teal-50">我的订单</router-link>
          <router-link to="/profile" class="nav-link text-[var(--color-foreground)] no-underline px-3.5 py-2 rounded-[var(--radius-btn)] text-sm transition-all duration-200 hover:text-[var(--color-primary)] hover:bg-teal-50">个人中心</router-link>
          <router-link v-if="userStore.isAdmin" to="/admin" class="nav-link text-[var(--color-foreground)] no-underline px-3.5 py-2 rounded-[var(--radius-btn)] text-sm transition-all duration-200 hover:text-[var(--color-primary)] hover:bg-teal-50">管理后台</router-link>
          <a href="#" @click.prevent="handleLogout" class="nav-link text-[var(--color-foreground)] no-underline px-3.5 py-2 rounded-[var(--radius-btn)] text-sm transition-all duration-200 hover:text-[var(--color-primary)] hover:bg-teal-50 cursor-pointer">退出</a>
        </template>
        <template v-else>
          <router-link to="/login" class="px-4 py-2 bg-[var(--color-primary)] text-white rounded-[var(--radius-btn)] text-sm font-medium no-underline hover:brightness-90 transition-all">登录 / 注册</router-link>
        </template>
      </div>
    </div>
  </nav>
</template>

<script setup>
import { useUserStore } from '@/stores/user'
import { useRouter } from 'vue-router'
import { GraduationCap, BookOpen, Sparkles } from 'lucide-vue-next'

const userStore = useUserStore()
const router = useRouter()

function handleLogout() {
  userStore.logout()
  router.push('/')
}
</script>

