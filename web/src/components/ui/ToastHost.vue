<script setup lang="ts">
import { CircleCheck, Info, TriangleAlert } from '@lucide/vue'
import { useToasts } from '@/stores/toast'

const { toasts, dismissToast } = useToasts()
const icons = { success: CircleCheck, error: TriangleAlert, info: Info }
</script>

<template>
  <div class="toasts" aria-live="polite" aria-atomic="false">
    <TransitionGroup name="toast">
      <button
        v-for="t in toasts"
        :key="t.id"
        type="button"
        class="toast"
        :class="`t-${t.type}`"
        :title="'点击关闭'"
        @click="dismissToast(t.id)"
      >
        <span class="toast-ic"><component :is="icons[t.type]" aria-hidden="true" /></span>
        <span class="toast-text">{{ t.message }}</span>
      </button>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toasts {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 200;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 8px;
  pointer-events: none;
  max-width: min(420px, calc(100vw - 40px));
}
.toast {
  pointer-events: auto;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 11px 14px 11px 12px;
  max-width: 100%;
  text-align: left;
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  background: var(--surface-glass);
  backdrop-filter: blur(14px) saturate(1.3);
  -webkit-backdrop-filter: blur(14px) saturate(1.3);
  color: var(--text);
  box-shadow: var(--shadow-md);
  font-size: 13.5px;
  line-height: 1.5;
  cursor: pointer;
  transition: border-color var(--t-fast) var(--ease-out);
}
.toast:hover {
  border-color: var(--line-strong);
}
.toast-ic {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 7px;
  flex: none;
  margin-top: 1px;
}
.toast-ic svg {
  width: 15px;
  height: 15px;
}
.t-success .toast-ic {
  color: var(--green);
  background: var(--green-soft);
}
.t-error .toast-ic {
  color: var(--red);
  background: var(--red-soft);
}
.t-info .toast-ic {
  color: var(--blue);
  background: var(--blue-soft);
}
.toast-text {
  word-break: break-word;
}

@media (max-width: 640px) {
  .toasts {
    right: 12px;
    left: 12px;
    bottom: calc(76px + env(safe-area-inset-bottom));
    align-items: stretch;
    max-width: none;
  }
}
</style>
