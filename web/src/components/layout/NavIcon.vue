<script setup lang="ts">
import type { Component } from 'vue'

/* name 对应 NAV_ITEMS 的路由名，每个图标按自身含义做选中动效 */
defineProps<{ icon: Component; name: string; active: boolean }>()
</script>

<template>
  <span class="nav-icon" :class="[`i-${name}`, { active }]" aria-hidden="true">
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
  overflow: visible;
}
.nav-icon :deep(svg > *) {
  transform-box: view-box;
}

/* 番剧库：场记板选中时打开，未选中时合上（板绕左下铰链转动） */
.i-library :deep(svg > :is(:nth-child(1), :nth-child(2), :nth-child(4))) {
  transform-origin: 3px 11px;
  transform: rotate(16deg);
  transition: transform 360ms var(--ease-spring);
}
.i-library.active :deep(svg > :is(:nth-child(1), :nth-child(2), :nth-child(4))) {
  transform: rotate(-6deg);
}

/* 最近条目：新条目落进收件箱 */
.i-items.active :deep(svg > :nth-child(1)) {
  animation: inbox-drop 520ms var(--ease-spring) both;
}
@keyframes inbox-drop {
  from { transform: translateY(-7px); opacity: 0; }
  60% { opacity: 1; }
  to { transform: none; opacity: 1; }
}

/* 订阅管理：圆点、内圈、外圈依次亮起，信号一格一格增强 */
.i-feeds.active :deep(svg > *) {
  transform-origin: 5px 19px;
  animation: signal-step 220ms var(--ease-out) both;
}
.i-feeds.active :deep(svg > :nth-child(1)) { animation-delay: 200ms; }
.i-feeds.active :deep(svg > :nth-child(2)) { animation-delay: 400ms; }
@keyframes signal-step {
  from { opacity: 0; transform: scale(0.6); }
  to { opacity: 1; transform: none; }
}

/* 搜索：放大镜绕一圈扫视 */
.i-discover.active :deep(svg) {
  animation: search-scan 700ms var(--ease-out) both;
}
@keyframes search-scan {
  0% { transform: none; }
  25% { transform: translate(-2px, -1px) rotate(-8deg); }
  50% { transform: translate(1px, -2px); }
  75% { transform: translate(2px, 1px) rotate(6deg); }
  100% { transform: none; }
}

/* 设置：齿轮转四分之一圈，取消选中时转回 */
.i-settings :deep(svg) {
  transition: transform 600ms var(--ease-spring);
}
.i-settings.active :deep(svg) {
  transform: rotate(90deg);
}
</style>
