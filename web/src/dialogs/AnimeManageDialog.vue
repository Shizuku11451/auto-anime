<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Merge, Trash2 } from '@lucide/vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import { confirmDialog } from '@/stores/confirm'
import { followRelocation } from '@/composables/relocation'
import type { MediaDeleteResult, RelocationStatus } from '@/api/types'

const dialogs = useDialogs()
const store = useAppStore()
const s = dialogs.state.animeManage
const target = ref(0)
const accepted = ref(false)
const saving = ref(false)
const error = ref('')
const moveFiles = ref(true)
const deleteFiles = ref(false)
const savedCount = computed(() => sources.value.reduce((sum, a) => sum + a.saved_count, 0))
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
  moveFiles.value = true
  deleteFiles.value = false
  error.value = ''
})
watch(target, () => { accepted.value = false })
function close() { if (!saving.value) s.open = false }
async function save() {
  if (!valid.value || saving.value) return
  const removeFiles = !merging.value && deleteFiles.value && savedCount.value > 0
  if (removeFiles && !(await confirmDialog('删除文件', `将从硬盘删除 ${savedCount.value} 条已保存资源的文件，无法恢复。`, { danger: true, okText: '删除文件' }))) return
  if (!s.open) return
  saving.value = true
  error.value = ''
  try {
    const result = await http.post<{ anime_count: number; item_count: number; feed_count: number; relocation?: RelocationStatus } & MediaDeleteResult>('/api/animes/manage', {
      action: s.action, ids: sources.value.map(a => a.id), target_id: target.value, delete_items: !merging.value,
      move_files: merging.value && moveFiles.value, delete_files: removeFiles,
    })
    const removed = new Set(sources.value.map(a => a.id))
    if (dialogs.state.animeDetail.id && removed.has(dialogs.state.animeDetail.id)) {
      dialogs.state.animeDetail.open = false
      dialogs.state.animeDetail.data = null
      dialogs.state.animeDetail.id = null
    }
    if (dialogs.state.animeEdit.anime && removed.has(dialogs.state.animeEdit.anime.id)) dialogs.state.animeEdit.open = false
    s.open = false
    if (merging.value) toast.success(`已合并 ${result.anime_count} 部番剧、${result.item_count} 条资源`)
    else toast.success(`已删除 ${result.anime_count} 部番剧、${result.item_count} 条资源记录${removeFiles ? `、${result.deleted_files ?? 0} 个文件` : ''}，暂停 ${result.feed_count} 个订阅`)
    if (result.file_errors?.length) toast.error(`${result.file_errors.length} 个文件未删除：${result.file_errors[0]}`)
    if (result.relocation) followRelocation(target.value, result.relocation)
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
    <p v-if="merging" class="consequence">资源与定向订阅归入目标番剧，保留原集数与季号；来源番剧的绑定、规则和存储设置会被删除。</p>
    <p v-else class="consequence">删除番剧与资源记录，移除下载器里对应的任务（下载器中的文件保留），暂停定向订阅。其他订阅再次同步时资源可能重新收录。</p>
    <label v-if="merging && savedCount > 0" class="option"><input v-model="moveFiles" type="checkbox" :disabled="saving" />同时把 {{ savedCount }} 条已保存的文件移到目标番剧的文件夹</label>
    <label v-if="!merging && savedCount > 0" class="option"><input v-model="deleteFiles" type="checkbox" :disabled="saving" />同时删除硬盘上 {{ savedCount }} 条已保存的文件</label>
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
.option { display: flex; align-items: flex-start; gap: 10px; margin-top: 14px; }
.option input { flex: none; margin-top: 4px; }
select { width: 100%; min-width: 0; }
</style>
