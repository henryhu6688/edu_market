<template>
  <div v-if="totalPages > 1" class="flex items-center justify-center gap-1.5 mt-6">
    <button :disabled="page <= 1" class="px-3 py-1.5 border border-[var(--color-border)] bg-white rounded-[6px] text-sm cursor-pointer hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] disabled:opacity-40 disabled:cursor-not-allowed transition-colors" @click="$emit('change', page - 1)">上一页</button>
    <template v-for="p in pages" :key="p">
      <button v-if="p !== '...'" :class="p === page ? 'px-3 py-1.5 border border-[var(--color-primary)] bg-[var(--color-primary)] text-white rounded-[6px] text-sm cursor-pointer transition-colors' : 'px-3 py-1.5 border border-[var(--color-border)] bg-white rounded-[6px] text-sm cursor-pointer hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] transition-colors'" @click="$emit('change', p)">{{ p }}</button>
      <span v-else class="px-1 py-1.5 text-[#999]">...</span>
    </template>
    <button :disabled="page >= totalPages" class="px-3 py-1.5 border border-[var(--color-border)] bg-white rounded-[6px] text-sm cursor-pointer hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] disabled:opacity-40 disabled:cursor-not-allowed transition-colors" @click="$emit('change', page + 1)">下一页</button>
    <span class="ml-3 text-[13px] text-[#999]">共 {{ total }} 条</span>
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
