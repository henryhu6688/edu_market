<template>
  <div class="pagination" v-if="totalPages > 1">
    <button :disabled="page <= 1" @click="$emit('change', page - 1)">上一页</button>
    <span v-for="p in pages" :key="p">
      <button v-if="p !== '...'" :class="{ active: p === page }" @click="$emit('change', p)">{{ p }}</button>
      <span v-else class="ellipsis">...</span>
    </span>
    <button :disabled="page >= totalPages" @click="$emit('change', page + 1)">下一页</button>
    <span class="total">共 {{ total }} 条</span>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  page: { type: Number, required: true },
  pageSize: { type: Number, required: true },
  total: { type: Number, required: true }
})

defineEmits(['change'])

const totalPages = computed(() => Math.ceil(props.total / props.pageSize))

const pages = computed(() => {
  const t = totalPages.value
  const p = props.page
  if (t <= 7) return Array.from({ length: t }, (_, i) => i + 1)
  if (p <= 4) return [1, 2, 3, 4, 5, '...', t]
  if (p >= t - 3) return [1, '...', t - 4, t - 3, t - 2, t - 1, t]
  return [1, '...', p - 1, p, p + 1, '...', t]
})
</script>

<style scoped>
.pagination {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 24px;
  justify-content: center;
}
.pagination button {
  padding: 6px 12px;
  border: 1px solid #ddd;
  background: #fff;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}
.pagination button:hover:not(:disabled) {
  border-color: #4f46e5;
  color: #4f46e5;
}
.pagination button.active {
  background: #4f46e5;
  color: #fff;
  border-color: #4f46e5;
}
.pagination button:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.ellipsis { padding: 6px 4px; color: #999; }
.total { margin-left: 12px; font-size: 13px; color: #999; }
</style>
