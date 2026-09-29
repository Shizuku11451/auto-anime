<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import type { CorrectInput, CorrectResult } from '@/api/types'

const { state, reloadAnimeDetail } = useDialogs()
const store = useAppStore()
const s = state.correct
const item = computed(() => s.item)

const form = reactive({ animeId: '', newTitle: '', season: 0, episode: '', move: false })
const saving = ref(false)

/** 标题分析解析出的季（3期 / S3 / 第三季）；解析到时提示，季号填 0 也可以用它归档。 */
const parsedSeason = computed(() => {
  const it = item.value
  if (!it || !('season_auto' in it)) return 0
  return it.season_auto > 0 ? it.season_auto : 0
})
const msg = ref('')

/** 下拉里保证当前番剧在列表内（即使番剧库列表尚未刷新） */
const options = computed(() => {
  const list = store.animes.value.map((a) => ({ id: a.id, title: a.title }))
  const it = item.value
  if (it?.anime_id && !list.some((a) => a.id === it.anime_id)) {
    const title = 'anime_title' in it ? it.anime_title : ''
    list.unshift({ id: it.anime_id, title: title || '当前番剧' })
  }
  return list
})

watch(
  () => s.open,
  (open) => {
    if (!open || !item.value) return
    form.animeId = item.value.anime_id ? String(item.value.anime_id) : ''
    form.newTitle = ''
    form.season = item.value.season || 0
    form.episode = item.value.episode || ''
    form.move = false
    msg.value = ''
  },
)

function close() {
  s.open = false
}

async function save() {
  msg.value = ''
  const it = item.value
  if (!it) return
  const body: CorrectInput = {
    season: Number.parseInt(String(form.season), 10) || 0,
    episode: form.episode.trim(),
    rearchive: form.move,
  }
  const newTitle = form.newTitle.trim()
  if (newTitle) body.anime_title = newTitle
  else if (form.animeId) body.anime_id = Number(form.animeId)
  else if (it.anime_id) body.anime_id = it.anime_id
  else {
    msg.value = '请选择目标番剧或输入新番剧名'
    return
  }
  saving.value = true
  try {
    const r = await http.patch<CorrectResult>('/api/items/' + it.id, body)
    if (r.cleanup_warning) toast.info(r.cleanup_warning)
    else toast.success(r.moved ? '纠错已保存，媒体文件已迁移' : '纠错已保存')
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
  <BaseDialog :open="s.open" title="手动纠错" @close="close">
    <p v-if="item" class="original">{{ item.title }}</p>
    <form @submit.prevent="save">
      <div class="field">
        <label for="cd-anime">改判到番剧</label>
        <select id="cd-anime" v-model="form.animeId" :disabled="!!form.newTitle.trim()">
          <option v-if="!item?.anime_id" value="">（不更改番剧）</option>
          <option v-for="a in options" :key="a.id" :value="String(a.id)">{{ a.title }}</option>
        </select>
        <input v-model="form.newTitle" type="text" placeholder="或直接输入新番剧名称（将新建番剧）" spellcheck="false" />
      </div>
      <div class="field-row">
        <div class="field">
          <label for="cd-season">季号（0 = 跟随番剧设置）</label>
          <input id="cd-season" v-model.number="form.season" type="number" min="0" max="999" />
          <p v-if="parsedSeason" class="field-hint">
            标题识别为第 {{ parsedSeason }} 季；季号填 0 时按「条目 > 番剧设置 > 标题识别」取季。
          </p>
        </div>
        <div class="field">
          <label for="cd-episode">集数（留空表示无集数资源）</label>
          <input id="cd-episode" v-model="form.episode" type="text" placeholder="例如 07" spellcheck="false" />
        </div>
      </div>
      <label class="check">
        <input v-model="form.move" type="checkbox" />
        同时迁移已保存的媒体文件到新位置
      </label>
      <p class="field-hint">纠错后的条目会标记为「已纠错」，不再被多来源规则重新归并。</p>
      <p class="form-msg" role="status">{{ msg }}</p>
      <button type="submit" hidden />
    </form>
    <template #footer>
      <BaseButton @click="close">取消</BaseButton>
      <BaseButton variant="primary" :loading="saving" @click="save">保存纠错</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.original {
  font-size: 12.5px;
  color: var(--text-2);
  margin: 0 0 14px;
  padding: 10px 12px;
  border-radius: var(--r-sm);
  background: var(--surface-2);
  border: 1px solid var(--line);
  overflow-wrap: anywhere;
  font-family: var(--mono);
}
.field input + input,
.field select + input {
  margin-top: 8px;
}
</style>
