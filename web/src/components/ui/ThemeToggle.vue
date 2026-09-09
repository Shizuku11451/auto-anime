<script setup lang="ts">
import { Moon, Sun } from '@lucide/vue'
import { useTheme } from '@/stores/theme'

const { isLight, toggle } = useTheme()
</script>

<template>
  <button
    type="button"
    class="theme-toggle"
    :title="isLight ? '切换到暗色主题' : '切换到亮色主题'"
    :aria-label="isLight ? '切换到暗色主题' : '切换到亮色主题'"
    @click="toggle($event)"
  >
    <Transition name="swap" mode="out-in">
      <Moon v-if="isLight" key="moon" />
      <Sun v-else key="sun" />
    </Transition>
  </button>
</template>

<style scoped>
.theme-toggle {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  border: 1px solid var(--line);
  border-radius: var(--r-sm);
  background: var(--surface-2);
  color: var(--text-2);
  cursor: pointer;
  transition:
    background var(--t-fast) var(--ease-out),
    border-color var(--t-fast) var(--ease-out),
    color var(--t-fast) var(--ease-out),
    transform var(--t-fast) var(--ease-out);
}
.theme-toggle:hover {
  color: var(--text);
  border-color: var(--line-strong);
  background: var(--surface-3);
}
.theme-toggle:active {
  transform: scale(0.94);
}
.theme-toggle svg {
  width: 17px;
  height: 17px;
}
.swap-enter-active,
.swap-leave-active {
  transition:
    transform 220ms var(--ease-out),
    opacity 220ms var(--ease-out);
}
.swap-enter-from {
  opacity: 0;
  transform: rotate(-70deg) scale(0.6);
}
.swap-leave-to {
  opacity: 0;
  transform: rotate(70deg) scale(0.6);
}
</style>
