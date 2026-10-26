<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ChevronDown, CircleAlert, CircleCheck, CircleHelp, Eye, Radar } from '@lucide/vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { api, http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import type { AvailabilityItem, AvailabilityVerdict, Feed, PreviewResult } from '@/api/types'
import { isAbsoluteStoragePath, isWindows, storageDirExample } from '@/utils/platform'

const { state } = useDialogs()
const store = useAppStore()
/* 当前运行平台（/api/status）：Windows 与容器/Linux 的路径示例、映射说明不同。 */
const platformOS = computed(() => store.platformOS.value)
const pathExample = computed(() => storageDirExample(platformOS.value))
const isWindowsOS = computed(() => isWindows(platformOS.value))
const s = state.feed

const editing = computed(() => !!(s.feed && s.feed.id))
const form = reactive({ name: '', url: '', include: '', exclude: '', auto: true, target: '', storage: '', season: 0, allowUnknown: false })
const preview = ref('')
const previewing = ref(false)
const saving = ref(false)
const msg = ref('')
const previewKey = ref('')
const previewCount = ref(0)
const seasonConfirmed = ref(false)
const filterKey = computed(() => JSON.stringify([form.url.trim(), form.include.trim(), form.exclude.trim(), form.season, form.allowUnknown]))
const previewReady = computed(() => previewKey.value === filterKey.value && previewCount.value > 0)
const seasonMode = computed({
  get: () => form.season === 0 ? '0' : form.season === -1 ? '-1' : 'specific',
  set: (value: string) => { form.season = value === 'specific' ? Math.max(1, form.season) : Number(value) },
})
const filterSummary = computed(() => [
  form.season > 0 ? `第 ${form.season} 季${form.allowUnknown ? '（含未标明）' : ''}` : form.season === -1 ? '仅季数未标明' : '不限季数',
  form.include.trim() ? '已设包含词' : '',
  form.exclude.trim() ? '已设排除词' : '',
].filter(Boolean).join(' · '))
let previewRequest = 0
let previewController: AbortController | undefined
let enabledAtOpen = true

