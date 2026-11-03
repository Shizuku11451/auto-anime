<script setup lang="ts">
import { computed } from 'vue'
import { FolderTree } from '@lucide/vue'
import SettingsSection from './SettingsSection.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useSettings } from '@/composables/useSettingsForm'
import { isWindows, storageDirExample } from '@/utils/platform'

const { form, info, fieldError, ready, saving, storageLoading, storageResult, storageError, checkStorage } = useSettings()
const isWindowsOS = computed(() => isWindows(info.value?.os))
const pathExample = computed(() => storageDirExample(info.value?.os))
</script>

<template>
  <SettingsSection id="library" title="媒体库" :icon="FolderTree" description="一部番一个文件夹，适配飞牛影视、Emby、Jellyfin">
    <div class="field">
      <label for="st-library">{{ isWindowsOS ? '保存目录' : '媒体库目录' }}</label>
      <div class="storage-control">
        <input id="st-library" v-model="form.library_dir" type="text" spellcheck="false" :placeholder="`例如 ${pathExample}`" :aria-invalid="!!fieldError('library_dir')" :aria-describedby="fieldError('library_dir') ? 'st-library-error' : undefined" />
        <BaseButton :icon="FolderTree" :loading="storageLoading" :disabled="!ready || saving" @click="checkStorage">检测目录</BaseButton>
      </div>
      <p v-if="fieldError('library_dir')" id="st-library-error" class="form-msg" role="alert">{{ fieldError('library_dir') }}</p>
      <p v-if="storageResult" class="storage-result" :class="storageResult.writable && storageResult.mount_status !== 'unmapped' ? 'ok' : 'warning'" role="status">{{ storageResult.message }}</p>
      <p v-if="storageError" class="form-msg" role="alert">{{ storageError }}</p>
      <p class="field-hint">留空使用下载目录 <code>{{ info?.download_dir || '未知' }}</code>；修改后已有文件不会搬迁。</p>
      <p v-if="isWindowsOS" class="field-hint">支持盘符路径或 UNC 共享。</p>
      <p v-else class="field-hint">
        目录需映射进容器。飞牛应用先在「应用设置 → 访问权限」授权并重启应用；Compose 可用<a href="/install" target="_blank" rel="noopener noreferrer">部署配置页</a>修改映射。
      </p>
    </div>
    <div class="field">
      <label class="check" for="st-keep-source">
        <input id="st-keep-source" v-model="form.keep_source_files" type="checkbox" />
        保留下载器原文件做种
      </label>
      <p class="field-hint">关闭后，归档成功才停止任务并删除源文件，只影响之后的归档；已删除的文件无法恢复。</p>
    </div>
    <div class="field">
      <label class="check" for="st-cleanup-empty">
        <input id="st-cleanup-empty" v-model="form.auto_cleanup_empty_anime" type="checkbox" />
        自动清理空番剧
      </label>
      <p class="field-hint">每分钟清理没有资源且未手动设置过的番剧记录，不删除媒体文件。</p>
    </div>
  </SettingsSection>
</template>

<style scoped>
.storage-control { display: flex; gap: 8px; }
.storage-control input { flex: 1; }
.storage-result { padding: 10px 12px; border-radius: var(--r-sm); font-size: 12px; overflow-wrap: anywhere; }
.storage-result.ok { color: var(--green); background: var(--green-soft); }
.storage-result.warning { color: var(--amber); background: var(--amber-soft); }
.field-hint { overflow-wrap: anywhere; }
input[aria-invalid="true"] { border-color: var(--red-border); }
@media (max-width: 380px) {
  .storage-control { flex-direction: column; }
}
</style>
