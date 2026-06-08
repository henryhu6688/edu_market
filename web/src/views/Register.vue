<template>
  <div class="auth-page">
    <div class="auth-card">
      <h2>注册</h2>
      <!-- 切换 tab -->
      <div class="tabs">
        <button :class="{ active: mode === 'user' }" @click="mode = 'user'">用户名注册</button>
        <button :class="{ active: mode === 'phone' }" @click="mode = 'phone'">手机号注册</button>
      </div>

      <!-- 用户名注册 -->
      <form v-if="mode === 'user'" @submit.prevent="handleRegister">
        <div class="form-group">
          <label>用户名</label>
          <input v-model="userForm.username" type="text" required minlength="3" placeholder="3-50位字符" />
        </div>
        <div class="form-group">
          <label>邮箱</label>
          <input v-model="userForm.email" type="email" required placeholder="请输入邮箱" />
        </div>
        <div class="form-group">
          <label>密码</label>
          <input v-model="userForm.password" type="password" required minlength="6" placeholder="至少6位密码" />
        </div>
        <p v-if="error" class="error">{{ error }}</p>
        <button type="submit" :disabled="loading" class="btn-submit">
          {{ loading ? '注册中...' : '注册' }}
        </button>
      </form>

      <!-- 手机号注册 -->
      <form v-else @submit.prevent="handlePhoneRegister">
        <div class="form-group">
          <label>手机号</label>
          <input v-model="phoneForm.phone" type="text" required maxlength="11" placeholder="请输入11位手机号" />
        </div>
        <div class="form-group code-row">
          <label>验证码</label>
          <div class="code-input">
            <input v-model="phoneForm.code" type="text" required maxlength="6" placeholder="6位验证码" />
            <button type="button" class="btn-code" :disabled="codeCooldown > 0" @click="sendVerificationCode">
              {{ codeCooldown > 0 ? codeCooldown + 's' : '发送验证码' }}
            </button>
          </div>
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
import { register, registerByPhone, sendCode } from '@/api/auth'

const router = useRouter()
const mode = ref('user')
const error = ref('')
const loading = ref(false)
const codeCooldown = ref(0)

const userForm = reactive({ username: '', email: '', password: '' })
const phoneForm = reactive({ phone: '', code: '' })

// 用户名注册
async function handleRegister() {
  error.value = ''
  loading.value = true
  try {
    const res = await register(userForm)
    if (res.code === 201) {
      router.push('/login?registered=1')
    }
  } catch (e) {
    error.value = e?.message || '注册失败'
  } finally {
    loading.value = false
  }
}

// 发送验证码
async function sendVerificationCode() {
  error.value = ''
  if (!phoneForm.phone || phoneForm.phone.length !== 11) {
    error.value = '请输入正确的11位手机号'
    return
  }
  try {
    await sendCode(phoneForm.phone)
    codeCooldown.value = 60
    const timer = setInterval(() => {
      codeCooldown.value--
      if (codeCooldown.value <= 0) clearInterval(timer)
    }, 1000)
  } catch (e) {
    error.value = e?.message || '发送失败'
  }
}

// 手机号注册
async function handlePhoneRegister() {
  error.value = ''
  loading.value = true
  try {
    const res = await registerByPhone(phoneForm)
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
  width: 420px; background: #fff; padding: 40px;
  border-radius: 16px; box-shadow: 0 4px 20px rgba(0,0,0,0.08);
}
.auth-card h2 { margin: 0 0 20px; text-align: center; color: #1a1a2e; }

.tabs { display: flex; gap: 0; margin-bottom: 24px; border-radius: 8px; overflow: hidden; border: 1px solid #ddd; }
.tabs button {
  flex: 1; padding: 10px; border: none; background: #f5f5f5; font-size: 14px; cursor: pointer; color: #666;
}
.tabs button.active { background: #4f46e5; color: #fff; }

.form-group { margin-bottom: 18px; }
.form-group label { display: block; margin-bottom: 6px; font-size: 14px; color: #555; }
.form-group input {
  width: 100%; padding: 10px 14px; border: 1px solid #ddd;
  border-radius: 8px; font-size: 14px; outline: none; box-sizing: border-box;
}
.form-group input:focus { border-color: #4f46e5; }

.code-row .code-input { display: flex; gap: 10px; }
.code-row .code-input input { flex: 1; }
.btn-code {
  white-space: nowrap; padding: 10px 16px; background: #fff; color: #4f46e5;
  border: 1px solid #4f46e5; border-radius: 8px; font-size: 13px; cursor: pointer;
}
.btn-code:disabled { color: #999; border-color: #ddd; cursor: not-allowed; }

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
