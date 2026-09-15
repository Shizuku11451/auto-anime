<script setup lang="ts">
import { computed, watch } from 'vue'
import { CircleCheckBig, Download, ExternalLink, RefreshCw, Rocket, TriangleAlert } from '@lucide/vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDialogs } from '@/stores/dialogs'
import { useUpdateStore } from '@/stores/update'
import { noteBlocks } from '@/utils/update'

const { state, closeUpdate } = useDialogs()
const store = useUpdateStore()

const op = state.update
const info = computed(() => store.info.value)
const current = computed(() => info.value?.version || '未知')
const latest = computed(() => info.value?.latest || '')
const notes = computed(() => noteBlocks(info.value?.notes || ''))
const releaseURL = computed(() => info.value?.release_url || '')
const busy = computed(() => store.installing.value)
const restarting = computed(() => store.restarting.value)
const status = computed(() => store.status.value)
const failed = computed(() => status.value === 'failed')
const progressText = computed(() => store.progressText.value)
/** 已有更新包写好，等待重启生效 */
const pending = computed(() => !!info.value?.pending)
const noAutoInstall = computed(() => !!info.value?.update_available && !info.value?.can_update)
const installHint = computed(() => store.installHint.value)
const sourceLabel = computed(() => info.value?.source || '')
const publishedAt = computed(() => formatTime(info.value?.published_at || ''))
const skippedVersion = computed(() => store.skipped.value)

/** 打开弹窗时就刷新一次，保证看到的是最新检查结果 */
watch(
  () => op.open,
  (open) => {
    if (open) void store.checkNow()
  },
)

