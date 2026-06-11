<template>
  <div class="doc-view" v-if="doc">
    <h1>{{ doc.title }}</h1>
    <span v-if="doc.is_free_preview" class="free-badge">免费试读</span>
    <div class="content">
      <editor-content :editor="editor" />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { useEditor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import { getDocument } from '@/api/document'

const route = useRoute()
const doc = ref(null)

const editor = useEditor({
  extensions: [StarterKit],
  content: null,
  editable: false
})

onMounted(async () => {
  const res = await getDocument(route.params.did)
  doc.value = res.data
  const json = JSON.parse(doc.value.content || '{"type":"doc","content":[]}')
  editor.value?.commands.setContent(json)
})

onBeforeUnmount(() => editor.value?.destroy())
</script>

<style scoped>
.doc-view { max-width: 800px; margin: 20px auto; padding: 20px; }
h1 { margin-bottom: 8px; }
.free-badge { background: #dbeafe; color: #1d4ed8; padding: 2px 10px; border-radius: 12px; font-size: 12px; }
.content { margin-top: 20px; font-size: 15px; line-height: 1.8; }
.content :deep(h1) { font-size: 24px; margin: 16px 0 8px; }
.content :deep(h2) { font-size: 20px; margin: 14px 0 6px; }
.content :deep(h3) { font-size: 17px; margin: 12px 0 4px; }
.content :deep(pre) { background: #1f2937; color: #f3f4f6; padding: 12px 16px; border-radius: 6px; font-size: 13px; overflow-x: auto; }
.content :deep(code) { background: #f3f4f6; padding: 2px 4px; border-radius: 3px; font-size: 13px; }
.content :deep(blockquote) { border-left: 3px solid #d1d5db; padding-left: 12px; color: #6b7280; margin: 8px 0; }
</style>
