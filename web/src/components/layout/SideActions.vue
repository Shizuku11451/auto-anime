<script setup lang="ts">
import { ref } from 'vue'
import { Download, FileUp, LogOut, Upload } from '@lucide/vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { api, http, errorMessage } from '@/api/client'
import { useAppStore } from '@/stores/app'
import { toast } from '@/stores/toast'
import { confirmDialog } from '@/stores/confirm'

withDefaults(defineProps<{ layout?: 'grid' | 'list' }>(), { layout: 'grid' })
const emit = defineEmits<{ done: [] }>()

const store = useAppStore()
const restoreInput = ref<HTMLInputElement | null>(null)
const opmlInput = ref<HTMLInputElement | null>(null)

function backup() {
  const a = document.createElement('a')
  a.href = '/api/backup'
  a.download = 'auto-anime-backup.json'
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  emit('done')
}

async function onRestore(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const ok = await confirmDialog('恢复订阅', '导入订阅与番剧配置，订阅按 RSS 地址去重；不会导入备份中的历史条目、下载器任务或媒体文件。', {
    okText: '开始恢复',
  })
  if (!ok) return
  try {
    const r = await api<{ restored: number }>('/api/restore', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: await file.text(),
    })
    toast.success(`已恢复 ${r.restored} 个订阅`)
    await store.refresh()
    emit('done')
  } catch (err) {
    toast.error(errorMessage(err))
  }
}

async function onOpml(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    const r = await api<{ imported: number }>('/api/import-opml', {
      method: 'POST',
      headers: { 'Content-Type': 'application/xml' },
      body: await file.text(),
    })
    toast.success(`已导入 ${r.imported} 个订阅`)
    await store.refresh()
    emit('done')
  } catch (err) {
    toast.error(errorMessage(err))
  }
}

async function logout() {
  try {
    await http.post('/api/logout')
  } catch {
    /* 会话可能已失效，直接回登录页 */
  }
  location.href = '/login'
}
</script>

<template>
  <div class="side-actions" :class="`l-${layout}`">
    <BaseButton size="sm" :icon="Download" @click="backup">下载备份</BaseButton>
    <BaseButton size="sm" :icon="Upload" @click="restoreInput?.click()">恢复订阅</BaseButton>
    <BaseButton size="sm" :icon="FileUp" @click="opmlInput?.click()">导入 OPML</BaseButton>
    <BaseButton v-if="store.authEnabled.value" size="sm" :icon="LogOut" @click="logout">退出登录</BaseButton>
    <input ref="restoreInput" type="file" accept="application/json,.json" hidden @change="onRestore" />
    <input ref="opmlInput" type="file" accept=".opml,application/xml,text/xml" hidden @change="onOpml" />
  </div>
</template>

<style scoped>
.side-actions {
  display: grid;
  gap: 6px;
}
.l-grid {
  grid-template-columns: 1fr 1fr;
}
.l-grid :deep(.btn) {
  justify-content: flex-start;
  padding: 0 10px;
  font-size: 12px;
}
.l-list {
  grid-template-columns: 1fr;
}
.l-list :deep(.btn) {
  justify-content: flex-start;
  height: 40px;
  font-size: 13.5px;
}
</style>
