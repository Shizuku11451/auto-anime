<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { WandSparkles } from '@lucide/vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import type { AnimeSummary, AnimeUpdateInput } from '@/api/types'
import { storageDirExample } from '@/utils/platform'

const { state, reloadAnimeDetail } = useDialogs()
const store = useAppStore()
const s = state.animeEdit
const a = computed(() => s.anime)
/* Windows 与容器/Linux 的绝对路径示例不同（盘符/UNC vs NAS 挂载）。 */
const pathExample = computed(() => storageDirExample(store.platformOS.value))

const weekdays = ['自动获取（Bangumi）', '周一', '周二', '周三', '周四', '周五', '周六', '周日']
const form = reactive({ title: '', search_name: '', season: 1, update_weekday: 0, storage_dir: '', include_rules: '', exclude_rules: '' })
const saving = ref(false)
const applying = ref(false)
const msg = ref('')

watch(
  () => s.open,
  (open) => {
    if (!open) return
    form.title = a.value?.title || ''
    form.search_name = a.value?.search_name || ''
    form.season = a.value?.season || 1
    form.update_weekday = a.value?.update_weekday || 0
    form.storage_dir = a.value?.storage_dir || ''
    form.include_rules = a.value?.include_rules || ''
    form.exclude_rules = a.value?.exclude_rules || ''
    msg.value = ''
  },
)

function close() {
  if (saving.value || applying.value) return
  s.open = false
}

function body(): AnimeUpdateInput {
  return {
    title: form.title.trim(),
    search_name: form.search_name.trim(),
    season: Number(form.season),
    update_weekday: Number(form.update_weekday),
    storage_dir: form.storage_dir.trim(),
    include_rules: form.include_rules.trim(),
    exclude_rules: form.exclude_rules.trim(),
  }
}

async function afterChange() {
  await Promise.all([store.refresh(), reloadAnimeDetail()])
}

async function save() {
  msg.value = ''
  if (saving.value || applying.value) return
  if (!form.title.trim()) {
    msg.value = '请填写番剧名称'
    return
  }
  if (!Number.isInteger(Number(form.season)) || Number(form.season) < 1 || Number(form.season) > 999) {
    msg.value = '默认季号必须是 1-999 的整数'
    return
  }
  saving.value = true
  try {
    const saved = a.value
      ? await http.put<AnimeSummary>('/api/animes/' + a.value.id, body())
      : await http.post<AnimeSummary>('/api/animes', body())
    if (saved?.storage_warning) toast.error('番剧已保存，但存储位置需要确认：' + saved.storage_warning)
    else toast.success(a.value ? '番剧已保存' : '番剧已创建')
    s.open = false
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
  if (!a.value) return
  if (!form.include_rules.trim()) {
    msg.value = '请先填写包含规则关键词'
    return
  }
  if (!form.title.trim()) {
    msg.value = '请填写番剧名称'
    return
  }
  if (!Number.isInteger(Number(form.season)) || Number(form.season) < 1 || Number(form.season) > 999) {
    msg.value = '默认季号必须是 1-999 的整数'
    return
  }
  applying.value = true
  try {
    await http.put('/api/animes/' + a.value.id, body())
    const r = await http.post<{ matched: number }>(`/api/animes/${a.value.id}/apply-rules`)
    toast.success(`规则已应用，归并了 ${r.matched} 条现有条目`)
    s.open = false
    await afterChange()
  } catch (e) {
    msg.value = errorMessage(e)
  } finally {
    applying.value = false
  }
}
</script>

<template>
  <BaseDialog :open="s.open" :title="a ? '编辑番剧' : '新建番剧'" size="lg" @close="close">
    <p v-if="a" class="subject">{{ a.title }}</p>
    <form @submit.prevent="save">
      <div class="field">
        <label for="ae-name">番剧名称（归档目录名，改动会级联到条目）</label>
        <input id="ae-name" v-model="form.title" type="text" maxlength="200" spellcheck="false" autofocus />
      </div>
      <div class="field-row">
        <div class="field">
          <label for="ae-search">元数据搜索词（留空用番剧名自动识别）</label>
          <input id="ae-search" v-model="form.search_name" type="text" spellcheck="false" />
        </div>
        <div class="field">
          <label for="ae-season">默认季号（用于 Season XX 目录与文件名）</label>
          <input id="ae-season" v-model.number="form.season" type="number" min="1" max="999" />
        </div>
      </div>
      <div class="field">
        <label for="ae-weekday">每周更新日</label>
        <select id="ae-weekday" v-model.number="form.update_weekday">
          <option v-for="(day, index) in weekdays" :key="index" :value="index">{{ day }}</option>
        </select>
        <p class="field-hint">{{ a?.broadcast_weekday ? `Bangumi 标注每${weekdays[a.broadcast_weekday]}放送。` : '尚未获取明确的放送星期，可绑定 Bangumi 资料或手动选择。' }}手动选择优先；改回自动后跟随资料。放送日不保证字幕组当天出片，不改变 RSS 同步或下载时间。</p>
      </div>
      <div class="field">
        <label for="ae-dir">存储位置（留空使用默认位置）</label>
        <input id="ae-dir" v-model="form.storage_dir" type="text" :placeholder="`例如 ${pathExample}`" spellcheck="false" />
      </div>
      <div class="field-row">
        <div class="field">
          <label for="ae-include">多来源包含规则（全部匹配，逗号分隔）</label>
          <input id="ae-include" v-model="form.include_rules" type="text" placeholder="例如：星海物语" />
          <p class="field-hint">任何订阅源的原始标题命中即归入本番剧</p>
        </div>
        <div class="field">
          <label for="ae-exclude">多来源排除规则（任一匹配即排除）</label>
          <input id="ae-exclude" v-model="form.exclude_rules" type="text" placeholder="例如：480p,预告" />
        </div>
      </div>
      <p v-if="a" class="field-hint">规则只收编未手动纠错过的条目；保存后可点「应用到现有条目」回溯归并。</p>
      <p class="form-msg" role="status">{{ msg }}</p>
      <button type="submit" hidden />
    </form>
    <template #footer>
      <BaseButton v-if="a" variant="ghost" :icon="WandSparkles" :loading="applying" :disabled="saving" @click="apply">应用到现有条目</BaseButton>
      <span class="spacer" />
      <BaseButton :disabled="saving || applying" @click="close">取消</BaseButton>
      <BaseButton variant="primary" :loading="saving" :disabled="applying" @click="save">{{ a ? '保存' : '创建' }}</BaseButton>
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
</style>
