<script setup lang="ts">
import { onMounted, ref, type Component } from 'vue'

defineProps<{ icon: Component; active: boolean }>()

const root = ref<HTMLElement>()

/* 统一各段线条长度为 1，选中时按比例从无到有描出整个图标 */
onMounted(() => {
  root.value?.querySelectorAll('path, line, polyline, polygon, circle, ellipse, rect').forEach((shape) => {
    shape.setAttribute('pathLength', '1')
  })
})
</script>

<template>
  <span ref="root" class="nav-icon" :class="{ active }" aria-hidden="true">
    <component :is="icon" />
  </span>
</template>

<style scoped>
.nav-icon {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  flex: none;
  transition: transform var(--t-med) var(--ease-spring);
}
.nav-icon :deep(svg) {
  width: 100%;
  height: 100%;
}
.nav-icon.active {
  animation: icon-pop 420ms var(--ease-spring) both;
}
.nav-icon.active :deep(svg > *) {
  stroke-dasharray: 1 1;
  animation: icon-draw 700ms cubic-bezier(0.65, 0, 0.35, 1) both;
}
.nav-icon.active :deep(svg > :nth-child(2)) { animation-delay: 70ms; }
.nav-icon.active :deep(svg > :nth-child(3)) { animation-delay: 140ms; }
.nav-icon.active :deep(svg > :nth-child(n + 4)) { animation-delay: 210ms; }
@keyframes icon-draw {
  from {
    stroke-dashoffset: 1;
    opacity: 0.35;
  }
  to {
    stroke-dashoffset: 0;
    opacity: 1;
  }
}
@keyframes icon-pop {
  0% { transform: scale(0.82); }
  60% { transform: scale(1.12); }
  100% { transform: scale(1.06) translateY(-1px); }
}
</style>
