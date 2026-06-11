<template>
  <div class="doc-editor">
    <aside class="doc-tree">
      <h4>📂 文档目录</h4>
      <div v-for="d in docs" :key="d.id" class="tree-item"
        :class="{ active: d.id === selectedId, folder: !d.parent_id }"
        :style="{ paddingLeft: (depth(d) * 16 + 8) + 'px' }"
        @click="selectDoc(d)">
        {{ d.parent_id ? '📄' : '📁' }} {{ d.title || '未命名' }}
      </div>
      <div class="tree-actions">
        <button @click="addDoc(null)">+ 新建文档</button>
        <label class="upload-btn">📎 导入文件
          <input type="file" hidden @change="uploadFile" accept=".pdf,.pptx,.docx,.md,.txt" />
        </label>
        <router-link :to="`/materials/${materialId}`" class="back-btn">← 返回资料</router-link>
      </div>
    </aside>
    <main class="editor-area" v-if="selectedDoc">
      <div class="editor-toolbar">
        <input v-model="selectedDoc.title" placeholder="文档标题" @change="saveTitle" class="title-input" />
        <span class="save-status">{{ saving ? '💾 保存中...' : '✅ 已保存' }}</span>
        <label class="free-check">
          <input type="checkbox" v-model="selectedDoc.is_free_preview" @change="saveMeta" /> 免费试读
        </label>
        <button @click="deleteCurrent" class="btn-del">🗑 删除</button>
      </div>
      <div class="editor-content">
        <Editor :value="markdownContent" :plugins="plugins" @change="onContentChange" />
      </div>
    </main>
    <div v-else class="editor-empty">选择或创建一篇文档开始编辑</div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Editor, Viewer } from '@bytemd/vue-next'
import gfm from '@bytemd/plugin-gfm'
import 'bytemd/dist/index.css'

import { getDocTree, getDocument, createDocument, updateDocument, deleteDocument, uploadFile as uploadDocFile } from '@/api/document'

const route = useRoute()
const materialId = ref(parseInt(route.params.mid))
const plugins = [gfm()]

const docs = ref([])
const selectedId = ref(null)
const selectedDoc = ref(null)
const markdownContent = ref('')
const saving = ref(false)
let saveTimer = null

function depth(doc) {
  if (!doc.parent_id) return 0
  const parent = docs.value.find(p => p.id === doc.parent_id)
  return parent ? depth(parent) + 1 : 1
}

async function loadTree() {
  const res = await getDocTree(materialId.value)
  docs.value = res.data || []
}

async function selectDoc(doc) {
  selectedId.value = doc.id
  const res = await getDocument(doc.id)
  selectedDoc.value = res.data
  markdownContent.value = selectedDoc.value.content || ''
}

async function addDoc(parentId) {
  const title = prompt('文档标题：')
  if (!title) return
  const res = await createDocument(materialId.value, { title, parent_id: parentId || undefined })
  docs.value.push(res.data)
  selectDoc(res.data)
}

async function saveTitle() {
  await updateDocument(selectedDoc.value.id, { title: selectedDoc.value.title })
}

async function saveMeta() {
  await updateDocument(selectedDoc.value.id, { is_free_preview: selectedDoc.value.is_free_preview })
}

async function onContentChange(v) {
  if (!selectedDoc.value) return
  markdownContent.value = v
  saving.value = true
  clearTimeout(saveTimer)
  saveTimer = setTimeout(async () => {
    selectedDoc.value.content = v
    await updateDocument(selectedDoc.value.id, { content: v })
    saving.value = false
  }, 2000)
}

async function uploadFile(e) {
  const file = e.target.files[0]
  if (!file) return
  const fd = new FormData()
  fd.append('file', file)
  const res = await uploadDocFile(materialId.value, fd)
  docs.value.push(res.data)
  selectDoc(res.data)
  e.target.value = ''
}

async function deleteCurrent() {
  if (!confirm('确定删除这篇文档？')) return
  await deleteDocument(selectedDoc.value.id)
  docs.value = docs.value.filter(d => d.id !== selectedDoc.value.id)
  selectedId.value = null
  selectedDoc.value = null
  markdownContent.value = ''
}

onMounted(loadTree)
onBeforeUnmount(() => clearTimeout(saveTimer))
</script>

<style scoped>
.doc-editor { display: flex; height: calc(100vh - 60px); max-width: 1400px; margin: 0 auto; }
.doc-tree { width: 240px; border-right: 1px solid #e5e7eb; padding: 16px; overflow-y: auto; background: #fafafa; flex-shrink: 0; }
.doc-tree h4 { margin: 0 0 12px; font-size: 14px; }
.tree-item { padding: 6px 8px; cursor: pointer; border-radius: 4px; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tree-item:hover { background: #e5e7eb; }
.tree-item.active { background: #dbeafe; color: #1d4ed8; }
.tree-actions { margin-top: 16px; display: flex; flex-direction: column; gap: 6px; }
.tree-actions button, .upload-btn, .back-btn { padding: 6px 10px; border: 1px solid #3b82f6; background: #fff; color: #3b82f6; border-radius: 4px; cursor: pointer; font-size: 12px; text-align: center; text-decoration: none; display: block; }
.tree-actions button:hover, .upload-btn:hover, .back-btn:hover { background: #eff6ff; }
.editor-area { flex: 1; display: flex; flex-direction: column; min-width: 0; overflow: hidden; }
.editor-toolbar { padding: 10px 16px; border-bottom: 1px solid #e5e7eb; display: flex; align-items: center; gap: 12px; }
.title-input { flex: 1; border: none; font-size: 18px; font-weight: 600; outline: none; }
.save-status { font-size: 12px; color: #6b7280; white-space: nowrap; }
.free-check { font-size: 12px; display: flex; align-items: center; gap: 4px; cursor: pointer; white-space: nowrap; }
.btn-del { background: none; border: none; cursor: pointer; font-size: 14px; }
.editor-content { flex: 1; overflow-y: auto; }
.editor-content :deep(.bytemd) { height: 100% !important; border: none; }
.editor-empty { flex: 1; display: flex; align-items: center; justify-content: center; color: #9ca3af; font-size: 16px; }
</style>
