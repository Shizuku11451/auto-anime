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
  <SettingsSection id="library" title="媒体库" :icon="FolderTree" description="一部番一个文件夹，按 Season 01 / 番名 - S01E07 组织，适配飞牛影视、Emby、Jellyfin">
    <div class="field">
      <label for="st-library">媒体库目录（番剧文件夹的根目录；留空使用下载目录）</label>
      <div class="storage-control">
        <input id="st-library" v-model="form.library_dir" type="text" spellcheck="false" :placeholder="`例如 ${pathExample}`" :aria-invalid="!!fieldError('library_dir')" :aria-describedby="fieldError('library_dir') ? 'st-library-error' : undefined" />
        <BaseButton :icon="FolderTree" :loading="storageLoading" :disabled="!ready || saving" @click="checkStorage">检测目录</BaseButton>
      </div>
      <p v-if="fieldError('library_dir')" id="st-library-error" class="form-msg" role="alert">{{ fieldError('library_dir') }}</p>
      <p v-if="storageResult" class="storage-result" :class="storageResult.writable && storageResult.mount_status !== 'unmapped' ? 'ok' : 'warning'" role="status">{{ storageResult.message }}</p>
      <p v-if="storageError" class="form-msg" role="alert">{{ storageError }}</p>
      <p v-if="isWindowsOS" class="field-hint">
        媒体服务器扫描该目录即可；网络共享请确认当前用户可以写入。当前下载目录：<code>{{ info?.download_dir || '未知' }}</code>
      </p>
      <p v-else class="field-hint">
        媒体服务器扫描该目录即可。目录需已映射进容器（docker-compose.yml 的 volumes），否则文件写进容器内部、NAS 上看不到。当前下载目录：<code>{{ info?.download_dir || '未知' }}</code>
      </p>
      <p v-if="isWindowsOS" class="field-hint">
        可填盘符路径（如 D:\番剧）或 UNC 共享（如 \\NAS\共享\番剧）；留空时归档到下载目录，不会自动搬迁已有文件。
      </p>
      <p v-else class="field-hint">
        飞牛应用包用户：先在「应用设置 → 访问权限」授权目录，再重启应用同步挂载，最后填写同一个路径。使用 Docker Compose 安装时，可用<a href="/install" target="_blank" rel="noopener noreferrer">部署配置页</a>修改映射，并保留原来的 data 目录。
      </p>
    </div>
    <div class="field">
      <label class="check" for="st-keep-source">
        <input id="st-keep-source" v-model="form.keep_source_files" type="checkbox" />
        保留下载器原文件做种
      </label>
      <p class="field-hint">默认开启。关闭后，成功归档并写入数据库才停止对应下载任务、清理源文件；失败会保留并提示原因。仅影响后续归档，不批量清理旧文件。外部下载器须配置明确的下载目录/本地映射。重新开启可取消待清理任务，无法恢复已删除文件。</p>
    </div>
    <div class="field">
      <label class="check" for="st-cleanup-empty">
        <input id="st-cleanup-empty" v-model="form.auto_cleanup_empty_anime" type="checkbox" />
        自动清理空番剧
      </label>
      <p class="field-hint">默认关闭。开启并保存后立即检查，之后每分钟自动检查。仅清理没有资源、没有订阅指定归属、没有手动元数据绑定或自定义搜索名、季号、存储位置、归并规则的空番剧记录。暂停的订阅也会保留其关联番剧；不会删除媒体文件。</p>
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
