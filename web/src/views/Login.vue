<template>
  <div class="auth-page">
    <div class="auth-card">
      <h2>登录</h2>
      <form @submit.prevent="handleLogin">
        <div class="form-group">
          <label>用户名</label>
          <input v-model="form.username" type="text" required placeholder="请输入用户名" />
        </div>
        <div class="form-group">
          <label>密码</label>
          <input v-model="form.password" type="password" required placeholder="请输入密码" />
        </div>
        <p v-if="error" class="error">{{ error }}</p>
        <button type="submit" :disabled="loading" class="btn-submit">
          {{ loading ? '登录中...' : '登录' }}
        </button>
      </form>
      <p class="switch">还没有账号？<router-link to="/register">立即注册</router-link></p>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { login } from '@/api/auth'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const userStore = useUserStore()
const form = reactive({ username: '', password: '' })
const error = ref('')
const loading = ref(false)

async function handleLogin() {
  error.value = ''
  loading.value = true
  try {
    const res = await login(form)
    if (res.code === 200) {
      userStore.setAuth(res.data.token, res.data.user)
      router.push('/')
    }
  } catch (e) {
    error.value = e?.message || '登录失败，请检查用户名和密码'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.auth-page {
  display: flex;
  justify-content: center;
  padding-top: 60px;
}
.auth-card {
  width: 400px;
  background: #fff;
  padding: 40px;
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0,0,0,0.08);
}
.auth-card h2 {
  margin: 0 0 24px;
  text-align: center;
  color: #1a1a2e;
}
.form-group { margin-bottom: 18px; }
.form-group label {
  display: block;
  margin-bottom: 6px;
  font-size: 14px;
  color: #555;
}
.form-group input {
  width: 100%;
  padding: 10px 14px;
  border: 1px solid #ddd;
  border-radius: 8px;
  font-size: 14px;
  outline: none;
  box-sizing: border-box;
}
.form-group input:focus { border-color: #4f46e5; }
.error { color: #e74c3c; font-size: 13px; margin-bottom: 12px; }
.btn-submit {
  width: 100%;
  padding: 12px;
  background: #4f46e5;
  color: #fff;
  border: none;
  border-radius: 8px;
  font-size: 16px;
  cursor: pointer;
}
.btn-submit:hover { background: #4338ca; }
.btn-submit:disabled { opacity: 0.6; cursor: not-allowed; }
.switch { text-align: center; margin-top: 18px; font-size: 14px; color: #888; }
.switch a { color: #4f46e5; }
</style>
