<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { FolderInput, Link, Merge, RefreshCw, Trash2, WandSparkles } from '@lucide/vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs, type AnimeManageTab } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import { confirmDialog } from '@/stores/confirm'
import { startRelocation } from '@/composables/relocation'
import type { AnimeDetail, AnimeSummary, AnimeUpdateInput, MetaRematchResult, RelocationPlan } from '@/api/types'
import { storageDirExample } from '@/utils/platform'
import { localDate } from '@/components/library/schedule'

const dialogs = useDialogs()
const { state, reloadAnimeDetail } = dialogs
const store = useAppStore()
const s = state.animeEdit
const a = computed(() => s.anime)
/** 绑定、识别后读取最新资料：详情或番剧库里同一部番剧的新数据优先。 */
const live = computed<AnimeDetail | AnimeSummary | null>(() => {
  const id = a.value?.id
  if (!id) return a.value
  if (state.animeDetail.data?.id === id) return state.animeDetail.data
  return store.animes.value.find(x => x.id === id) ?? a.value
})

const tabs = computed<{ key: AnimeManageTab; label: string }[]>(() => a.value
  ? [{ key: 'basic', label: '基本信息' }, { key: 'location', label: '位置与季号' }, { key: 'metadata', label: '元数据' }, { key: 'danger', label: '合并与删除' }]
  : [{ key: 'basic', label: '基本信息' }, { key: 'location', label: '位置与季号' }])

const weekdays = ['自动获取（Bangumi）', '周一', '周二', '周三', '周四', '周五', '周六', '周日']
const form = reactive({ title: '', search_name: '', season: 1, update_weekday: 0, storage_dir: '', include_rules: '', exclude_rules: '' })
const saving = ref(false)
const applying = ref(false)
const rematching = ref(false)
const msg = ref('')
const advanced = ref(false)
const plan = ref<RelocationPlan | null>(null)
const moveFiles = ref(true)

watch(
  () => s.open,
  (open) => {
    if (!open) return
    form.title = a.value?.title || ''
    form.search_name = a.value?.search_name || ''
    form.season = a.value ? a.value.season || 0 : 1
    form.update_weekday = a.value?.update_weekday || 0
    form.storage_dir = a.value?.storage_dir || ''
    form.include_rules = a.value?.include_rules || ''
    form.exclude_rules = a.value?.exclude_rules || ''
    advanced.value = !!(form.search_name || form.include_rules || form.exclude_rules)
    moveFiles.value = true
    plan.value = null
    msg.value = ''
  },
)

const minSeason = computed(() => (a.value ? 0 : 1))
const seasonValid = computed(() => Number.isInteger(Number(form.season)) && Number(form.season) >= minSeason.value && Number(form.season) <= 999)
const storageChanged = computed(() => !!a.value && form.storage_dir.trim() !== (a.value.storage_dir || ''))
/** 名称、位置或季号变化会改变已保存文件应在的目录。 */
const locationChanged = computed(() => !!a.value && (
  form.title.trim() !== a.value.title || storageChanged.value || Number(form.season) !== (a.value.season || 0)
))
const willMove = computed(() => !!a.value && a.value.saved_count > 0 && locationChanged.value && moveFiles.value)
const relocating = computed(() => !!a.value && state.relocation.animeId === a.value.id && !!state.relocation.status?.running)

let previewTimer: ReturnType<typeof setTimeout> | undefined
let previewVersion = 0
watch(
  () => [s.open, form.title, form.storage_dir, form.season] as const,
  () => {
    clearTimeout(previewTimer)
    const version = ++previewVersion
    plan.value = null
    const anime = a.value
    if (!s.open || !anime || !anime.saved_count || !locationChanged.value || !seasonValid.value || !form.title.trim()) return
    const body = { title: form.title.trim(), storage_dir: form.storage_dir.trim(), season: Number(form.season) }
    previewTimer = setTimeout(async () => {
      try {
        const next = await http.post<RelocationPlan>(`/api/animes/${anime.id}/relocation-preview`, body)
        if (version === previewVersion && s.open) plan.value = next
      } catch {
        /* 预估失败不影响保存，保存后仍按实际情况移动。 */
      }
    }, 350)
  },
)

