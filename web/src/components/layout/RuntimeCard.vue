<script setup lang="ts">
import { computed } from 'vue'
import { Activity } from '@lucide/vue'
import AnimatedNumber from '@/components/ui/AnimatedNumber.vue'
import SkeletonBlock from '@/components/ui/SkeletonBlock.vue'
import { useAppStore } from '@/stores/app'
import { MODE_LABEL } from '@/api/types'

const store = useAppStore()
const s = computed(() => store.status.value)

const libraryLabel = computed(() => {
  const st = s.value
  if (!st) return ''
  if (st.library_dir) return st.library_dir
  return st.download_dir ? `${st.download_dir}（下载目录）` : '默认'
})

const countRows = computed(() => Object.entries(store.counts.value).sort((a, b) => b[1] - a[1]))
</script>

<template>
  <section class="runtime card">
    <header class="run-head">
      <Activity aria-hidden="true" />
      <span>运行状态</span>
      <i class="run-dot" :class="{ off: store.runtimeError.value }" aria-hidden="true" />
    </header>

    <div v-if="!s && !store.runtimeError.value" class="run-rows">
      <SkeletonBlock height="12px" width="80%" />
      <SkeletonBlock height="12px" width="65%" />
      <SkeletonBlock height="12px" width="90%" />
    </div>

    <p v-else-if="!s" class="run-off">运行状态不可用</p>

    <div v-else class="run-rows">
      <div class="run-row">
        <span>下载模式</span><b>{{ MODE_LABEL[s.download_mode] || s.download_mode }}</b>
      </div>
      <div class="run-row">
        <span>同步周期</span><b>每 {{ s.poll_minutes }} 分钟</b>
      </div>
      <div class="run-row">
        <span>媒体库</span><b class="truncate" :title="libraryLabel">{{ libraryLabel }}</b>
      </div>
      <div class="run-row">
        <span>版本</span><b class="truncate" :title="s.version_label || s.version">{{ s.version || 'dev' }}</b>
      </div>
      <div v-if="s.install_ready" class="run-row">
        <span>更新</span><b class="hot">待重启生效</b>
      </div>
      <div v-if="s.proxy" class="run-row">
        <span>出站代理</span><b class="truncate" :title="s.proxy">{{ s.proxy }}</b>
      </div>
      <div v-if="countRows.length" class="run-divider" />
      <div v-for="[k, v] in countRows" :key="k" class="run-row">
        <span>{{ k }}</span><b><AnimatedNumber :value="v" /></b>
      </div>
      <div v-if="!countRows.length" class="run-row">
        <span>下载任务</span><b>暂无</b>
      </div>
    </div>
  </section>
</template>

<style scoped>
.runtime {
  padding: 13px 14px 12px;
  font-size: 12.5px;
  color: var(--text-2);
  border-radius: var(--r-lg);
  animation: fade-in var(--t-slow) var(--ease-out) both;
}
.run-head {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  color: var(--text-3);
  margin-bottom: 10px;
}
.run-head svg {
  width: 13px;
  height: 13px;
}
.run-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--green);
  margin-left: auto;

}
.run-dot.off {
  background: var(--red);
  animation: none;
}
.run-off {
  color: var(--text-3);
}
.run-rows {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 6px;
}
.run-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 12px;
  min-width: 0;
}
.run-row > span {
  color: var(--text-3);
  flex: none;
}
.run-row > b {
  font-weight: 600;
  color: var(--text);
  text-align: right;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.run-divider {
  height: 1px;
  background: var(--line);
  margin: 3px 0;
}
.run-row > b.hot {
  color: var(--accent);
}
</style>
