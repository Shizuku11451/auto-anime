<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Link, Search, Unlink } from '@lucide/vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import type { MetaCandidate } from '@/api/types'

const { state, reloadAnimeDetail } = useDialogs()
const store = useAppStore()
const s = state.bind
const a = computed(() => s.anime)

const query = ref('')
const results = ref<MetaCandidate[]>([])
const searched = ref(false)
const searching = ref(false)
const binding = ref<number | null>(null)
const msg = ref('')

/** 去掉 [标签]、括号说明和 " / " 之后的外文名，得到更适合 Bangumi 搜索的关键词 */
function cleanTitle(raw: string): string {
  const head = raw
    .replace(/[[【][^\]】]*[\]】]/g, ' ')
    .replace(/[（(][^）)]*[）)]/g, ' ')
    .split(/\s+[/|]\s+/)[0]
  return (head ?? raw).replace(/\s+/g, ' ').trim()
}

watch(
  () => s.open,
  (open) => {
    if (!open || !a.value) return
    query.value = a.value.search_name || a.value.meta_name_cn || cleanTitle(a.value.title)
    results.value = []
    searched.value = false
    msg.value = ''
    if (query.value.trim().length >= 2) void search()
  },
)

function close() {
  s.open = false
}

async function search() {
  const q = query.value.trim()
  if (q.length < 2) {
    msg.value = '请输入至少 2 个字符的关键词'
    return
  }
  msg.value = ''
  searching.value = true
  try {
    const data = await http.get<{ items: MetaCandidate[] }>('/api/meta/search?q=' + encodeURIComponent(q))
    results.value = data.items ?? []
    searched.value = true
  } catch (e) {
    results.value = []
    msg.value = errorMessage(e)
  } finally {
    searching.value = false
  }
}

async function bind(bangumiId: number) {
  if (!a.value) return
  msg.value = ''
  binding.value = bangumiId
  try {
    await http.put(`/api/animes/${a.value.id}/metadata`, { bangumi_id: bangumiId })
    toast.success(bangumiId > 0 ? '元数据绑定成功，封面已更新' : '已解除绑定')
    close()
    await Promise.all([store.refresh(), reloadAnimeDetail()])
  } catch (e) {
    msg.value = errorMessage(e)
  } finally {
    binding.value = null
  }
}
</script>

<template>
  <BaseDialog :open="s.open" title="元数据绑定" @close="close">
    <template v-if="a">
      <p class="subject">{{ a.title }}</p>
      <div class="current" :class="{ bound: a.bangumi_id }">
        <template v-if="a.bangumi_id">
          当前绑定：Bangumi #{{ a.bangumi_id }}<template v-if="a.meta_name_cn"> · {{ a.meta_name_cn }}</template>
        </template>
        <template v-else>当前未绑定，封面由自动识别获得</template>
      </div>
      <form class="search-row" @submit.prevent="search">
        <input v-model="query" type="text" placeholder="在 Bangumi 搜索作品名（至少 2 个字符）" autofocus />
        <BaseButton type="submit" :icon="Search" :loading="searching">搜索</BaseButton>
      </form>
      <div class="results">
        <p v-if="searching && !results.length" class="note">搜索中…</p>
        <p v-else-if="searched && !results.length" class="note">没有找到匹配的 Bangumi 条目，换个关键词试试</p>
        <TransitionGroup name="list" tag="div" class="result-list">
          <div v-for="(c, i) in results" :key="c.id" class="candidate" :style="{ '--i': i }">
            <div class="names">
              <b>{{ c.name_cn || c.name }}</b>
              <span v-if="c.name_cn && c.name !== c.name_cn">{{ c.name }}</span>
              <span class="bid">#{{ c.id }}</span>
            </div>
            <BaseButton
              size="sm"
              :variant="a.bangumi_id === c.id ? 'soft' : 'default'"
              :icon="Link"
              :loading="binding === c.id"
              :disabled="a.bangumi_id === c.id"
              @click="bind(c.id)"
            >
              {{ a.bangumi_id === c.id ? '已绑定' : '绑定' }}
            </BaseButton>
          </div>
        </TransitionGroup>
      </div>
      <p class="field-hint">绑定后封面与名称立即更新，归档目录会写入 tvshow.nfo；「解除绑定」回到自动识别。</p>
      <p class="form-msg" role="status">{{ msg }}</p>
    </template>
    <template #footer>
      <BaseButton v-if="a?.bangumi_id" variant="danger" :icon="Unlink" :loading="binding === 0" @click="bind(0)">解除绑定</BaseButton>
      <span class="spacer" />
      <BaseButton @click="close">关闭</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.subject {
  font-weight: 650;
  margin: 0 0 12px;
  padding: 10px 12px;
  border-radius: var(--r-sm);
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 13.5px;
  overflow-wrap: anywhere;
}
.current {
  font-size: 12.5px;
  color: var(--text-2);
  margin-bottom: 14px;
}
.current.bound {
  color: var(--green);
}
.search-row {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.results {
  min-height: 60px;
  max-height: 320px;
  overflow: auto;
  margin-bottom: 12px;
  position: relative;
}
.result-list {
  display: grid;
  gap: 6px;
}
.note {
  color: var(--text-3);
  font-size: 13px;
  padding: 14px 4px;
}
.candidate {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  background: var(--surface-2);
  transition: border-color var(--t-fast) var(--ease-out);
}
.candidate:hover {
  border-color: var(--line-strong);
}
.names {
  display: grid;
  gap: 2px;
  min-width: 0;
}
.names b {
  font-weight: 650;
  font-size: 13.5px;
}
.names span {
  font-size: 12px;
  color: var(--text-3);
}
.bid {
  font-family: var(--mono);
}
</style>
