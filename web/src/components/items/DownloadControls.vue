<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Save, SlidersHorizontal } from '@lucide/vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useAppStore } from '@/stores/app'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import { isDownloading } from '@/utils/status'

const store = useAppStore()
const limit = ref<number | string>('')
const dirty = ref(false)
const saving = ref(false)
const error = ref('')
const currentLimit = computed(() => store.status.value?.max_concurrent_downloads)
const active = computed(() => Object.entries(store.counts.value).reduce((sum, [status, count]) => sum + (isDownloading(status) ? count : 0), 0))
const queued = computed(() => (store.counts.value['等待下载'] ?? 0) + (store.counts.value['等待继续'] ?? 0))
const paused = computed(() => store.counts.value['已暂停'] ?? 0)

watch(currentLimit, (value) => {
  if (value !== undefined && !dirty.value && !saving.value) limit.value = value
}, { immediate: true })

async function saveLimit() {
  if (saving.value || currentLimit.value === undefined) return
  error.value = ''
  const value = Number(limit.value)
  if (!Number.isInteger(value) || value < 1 || value > 32) {
    error.value = '同时下载数量必须是 1–32 的整数'
    return
  }
  saving.value = true
  try {
    await http.put('/api/settings', { max_concurrent_downloads: value })
    if (store.status.value) store.status.value = { ...store.status.value, max_concurrent_downloads: value }
    limit.value = value
    dirty.value = false
    toast.success(`同时下载数量已设为 ${value}`)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="download-controls" aria-labelledby="download-controls-title">
    <div class="control-heading">
      <div class="control-title"><SlidersHorizontal aria-hidden="true" /><h2 id="download-controls-title">下载控制</h2></div>
      <div v-if="store.status.value" class="queue-summary" aria-label="全部任务状态">
        <span><b>{{ active }}</b> 下载中</span><span><b>{{ queued }}</b> 排队中</span><span><b>{{ paused }}</b> 已暂停</span>
      </div>
    </div>
    <p v-if="currentLimit === undefined && !store.runtimeError.value" class="control-hint" role="status">正在读取下载配置…</p>
    <div v-else-if="store.runtimeError.value" class="control-error" role="alert">
      下载状态读取失败
      <BaseButton size="sm" @click="store.loadRuntime()">重新读取</BaseButton>
    </div>
    <form class="limit-form" novalidate @submit.prevent="saveLimit">
      <label for="download-limit">同时下载数量</label>
      <div class="limit-input">
        <input id="download-limit" v-model.number="limit" type="number" min="1" max="32" step="1" required :disabled="saving || currentLimit === undefined" :aria-invalid="!!error" aria-describedby="download-limit-hint" @input="dirty = true; error = ''" />
        <span>个任务</span>
      </div>
      <BaseButton type="submit" size="sm" variant="soft" :icon="Save" :loading="saving" :disabled="currentLimit === undefined || limit === currentLimit" >保存</BaseButton>
    </form>
    <p v-if="error" class="control-error" role="alert">{{ error }}</p>
    <p id="download-limit-hint" class="control-hint">范围 1–32。降低数量后，当前下载继续完成；暂停释放名额，继续后按上限排队。</p>
    <p class="control-hint">下载器任务暂停后保留已有分片；本程序的直链下载继续时从头下载。</p>
  </section>
</template>

<style scoped>
.download-controls { display: grid; gap: 12px; padding: 16px 18px; margin-bottom: 20px; background: var(--surface); border: 1px solid var(--line); border-radius: var(--r-lg); box-shadow: var(--shadow-sm); }
.control-heading { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
.control-title { display: flex; align-items: center; gap: 8px; }
.control-title svg { width: 17px; height: 17px; color: var(--accent); }
.control-title h2 { font-size: 14px; font-weight: 650; }
.queue-summary { display: flex; flex-wrap: wrap; gap: 6px 16px; color: var(--text-3); font-size: 12px; }
.queue-summary b { color: var(--text); font-weight: 650; font-variant-numeric: tabular-nums; }
.limit-form { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; font-size: 13px; }
.limit-form label { color: var(--text-2); font-weight: 550; }
.limit-input { display: flex; align-items: center; gap: 8px; color: var(--text-3); }
.limit-input input { width: 76px; min-height: 32px; padding: 5px 9px; border: 1px solid var(--line); border-radius: var(--r-sm); background: var(--surface-2); color: var(--text); font: inherit; font-variant-numeric: tabular-nums; }
.limit-input input[aria-invalid="true"] { border-color: var(--red); }
.control-hint, .control-error { font-size: 12px; line-height: 1.6; overflow-wrap: anywhere; }
.control-hint { color: var(--text-3); }
.control-error { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; color: var(--red); }
@media (max-width: 420px) {
  .download-controls { padding: 14px; }
  .limit-form { column-gap: 8px; }
  .limit-input input { width: 68px; }
  .limit-input span { display: none; }
}
</style>