const metaState = computed(() => {
  const x = live.value
  if (!x) return { label: '', detail: '' }
  const name = x.meta_name_cn || x.meta_name
  if (x.bangumi_id > 0 || ('meta_source' in x && x.meta_source === 'manual')) {
    return { label: '手动绑定', detail: [x.bangumi_id > 0 ? `Bangumi #${x.bangumi_id}` : '', name].filter(Boolean).join(' · ') }
  }
  const subjectId = 'meta_subject_id' in x ? x.meta_subject_id ?? 0 : 0
  if (subjectId > 0 || name) {
    return { label: '自动识别', detail: [subjectId > 0 ? `Bangumi #${subjectId}` : '', name].filter(Boolean).join(' · ') }
  }
  return { label: '未匹配', detail: '' }
})
const isManual = computed(() => metaState.value.label === '手动绑定')

const weekdayHint = computed(() => {
  const x = live.value
  if (x?.broadcast_end && x.broadcast_end < localDate()) return `Bangumi：已于 ${x.broadcast_end} 完结`
  if (x?.broadcast_weekday) return `Bangumi：每${weekdays[x.broadcast_weekday]}放送`
  return ''
})

function close() {
  if (saving.value || applying.value) return
  s.open = false
}

function body(): AnimeUpdateInput {
  const out: AnimeUpdateInput = {
    title: form.title.trim(),
    search_name: form.search_name.trim(),
    season: Number(form.season),
    update_weekday: Number(form.update_weekday),
    include_rules: form.include_rules.trim(),
    exclude_rules: form.exclude_rules.trim(),
  }
  // 位置未改时不发送，服务端才会在改名时顺带重命名空的番剧文件夹。
  if (!a.value || storageChanged.value) out.storage_dir = form.storage_dir.trim()
  return out
}

function validate() {
  if (!form.title.trim()) {
    msg.value = '请填写番剧名称'
    s.tab = 'basic'
    return false
  }
  if (!seasonValid.value) {
    msg.value = a.value ? '默认季号必须是 0-999 的整数' : '默认季号必须是 1-999 的整数'
    s.tab = 'location'
    return false
  }
  return true
}

async function afterChange() {
  await Promise.all([store.refresh(), reloadAnimeDetail()])
}

