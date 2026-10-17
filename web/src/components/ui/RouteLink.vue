<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, type RouteLocationRaw } from 'vue-router'

/**
 * 站内跳转链接。
 *
 * 故意不输出 href：桌面端浏览器（含 PWA/应用窗口）会在鼠标悬停时于窗口左下角
 * 弹出完整地址预览，对本地应用界面来说很碍眼。链接语义、键盘 Enter、
 * Ctrl/⌘/Shift + 点击与鼠标中键打开新标签页都保留。
 */
const props = withDefaults(
  defineProps<{
    to: RouteLocationRaw
    /** 当前路由是否命中本项，用于 aria-current */
    active?: boolean
  }>(),
  { active: false },
)

const router = useRouter()
const href = computed(() => router.resolve(props.to).href)

function activate(event: MouseEvent) {
  // 保留浏览器里“按住修饰键在新标签页打开”的习惯
  if (event.metaKey || event.ctrlKey || event.shiftKey) {
    event.preventDefault()
    window.open(href.value, '_blank', 'noopener')
    return
  }
  if (event.button !== 0) return
  event.preventDefault()
  router.push(props.to)
}

function onAux(event: MouseEvent) {
  if (event.button !== 1) return
  event.preventDefault()
  window.open(href.value, '_blank', 'noopener')
}
</script>

<template>
  <a
    role="link"
    tabindex="0"
    :aria-current="active ? 'page' : undefined"
    @click="activate"
    @auxclick="onAux"
    @keydown.enter.prevent="router.push(to)"
  >
    <slot />
  </a>
</template>

<style scoped>
/* 无 href 的 <a> 默认是文本光标，这里补回链接手感 */
a {
  cursor: pointer;
}
</style>
