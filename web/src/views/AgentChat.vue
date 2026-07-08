<template>
  <div class="flex h-[calc(100vh-60px)] max-w-[1200px] mx-auto">
    <!-- 侧边栏 -->
    <aside
      :class="[
        'flex flex-col bg-[var(--color-background)] border-r border-[var(--color-border)] flex-shrink-0 transition-all duration-300',
        sidebarCollapsed ? 'w-[48px]' : 'w-[260px]'
      ]"
    >
      <div v-if="!sidebarCollapsed" class="p-4 border-b border-[var(--color-border)] flex justify-between items-center">
        <h3 class="m-0 text-base font-semibold">AI 对话</h3>
        <div class="flex gap-1">
          <button @click="newChat" class="px-3 py-1 text-xs border border-[var(--color-primary)] bg-white text-[var(--color-primary)] rounded-[var(--radius-btn)] cursor-pointer hover:bg-teal-50 transition-all">
            + 新对话
          </button>
          <button @click="sidebarCollapsed = true" aria-label="折叠侧边栏" class="p-1 border-none bg-transparent cursor-pointer text-[var(--color-muted)] hover:text-[var(--color-foreground)]">
            <PanelLeftClose :size="16" />
          </button>
        </div>
      </div>
      <button
        v-else
        @click="sidebarCollapsed = false"
        aria-label="展开侧边栏"
        class="p-3 border-none bg-transparent cursor-pointer text-[var(--color-muted)] hover:text-[var(--color-primary)] transition-colors"
      >
        <PanelLeft :size="20" />
      </button>

      <div v-if="!sidebarCollapsed" class="flex-1 overflow-y-auto">
        <div v-if="sessions.length === 0" class="p-4 text-[13px] text-[var(--color-muted)] text-center">
          暂无对话，点击上方按钮开始
        </div>
        <div
          v-for="s in sessions"
          :key="s.id"
          @click="switchSession(s.id)"
          :class="[
            'flex items-center gap-2 px-4 py-3 border-b border-[var(--color-border)] cursor-pointer text-sm hover:bg-teal-50/50 transition-colors',
            s.id === currentSessionId ? 'bg-teal-50 border-l-[3px] border-l-[var(--color-primary)]' : 'border-l-[3px] border-l-transparent'
          ]"
        >
          <span class="flex-1 truncate">{{ s.title || '新对话' }}</span>
          <button @click.stop="removeSession(s.id)" aria-label="删除对话" class="border-none bg-transparent text-lg text-[var(--color-muted)] cursor-pointer p-0 leading-none hover:text-[var(--color-destructive)] transition-colors">&times;</button>
        </div>
      </div>
    </aside>

    <!-- 对话区 -->
    <main class="flex-1 flex flex-col min-w-0">
      <!-- 顶栏 -->
      <div class="flex items-center gap-2 px-4 py-2.5 border-b border-[var(--color-border)] text-sm">
        <button v-if="sidebarCollapsed" @click="sidebarCollapsed = false" aria-label="展开侧边栏" class="border-none bg-transparent cursor-pointer text-[var(--color-muted)] p-1">
          <PanelLeft :size="16" />
        </button>
        <span class="font-semibold text-[var(--color-foreground)]">{{ currentSessionTitle || '新对话' }}</span>
        <span v-if="currentAgentType" class="ml-auto px-2.5 py-0.5 text-[11px] rounded-[var(--radius-pill)] bg-teal-50 text-[var(--color-primary)] font-medium">
          {{ currentAgentType === 'shopping' ? '购物模式' : currentAgentType === 'tutoring' ? '辅导模式' : currentAgentType }}
        </span>
      </div>

      <!-- 消息区 -->
      <div class="flex-1 overflow-y-auto p-5" ref="msgContainer">
        <!-- 欢迎 -->
        <div v-if="messages.length === 0 && !currentSessionId" class="text-center pt-20 text-[var(--color-muted)]">
          <MessageCircle :size="48" class="mx-auto mb-4 opacity-30" />
          <h2 class="text-lg font-semibold text-[var(--color-foreground)] mb-2">有什么可以帮你的？</h2>
          <p class="text-sm">我可以帮你解答课程问题、推荐资料、处理订单咨询</p>
        </div>

        <!-- 消息列表 -->
        <div v-for="(msg, i) in messages" :key="i" :class="['flex mb-4', msg.role === 'user' ? 'justify-end' : 'justify-start']">
          <!-- thinking 气泡 -->
          <div v-if="msg.role === 'thinking'" class="bg-[var(--color-thinking-bg)] text-[var(--color-thinking-fg)] px-3 py-2 rounded-lg text-[13px] animate-[fadeIn_0.3s_ease] max-w-[75%]">
            &#128269; {{ msg.content }}
          </div>

          <!-- action 资料推荐卡 (signature，用 purchase_offer 数据) -->
          <div v-else-if="msg.role === 'action' && msg.action?.type === 'purchase'" class="bg-[var(--color-card)] border border-[var(--color-border)] rounded-xl overflow-hidden max-w-[75%] w-[300px] shadow-sm">
            <div
              @click.stop="$router.push(`/materials/${msg.action.material_id}`)"
              class="flex gap-3 p-3 cursor-pointer hover:bg-[var(--color-primary)]/5 transition-colors"
            >
              <!-- 封面 -->
              <img
                v-if="msg.action.cover_image"
                :src="msg.action.cover_image"
                :alt="msg.action.title"
                class="w-16 h-16 rounded-lg object-cover flex-shrink-0"
              />
              <div
                v-else
                class="w-16 h-16 rounded-lg bg-gradient-to-br from-[var(--color-primary)] to-[var(--color-primary)]/60 flex items-center justify-center flex-shrink-0"
              >
                <span class="text-[var(--color-primary-foreground)] text-2xl font-bold">{{ (msg.action.title || '?').charAt(0) }}</span>
              </div>
              <!-- 信息 -->
              <div class="min-w-0 flex flex-col justify-center">
                <div class="text-[11px] text-[var(--color-muted)] mb-0.5">为你推荐</div>
                <div class="text-sm font-bold text-[var(--color-foreground)] leading-snug line-clamp-2">{{ msg.action.title }}</div>
                <div class="text-lg font-bold text-[var(--color-accent)] mt-0.5">¥{{ msg.action.price }}</div>
              </div>
            </div>
            <router-link
              to="/orders"
              class="block px-4 py-2.5 bg-[var(--color-primary)] text-[var(--color-primary-foreground)] text-center text-sm font-medium no-underline hover:brightness-90 transition-all"
            >立即购买 &rarr;</router-link>
          </div>

          <!-- 用户气泡 -->
          <div
            v-else-if="msg.role === 'user'"
            class="bg-[var(--color-primary)] text-white px-4 py-2.5 rounded-[12px_12px_2px_12px] text-sm leading-relaxed whitespace-pre-wrap break-words max-w-[75%]"
          >{{ msg.content }}</div>

          <!-- AI 气泡 -->
          <div
            v-else-if="msg.role === 'assistant'"
            class="bg-[var(--color-card)] border border-[var(--color-border)] px-4 py-2.5 rounded-[12px_12px_12px_2px] text-sm leading-relaxed whitespace-pre-wrap break-words max-w-[75%] text-[var(--color-foreground)]"
          >
            {{ msg.content }}
          </div>
        </div>

        <!-- 加载中 dots -->
        <div v-if="loading" class="flex mb-4">
          <div class="bg-[var(--color-card)] border border-[var(--color-border)] px-4 py-3 rounded-[12px_12px_12px_2px] flex gap-1.5">
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--color-muted)] animate-bounce"></span>
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--color-muted)] animate-bounce" style="animation-delay:0.15s"></span>
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--color-muted)] animate-bounce" style="animation-delay:0.3s"></span>
          </div>
        </div>
      </div>

      <!-- 输入区 -->
      <div class="p-4 border-t border-[var(--color-border)] flex gap-3">
        <input
          v-model="input"
          @keyup.enter="send"
          placeholder="输入你的问题..."
          :disabled="loading"
          ref="inputBox"
          class="flex-1 px-4 py-2.5 border border-[var(--color-border)] rounded-[var(--radius-input)] text-sm outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-teal-100 transition-all disabled:opacity-50"
        />
        <button
          @click="send"
          :disabled="loading || !input.trim()"
          class="px-6 py-2.5 bg-[var(--color-primary)] text-white border-none rounded-[var(--radius-btn)] text-sm font-medium cursor-pointer hover:brightness-90 disabled:opacity-40 disabled:cursor-not-allowed transition-all"
        >
          发送
        </button>
      </div>
    </main>

    <!-- 删除确认 Dialog -->
    <Dialog :open="showDelDialog" @update:open="showDelDialog = $event">
      <DialogContent class="sm:max-w-[380px]">
        <DialogHeader>
          <DialogTitle>删除对话</DialogTitle>
          <DialogDescription>
            确定要删除这个对话吗？<br />
            <span class="text-[var(--color-destructive)] text-sm">删除后无法恢复。</span>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter class="flex justify-end gap-3 mt-4">
          <Button variant="outline" @click="showDelDialog = false">取消</Button>
          <Button variant="destructive" @click="confirmDelete">删除</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup>
