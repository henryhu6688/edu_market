<template>
  <div class="auth-page">
    <div class="auth-card">
      <h2>注册</h2>
      <form @submit.prevent="handleRegister">
        <div class="form-group">
          <label>用户名</label>
          <input v-model="form.username" type="text" required minlength="3" placeholder="3-50位字符" />
        </div>
        <div class="form-group">
          <label>邮箱</label>
          <input v-model="form.email" type="email" required placeholder="请输入邮箱" />
        </div>
        <div class="form-group">
          <label>密码</label>
          <input v-model="form.password" type="password" required minlength="6" placeholder="至少6位密码" />
        </div>
        <p v-if="error" class="error">{{ error }}</p>
        <button type="submit" :disabled="loading" class="btn-submit">
          {{ loading ? '注册中...' : '注册' }}
        </button>
      </form>
      <p class="switch">已有账号？<router-link to="/login">立即登录</router-link></p>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { register } from '@/api/auth'

const router = useRouter()
const form = reactive({ username: '', email: '', password: '' })
const error = ref('')
const loading = ref(false)

async function handleRegister() {
  error.value = ''
  loading.value = true
  try {
    const res = await register(form)
    if (res.code === 201) {
      router.push('/login?registered=1')
    }
  } catch (e) {
    error.value = e?.message || '注册失败'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.auth-page { display: flex; justify-content: center; padding-top: 60px; }
.auth-card {
  width: 400px; background: #fff; padding: 40px;
  border-radius: 16px; box-shadow: 0 4px 20px rgba(0,0,0,0.08);
}
.auth-card h2 { margin: 0 0 24px; text-align: center; color: #1a1a2e; }
.form-group { margin-bottom: 18px; }
.form-group label { display: block; margin-bottom: 6px; font-size: 14px; color: #555; }
.form-group input {
  width: 100%; padding: 10px 14px; border: 1px solid #ddd;
  border-radius: 8px; font-size: 14px; outline: none; box-sizing: border-box;
}
.form-group input:focus { border-color: #4f46e5; }
.error { color: #e74c3c; font-size: 13px; margin-bottom: 12px; }
.btn-submit {
  width: 100%; padding: 12px; background: #4f46e5; color: #fff;
  border: none; border-radius: 8px; font-size: 16px; cursor: pointer;
}
.btn-submit:hover { background: #4338ca; }
.btn-submit:disabled { opacity: 0.6; cursor: not-allowed; }
.switch { text-align: center; margin-top: 18px; font-size: 14px; color: #888; }
.switch a { color: #4f46e5; }
</style>
