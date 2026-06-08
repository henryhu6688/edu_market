<template>
  <div class="ai-chat-page">
    <div class="chat-container">
      <div class="chat-header">
        <h2>🤖 AI 智能答疑</h2>
        <span class="model-tag">Deepseek V4 Pro</span>
      </div>

      <!-- 对话区域 -->
      <div class="chat-messages" ref="msgContainer">
        <div v-if="messages.length === 0" class="welcome">
          <p>👋 欢迎使用 AI 答疑，有任何学习问题都可以问我！</p>
        </div>
        <div v-for="(msg, i) in messages" :key="i" class="message" :class="msg.role">
          <div class="msg-avatar">{{ msg.role === 'user' ? '我' : 'AI' }}</div>
          <div class="msg-content" v-text="msg.content"></div>
        </div>
        <div v-if="loading" class="message assistant">
          <div class="msg-avatar">AI</div>
          <div class="msg-content typing">思考中...</div>
        </div>
      </div>

      <!-- 输入区 -->
      <div class="chat-input">
        <textarea v-model="question" @keydown.enter.exact.prevent="send" placeholder="输入你的问题，回车发送..." rows="2"></textarea>
        <button @click="send" :disabled="loading || !question.trim()">发送</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, nextTick, onMounted } from 'vue'
import { chat, getHistory } from '@/api/ai'

const messages = ref([])
const question = ref('')
const loading = ref(false)
const msgContainer = ref(null)

function scrollBottom() {
  nextTick(() => {
    if (msgContainer.value) {
      msgContainer.value.scrollTop = msgContainer.value.scrollHeight
    }
  })
}

async function send() {
  const q = question.value.trim()
  if (!q || loading.value) return
  messages.value.push({ role: 'user', content: q })
  question.value = ''
  scrollBottom()
  loading.value = true
  try {
    const res = await chat({ question: q })
    if (res.code === 200) {
      messages.value.push({ role: 'assistant', content: res.data.answer })
    }
  } catch (e) {
    messages.value.push({ role: 'assistant', content: '抱歉，AI 服务暂时不可用: ' + (e?.message || '未知错误') })
  } finally {
    loading.value = false
    scrollBottom()
  }
}

onMounted(async () => {
  try {
    const res = await getHistory({ page: 1, page_size: 20 })
    if (res.code === 200) {
      const list = res.data.list || []
      list.reverse().forEach(item => {
        messages.value.push({ role: 'user', content: item.question })
        messages.value.push({ role: 'assistant', content: item.answer })
      })
      scrollBottom()
    }
  } catch (e) { /* 静默 */ }
})
</script>

<style scoped>
.ai-chat-page { max-width: 750px; margin: 0 auto; }
.chat-container {
  background: #fff; border-radius: 16px; box-shadow: 0 4px 20px rgba(0,0,0,0.08);
  display: flex; flex-direction: column; height: calc(100vh - 120px);
}
.chat-header {
  padding: 16px 20px; border-bottom: 1px solid #f0f0f0;
  display: flex; align-items: center; gap: 10px;
}
.chat-header h2 { margin: 0; font-size: 18px; }
.model-tag { font-size: 11px; background: #eef2ff; color: #4f46e5; padding: 2px 8px; border-radius: 4px; }
.chat-messages {
  flex: 1; overflow-y: auto; padding: 20px; display: flex; flex-direction: column; gap: 14px;
}
.welcome { text-align: center; color: #999; padding: 40px 0; }
.message { display: flex; gap: 10px; max-width: 80%; }
.message.user { align-self: flex-end; flex-direction: row-reverse; }
.message.assistant { align-self: flex-start; }
.msg-avatar {
  width: 32px; height: 32px; border-radius: 50%; display: flex;
  align-items: center; justify-content: center; font-size: 12px; font-weight: 700;
  flex-shrink: 0;
}
.message.user .msg-avatar { background: #4f46e5; color: #fff; }
.message.assistant .msg-avatar { background: #f0f0ff; color: #4f46e5; }
.msg-content {
  padding: 10px 14px; border-radius: 12px; font-size: 14px; line-height: 1.6; white-space: pre-wrap;
}
.message.user .msg-content { background: #4f46e5; color: #fff; }
.message.assistant .msg-content { background: #f4f6fb; color: #333; }
.typing { opacity: 0.6; }
.chat-input {
  padding: 14px 20px; border-top: 1px solid #f0f0f0; display: flex; gap: 10px;
}
.chat-input textarea {
  flex: 1; padding: 10px 14px; border: 1px solid #ddd;
  border-radius: 8px; font-size: 14px; outline: none; resize: none; font-family: inherit;
}
.chat-input textarea:focus { border-color: #4f46e5; }
.chat-input button {
  padding: 10px 24px; background: #4f46e5; color: #fff;
  border: none; border-radius: 8px; cursor: pointer; font-size: 14px; white-space: nowrap;
}
.chat-input button:hover { background: #4338ca; }
.chat-input button:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
