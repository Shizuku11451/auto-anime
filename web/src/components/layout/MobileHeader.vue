<script setup lang="ts">
import { ref } from 'vue'
import { Menu } from '@lucide/vue'
import BrandMark from '@/components/ui/BrandMark.vue'
import ThemeToggle from '@/components/ui/ThemeToggle.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import RuntimeCard from './RuntimeCard.vue'
import SideActions from './SideActions.vue'

const menuOpen = ref(false)
</script>

<template>
  <header class="m-header">
    <BrandMark />
    <div class="m-tools">
      <ThemeToggle />
      <BaseButton icon-only :icon="Menu" aria-label="更多操作" @click="menuOpen = true" />
    </div>
    <!-- 弹窗内部会 Teleport 到 body，放在 header 内只是为了保持单根节点，让父级的 class 能落到 header 上 -->
    <BaseDialog :open="menuOpen" title="服务与备份" size="sm" @close="menuOpen = false">
      <div class="m-menu">
        <RuntimeCard />
        <p class="field-hint">备份包含订阅配置，不包含历史条目、下载任务或媒体文件。</p>
        <SideActions layout="list" @done="menuOpen = false" />
      </div>
    </BaseDialog>
  </header>
</template>

<style scoped>
.m-header {
  position: sticky;
  top: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  padding-top: calc(12px + env(safe-area-inset-top));
  background: var(--surface-glass);
  border-bottom: 1px solid var(--line);
  backdrop-filter: blur(18px) saturate(1.2);
  -webkit-backdrop-filter: blur(18px) saturate(1.2);
}
.m-tools {
  display: flex;
  align-items: center;
  gap: 8px;
}
.m-menu {
  display: grid;
  gap: 14px;
  padding: 4px 0 12px;
}
</style>
