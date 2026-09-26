<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import type { StorageInfo } from '@/api/types'

const { state, reloadAnimeDetail } = useDialogs()
const store = useAppStore()
const s = state.storage

const form = reactive({ dir: '', season: 1 })
const defaultPath = ref('')
const loading = ref(false)
const saving = ref(false)
const msg = ref('')

watch(
  () => s.open,
  async (open) => {
    if (!open) return
    form.dir = ''
    form.season = 1
    defaultPath.value = ''
    msg.value = ''
    loading.value = true
    try {
      const info = await http.get<StorageInfo>('/api/anime/' + encodeURIComponent(s.title) + '/storage')
      form.dir = info.storage_dir || ''
      form.season = info.season || 1
      defaultPath.value = info.default_path
    } catch (e) {
      msg.value = '读取失败：' + errorMessage(e)
    } finally {
      loading.value = false
    }
  },
)

function close() {
  s.open = false
}

async function save() {
  msg.value = ''
  const season = Number(form.season)
  saving.value = true
  try {
    const r = await http.put<{ storage_warning?: string }>('/api/anime/' + encodeURIComponent(s.title) + '/storage', {
      storage_dir: form.dir.trim(),
      season: Number.isInteger(season) && season > 0 ? season : 1,
    })
    if (r?.storage_warning) toast.error('存储位置已保存，但需要确认：' + r.storage_warning)
    else toast.success('存储位置已保存，之后归档的集数使用新位置')
    close()
    await Promise.all([store.refresh(), reloadAnimeDetail()])
  } catch (e) {
    msg.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <BaseDialog :open="s.open" title="存储位置" @close="close">
    <p class="subject">{{ s.title }}</p>
    <form :class="{ loading }" @submit.prevent="save">
      <div class="field">
        <label for="s-dir">自定义目录（留空使用默认位置）</label>
        <input id="s-dir" v-model="form.dir" type="text" placeholder="例如 /volume1/anime/番剧名" spellcheck="false" autofocus />
      </div>
      <div class="field">
        <label for="s-season">季号（用于 Season XX 目录和 SXXEYY 文件名）</label>
        <input id="s-season" v-model.number="form.season" type="number" min="1" max="999" />
      </div>
      <Transition name="fade">
        <p v-if="defaultPath" class="field-hint">默认位置：<code>{{ defaultPath }}</code></p>
      </Transition>
      <p class="form-msg" role="status">{{ msg }}</p>
      <button type="submit" hidden />
    </form>
    <template #footer>
      <BaseButton @click="close">取消</BaseButton>
      <BaseButton variant="primary" :loading="saving" :disabled="loading" @click="save">保存</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.subject {
  font-weight: 650;
  margin: 0 0 14px;
  padding: 10px 12px;
  border-radius: var(--r-sm);
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 13.5px;
  overflow-wrap: anywhere;
}
form {
  transition: opacity var(--t-med) var(--ease-out);
}
form.loading {
  opacity: 0.6;
}
</style>