function formatTime(value: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function close() {
  // 更新进行中不允许关掉进度（Esc/点遮罩同样被 persistent 拦住）
  if (busy.value) return
  closeUpdate()
}

function install() {
  void store.install()
}

function skip() {
  store.skipVersion()
  closeUpdate()
}
</script>

<template>
  <BaseDialog
    :open="op.open"
    :title="failed ? '更新失败' : pending ? '更新待生效' : '发现新版本'"
    size="md"
    :closable="!busy"
    :persistent="busy"
    @close="close"
  >
    <div class="vers">
      <div class="ver">
        <span class="ver-label">当前版本</span>
        <b class="ver-value mono">{{ current }}</b>
      </div>
      <div class="ver-arrow" aria-hidden="true">→</div>
      <div class="ver">
        <span class="ver-label">最新版本</span>
        <b class="ver-value mono accent">{{ latest || '—' }}</b>
      </div>
    </div>

    <p v-if="info?.commit || publishedAt" class="meta-line">
      <span v-if="info?.commit">提交 <code>{{ info.commit.slice(0, 7) }}</code></span>
      <span v-if="info?.platform">{{ info.platform }}</span>
      <span v-if="publishedAt">发布于 {{ publishedAt }}</span>
      <span v-if="sourceLabel">来源 {{ sourceLabel }}</span>
    </p>

    <section class="notes">
      <h3 class="notes-head">更新内容</h3>
      <div v-if="notes.length" class="notes-body">
        <template v-for="(block, i) in notes" :key="i">
          <h4 v-if="block.kind === 'heading'" class="note-head">{{ block.text }}</h4>
          <p v-else-if="block.kind === 'bullet'" class="note-bullet">{{ block.text }}</p>
          <p v-else class="note-text">{{ block.text }}</p>
        </template>
      </div>
      <p v-else class="notes-empty">
        该版本没有填写更新说明，可点击「查看 Release」了解详情。
      </p>
    </section>

    <div v-if="busy" class="state running" role="status">
      <RefreshCw class="spin" aria-hidden="true" />
      <span>{{ restarting ? '服务正在重启，页面会自动刷新…' : progressText || '正在准备更新…' }}</span>
    </div>
    <div v-else-if="failed" class="state error" role="alert">
      <TriangleAlert aria-hidden="true" />
      <span>{{ progressText || '更新失败，请稍后重试' }}</span>
    </div>
    <div v-else-if="pending" class="state ok" role="status">
      <CircleCheckBig aria-hidden="true" />
      <span>{{ installHint || '新版本已下载，重启服务后生效。' }}</span>
    </div>
    <p v-else-if="!info?.update_available" class="state ok" role="status">
      <CircleCheckBig aria-hidden="true" />
      <span>已是最新版本{{ info?.checked_at ? '（' + formatTime(info.checked_at) + ' 检查）' : '' }}。</span>
    </p>
    <p v-else-if="noAutoInstall" class="state warn" role="alert">
      <TriangleAlert aria-hidden="true" />
      <span>{{ installHint || '当前环境不支持自动更新，请手动更新。' }}</span>
    </p>
    <p v-else class="hint">
      一键更新会下载 {{ info?.platform }} 更新包，校验后替换程序并重启服务；数据库与媒体文件不受影响。
    </p>

    <p v-if="store.checkError.value" class="form-msg" role="alert">{{ store.checkError.value }}</p>
    <p v-else-if="skippedVersion === latest && latest" class="hint">
      已忽略该版本，可在下方「恢复提醒」重新显示。
    </p>

    <template #footer>
      <BaseButton v-if="skippedVersion === latest && latest && !busy" variant="ghost" size="sm" @click="store.restoreSkipped()">
        恢复提醒
      </BaseButton>
      <a v-if="releaseURL" class="release-link" :href="releaseURL" target="_blank" rel="noopener noreferrer">
        <ExternalLink aria-hidden="true" />查看 Release
      </a>
      <span class="spacer" />
      <BaseButton v-if="!busy && !pending" :disabled="!info?.update_available" @click="skip">忽略此版本</BaseButton>
      <BaseButton v-if="busy" variant="primary" :loading="true" disabled>
        {{ restarting ? '重启中' : '更新中' }}
      </BaseButton>
      <BaseButton
        v-else-if="pending"
        variant="primary"
        :icon="RefreshCw"
        @click="store.checkNow()"
      >
        重新检查
      </BaseButton>
      <BaseButton
        v-else-if="failed"
        variant="primary"
        :icon="RefreshCw"
        :disabled="!info?.can_update"
        @click="install"
      >
        重试更新
      </BaseButton>
      <BaseButton
        v-else-if="info?.update_available && info?.can_update"
        variant="primary"
        :icon="Rocket"
        @click="install"
      >
        一键更新
      </BaseButton>
      <BaseButton v-else-if="!releaseURL" :icon="Download" disabled>暂无可用更新</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.vers {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 14px;
  margin-bottom: 10px;
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  background: var(--surface-2);
}
.ver {
  display: grid;
  gap: 3px;
  min-width: 0;
}
.ver-label {
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.1em;
  color: var(--text-3);
}
.ver-value {
  font-size: 16px;
  font-weight: 700;
  overflow-wrap: anywhere;
}
.ver-value.accent {
  color: var(--accent);
}
.ver-arrow {
  flex: none;
  color: var(--text-3);
}
.meta-line {
  margin: 0 0 12px;
}
.meta-line code {
  font-family: var(--mono);
  font-size: 11.5px;
}

.notes {
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  overflow: hidden;
  margin-bottom: 12px;
}
.notes-head {
  margin: 0;
  padding: 9px 13px;
  font-size: 11.5px;
  font-weight: 700;
  letter-spacing: 0.12em;
  color: var(--text-3);
  background: var(--surface-2);
  border-bottom: 1px solid var(--line);
}
.notes-body {
  max-height: 260px;
  overflow: auto;
  padding: 12px 14px;
  overscroll-behavior: contain;
}
.note-head {
  margin: 12px 0 6px;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text);
}
.note-head:first-child {
  margin-top: 0;
}
.note-bullet {
  position: relative;
  margin: 0 0 5px;
  padding-left: 14px;
  font-size: 13px;
  line-height: 1.65;
  color: var(--text-2);
  overflow-wrap: anywhere;
}
.note-bullet::before {
  content: '';
  position: absolute;
  left: 3px;
  top: 9px;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--accent);
}
.note-text {
  margin: 0 0 6px;
  font-size: 13px;
  line-height: 1.65;
  color: var(--text-2);
  overflow-wrap: anywhere;
}
.notes-empty {
  margin: 0;
  padding: 14px;
  font-size: 13px;
  color: var(--text-3);
}

.state {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 0 0 8px;
  padding: 10px 12px;
  border-radius: var(--r-sm);
  font-size: 13px;
  line-height: 1.55;
}
.state svg {
  width: 15px;
  height: 15px;
  flex: none;
  margin-top: 1px;
}
.state.running {
  color: var(--accent);
  background: var(--accent-soft);
}
.state.ok {
  color: var(--green);
  background: var(--green-soft);
}
.state.error {
  color: var(--red);
  background: var(--red-soft);
}
.state.warn {
  color: var(--amber);
  background: var(--amber-soft);
}
.hint {
  margin: 0 0 6px;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--text-3);
}
.release-link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12.5px;
  font-weight: 550;
  color: var(--text-2);
  text-decoration: none;
}
.release-link:hover {
  color: var(--accent);
}
.release-link svg {
  width: 14px;
  height: 14px;
}
.spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .spin {
    animation: none;
  }
}
</style>