import { ref, nextTick, onMounted, computed } from 'vue'
import { useUserStore } from '@/stores/user'
import { agentChat, getSessions, deleteSession, getMessages } from '@/api/agent'
import {
  PanelLeft, PanelLeftClose, MessageCircle
} from 'lucide-vue-next'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

const userStore = useUserStore()

const sessions = ref([])
const currentSessionId = ref(null)
const currentAgentType = ref('')
const messages = ref([])
const input = ref('')
const loading = ref(false)
const msgContainer = ref(null)
const inputBox = ref(null)
const showDelDialog = ref(false)
const pendingDeleteId = ref(null)
const sidebarCollapsed = ref(false)

const currentSessionTitle = computed(() => {
  const s = sessions.value.find(s => s.id === currentSessionId.value)
  return s?.title || ''
})

// ===== 会话管理 =====
async function loadSessions() {
  try {
    const res = await getSessions({ page: 1, page_size: 50 })
    sessions.value = res.data.list || []
  } catch (e) { console.error('加载会话列表失败', e) }
}

function parseHistoryMsg(m) {
  if (m.role === 'assistant' && m.content) {
    try {
      const parsed = JSON.parse(m.content)
      if (parsed.type === 'purchase_offer' && parsed.payload) {
        return { role: 'action', action: { type: 'purchase', ...parsed.payload } }
      }
    } catch {}
  }
  return { role: m.role, content: m.content }
}