// 保存位置是「番剧根目录」：归档时在其下按番剧名建立子文件夹。留空表示使用默认
// 位置（设置页的媒体库目录，未设置时为下载目录），所以界面要写出实际落点。
const trimSlash = (value: string) => value.trim().replace(/[\\/]+$/, '')
const defaultBase = computed(() => trimSlash(store.status?.value?.library_dir || store.status?.value?.download_dir || ''))
const defaultBaseSource = computed(() => (store.status?.value?.library_dir ? '媒体库目录' : '下载目录'))
const saveBase = computed(() => trimSlash(form.storage))
// 与服务端 safeComponent 一致地替换非法字符，预览里不显示会逃出目录的名字。
const folderName = computed(() => form.target.trim().replace(/[\\/:*?"<>|]/g, '_').replace(/^[ .]+|[ .]+$/g, ''))
const storagePreview = computed(() => {
  const base = saveBase.value || defaultBase.value
  return base && folderName.value ? `${base}/${folderName.value}` : ''
})

// 资源可用性检测（找番添加流程）
const previewItems = ref<PreviewResult['items']>([])
const availability = ref<Record<string, AvailabilityItem>>({})
const availabilityRunning = ref(false)
const availabilityDone = ref(false)
let availabilityRequest = 0
let availabilityController: AbortController | undefined

const btLinks = computed(() =>
  previewItems.value
    .map((i) => i.link)
    .filter((link) => {
      try {
        const url = new URL(link)
        return url.protocol === 'magnet:' ||
          ((url.protocol === 'http:' || url.protocol === 'https:') && url.pathname.toLowerCase().endsWith('.torrent'))
      } catch {
        return false
      }
    })
    .slice(0, 20),
)

const availabilitySummary = computed(() => {
  const items = btLinks.value.map((link) => availability.value[link]).filter(Boolean)
  if (!items.length) return ''
  const alive = items.filter((r) => r.verdict === 'available')
  const dead = items.filter((r) => r.verdict === 'no_seeds')
  const blind = items.filter((r) => r.verdict === 'unreachable' || r.verdict === 'unknown')
  const parts = [`${items.length} 条资源中 ${alive.length} 条有做种者`]
  if (dead.length) parts.push(`${dead.length} 条当前无做种`)
  if (blind.length) parts.push(`${blind.length} 条无法判断`)
  return parts.join('，')
})

const verdictMeta: Record<AvailabilityVerdict, { label: string; cls: string }> = {
  available: { label: '有做种', cls: 'ok' },
  no_seeds: { label: '无做种', cls: 'dead' },
  unreachable: { label: '检测不到', cls: 'blind' },
  unknown: { label: '无法检测', cls: 'blind' },
}

function resetAvailability() {
  availabilityRequest++
  availabilityController?.abort()
  availabilityController = undefined
  availability.value = {}
  availabilityDone.value = false
  availabilityRunning.value = false
  previewItems.value = []
}

watch(filterKey, resetAvailability)

watch(
  () => s.open,
  (open) => {
    previewController?.abort()
    previewRequest++
    resetAvailability()
    if (!open) return
    const f = s.feed
    form.name = f?.name ?? ''
    form.url = f?.url ?? ''
    form.include = f?.include_words ?? ''
    form.exclude = f?.exclude_words ?? ''
    form.auto = f ? f.auto_download !== false : true
    form.target = f?.target_title ?? ''
    form.storage = f?.storage_dir ?? ''
    form.season = f?.season_filter ?? 0
    form.allowUnknown = f?.allow_unknown_season ?? false
    seasonConfirmed.value = false
    enabledAtOpen = f ? f.enabled !== false : true
    preview.value = ''
    previewKey.value = ''
    previewCount.value = 0
    previewing.value = false
    msg.value = ''
    if (f?.from_discover) void runPreview()
  },
)

function close() {
  s.open = false
}

async function runPreview() {
  previewController?.abort()
  resetAvailability()
  const request = ++previewRequest
  const key = filterKey.value
  const url = form.url.trim()
  if (!url) {
    msg.value = '请先填写 RSS 地址'
    return
  }
  msg.value = ''
  previewing.value = true
  preview.value = '正在读取 RSS，支持翻页的来源会继续查找早期集数…'
  previewController = new AbortController()
  try {
    const data = await api<PreviewResult>('/api/preview', {
      method: 'POST',
      json: { url, include_words: form.include.trim(), exclude_words: form.exclude.trim(), season_filter: form.season, allow_unknown_season: form.allowUnknown },
      signal: previewController.signal,
    })
    if (request !== previewRequest) return
    previewKey.value = key
    previewCount.value = data.count
    if (key === filterKey.value) previewItems.value = data.items
    preview.value = data.count
      ? `共读取 ${data.pages} 页、${data.total} 条，过滤后匹配 ${data.count} 条（已入库的资源不会重复添加）：\n` +
        data.items
          .map((i) => '· ' + i.title)
          .join('\n') +
        (data.count > data.items.length ? '\n仅展示前 20 条' : '')
      : `共读取 ${data.pages} 页、${data.total} 条，没有符合过滤规则的资源，请调整关键词后重新预览`
    preview.value += '\n\n' + (data.warning || (data.history
      ? '保存后同步同样的历史页；以上为当前来源已发现的资源，不代表全季已发布。'
      : '此来源仅提供当前 RSS 列表，未找到的早期集数可换源或缩小搜索范围。'))
    if (s.feed?.from_discover && btLinks.value.length) void runAvailability()
  } catch (e) {
    if (request !== previewRequest) return
    previewKey.value = ''
    preview.value = '预览失败：' + errorMessage(e)
  } finally {
    if (request === previewRequest) previewing.value = false
  }
}

async function runAvailability() {
  if (availabilityRunning.value || !previewReady.value || !btLinks.value.length) return
  const request = ++availabilityRequest
  const controller = new AbortController()
  availabilityController = controller
  availabilityRunning.value = true
  try {
    const data = await api<{ results: AvailabilityItem[] }>('/api/discover/availability', {
      method: 'POST',
      json: { urls: [...btLinks.value] },
      signal: controller.signal,
    })
    if (request !== availabilityRequest) return
    const map: Record<string, AvailabilityItem> = {}
    for (const item of data.results) map[item.url] = item
    availability.value = map
    availabilityDone.value = true
  } catch (e) {
    if (request !== availabilityRequest) return
    toast.error('可用性检测失败：' + errorMessage(e))
  } finally {
    if (request === availabilityRequest) {
      availabilityRunning.value = false
      availabilityController = undefined
    }
  }
}

type FeedSaveResult = { storage_path?: string; storage_warning?: string }

// 与后端一致的绝对路径判断：容器内是 /...，Windows 本机要求盘符或 UNC 共享。
function isAbsolutePath(value: string) {
  return isAbsoluteStoragePath(value, platformOS.value)
}

async function save() {
  if (saving.value) return
  msg.value = ''
  if (s.feed?.requires_season_confirmation && !seasonConfirmed.value) {
    msg.value = '该资源没有明确季数，请核对原始标题并确认后再保存'
    return
  }
  if (s.feed?.from_discover && (!previewReady.value || previewing.value)) {
    msg.value = '请先预览当前规则，确认有匹配的资源后再保存'
    return
  }
  if (s.feed?.from_discover && !form.target.trim()) {
    msg.value = '请填写归入的番剧名称，或选择番剧库中已有的番剧'
    return
  }
  if (!Number.isInteger(form.season) || form.season < -1 || form.season > 999) {
    msg.value = '请展开筛选规则，填写 1–999 之间的整数季号，或选择不限季数、仅季数未标明'
    return
  }
  const storage = form.storage.trim()
  if (storage && !isAbsolutePath(storage)) {
    msg.value = `保存位置必须是绝对路径，例如 ${pathExample.value}`
    return
  }
  if (storage.split(/[\\/]+/).includes('..')) {
    msg.value = '保存位置不能包含 ..'
    return
  }
  const body = {
    name: form.name.trim(),
    url: form.url.trim(),
    include_words: form.include.trim(),
    exclude_words: form.exclude.trim(),
    auto_download: form.auto,
    enabled: enabledAtOpen,
    target_title: form.target.trim(),
    storage_dir: storage,
    season_filter: form.season,
    allow_unknown_season: form.allowUnknown,
  }
  if (!body.name) {
    msg.value = '请填写订阅名称'
    return
  }
  if (!editing.value && !body.url) {
    msg.value = '请填写 RSS 地址'
    return
  }
  saving.value = true
  try {
    let feedId: number
    let storagePath = ''
    let storageWarning = ''
    if (editing.value) {
      feedId = (s.feed as Feed).id
      const saved = await http.put<FeedSaveResult>('/api/feeds/' + feedId, body)
      storagePath = saved?.storage_path ?? ''
      storageWarning = saved?.storage_warning ?? ''
    } else {
      const created = await http.post<FeedSaveResult & { id: number }>('/api/feeds', body)
      feedId = created.id
      storagePath = created.storage_path ?? ''
      storageWarning = created.storage_warning ?? ''
    }
    if (storageWarning) toast.error('订阅已保存，但保存位置需要确认：' + storageWarning)
    else if (storagePath) toast.info('番剧文件夹：' + storagePath + '/')
    if (feedId && (!editing.value || s.feed?.from_discover)) {
      try {
        const r = await http.post<{ added: number }>('/api/sync/' + feedId)
        toast.success(`订阅已保存并完成同步，新增 ${r.added} 条`)
      } catch (e) {
        toast.error('订阅已保存，但同步未完整完成：' + errorMessage(e))
      }
    } else {
      toast.success('订阅已保存')
    }
    close()
    await store.refresh()
  } catch (e) {
    msg.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <BaseDialog :open="s.open" :title="editing ? '编辑订阅' : '添加订阅'" @close="close">
    <form @submit.prevent="save">
      <p v-if="s.feed?.from_discover" class="intro field-hint">
        {{ editing ? '此 RSS 已订阅，保存将更新规则并同步，保留已有条目和文件。' : '已带入所选版本，核对下方匹配资源后即可订阅。' }}
      </p>
      <div class="field">
        <label for="f-name">订阅名称</label>
        <input id="f-name" v-model="form.name" type="text" placeholder="例如：我的追番订阅" autofocus />
      </div>
      <div v-if="!s.feed?.from_discover" class="field">
        <label for="f-url">RSS 地址</label>
        <input id="f-url" v-model="form.url" type="url" placeholder="https://example.com/rss.xml" :disabled="editing" spellcheck="false" />
        <p v-if="editing" class="field-hint">编辑时不能修改地址；换源请新建订阅。</p>
      </div>
      <div class="field">
        <label for="f-target">归入番剧</label>
        <input id="f-target" v-model="form.target" list="feed-anime-titles" type="text" maxlength="200" placeholder="留空时按每条资源标题自动识别" />
        <datalist id="feed-anime-titles">
          <option v-for="anime in store.animes.value" :key="anime.id" :value="anime.title" />
        </datalist>
        <p class="field-hint">{{ s.feed?.from_discover ? '后续集数归入此番剧，可选择已有番剧；不会合并已有记录。' : '后续集数归入此番剧；留空自动识别，不合并已有记录。' }}</p>
      </div>
      <label class="check">
        <input v-model="form.auto" type="checkbox" />
        新条目自动发送下载
      </label>
      <div class="options">
        <details class="option-section">
          <summary tabindex="0"><span>筛选规则<small>{{ filterSummary }}</small></span><ChevronDown aria-hidden="true" /></summary>
          <div class="option-body">
            <div class="field-row">
              <div class="field">
                <label for="f-include">包含关键词</label>
                <input id="f-include" v-model="form.include" type="text" placeholder="例如：1080p,简中" />
                <p class="field-hint">逗号分隔，需全部匹配。</p>
              </div>
              <div class="field">
                <label for="f-exclude">排除关键词</label>
                <input id="f-exclude" v-model="form.exclude" type="text" placeholder="例如：480p,广告" />
                <p class="field-hint">逗号分隔，任一匹配即排除。</p>
              </div>
            </div>
            <div class="field">
              <label for="f-season">季数条件</label>
              <div class="season-fields">
                <select id="f-season" v-model="seasonMode">
                  <option value="0">不限季数</option>
                  <option value="-1">仅季数未标明</option>
                  <option value="specific">指定季数</option>
                </select>
                <input v-if="seasonMode === 'specific'" v-model.number="form.season" aria-label="指定季号" type="number" min="1" max="999" step="1" />
              </div>
              <p class="field-hint">仅筛选资源，不修改归档季号；指定季数不接受其他季或冲突资源。</p>
              <label v-if="form.season > 0" class="check"><input v-model="form.allowUnknown" type="checkbox" /> 已核对，同时接受季数未标明的资源</label>
            </div>
          </div>
        </details>
        <details class="option-section">
          <summary tabindex="0"><span>来源与保存位置<small>{{ saveBase ? '自定义目录' : `默认${defaultBaseSource}` }}</small></span><ChevronDown aria-hidden="true" /></summary>
          <div class="option-body">
            <div v-if="s.feed?.from_discover" class="field">
              <label for="f-url">RSS 地址</label>
              <input id="f-url" v-model="form.url" type="url" placeholder="https://example.com/rss.xml" :disabled="editing" spellcheck="false" />
              <p v-if="editing" class="field-hint">编辑时不能修改地址；换源请新建订阅。</p>
            </div>
            <div class="field">
              <label for="f-storage">保存位置（番剧根目录）</label>
              <input id="f-storage" v-model="form.storage" type="text" spellcheck="false" :placeholder="`留空使用默认位置，例如 ${pathExample}`" />
              <p class="field-hint">
                按 <code>根目录/番剧名/Season 01/…</code> 归档，留空使用默认位置。
                <template v-if="isWindowsOS">网络共享需当前用户有写入权限。</template>
                <template v-else>自定义目录需映射进容器，否则 NAS 上看不到文件。</template>
              </p>
              <p v-if="storagePreview" class="field-hint">实际保存到：<code>{{ storagePreview }}/</code></p>
              <p v-else-if="saveBase" class="field-hint">识别番剧后，在此目录下建立番剧文件夹。</p>
              <p v-else-if="!defaultBase" class="field-hint">未读取到默认位置，请填写绝对路径，或先在设置页配置媒体库目录。</p>
            </div>
          </div>
        </details>
      </div>
      <div v-if="s.feed?.requires_season_confirmation" class="season-confirmation">
        <p>此版本未标明季数，请核对原始标题。确认不会指定归档季号。</p>
        <label class="check"><input v-model="seasonConfirmed" type="checkbox" /> 已核对，这是我要订阅的版本</label>
      </div>
      <h3 v-if="preview" class="preview-title">匹配预览</h3>
      <Transition name="slide-up">
        <pre v-if="preview" class="pre-box">{{ preview }}</pre>
      </Transition>
      <div v-if="s.feed?.from_discover && btLinks.length" class="avail-box">
        <div class="avail-head">
          <span class="avail-title">资源可用性检测</span>
          <BaseButton variant="ghost" size="sm" :icon="Radar" :loading="availabilityRunning" @click="runAvailability">
            {{ availabilityDone ? '重新检测' : '检测可用性' }}
          </BaseButton>
        </div>
        <p v-if="availabilityRunning" class="field-hint">正在查询做种状态，每条资源最长约 15 秒，批量检测会分批执行…</p>
        <template v-else-if="availabilityDone">
          <p class="avail-summary" :class="{ warn: btLinks.some((l) => availability[l]?.verdict === 'no_seeds') }">
            {{ availabilitySummary }}
          </p>
          <details class="availability-details">
            <summary tabindex="0">查看逐条检测结果</summary>
            <ul class="avail-list">
              <li v-for="link in btLinks" :key="link">
                <span class="avail-badge" :class="verdictMeta[availability[link]?.verdict ?? 'unknown'].cls">
                  <CircleCheck v-if="availability[link]?.verdict === 'available'" />
                  <CircleAlert v-else-if="availability[link]?.verdict === 'no_seeds'" />
                  <CircleHelp v-else />
                  {{ verdictMeta[availability[link]?.verdict ?? 'unknown'].label }}
                  <em v-if="availability[link]?.verdict === 'available'">× {{ availability[link].seeds }}</em>
                </span>
                <span class="avail-name" :title="previewItems.find((i) => i.link === link)?.title">
                  {{ previewItems.find((i) => i.link === link)?.title }}
                </span>
              </li>
            </ul>
            <p class="field-hint">做种数来自 tracker，仅供参考；无做种时保留任务，等待做种者上线。</p>
          </details>
        </template>
      </div>
      <p v-if="previewKey && previewKey !== filterKey" class="field-hint">过滤规则已修改，请重新预览。</p>
      <p v-if="msg" class="form-msg" role="status">{{ msg }}</p>
      <button type="submit" hidden />
    </form>
    <template #footer>
      <BaseButton variant="ghost" :icon="Eye" :loading="previewing" @click="runPreview">预览</BaseButton>
      <span class="spacer" />
      <BaseButton @click="close">取消</BaseButton>
      <BaseButton variant="primary" :loading="saving" :disabled="!!s.feed?.from_discover && (!previewReady || previewing)" @click="save">{{ editing ? (s.feed?.from_discover ? '保存并同步' : '保存') : '保存并获取' }}</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.intro { margin-bottom: 16px; }
.options { margin-bottom: 16px; border-block: 1px solid var(--line); }
.option-section + .option-section { border-top: 1px solid var(--line); }
.option-section > summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 0;
  list-style: none;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
}
.option-section > summary::-webkit-details-marker { display: none; }
.option-section > summary span { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 12px; min-width: 0; }
.option-section small { color: var(--text-3); font-size: 12px; font-weight: 400; overflow-wrap: anywhere; }
.option-section > summary svg { width: 15px; height: 15px; flex: none; color: var(--text-3); }
.option-section[open] > summary svg { transform: rotate(180deg); }
summary:focus-visible { outline: 2px solid var(--accent-border); outline-offset: 2px; border-radius: var(--r-sm); }
.option-body { padding: 4px 0; }
.option-body .field-hint { overflow-wrap: anywhere; }
.season-fields { display: flex; gap: 8px; }
.season-fields select { flex: 1; min-width: 0; }
.season-fields input { width: 90px; }
.season-confirmation { padding: 12px; margin-bottom: 16px; border-radius: var(--r-sm); background: var(--amber-soft); color: var(--amber); font-size: 12px; line-height: 1.6; }
.season-confirmation .check { margin: 8px 0 0; align-items: flex-start; }
.season-confirmation input { flex: none; margin-top: 4px; }
.preview-title { margin-bottom: 8px; font-size: 13px; font-weight: 600; }
.availability-details > summary { cursor: pointer; font-size: 12px; color: var(--text-2); margin-bottom: 8px; }
.pre-box {
  margin: 0 0 12px;
  max-height: 180px;
  overflow: auto;
}

.avail-box {
  margin: 0 0 12px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--surface-2);
}

.avail-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
}

.avail-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}

.avail-summary {
  margin: 2px 0 8px;
  font-size: 12.5px;
  color: var(--text-2);
}

.avail-summary.warn {
  color: var(--amber);
}

.avail-list {
  list-style: none;
  margin: 0 0 8px;
  padding: 0;
  max-height: 168px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.avail-list li {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.avail-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: none;
  font-size: 11.5px;
  line-height: 1;
  padding: 4px 8px;
  border-radius: 999px;
  border: 1px solid var(--line);
  white-space: nowrap;
}

.avail-badge svg {
  width: 12px;
  height: 12px;
}

.avail-badge em {
  font-style: normal;
  font-weight: 600;
}

.avail-badge.ok {
  color: var(--green);
}

.avail-badge.dead {
  color: var(--red);
}

.avail-badge.blind {
  color: var(--text-3);
}

.avail-name {
  font-size: 12px;
  color: var(--text-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
</style>
