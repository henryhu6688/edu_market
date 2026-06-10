<template>
  <div class="agent-chat">
    <!-- 左侧会话列表 -->
    <aside class="sidebar">
      <div class="sidebar-header">
        <h3>AI 助手</h3>
        <button class="btn-new" @click="newChat">+ 新对话</button>
      </div>
      <div class="session-list">
        <div
          v-for="s in sessions"
          :key="s.id"
          class="session-item"
          :class="{ active: s.id === currentSessionId }"
          @click="switchSession(s.id)"
        >
          <span class="session-title">{{ s.title || '新对话' }}</span>
          <span class="session-tag">{{ agentTypeLabel(s.agent_type) }}</span>
          <button class="btn-del" @click.stop="removeSession(s.id)">&times;</button>
        </div>
        <div v-if="sessions.length === 0" class="empty-hint">暂无对话，点击上方按钮开始</div>
      </div>
    </aside>

    <!-- 右侧聊天区域 -->
    <main class="chat-area">
      <div v-if="currentSessionId" class="chat-header">
        <span class="agent-badge">{{ agentTypeLabel(currentAgentType) }}</span>
        <span>{{ currentTitle }}</span>
      </div>

      <div class="messages" ref="msgContainer">
        <div v-if="messages.length === 0 && !currentSessionId" class="welcome">
          <h2>有什么可以帮你的？</h2>
          <p>我可以帮你解答课程问题、推荐课程、处理订单咨询</p>
        </div>
        <div v-for="(msg, i) in messages" :key="i" class="msg" :class="msg.role">
          <div v-if="msg.role === 'thinking'" class="thinking-bubble">
            &#128269; {{ msg.content }}
          </div>
          <div v-else class="bubble" :class="msg.role">
            {{ msg.content }}
          </div>
        </div>
        <div v-if="loading" class="msg assistant">
          <div class="bubble assistant thinking-dots">
            <span class="dot">&#9679;</span><span class="dot">&#9679;</span><span class="dot">&#9679;</span>
          </div>
        </div>
      </div>

      <div class="input-area">
        <input
          v-model="input"
          @keyup.enter="send"
          placeholder="输入你的问题..."
          :disabled="loading"
          ref="inputBox"
        />
        <button @click="send" :disabled="loading || !input.trim()">发送</button>
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, nextTick, onMounted } from 'vue'
import { useUserStore } from '@/stores/user'
import { agentChat, getSessions, deleteSession, getMessages } from '@/api/agent'

const userStore = useUserStore()

const sessions = ref([])
const currentSessionId = ref(null)
const currentAgentType = ref('')
const currentTitle = ref('')
const messages = ref([])
const input = ref('')
const loading = ref(false)
const msgContainer = ref(null)
const inputBox = ref(null)

const agentTypeLabel = (t) => ({
  customer_service: '客服',
  course_recommend: '推荐',
  qa: '答疑'
}[t] || t || '客服')

async function loadSessions() {
  try {
    const res = await getSessions({ page: 1, page_size: 50 })
    sessions.value = res.data.list || []
  } catch (e) {
    console.error('加载会话列表失败', e)
  }
}

async function loadMessages(sessionId) {
  try {
    const res = await getMessages(sessionId, { page: 1, page_size: 100 })
    messages.value = (res.data.list || []).map(m => ({
      role: m.role,
      content: m.content
    }))
    scrollToBottom()
  } catch (e) {
    console.error('加载消息失败', e)
  }
}

function newChat() {
  currentSessionId.value = null
  currentAgentType.value = ''
  currentTitle.value = ''
  messages.value = []
  input.value = ''
  nextTick(() => inputBox.value?.focus())
}

function switchSession(id) {
  currentSessionId.value = id
  const s = sessions.value.find(s => s.id === id)
  if (s) {
    currentAgentType.value = s.agent_type
    currentTitle.value = s.title
  }
  loadMessages(id)
}

async function removeSession(id) {
  try {
    await deleteSession(id)
    sessions.value = sessions.value.filter(s => s.id !== id)
    if (currentSessionId.value === id) newChat()
  } catch (e) {
    console.error('删除会话失败', e)
  }
}

