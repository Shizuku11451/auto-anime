<script setup lang="ts">
import { Play } from '@lucide/vue'

withDefaults(defineProps<{ size?: 'md' | 'lg'; subtitle?: string }>(), {
  size: 'md',
  subtitle: 'RSS · NAS MEDIA LIBRARY',
})
</script>

<template>
  <div class="brand" :class="`s-${size}`">
    <div class="logo" aria-hidden="true">
      <Play class="logo-ic" />
      <span class="logo-shine" />
    </div>
    <div class="brand-text">
      <div class="brand-name">自动追番</div>
      <div class="brand-sub">{{ subtitle }}</div>
    </div>
  </div>
</template>

<style scoped>
.brand {
  display: flex;
  align-items: center;
  gap: 11px;
  min-width: 0;
}
.logo {
  position: relative;
  width: 38px;
  height: 38px;
  border-radius: 12px;
  background: var(--brand-gradient);
  display: grid;
  place-items: center;
  color: #fff;
  flex: none;
  box-shadow:
    0 6px 18px -4px var(--accent-glow),
    inset 0 1px 0 rgba(255, 255, 255, 0.35);
  overflow: hidden;
}
.logo-ic {
  width: 17px;
  height: 17px;
  fill: currentColor;
  stroke: none;
  translate: 1px 0;
  position: relative;
  z-index: 1;
}
.logo-shine {
  position: absolute;
  inset: -40%;
  background: linear-gradient(115deg, transparent 40%, rgba(255, 255, 255, 0.5) 50%, transparent 60%);
  transform: translateX(-60%);
  transition: transform 900ms var(--ease-out);
}
.brand:hover .logo-shine {
  transform: translateX(60%);
}
.brand-text {
  min-width: 0;
}
.brand-name {
  font-size: 16.5px;
  font-weight: 760;
  letter-spacing: -0.01em;
  line-height: 1.25;
  white-space: nowrap;
}
.brand-sub {
  margin-top: 2px;
  color: var(--text-3);
  font-size: 10px;
  font-weight: 650;
  letter-spacing: 0.14em;
  white-space: nowrap;
}
.s-lg .logo {
  width: 52px;
  height: 52px;
  border-radius: 16px;
}
.s-lg .logo-ic {
  width: 23px;
  height: 23px;
}
.s-lg .brand-name {
  font-size: 21px;
}
.s-lg .brand-sub {
  font-size: 11px;
}
</style>
