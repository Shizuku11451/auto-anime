<script setup lang="ts">
import type { Component } from 'vue'

withDefaults(
  defineProps<{
    icon: Component
    title: string
    hint?: string
    compact?: boolean
    /** 加载中：图标旋转，不做漂浮动画 */
    loading?: boolean
    tone?: 'default' | 'error'
  }>(),
  { hint: '', compact: false, loading: false, tone: 'default' },
)
</script>

<template>
  <div class="empty" :class="{ compact, [`tone-${tone}`]: true }">
    <div class="empty-ico" :class="{ loading }">
      <component :is="icon" :class="{ spin: loading }" aria-hidden="true" />
    </div>
    <p class="empty-title">{{ title }}</p>
    <p v-if="hint" class="empty-hint">{{ hint }}</p>
    <div v-if="$slots.default" class="empty-actions">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 64px 24px;
  color: var(--text-2);
  animation: rise-in var(--t-slow) var(--ease-out) both;
}
.empty.compact {
  padding: 36px 20px;
}
.empty-ico {
  width: 64px;
  height: 64px;
  border-radius: 22px;
  display: grid;
  place-items: center;
  color: var(--accent);
  background: var(--accent-soft);
  box-shadow: inset 0 0 0 1px var(--accent-border);
  margin-bottom: 18px;
  animation: float-y 5s var(--ease-in-out) infinite;
}
.empty-ico.loading {
  animation: none;
}
.empty-ico svg {
  width: 28px;
  height: 28px;
}
.compact .empty-ico {
  width: 52px;
  height: 52px;
  border-radius: 18px;
  margin-bottom: 14px;
}
.compact .empty-ico svg {
  width: 22px;
  height: 22px;
}
.tone-error .empty-ico {
  color: var(--red);
  background: var(--red-soft);
  box-shadow: inset 0 0 0 1px var(--red-border);
}
.empty-title {
  font-size: 15px;
  font-weight: 650;
  color: var(--text);
}
.empty-hint {
  margin-top: 6px;
  font-size: 13px;
  color: var(--text-3);
  max-width: 420px;
  line-height: 1.6;
}
.empty-actions {
  margin-top: 18px;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: center;
}
</style>
