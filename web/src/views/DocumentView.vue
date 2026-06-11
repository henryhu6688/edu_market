<template>
  <div class="doc-view" v-if="doc">
    <h1>{{ doc.title }}</h1>
    <span v-if="doc.is_free_preview" class="free-badge">免费试读</span>
    <div class="content">
      <Viewer :value="doc.content || ''" :plugins="plugins" />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Viewer } from '@bytemd/vue-next'
import gfm from '@bytemd/plugin-gfm'
import 'bytemd/dist/index.css'
import { getDocument } from '@/api/document'

const route = useRoute()
const doc = ref(null)
const plugins = [gfm()]

onMounted(async () => {
  const res = await getDocument(route.params.did)
  doc.value = res.data
})
</script>

<style scoped>
.doc-view { max-width: 800px; margin: 20px auto; padding: 20px; }
h1 { margin-bottom: 8px; }
.free-badge { background: #dbeafe; color: #1d4ed8; padding: 2px 10px; border-radius: 12px; font-size: 12px; }
.content { margin-top: 20px; }
.content :deep(.bytemd) { border: none; height: auto !important; }
.content :deep(.bytemd-body) { height: auto !important; }
.content :deep(.bytemd-preview) { padding: 0; }
</style>
