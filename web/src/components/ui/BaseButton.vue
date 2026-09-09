<script setup lang="ts">
import type { Component } from 'vue'
import { LoaderCircle } from '@lucide/vue'

withDefaults(
  defineProps<{
    variant?: 'default' | 'primary' | 'ghost' | 'soft' | 'danger'
    size?: 'sm' | 'md' | 'lg'
    icon?: Component
    loading?: boolean
    disabled?: boolean
    block?: boolean
    iconOnly?: boolean
    type?: 'button' | 'submit' | 'reset'
  }>(),
  { variant: 'default', size: 'md', type: 'button' },
)
</script>

<template>
  <button
    :type="type"
    class="btn"
    :class="[`v-${variant}`, `s-${size}`, { block, 'icon-only': iconOnly, 'is-loading': loading }]"
    :disabled="disabled || loading"
  >
    <span v-if="loading || icon" class="btn-ic" aria-hidden="true">
      <LoaderCircle v-if="loading" class="spin" />
      <component :is="icon" v-else />
    </span>
    <span v-if="$slots.default" class="btn-label"><slot /></span>
  </button>
</template>

<style scoped>
.btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 36px;
  padding: 0 14px;
  border: 1px solid var(--line);
  border-radius: var(--r-sm);
  background: var(--surface-2);
  color: var(--text);
  font-weight: 550;
  font-size: 13.5px;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  user-select: none;
  -webkit-tap-highlight-color: transparent;
  transition:
    background var(--t-fast) var(--ease-out),
    border-color var(--t-fast) var(--ease-out),
    color var(--t-fast) var(--ease-out),
    box-shadow var(--t-med) var(--ease-out),
    transform var(--t-fast) var(--ease-out),
    filter var(--t-fast) var(--ease-out);
}
.btn:hover:not(:disabled) {
  border-color: var(--line-strong);
  background: var(--surface-3);
}
.btn:active:not(:disabled) {
  transform: scale(0.965);
}
.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btn.block {
  width: 100%;
}

.btn-ic {
  display: inline-flex;
  flex: none;
}
.btn-ic :deep(svg) {
  width: 16px;
  height: 16px;
}
.s-sm {
  height: 30px;
  padding: 0 11px;
  font-size: 12.5px;
  gap: 5px;
  border-radius: 8px;
}
.s-sm .btn-ic :deep(svg) {
  width: 14px;
  height: 14px;
}
.s-lg {
  height: 42px;
  padding: 0 20px;
  font-size: 14.5px;
  border-radius: var(--r-md);
}
.s-lg .btn-ic :deep(svg) {
  width: 18px;
  height: 18px;
}
.icon-only {
  padding: 0;
  width: 36px;
}
.icon-only.s-sm {
  width: 30px;
}
.icon-only.s-lg {
  width: 42px;
}

/* 主按钮：渐变 + 光晕 */
.v-primary {
  color: var(--accent-contrast);
  border-color: transparent;
  background: linear-gradient(180deg, var(--accent-strong), var(--accent));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.22),
    var(--glow-accent);
  font-weight: 650;
}
.v-primary:hover:not(:disabled) {
  border-color: transparent;
  background: linear-gradient(180deg, var(--accent-strong), var(--accent));
  filter: brightness(1.06) saturate(1.05);
  transform: translateY(-1px);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.25),
    0 12px 32px -8px var(--accent-glow);
}
.v-primary:active:not(:disabled) {
  transform: scale(0.97);
}

.v-ghost {
  background: transparent;
  border-color: transparent;
  color: var(--text-2);
}
.v-ghost:hover:not(:disabled) {
  background: var(--surface-2);
  border-color: transparent;
  color: var(--text);
}

.v-soft {
  background: var(--accent-soft);
  border-color: transparent;
  color: var(--accent);
}
.v-soft:hover:not(:disabled) {
  background: var(--accent-soft);
  border-color: var(--accent-border);
}

.v-danger {
  color: var(--red);
}
.v-danger:hover:not(:disabled) {
  border-color: var(--red-border);
  background: var(--red-soft);
}

.is-loading .btn-label {
  opacity: 0.75;
}
</style>
