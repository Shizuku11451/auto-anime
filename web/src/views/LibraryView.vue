<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Clapperboard, SearchX, Plus, ListChecks, Merge, Trash2 } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import SkeletonBlock from '@/components/ui/SkeletonBlock.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import AnimeCard from '@/components/library/AnimeCard.vue'
import { useAppStore } from '@/stores/app'
import { useDialogs } from '@/stores/dialogs'

const store = useAppStore()
const dialogs = useDialogs()
const router = useRouter()

const filter = ref('')
const selecting = ref(false)
const selected = ref<number[]>([])
const scope = ref('all')
const sort = ref('recent')
const chosen = computed(() => store.animes.value.filter(a => selected.value.includes(a.id)))
watch(() => store.animes.value, all => { selected.value = selected.value.filter(id => all.some(a => a.id === id)) })
function toggle(id: number) { selected.value = selected.value.includes(id) ? selected.value.filter(x => x !== id) : [...selected.value, id] }
function toggleMode() { selecting.value = !selecting.value; selected.value = [] }
const list = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const all = store.animes.value.filter(a => (scope.value !== 'empty' || !a.item_count) && (scope.value !== 'saved' || a.saved_count > 0))
    .filter(a => !q || [a.title, a.meta_name_cn, a.meta_name, a.search_name].some(s => s && s.toLowerCase().includes(q)))
  if (sort.value === 'title') all.sort((a, b) => a.title.localeCompare(b.title, 'zh-CN'))
  if (sort.value === 'resources') all.sort((a, b) => b.item_count - a.item_count)
  return all
})
const allSelected = computed(() => list.value.length > 0 && list.value.every(a => selected.value.includes(a.id)))
function selectVisible() {
  selected.value = allSelected.value ? selected.value.filter(id => !list.value.some(a => a.id === id)) : [...new Set([...selected.value, ...list.value.map(a => a.id)])]
}
</script>

<template>
  <div class="page">
    <PageHeader
      title="番剧库"
      :count="store.loaded.value ? store.animes.value.length : undefined"
    >
      <SearchInput v-model="filter" placeholder="筛选番剧" width="240px" />
      <BaseButton :icon="Plus" variant="primary" @click="dialogs.openAnimeEdit()">新建番剧</BaseButton>
      <BaseButton :icon="ListChecks" :variant="selecting ? 'soft' : 'default'" :disabled="!store.animes.value.length" @click="toggleMode">{{ selecting ? '完成管理' : '批量管理' }}</BaseButton>
    </PageHeader>

    <div class="library-toolbar">
      <label>范围 <select v-model="scope"><option value="all">全部番剧</option><option value="empty">无资源</option><option value="saved">有已保存资源</option></select></label>
      <label>排序 <select v-model="sort"><option value="recent">最近收录</option><option value="title">名称</option><option value="resources">资源数量</option></select></label>
      <span class="result-count">{{ list.length }} 部</span>
    </div>
    <div v-if="selecting" class="batch-toolbar">
      <label class="select-all"><input type="checkbox" :checked="allSelected" :indeterminate="!allSelected && list.some(a => selected.includes(a.id))" @change="selectVisible" />全选当前结果</label>
      <span>已选 {{ chosen.length }} 部</span>
      <BaseButton size="sm" :icon="Merge" :disabled="!chosen.length || chosen.length > 500 || store.animes.value.length < 2" @click="dialogs.openAnimeManage('merge', chosen)">合并</BaseButton>
      <BaseButton size="sm" variant="danger" :icon="Trash2" :disabled="!chosen.length || chosen.length > 500" @click="dialogs.openAnimeManage('delete', chosen)">删除</BaseButton>
    </div>

    <div v-if="!store.loaded.value" class="grid" aria-busy="true">
      <div v-for="i in 8" :key="i" class="sk-card" :style="{ '--i': i }">
        <SkeletonBlock height="auto" radius="0" class="sk-poster" />
        <div class="sk-body">
          <SkeletonBlock height="14px" width="80%" />
          <SkeletonBlock height="11px" width="55%" />
        </div>
      </div>
    </div>

    <EmptyState
      v-else-if="!store.animes.value.length"
      :icon="Clapperboard"
      title="番剧库还是空的"
      hint="添加 RSS 订阅并同步后，识别出的番剧会自动出现在这里"
    >
      <BaseButton variant="primary" @click="router.push('/feeds')">去添加订阅</BaseButton>
      <BaseButton @click="router.push('/discover')">先去找番</BaseButton>
    </EmptyState>

    <EmptyState v-else-if="!list.length" :icon="SearchX" title="没有匹配的番剧" hint="换个关键词试试" compact />

    <TransitionGroup v-else tag="div" class="grid" name="list" appear>
      <AnimeCard
        v-for="(a, i) in list"
        :key="a.id"
        :anime="a"
        :selecting="selecting"
        :selected="selected.includes(a.id)"
        :can-merge="store.animes.value.length > 1"
        :style="{ '--i': i }"
        @open="dialogs.openAnimeDetail"
        @toggle="toggle"
        @edit="dialogs.openAnimeEdit(a)"
        @merge="dialogs.openAnimeManage('merge', [a])"
        @remove="dialogs.openAnimeManage('delete', [a])"
      />
    </TransitionGroup>
  </div>
</template>

<style scoped>
.library-toolbar, .batch-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; margin-bottom: 18px; font-size: 13px; }
.library-toolbar label, .select-all { display: inline-flex; align-items: center; gap: 8px; }
.library-toolbar select { width: auto; max-width: 180px; }
.result-count { color: var(--text-3); margin-left: auto; }
.batch-toolbar { padding: 12px 0; border-block: 1px solid var(--line); }
.grid {
  position: relative;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 172px), 1fr));
  gap: 18px;
}
@media (max-width: 640px) {
  :deep(.page-actions .search) { flex: 1 0 100%; width: 100% !important; }
  .grid {
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 150px), 1fr));
    gap: 12px;
  }
}
.sk-card {
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  overflow: hidden;
  background: var(--surface);
  animation: fade-in var(--t-slow) var(--ease-out) both;
  animation-delay: calc(var(--i) * 40ms);
}
.sk-poster {
  aspect-ratio: 2 / 3;
}
.sk-body {
  display: grid;
  gap: 8px;
  padding: 12px 13px 14px;
}
</style>