async function loadMessages(sessionId) {
  try {
    const res = await getMessages(sessionId, { page: 1, page_size: 100 })
    messages.value = (res.data.list || [])
      .filter(m => m.role !== 'tool')
      .map(parseHistoryMsg)
    scrollToBottom()
  } catch (e) { console.error('加载消息失败', e) }
}

function newChat() {
  currentSessionId.value = null
  currentAgentType.value = ''
  messages.value = []
  input.value = ''
  nextTick(() => inputBox.value?.focus())
}

function switchSession(id) {
  currentSessionId.value = id
  const s = sessions.value.find(s => s.id === id)
  if (s) currentAgentType.value = s.agent_type
  loadMessages(id)
}

function removeSession(id) {
  pendingDeleteId.value = id
  showDelDialog.value = true
}

async function confirmDelete() {
  const id = pendingDeleteId.value
  showDelDialog.value = false
  pendingDeleteId.value = null
  if (!id) return
  try {
    await deleteSession(id)
    sessions.value = sessions.value.filter(s => s.id !== id)
    if (currentSessionId.value === id) newChat()
  } catch (e) { console.error('删除会话失败', e) }
}

function scrollToBottom() {
  nextTick(() => {
    if (msgContainer.value) {
      msgContainer.value.scrollTop = msgContainer.value.scrollHeight
    }
  })
}

// ===== 发送消息 + SSE 流式（逻辑与原版相同，调整了气泡样式）=====
async function send() {
  if (!input.value.trim() || loading.value) return
  const question = input.value.trim()
  input.value = ''
  loading.value = true

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

    while (!streamEnded) {
      let readResult
      try {
        readResult = await Promise.race([
          reader.read(),
          new Promise((_, reject) => setTimeout(() => reject(new Error('STREAM_TIMEOUT')), 30000))
        ])
      } catch (raceErr) {
        if (raceErr.message === 'STREAM_TIMEOUT') console.log('SSE stream timeout, forcing end')
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

          if (currentEvent === 'delta') {
            try {
              const d = JSON.parse(payload)
              if (currentAssistantIdx === -1) {
                messages.value.push({ role: 'assistant', content: '' })
                currentAssistantIdx = messages.value.length - 1
              }
              messages.value[currentAssistantIdx].content += d.content
            } catch {}
          } else if (currentEvent === 'thinking') {
            try {
              const d = JSON.parse(payload)
              // 移除旧的 thinking 气泡（只保留最新一条），再插入新的
              messages.value = messages.value.filter(m => m.role !== 'thinking')
              messages.value.push({ role: 'thinking', content: `正在使用 ${d.tool}...` })
            } catch {}
          } else if (currentEvent === 'action') {
            try {
              const d = JSON.parse(payload)
              if (d.type === 'purchase_offer') {
                messages.value.push({
                  role: 'action',
                  action: { type: 'purchase', ...d.payload }
                })
              }
            } catch {}
          } else if (currentEvent === 'done') {
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
      if (streamEnded) break
    }
  } catch (e) {
    messages.value.push({ role: 'assistant', content: `连接失败: ${e.message}` })
  } finally {
    messages.value = messages.value.filter(m => m.role !== 'thinking')
    currentAssistantIdx = -1
    loading.value = false
  }
}

onMounted(() => loadSessions())
</script>