async function save() {
  msg.value = ''
  if (saving.value || applying.value || !validate()) return
  const anime = a.value
  const move = willMove.value
  saving.value = true
  try {
    const saved = anime
      ? await http.put<AnimeSummary>('/api/animes/' + anime.id, body())
      : await http.post<AnimeSummary>('/api/animes', body())
    if (saved?.storage_warning) toast.error('番剧已保存，但存储位置需要确认：' + saved.storage_warning)
    else toast.success(anime ? (move ? '番剧已保存，正在移动文件' : '番剧已保存') : '番剧已创建')
    s.open = false
    if (anime && move) await startRelocation(anime.id)
    await afterChange()
  } catch (e) {
    msg.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

/** 先保存再应用，保证回溯用的是最新规则 */
async function apply() {
  if (saving.value || applying.value) return
  msg.value = ''
  const anime = a.value
  if (!anime) return
  if (!form.include_rules.trim()) {
    msg.value = '请先填写包含规则'
    return
  }
  if (!validate()) return
  const move = willMove.value
  applying.value = true
  try {
    await http.put('/api/animes/' + anime.id, body())
    const r = await http.post<{ matched: number }>(`/api/animes/${anime.id}/apply-rules`)
    toast.success(`规则已应用，归并了 ${r.matched} 条现有条目`)
    s.open = false
    if (move) await startRelocation(anime.id)
    await afterChange()
  } catch (e) {
    msg.value = errorMessage(e)
  } finally {
    applying.value = false
  }
}

/** 恢复自动识别：手动绑定的先解除绑定，否则清除当前匹配重新识别。 */
async function restoreAuto() {
  const anime = live.value
  if (!anime || rematching.value) return
  const manual = isManual.value
  const ok = await confirmDialog(
    '恢复自动识别',
    manual ? `解除「${anime.title}」的手动绑定，之后按名称和搜索词自动识别。` : `清除「${anime.title}」当前的封面和元数据匹配，并在后台重新识别。`,
    { okText: manual ? '解除绑定' : '重新识别' },
  )
  if (!ok || !s.open || s.anime?.id !== anime.id) return
  rematching.value = true
  try {
    if (manual) await http.put(`/api/animes/${anime.id}/metadata`, { bangumi_id: 0 })
    else await http.post<MetaRematchResult>(`/api/animes/${anime.id}/rematch`)
    toast.success(manual ? '已解除绑定，正在自动识别' : '已加入重新识别队列，结果将自动刷新')
    await afterChange()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    rematching.value = false
  }
}
</script>

<template>
  <BaseDialog :open="s.open" :title="a ? '管理番剧' : '新建番剧'" size="lg" @close="close">
    <p v-if="a" class="subject">{{ a.title }}</p>
    <div class="tabs" role="tablist" aria-label="番剧设置">
      <button
        v-for="t in tabs"
        :id="`ae-tab-${t.key}`"
        :key="t.key"
        type="button"
        role="tab"
        class="tab"
        :class="{ on: s.tab === t.key }"
        :aria-selected="s.tab === t.key"
        :aria-controls="`ae-panel-${t.key}`"
        @click="s.tab = t.key"
      >{{ t.label }}</button>
    </div>

    <form @submit.prevent="save">
      <div v-show="s.tab === 'basic'" id="ae-panel-basic" role="tabpanel" aria-labelledby="ae-tab-basic">
        <div class="field">
          <label for="ae-name">番剧名称</label>
          <input id="ae-name" v-model="form.title" type="text" maxlength="200" spellcheck="false" autofocus />
          <p class="field-hint">也是番剧文件夹名</p>
        </div>
        <div class="field">
          <label for="ae-weekday">每周更新日</label>
          <select id="ae-weekday" v-model.number="form.update_weekday">
            <option v-for="(day, index) in weekdays" :key="index" :value="index">{{ day }}</option>
          </select>
          <p v-if="weekdayHint" class="field-hint">{{ weekdayHint }}</p>
        </div>
        <details class="advanced" :open="advanced" @toggle="advanced = ($event.target as HTMLDetailsElement).open">
          <summary>高级</summary>
          <div class="field">
            <label for="ae-search">元数据搜索词</label>
            <input id="ae-search" v-model="form.search_name" type="text" placeholder="留空用番剧名" spellcheck="false" />
          </div>
          <div class="field-row">
            <div class="field">
              <label for="ae-include">包含规则（全部匹配）</label>
              <input id="ae-include" v-model="form.include_rules" type="text" placeholder="逗号分隔" />
            </div>
            <div class="field">
              <label for="ae-exclude">排除规则（任一匹配）</label>
              <input id="ae-exclude" v-model="form.exclude_rules" type="text" placeholder="例如 480p,预告" />
            </div>
          </div>
          <p class="field-hint">任意订阅的标题命中规则即归入本番剧，已调整过的条目除外</p>
          <BaseButton v-if="a" size="sm" :icon="WandSparkles" :loading="applying" :disabled="saving" @click="apply">保存并应用到现有条目</BaseButton>
        </details>
      </div>

      <div v-show="s.tab === 'location'" id="ae-panel-location" role="tabpanel" aria-labelledby="ae-tab-location">
        <div class="field">
          <label for="ae-dir">存储位置</label>
          <input id="ae-dir" v-model="form.storage_dir" type="text" :placeholder="`留空使用默认位置，例如 ${storageDirExample}`" spellcheck="false" />
        </div>
        <div class="field">
          <label for="ae-season">默认季号</label>
          <input id="ae-season" v-model.number="form.season" type="number" :min="minSeason" max="999" />
          <p class="field-hint">{{ a ? '0 表示按标题识别；单集调整过的季号优先' : '用于 Season XX 目录与文件名' }}</p>
        </div>
      </div>

      <div v-if="a" v-show="s.tab === 'metadata'" id="ae-panel-metadata" role="tabpanel" aria-labelledby="ae-tab-metadata">
        <div class="meta-state">
          <span class="tag" :class="{ accent: metaState.label !== '未匹配' }">{{ metaState.label }}</span>
          <span v-if="metaState.detail">{{ metaState.detail }}</span>
        </div>
        <div class="actions">
          <BaseButton :icon="Link" @click="live && dialogs.openBind(live)">{{ isManual ? '更换绑定' : '搜索并绑定' }}</BaseButton>
          <BaseButton :icon="RefreshCw" :loading="rematching" @click="restoreAuto">{{ isManual ? '恢复自动识别' : '重新识别' }}</BaseButton>
        </div>
      </div>

      <div v-if="a" v-show="s.tab === 'danger'" id="ae-panel-danger" role="tabpanel" aria-labelledby="ae-tab-danger" class="danger">
        <div class="danger-row">
          <div>
            <b>合并到其他番剧</b>
            <p class="field-hint">资源与订阅归入目标番剧，可同时移动文件</p>
          </div>
          <BaseButton :icon="Merge" :disabled="store.animes.value.length < 2" @click="live && dialogs.openAnimeManage('merge', [live])">合并</BaseButton>
        </div>
        <div class="danger-row">
          <div>
            <b>删除番剧</b>
            <p class="field-hint">删除记录；硬盘上的文件默认保留</p>
          </div>
          <BaseButton :icon="Trash2" variant="danger" @click="live && dialogs.openAnimeManage('delete', [live])">删除</BaseButton>
        </div>
      </div>

      <div v-if="a && a.saved_count > 0 && locationChanged && plan && plan.moving > 0" class="relocate" role="group" aria-label="已保存的文件">
        <p class="relocate-head"><FolderInput :size="16" aria-hidden="true" />{{ plan.moving }} 集已保存的文件不在新位置</p>
        <p class="relocate-path"><span>{{ plan.from.join('、') }}</span> → <span>{{ plan.to.join('、') }}</span></p>
        <label class="check"><input v-model="moveFiles" type="radio" :value="true" />移动已有文件</label>
        <label class="check"><input v-model="moveFiles" type="radio" :value="false" />只影响之后的下载</label>
        <p v-if="relocating" class="field-hint">该番剧正在移动文件，完成后再修改位置</p>
      </div>

      <p class="form-msg" role="status">{{ msg }}</p>
      <button type="submit" hidden />
    </form>
    <template #footer>
      <BaseButton :disabled="saving || applying" @click="close">取消</BaseButton>
      <BaseButton variant="primary" :loading="saving" :disabled="applying || (willMove && relocating)" @click="save">{{ a ? '保存' : '创建' }}</BaseButton>
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
.tabs {
  display: flex;
  gap: 4px;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--line);
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: none;
}
.tab {
  flex: none;
  padding: 8px 12px;
  border: 0;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  background: none;
  color: var(--text-2);
  font: inherit;
  font-size: 13.5px;
  cursor: pointer;
}
.tab:hover { color: var(--text); }
.tab.on { color: var(--accent); border-bottom-color: var(--accent); font-weight: 600; }
.advanced { margin-top: 4px; }
.advanced > summary { font-size: 13px; color: var(--text-2); }
.advanced[open] > summary { margin-bottom: 10px; }
.advanced .field-hint { margin-bottom: 10px; }
.meta-state { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; font-size: 13.5px; overflow-wrap: anywhere; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
.danger { display: grid; gap: 10px; }
.danger-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: var(--r-md);
}
.danger-row b { font-size: 13.5px; }
.danger-row .field-hint { margin: 2px 0 0; }
.relocate {
  display: grid;
  gap: 6px;
  margin-top: 14px;
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  background: var(--surface-2);
  font-size: 13px;
}
.relocate-head { display: flex; align-items: center; gap: 8px; font-weight: 600; }
.relocate-path { color: var(--text-3); font-size: 12px; overflow-wrap: anywhere; font-family: var(--mono); }
.relocate .check { display: flex; margin: 0; }
</style>
