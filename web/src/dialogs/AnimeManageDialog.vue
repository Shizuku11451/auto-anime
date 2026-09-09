<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Merge, Trash2 } from '@lucide/vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'

const dialogs = useDialogs()
const store = useAppStore()
const s = dialogs.state.animeManage
const target = ref(0)
const accepted = ref(false)
const saving = ref(false)
const error = ref('')
const merging = computed(() => s.action === 'merge')
const sources = computed(() => s.animes.filter(a => !merging.value || a.id !== target.value))
const resources = computed(() => sources.value.reduce((sum, a) => sum + a.item_count, 0))
const feeds = computed(() => store.feeds.value.filter(f => sources.value.some(a => a.title === f.target_title)))
const candidates = computed(() => store.animes.value.filter(a => s.animes.length > 1 || a.id !== s.animes[0]?.id))
const valid = computed(() => sources.value.length > 0 && accepted.value && (!merging.value || candidates.value.some(a => a.id === target.value)))
watch(() => s.open, open => {
  if (!open) return
  target.value = s.animes.length > 1 ? s.animes[0]!.id : 0
  accepted.value = false
  error.value = ''
})
watch(target, () => { accepted.value = false })
function close() { if (!saving.value) s.open = false }
async function save() {
  if (!valid.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    const result = await http.post<{ anime_count: number; item_count: number; feed_count: number }>('/api/animes/manage', {
      action: s.action, ids: sources.value.map(a => a.id), target_id: target.value, delete_items: !merging.value,
    })
    const removed = new Set(sources.value.map(a => a.id))
    if (dialogs.state.animeDetail.id && removed.has(dialogs.state.animeDetail.id)) {
      dialogs.state.animeDetail.open = false
      dialogs.state.animeDetail.data = null
      dialogs.state.animeDetail.id = null
    }
    s.open = false
    toast.success(merging.value ? `已合并 ${result.anime_count} 部番剧、${result.item_count} 条资源` : `已删除 ${result.anime_count} 部番剧、${result.item_count} 条资源记录，暂停 ${result.feed_count} 个订阅`)
    await store.refresh()
    await dialogs.reloadAnimeDetail()
  } catch (e) { error.value = errorMessage(e) }
  finally { saving.value = false }
}
</script>

<template>
  <BaseDialog :open="s.open" :title="merging ? '合并番剧' : '删除番剧'" @close="close">
    <div v-if="merging" class="field">
      <label for="am-target">合并到</label>
      <select id="am-target" v-model.number="target" :disabled="saving">
        <option :value="0" disabled>选择保留的番剧</option>
        <option v-for="a in candidates" :key="a.id" :value="a.id">{{ a.title }}</option>
      </select>
    </div>
    <p class="summary">{{ merging ? '待合并' : '待删除' }} {{ sources.length }} 部番剧 · {{ resources }} 条资源 · {{ feeds.length }} 个定向订阅</p>
    <ul class="subjects"><li v-for="a in sources" :key="a.id">{{ a.title }}</li></ul>
    <p v-if="merging" class="consequence">资源与定向订阅归入目标番剧，保留原集数与季号。来源番剧及其封面绑定、规则和存储设置将被删除，今后使用目标番剧的配置。已保存文件与本地 NFO 保持原位置和内容。</p>
    <p v-else class="consequence">删除番剧、资源记录及封面绑定，暂停并解除定向订阅的归属。磁盘文件和外部下载器任务保留。其他订阅再次同步或手动恢复订阅后，资源可能重新收录。</p>
    <label class="ack"><input v-model="accepted" type="checkbox" :disabled="saving" />{{ merging ? '确认合并并移除来源番剧' : '确认删除上述番剧及全部资源记录' }}</label>
    <p v-if="error" class="form-msg" role="alert">{{ error }}</p>
    <template #footer>
      <BaseButton :disabled="saving" @click="close">取消</BaseButton>
      <BaseButton :icon="merging ? Merge : Trash2" :variant="merging ? 'primary' : 'danger'" :disabled="!valid" :loading="saving" @click="save">{{ merging ? '合并' : '删除记录' }}</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.summary { font-weight: 600; }
.subjects { max-height: 160px; overflow: auto; padding-left: 22px; margin: 12px 0; overflow-wrap: anywhere; }
.consequence { color: var(--text-2); line-height: 1.7; }
.ack { display: flex; align-items: flex-start; gap: 10px; margin-top: 20px; }
.ack input { flex: none; margin-top: 4px; }
select { width: 100%; min-width: 0; }
</style>
