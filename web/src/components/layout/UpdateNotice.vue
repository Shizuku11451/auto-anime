<script setup lang="ts">
import { computed } from 'vue'
import { Rocket, Sparkles } from '@lucide/vue'
import { useDialogs } from '@/stores/dialogs'
import { useUpdateStore } from '@/stores/update'

const store = useUpdateStore()
const { openUpdate } = useDialogs()

const latest = computed(() => store.info.value?.latest || '')
const pending = computed(() => store.pending.value)

function open() {
  store.dismiss()
  openUpdate()
}
</script>

<template>
  <section v-if="store.notify.value" class="notice" aria-live="polite">
    <header class="notice-head">
      <Sparkles aria-hidden="true" />
      <span>发现新版本</span>
      <b class="mono">{{ latest }}</b>
    </header>
    <p class="notice-text">
      {{ pending ? '更新包已下载，重启后生效。' : '更新内容已就绪，可一键更新并自动重启服务。' }}
    </p>
    <button type="button" class="notice-btn" @click="open">
      <Rocket aria-hidden="true" />
      <span>查看并更新</span>
    </button>
  </section>
</template>

<style scoped>
.notice {
  display: grid;
  gap: 8px;
  padding: 12px 13px;
  margin-bottom: 10px;
  border-radius: var(--r-lg);
  border: 1px solid var(--accent-border);
  background: linear-gradient(180deg, var(--accent-soft), transparent);
  animation: fade-in var(--t-slow) var(--ease-out) both;
}
.notice-head {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  font-weight: 700;
  letter-spacing: 0.06em;
  color: var(--accent);
}
.notice-head svg {
  width: 13px;
  height: 13px;
}
.notice-head b {
  margin-left: auto;
  font-size: 11.5px;
  color: var(--text-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.notice-text {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.55;
  color: var(--text-2);
}
.notice-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 32px;
  border: 1px solid transparent;
  border-radius: var(--r-sm);
  color: var(--accent-contrast);
  background: linear-gradient(180deg, var(--accent-strong), var(--accent));
  font-size: 12.5px;
  font-weight: 650;
  cursor: pointer;
  transition:
    filter var(--t-fast) var(--ease-out),
    transform var(--t-fast) var(--ease-out);
}
.notice-btn:hover {
  filter: brightness(1.06) saturate(1.05);
}
.notice-btn:active {
  transform: scale(0.97);
}
.notice-btn svg {
  width: 14px;
  height: 14px;
}
</style>
