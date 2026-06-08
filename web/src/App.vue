<template>
  <div id="app">
    <Navbar />
    <main class="main-content">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import Navbar from '@/components/Navbar.vue'
</script>

<style scoped>
.main-content {
  min-height: calc(100vh - 60px);
  padding: 20px;
  max-width: 1200px;
  margin: 0 auto;
}
</style>
