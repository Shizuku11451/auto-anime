<script setup lang="ts">
import { ref } from 'vue'
import { SlidersHorizontal } from '@lucide/vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import RuntimeCard from './RuntimeCard.vue'
import SideActions from './SideActions.vue'
const open = ref(false)
</script>

<template>
  <div class="service-tools">
    <BaseButton block variant="ghost" :icon="SlidersHorizontal" @click="open = true">服务与备份</BaseButton>
    <BaseDialog :open="open" title="服务与备份" size="sm" @close="open = false">
      <div class="tools-body">
        <RuntimeCard />
        <p class="field-hint">JSON 备份包含订阅、番剧配置及历史条目摘要（含文件路径），不包含下载器任务、账户凭据或媒体文件。恢复仅导入订阅与番剧配置，不恢复历史条目。</p>
        <SideActions layout="list" @done="open = false" />
      </div>
    </BaseDialog>
  </div>
</template>

<style scoped>
.tools-body { display: grid; gap: 16px; padding-bottom: 16px; }
.service-tools :deep(> .btn) { justify-content: flex-start; }
</style>
