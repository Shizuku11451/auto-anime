<script setup lang="ts">
import BrandMark from '@/components/ui/BrandMark.vue'
import ThemeToggle from '@/components/ui/ThemeToggle.vue'

withDefaults(defineProps<{ subtitle?: string; wide?: boolean }>(), { subtitle: '番剧收藏计划', wide: false })
</script>

<template>
  <div class="auth">
    <div class="auth-tools">
      <ThemeToggle />
    </div>
    <div class="auth-card" :class="{ wide }">
      <BrandMark size="lg" :subtitle="subtitle" class="auth-brand" />
      <slot />
    </div>
    <p class="auth-foot">Auto-Anime</p>
  </div>
</template>

<style scoped>
.auth {
  position: relative;
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 16px;
  overflow: hidden;
}
.auth-tools {
  position: absolute;
  top: 18px;
  right: 18px;
}
.auth-card {
  position: relative;
  width: 100%;
  max-width: 420px;
  padding: 34px 34px 30px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--r-xl);
  box-shadow: var(--shadow-lg);
  animation: rise-in var(--t-slow) var(--ease-out) both;
}
.auth-card.wide {
  max-width: 620px;
}
.auth-brand {
  margin-bottom: 26px;
}
.auth-foot {
  margin-top: 22px;
  color: var(--text-3);
  font-size: 12px;
  animation: fade-in 800ms var(--ease-out) 300ms both;
}
@media (max-width: 480px) {
  .auth-card {
    padding: 26px 22px 24px;
  }
}
</style>
