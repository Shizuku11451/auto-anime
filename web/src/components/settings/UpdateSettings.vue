<script setup lang="ts">
import { computed } from 'vue'
import { RefreshCw, Rocket } from '@lucide/vue'
import SettingsSection from './SettingsSection.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useSettings } from '@/composables/useSettingsForm'
import { useDialogs } from '@/stores/dialogs'
import { useUpdateStore } from '@/stores/update'
import { isWindows } from '@/utils/platform'

const update = useUpdateStore()
const { openUpdate } = useDialogs()
const { info } = useSettings()
const isWindowsOS = computed(() => isWindows(info.value?.os))
</script>

<template>
  <SettingsSection id="update" title="版本更新" :icon="Rocket" description="启动后自动检查新版本；有新版本时侧边栏会出现提醒，可一键更新并重启服务">
    <div class="meta-line update-meta">
      <span>当前版本 <code>{{ update.info.value?.version || 'dev' }}</code></span>
      <span v-if="update.info.value?.commit">提交 <code>{{ update.info.value.commit.slice(0, 7) }}</code></span>
      <span v-if="update.info.value?.build_time">构建 {{ update.info.value.build_time }}</span>
      <span v-if="update.info.value?.platform">{{ update.info.value.platform }}</span>
      <span v-if="update.info.value?.source">更新源 {{ update.info.value.source }}</span>
    </div>
    <p class="field-hint">
      {{
        update.info.value?.enabled === false
          ? '已通过 UPDATE_CHECK=0 关闭版本检查。'
          : update.info.value?.latest
            ? `最新版本 ${update.info.value.latest}`
            : '尚未获取到版本信息。'
      }}
      <template v-if="update.info.value?.checked_at">（上次检查 {{ update.info.value.checked_at }}）</template>
    </p>
    <p v-if="update.updateAvailable.value" class="field-hint hot">
      发现新版本 {{ update.info.value?.latest }}，可在弹窗中查看更新内容并一键更新。
    </p>
    <p v-else-if="update.pending.value" class="field-hint hot">
      已下载的新版本正在等待生效：{{ isWindowsOS ? '退出并重新打开程序后即为新版本。' : '重启容器后即为新版本。' }}
    </p>
    <p v-if="update.installHint.value" class="field-hint">{{ update.installHint.value }}</p>
    <p v-if="update.checkError.value" class="form-msg" role="alert">{{ update.checkError.value }}</p>
    <div class="update-actions">
      <BaseButton :icon="RefreshCw" :loading="update.checking.value" @click="update.checkNow()">检查更新</BaseButton>
      <BaseButton v-if="update.updateAvailable.value" variant="primary" :icon="Rocket" @click="openUpdate()">查看更新内容</BaseButton>
      <BaseButton v-if="update.skipped.value" variant="ghost" size="sm" @click="update.restoreSkipped()">恢复 {{ update.skipped.value }} 的提醒</BaseButton>
    </div>
  </SettingsSection>
</template>

<style scoped>
.update-meta { margin-bottom: 6px; overflow-wrap: anywhere; }
.update-meta code { font-family: var(--mono); font-size: 11.5px; }
.field-hint { overflow-wrap: anywhere; }
.field-hint.hot { color: var(--accent); font-weight: 550; }
.update-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin: 12px 0 4px;
}
</style>
