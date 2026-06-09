<template>
  <div class="auth-page">
    <div class="auth-card">
      <h2>登录 / 注册</h2>
      <p class="subtitle">新用户自动注册，老用户直接登录</p>

      <form @submit.prevent="handleLogin">
        <!-- 手机号 -->
        <div class="form-group">
          <label>手机号</label>
          <input v-model="phone" type="text" required maxlength="11" placeholder="请输入11位手机号" />
        </div>

        <!-- 验证码 -->
        <div class="form-group code-row">
          <label>验证码</label>
          <div class="code-input">
            <input v-model="code" type="text" required maxlength="6" placeholder="6位验证码" />
            <button type="button" class="btn-code" :disabled="codeCooldown > 0 || sending" @click="sendVerificationCode">
              {{ codeCooldown > 0 ? codeCooldown + 's' : sending ? '发送中...' : '发送验证码' }}
            </button>
          </div>
        </div>

        <!-- 图形验证码弹窗 -->
        <div v-if="showCaptcha" class="captcha-overlay">
          <div class="captcha-box">
            <h4>请输入图形验证码</h4>
            <img :src="captchaImage" alt="图形验证码" class="captcha-img" @click="loadCaptcha" />
            <p class="captcha-hint">点击图片刷新</p>
            <input v-model="captchaCode" type="text" maxlength="4" placeholder="4位验证码" class="captcha-input" @keyup.enter="confirmCaptcha" />
            <div class="captcha-btns">
              <button type="button" class="btn-cancel" @click="showCaptcha = false">取消</button>
              <button type="button" class="btn-confirm" :disabled="!captchaCode" @click="confirmCaptcha">确认发送</button>
            </div>
          </div>
        </div>

        <p v-if="error" class="error">{{ error }}</p>
        <button type="submit" :disabled="loading" class="btn-submit">
          {{ loading ? '登录中...' : '登录 / 注册' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { loginByCode, sendCode, getCaptcha } from '@/api/auth'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const userStore = useUserStore()

const phone = ref('')
const code = ref('')
const error = ref('')
const loading = ref(false)
const sending = ref(false)
const codeCooldown = ref(0)

// 图形验证码
const showCaptcha = ref(false)
const captchaId = ref('')
const captchaImage = ref('')
const captchaCode = ref('')
let pendingCaptchaResolve = null

// 加载图形验证码
async function loadCaptcha() {
  try {
    const res = await getCaptcha()
    captchaId.value = res.data.captcha_id
    captchaImage.value = res.data.captcha_image
  } catch (e) {
    error.value = '获取图形验证码失败'
    showCaptcha.value = false
  }
}

// 点击"发送验证码"
function sendVerificationCode() {
  error.value = ''
  if (!phone.value || phone.value.length !== 11) {
    error.value = '请输入正确的11位手机号'
    return
  }

  // 弹出图形验证码
  captchaCode.value = ''
  loadCaptcha()
  showCaptcha.value = true
}

// 确认图形验证码 → 发短信
async function confirmCaptcha() {
  if (!captchaCode.value) return
  showCaptcha.value = false
  sending.value = true
  try {
    await sendCode({
      phone: phone.value,
      captcha_id: captchaId.value,
      captcha_code: captchaCode.value
    })
    // 倒计时
    codeCooldown.value = 60
    const timer = setInterval(() => {
      codeCooldown.value--
      if (codeCooldown.value <= 0) clearInterval(timer)
    }, 1000)
  } catch (e) {
    error.value = e?.message || '发送失败'
  } finally {
    sending.value = false
  }
}

// 登录/注册
async function handleLogin() {
  error.value = ''
  if (!code.value) {
    error.value = '请输入验证码'
    return
  }
  loading.value = true
  try {
    const res = await loginByCode({ phone: phone.value, code: code.value })
    if (res.code === 200) {
      userStore.setAuth(
        res.data.access_token,
        res.data.refresh_token,
        res.data.user
      )
      router.push('/')
    }
  } catch (e) {
    error.value = e?.message || '登录失败'
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
  position: relative;
}
.auth-card h2 { margin: 0 0 8px; text-align: center; color: #1a1a2e; }
.subtitle { text-align: center; color: #999; font-size: 13px; margin: 0 0 24px; }

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

/* 图形验证码弹窗 */
.captcha-overlay {
  position: fixed; top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0,0,0,0.4); display: flex; align-items: center; justify-content: center; z-index: 1000;
}
.captcha-box {
  background: #fff; padding: 30px; border-radius: 12px; width: 320px; text-align: center;
}
.captcha-box h4 { margin: 0 0 16px; color: #333; }
.captcha-img { border: 1px solid #ddd; border-radius: 6px; cursor: pointer; max-width: 100%; height: auto; }
.captcha-hint { font-size: 12px; color: #999; margin: 4px 0 16px; }
.captcha-input {
  width: 100%; padding: 10px; border: 1px solid #ddd; border-radius: 8px;
  font-size: 18px; text-align: center; letter-spacing: 8px; outline: none; box-sizing: border-box;
}
.captcha-input:focus { border-color: #4f46e5; }
.captcha-btns { display: flex; gap: 10px; margin-top: 16px; }
.btn-cancel {
  flex: 1; padding: 10px; border: 1px solid #ddd; background: #fff; border-radius: 8px; cursor: pointer; color: #666;
}
.btn-confirm {
  flex: 2; padding: 10px; border: none; background: #4f46e5; color: #fff; border-radius: 8px; cursor: pointer;
}
.btn-confirm:disabled { opacity: 0.5; cursor: not-allowed; }

.switch { text-align: center; margin-top: 18px; font-size: 14px; color: #888; }
.switch a { color: #4f46e5; }
</style>
