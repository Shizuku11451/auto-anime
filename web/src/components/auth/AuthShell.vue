<script setup lang="ts">
import BrandMark from '@/components/ui/BrandMark.vue'
import ThemeToggle from '@/components/ui/ThemeToggle.vue'

withDefaults(defineProps<{ subtitle?: string; wide?: boolean }>(), { subtitle: 'RSS · NAS MEDIA LIBRARY', wide: false })
</script>

<template>
  <div class="auth">
    <div class="blob b1" aria-hidden="true" />
    <div class="blob b2" aria-hidden="true" />
    <div class="blob b3" aria-hidden="true" />
    <div class="auth-tools">
      <ThemeToggle />
    </div>
    <div class="auth-card" :class="{ wide }">
      <BrandMark size="lg" :subtitle="subtitle" class="auth-brand" />
      <slot />
    </div>
    <p class="auth-foot">自动追番 · 本地服务，无公共 CDN 依赖</p>
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
.blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(70px);
  opacity: 0.55;
  pointer-events: none;
  animation: drift 18s var(--ease-in-out) infinite alternate;
}
.b1 {
  width: 520px;
  height: 520px;
  left: -160px;
  top: -140px;
  background: var(--accent-glow);
}
.b2 {
  width: 420px;
  height: 420px;
  right: -140px;
  top: 20%;
  background: rgba(94, 225, 196, 0.22);
  animation-delay: -6s;
  animation-duration: 22s;
}
.b3 {
  width: 380px;
  height: 380px;
  left: 30%;
  bottom: -200px;
  background: rgba(96, 165, 250, 0.18);
  animation-delay: -12s;
  animation-duration: 26s;
}
html[data-theme="light"] .blob {
  opacity: 0.4;
}
@keyframes drift {
  from {
    transform: translate(0, 0) scale(1);
  }
  to {
    transform: translate(40px, 30px) scale(1.08);
  }
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
  background: var(--surface-glass);
  border: 1px solid var(--line);
  border-radius: var(--r-xl);
  box-shadow: var(--shadow-lg);
  backdrop-filter: blur(22px) saturate(1.3);
  -webkit-backdrop-filter: blur(22px) saturate(1.3);
  animation: rise-in 600ms var(--ease-out) both;
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
