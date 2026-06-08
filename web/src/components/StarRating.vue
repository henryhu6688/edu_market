<template>
  <div class="star-rating">
    <span v-for="i in 5" :key="i" class="star" :class="{ filled: i <= modelValue, interactive: interactive }" @click="interactive && $emit('update:modelValue', i)">
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

<style scoped>
.star-rating { display: inline-flex; gap: 2px; }
.star {
  font-size: 20px;
  color: #ddd;
  transition: color 0.2s;
}
.star.filled { color: #f59e0b; }
.star.interactive { cursor: pointer; }
.star.interactive:hover { color: #f59e0b; }
</style>
