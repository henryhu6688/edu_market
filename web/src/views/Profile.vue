<template>
  <div class="profile-page">
    <div class="profile-card">
      <h2>个人中心</h2>
      <div class="avatar-section">
        <div class="avatar">{{ userStore.user?.username?.charAt(0)?.toUpperCase() }}</div>
        <span class="role-badge">{{ userStore.user?.role === 'admin' ? '管理员' : '学生' }}</span>
      </div>
      <form @submit.prevent="handleUpdate">
        <div class="form-group">
          <label>用户名</label>
          <input v-model="form.username" type="text" minlength="3" />
        </div>
        <div class="form-group">
          <label>邮箱</label>
          <input v-model="form.email" type="email" />
        </div>
        <p v-if="msg" class="msg">{{ msg }}</p>
        <button type="submit" class="btn-save">保存修改</button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { reactive, ref, onMounted } from 'vue'
import { useUserStore } from '@/stores/user'
import api from '@/api/index'

const userStore = useUserStore()
const form = reactive({ username: '', email: '' })
const msg = ref('')

onMounted(async () => {
  try {
    const res = await api.get('/user/profile')
    if (res.code === 200) {
      form.username = res.data.username
      form.email = res.data.email
    }
  } catch (e) { /* 静默 */ }
})

async function handleUpdate() {
  try {
    const res = await api.put('/user/profile', form)
    if (res.code === 200) {
      userStore.updateUser(res.data)
      msg.value = '保存成功'
    }
  } catch (e) { msg.value = e?.message || '保存失败' }
}
</script>

<style scoped>
.profile-page { display: flex; justify-content: center; padding-top: 40px; }
.profile-card {
  width: 480px; background: #fff; padding: 40px;
  border-radius: 16px; box-shadow: 0 4px 20px rgba(0,0,0,0.08);
}
.profile-card h2 { margin: 0 0 24px; text-align: center; }
.avatar-section { display: flex; align-items: center; gap: 12px; margin-bottom: 24px; justify-content: center; }
.avatar {
  width: 64px; height: 64px; background: #4f46e5; color: #fff;
  border-radius: 50%; display: flex; align-items: center; justify-content: center;
  font-size: 24px; font-weight: 700;
}
.role-badge { font-size: 12px; background: #eef2ff; color: #4f46e5; padding: 4px 12px; border-radius: 20px; }
.form-group { margin-bottom: 16px; }
.form-group label { display: block; margin-bottom: 6px; font-size: 14px; color: #555; }
.form-group input {
  width: 100%; padding: 10px 14px; border: 1px solid #ddd;
  border-radius: 8px; font-size: 14px; outline: none; box-sizing: border-box;
}
.form-group input:focus { border-color: #4f46e5; }
.msg { color: #22c55e; font-size: 13px; margin-bottom: 12px; }
.btn-save {
  width: 100%; padding: 12px; background: #4f46e5; color: #fff;
  border: none; border-radius: 8px; font-size: 16px; cursor: pointer;
}
.btn-save:hover { background: #4338ca; }
</style>
