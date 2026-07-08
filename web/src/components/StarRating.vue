<template>
  <div class="inline-flex gap-0.5">
    <span v-for="i in 5" :key="i" class="text-[20px] transition-colors duration-200" :class="[i <= modelValue ? 'text-[var(--color-accent)]' : 'text-[#ddd]', interactive ? 'cursor-pointer hover:text-[var(--color-accent)]' : '']" @click="interactive && $emit('update:modelValue', i)">
      {{ i <= modelValue ? '★' : '☆' }}
    </span>
  </div>
</template>

<script setup>
defineProps({
  modelValue: { type: Number, default: 0 },
  interactive: { type: Boolean, default: false }
})
defineEmits(['update:modelValue'])
</script>
