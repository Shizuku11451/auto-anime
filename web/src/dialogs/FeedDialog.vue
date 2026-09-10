<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { Eye } from '@lucide/vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { api, http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import type { Feed, PreviewResult } from '@/api/types'

const { state } = useDialogs()
const store = useAppStore()
const s = state.feed

const editing = computed(() => !!(s.feed && s.feed.id))
const form = reactive({ name: '', url: '', include: '', exclude: '', auto: true, target: '' })
const preview = ref('')
const previewing = ref(false)
const saving = ref(false)
const msg = ref('')
const previewKey = ref('')
const previewCount = ref(0)
const filterKey = computed(() => JSON.stringify([form.url.trim(), form.include.trim(), form.exclude.trim()]))
const previewReady = computed(() => previewKey.value === filterKey.value && previewCount.value > 0)
let previewRequest = 0
let previewController: AbortController | undefined
let enabledAtOpen = true

watch(
  () => s.open,
  (open) => {
    previewController?.abort()
    previewRequest++
    if (!open) return
    const f = s.feed
    form.name = f?.name ?? ''
    form.url = f?.url ?? ''
    form.include = f?.include_words ?? ''
    form.exclude = f?.exclude_words ?? ''
    form.auto = f ? f.auto_download !== false : true
    form.target = f?.target_title ?? ''
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
      json: { url, include_words: form.include.trim(), exclude_words: form.exclude.trim() },
      signal: previewController.signal,
    })
    if (request !== previewRequest) return
    previewKey.value = key
    previewCount.value = data.count
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
  } catch (e) {
    if (request !== previewRequest) return
    previewKey.value = ''
    preview.value = '预览失败：' + errorMessage(e)
  } finally {
    if (request === previewRequest) previewing.value = false
  }
}

async function save() {
  if (saving.value) return
  msg.value = ''
  if (s.feed?.from_discover && (!previewReady.value || previewing.value)) {
    msg.value = '请先预览当前规则，确认有匹配的资源后再保存'
    return
  }
  if (s.feed?.from_discover && !form.target.trim()) {
    msg.value = '请填写归入的番剧名称，或选择番剧库中已有的番剧'
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
    if (editing.value) {
      feedId = (s.feed as Feed).id
      await http.put('/api/feeds/' + feedId, body)
    } else {
      const created = await http.post<{ ok: boolean; id: number }>('/api/feeds', body)
      feedId = created.id
    }
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
      <p v-if="s.feed?.from_discover" class="field-hint">
        {{ editing ? '此搜索源已订阅：保存将调整版本并立即同步，补入尚未收录的匹配资源。已有条目和文件保留。' : '只订阅下方规则匹配的版本，同时读取来源支持的历史页。请核对预览中的字幕组、语言和集数。' }}
      </p>
      <div class="field">
        <label for="f-name">订阅名称</label>
        <input id="f-name" v-model="form.name" type="text" placeholder="例如：动漫花园 · 完结动画" autofocus />
      </div>
      <div class="field">
        <label for="f-url">RSS 地址</label>
        <input id="f-url" v-model="form.url" type="url" placeholder="https://example.com/rss.xml" :disabled="editing" spellcheck="false" />
        <p v-if="editing" class="field-hint">编辑时不能修改地址；换源请新建订阅。</p>
      </div>
      <div class="field-row">
        <div class="field">
          <label for="f-include">包含关键词（全部匹配，逗号分隔）</label>
          <input id="f-include" v-model="form.include" type="text" placeholder="例如：1080p,简中" />
        </div>
        <div class="field">
          <label for="f-exclude">排除关键词（任一匹配，逗号分隔）</label>
          <input id="f-exclude" v-model="form.exclude" type="text" placeholder="例如：480p,广告" />
        </div>
      </div>
      <div class="field">
        <label for="f-target">归入番剧</label>
        <input id="f-target" v-model="form.target" list="feed-anime-titles" type="text" maxlength="200" placeholder="留空时按每条资源标题自动识别" />
        <datalist id="feed-anime-titles">
          <option v-for="anime in store.animes.value" :key="anime.id" :value="anime.title" />
        </datalist>
        <p class="field-hint">选择已有番剧或填写统一名称，让不同标题的后续集数进入同一张番剧卡片；不会合并已有记录。</p>
      </div>
      <label class="check">
        <input v-model="form.auto" type="checkbox" />
        新条目自动发送下载
      </label>
      <Transition name="slide-up">
        <pre v-if="preview" class="pre-box">{{ preview }}</pre>
      </Transition>
      <p v-if="previewKey && previewKey !== filterKey" class="field-hint">过滤规则已修改，请重新预览。</p>
      <p class="form-msg" role="status">{{ msg }}</p>
      <button type="submit" hidden />
    </form>
    <template #footer>
      <BaseButton variant="ghost" :icon="Eye" :loading="previewing" @click="runPreview">预览匹配条目</BaseButton>
      <span class="spacer" />
      <BaseButton @click="close">取消</BaseButton>
      <BaseButton variant="primary" :loading="saving" :disabled="!!s.feed?.from_discover && (!previewReady || previewing)" @click="save">{{ editing ? (s.feed?.from_discover ? '保存并同步' : '保存') : '保存并获取' }}</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.pre-box {
  margin: 0 0 12px;
  max-height: 240px;
  overflow: auto;
}
</style>