async function send() {
  if (!input.value.trim() || loading.value) return
  const question = input.value.trim()
  input.value = ''
  loading.value = true

  // 用户消息立即显示
  messages.value.push({ role: 'user', content: question })
  scrollToBottom()

  let currentAssistantIdx = -1

  try {
    const resp = await agentChat({
      session_id: currentSessionId.value || undefined,
      question
    }, userStore.accessToken)

    if (!resp.ok) {
      messages.value.push({ role: 'assistant', content: `请求失败 (${resp.status})` })
      loading.value = false
      return
    }

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let currentEvent = ''
    let streamEnded = false
    const streamStart = Date.now()

    while (!streamEnded) {
      // 30 秒超时保护，防止流卡住
      let readResult
      try {
        readResult = await Promise.race([
          reader.read(),
          new Promise((_, reject) => setTimeout(() => reject(new Error('STREAM_TIMEOUT')), 30000))
        ])
      } catch (raceErr) {
        if (raceErr.message === 'STREAM_TIMEOUT') {
          console.log('SSE stream timeout, forcing end')
        }
        break
      }
      const { done, value } = readResult
      if (done) { console.log('SSE stream closed by server'); break }

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i]
        if (line.startsWith('event: ')) {
          currentEvent = line.slice(7).trim()
          continue
        }
        if (line.startsWith('data: ')) {
          const payload = line.slice(6)

          if (currentEvent === 'thinking') {
            try {
              const d = JSON.parse(payload)
              const thinkIdx = messages.value.length
              messages.value.push({ role: 'thinking', content: `正在查询: ${d.tool || ''}` })
              setTimeout(() => {
                if (messages.value[thinkIdx]?.role === 'thinking') {
                  messages.value.splice(thinkIdx, 1)
                }
              }, 3000)
            } catch {}
          } else if (currentEvent === 'delta') {
            try {
              const d = JSON.parse(payload)
              if (currentAssistantIdx === -1) {
                messages.value.push({ role: 'assistant', content: '' })
                currentAssistantIdx = messages.value.length - 1
              }
              messages.value[currentAssistantIdx].content += d.content
            } catch {}
          } else if (currentEvent === 'done') {
            console.log('SSE received done:', payload)
            streamEnded = true
            try {
              const d = JSON.parse(payload)
              if (d.session_id && !currentSessionId.value) {
                currentSessionId.value = d.session_id
                currentAgentType.value = d.agent_type
                loadSessions()
              }
            } catch {}
          } else if (currentEvent === 'error') {
            streamEnded = true
            try {
              const d = JSON.parse(payload)
              messages.value.push({ role: 'assistant', content: d.message || '发生错误，请重试' })
            } catch {
              messages.value.push({ role: 'assistant', content: '发生错误，请重试' })
            }
          }
          scrollToBottom()
        }
      }

      // 收到 done/error 后主动取消 stream，释放连接
      if (streamEnded) {
        reader.cancel()
      }
    }
  } catch (e) {
    messages.value.push({ role: 'assistant', content: `连接失败: ${e.message}` })
  } finally {
    // 清理 thinking 消息
    messages.value = messages.value.filter(m => m.role !== 'thinking')
    currentAssistantIdx = -1
    loading.value = false
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (msgContainer.value) {
      msgContainer.value.scrollTop = msgContainer.value.scrollHeight
    }
  })
}

onMounted(() => {
  loadSessions()
})
</script>

<style scoped>
.agent-chat {
  display: flex;
  height: calc(100vh - 60px);
  max-width: 1200px;
  margin: 0 auto;
}

.sidebar {
  width: 260px;
  border-right: 1px solid #e5e7eb;
  display: flex;
  flex-direction: column;
  background: #f9fafb;
  flex-shrink: 0;
}

.sidebar-header {
  padding: 16px;
  border-bottom: 1px solid #e5e7eb;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.sidebar-header h3 { margin: 0; font-size: 16px; }

.btn-new {
  padding: 4px 10px;
  border: 1px solid #3b82f6;
  background: #fff;
  color: #3b82f6;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  white-space: nowrap;
}
.btn-new:hover { background: #eff6ff; }

.session-list { flex: 1; overflow-y: auto; }

.empty-hint {
  padding: 20px 16px;
  color: #9ca3af;
  font-size: 13px;
  text-align: center;
}

.session-item {
  padding: 10px 16px;
  border-bottom: 1px solid #f3f4f6;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
}
.session-item:hover { background: #f3f4f6; }
.session-item.active { background: #eff6ff; }

.session-title {
  flex: 1;
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-tag {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 4px;
  background: #e5e7eb;
  color: #6b7280;
  white-space: nowrap;
}

.btn-del {
  background: none;
  border: none;
  font-size: 18px;
  color: #9ca3af;
  cursor: pointer;
  padding: 0 4px;
  line-height: 1;
}
.btn-del:hover { color: #ef4444; }

.chat-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.chat-header {
  padding: 12px 20px;
  border-bottom: 1px solid #e5e7eb;
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 15px;
}

.agent-badge {
  padding: 2px 10px;
  border-radius: 12px;
  background: #dbeafe;
  color: #1d4ed8;
  font-size: 12px;
  white-space: nowrap;
}

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
}

.welcome {
  text-align: center;
  padding-top: 80px;
  color: #6b7280;
}
.welcome h2 { margin-bottom: 8px; color: #374151; }

.msg { margin-bottom: 16px; display: flex; }
.msg.user { justify-content: flex-end; }
.msg.assistant { justify-content: flex-start; }

.bubble {
  max-width: 75%;
  padding: 10px 16px;
  border-radius: 12px;
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}

.msg.user .bubble {
  background: #3b82f6;
  color: #fff;
  border-bottom-right-radius: 4px;
}

.msg.assistant .bubble {
  background: #f3f4f6;
  color: #1f2937;
  border-bottom-left-radius: 4px;
}

.thinking-bubble {
  background: #fef3c7;
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 13px;
  color: #92400e;
  animation: fadeIn 0.3s ease;
}

.thinking-dots .dot {
  animation: blink 1.4s infinite both;
  margin: 0 2px;
  color: #9ca3af;
}
.thinking-dots .dot:nth-child(2) { animation-delay: 0.2s; }
.thinking-dots .dot:nth-child(3) { animation-delay: 0.4s; }

@keyframes blink {
  0% { opacity: 0.2; }
  20% { opacity: 1; }
  100% { opacity: 0.2; }
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

.input-area {
  padding: 16px 20px;
  border-top: 1px solid #e5e7eb;
  display: flex;
  gap: 10px;
}

.input-area input {
  flex: 1;
  padding: 10px 16px;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  font-size: 14px;
  outline: none;
}
.input-area input:focus { border-color: #3b82f6; }

.input-area button {
  padding: 10px 24px;
  background: #3b82f6;
  color: #fff;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
}
.input-area button:hover { background: #2563eb; }
.input-area button:disabled { background: #9ca3af; cursor: not-allowed; }
</style>
