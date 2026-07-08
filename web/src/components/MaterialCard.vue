<template>
  <div
    class="group cursor-pointer rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-card)] overflow-hidden transition-all duration-200 hover:-translate-y-0.5 hover:shadow-lg"
    @click="$router.push(`/materials/${material.id}`)"
  >
    <!-- 封面 -->
    <div v-if="material.cover_image" class="h-[140px]">
      <img :src="material.cover_image" :alt="material.title" class="w-full h-full object-cover" />
    </div>
    <div v-else class="h-[140px] bg-gradient-to-br from-teal-600 to-teal-400 flex items-center justify-center">
      <span class="text-white text-3xl font-bold">{{ (material.title || '?').charAt(0) }}</span>
    </div>

    <!-- 信息 -->
    <div class="p-3.5">
      <span class="inline-block px-2 py-0.5 text-[11px] rounded-[var(--radius-pill)] bg-teal-50 text-[var(--color-primary)] font-medium mb-1.5">
        {{ material.category_name || '未分类' }}
      </span>
      <h3 class="font-semibold text-[15px] text-[var(--color-foreground)] truncate mb-0.5">{{ material.title }}</h3>
      <p class="text-[13px] text-[var(--color-muted)] leading-relaxed line-clamp-2 mb-2.5">
        {{ material.description?.substring(0, 80) || '' }}
      </p>
      <div class="flex items-center justify-between">
        <span class="text-base font-bold text-[var(--color-accent)]">¥{{ material.price }}</span>
        <span class="text-xs text-[var(--color-muted)]">{{ material.buy_count || 0 }}人购买</span>
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  material: { type: Object, required: true }
})
</script>
