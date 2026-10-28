<script setup lang="ts">
import { computed } from 'vue'
import { HardDrive } from '@lucide/vue'
import SettingsSection from './SettingsSection.vue'
import { useSettings } from '@/composables/useSettingsForm'
import { MODE_LABEL } from '@/api/types'
import { isWindows } from '@/utils/platform'

const { form, info } = useSettings()
const isWindowsOS = computed(() => isWindows(info.value?.os))

const ALL_MODES = ['native', 'direct', 'aria2', 'qbittorrent', 'transmission'] as const
const MODE_DESC: Record<string, string> = {
  native: '程序内置 BT 引擎下载磁力与种子，完成后归档；普通直链仍由本程序下载，无需安装 qBittorrent',
  direct: 'HTTP/HTTPS 直链由本程序直接下载，磁力与种子需要外部下载器',
  aria2: '通过 JSON-RPC 提交任务，完成后从映射路径复制归档',
  qbittorrent: '通过 WebUI 接口提交任务并按标签跟踪进度',
  transmission: '通过 RPC 提交任务并轮询完成状态',
}
/* 列表外的旧值也要保留，不能在保存时被悄悄改掉。 */
const availableModes = computed(() => {
  const modes: string[] = [...ALL_MODES]
  if (form.download_mode && !modes.includes(form.download_mode)) modes.push(form.download_mode)
  return modes
})
</script>

<template>
  <SettingsSection id="download" title="下载方式" :icon="HardDrive" description="切换仅影响新任务，不会更改已有下载任务">
    <fieldset class="mode-options">
      <legend>下载模式</legend>
      <label v-for="m in availableModes" :key="m" class="mode-option" :class="{ active: form.download_mode === m }">
        <input v-model="form.download_mode" type="radio" name="download-mode" :value="m" />
        <span>{{ MODE_LABEL[m] || m }}</span>
      </label>
    </fieldset>
    <p class="field-hint mode-description">{{ MODE_DESC[form.download_mode] || '当前下载方式已不受支持，请选择其他方式。' }}</p>

    <Transition name="fade" mode="out-in">
      <div v-if="form.download_mode === 'native'" key="native" class="mode-panel">
        <p class="field-hint">任务和续传数据保存在数据目录的 <code>bt</code> 子目录，重启后继续。</p>
        <p v-if="isWindowsOS" class="field-hint">首次使用时 Windows 防火墙可能询问网络访问，请允许专用网络；默认从 BT 端口 6881 开始监听。</p>
        <p v-else class="field-hint">容器需要把 BT 端口段映射进来（默认 6881-6890，一个任务占一个端口）；换端口时按部署说明同时改端口映射与 <code>NATIVE_BT_PORT</code>。</p>
      </div>
      <div v-else-if="form.download_mode === 'aria2'" key="aria2" class="mode-panel">
        <div class="field-row">
          <div class="field">
            <label for="st-aria2-url">Aria2 RPC 地址</label>
            <input id="st-aria2-url" v-model="form.aria2_url" type="text" spellcheck="false" placeholder="http://127.0.0.1:6800/jsonrpc" />
          </div>
          <div class="field">
            <label for="st-aria2-token">RPC 密钥{{ info?.aria2_token_set ? '（留空保持不变）' : '（可选）' }}</label>
            <input id="st-aria2-token" v-model="form.aria2_token" type="password" autocomplete="off" />
          </div>
        </div>
        <div class="field-row">
          <div class="field">
            <label for="st-aria2-dir">Aria2 下载目录（Aria2 侧路径）</label>
            <input id="st-aria2-dir" v-model="form.aria2_dir" type="text" spellcheck="false" placeholder="/downloads" />
          </div>
          <div class="field">
            <label for="st-aria2-local">映射到本程序的路径</label>
            <input id="st-aria2-local" v-model="form.aria2_local_dir" type="text" spellcheck="false" placeholder="两侧相同时可留空" />
          </div>
        </div>
      </div>
      <div v-else-if="form.download_mode === 'qbittorrent'" key="qbit" class="mode-panel">
        <div class="field-row">
          <div class="field">
            <label for="st-qbit-url">qBittorrent WebUI 地址</label>
            <input id="st-qbit-url" v-model="form.qbit_url" type="text" spellcheck="false" placeholder="http://127.0.0.1:8080" />
          </div>
          <div class="field">
            <label for="st-qbit-user">用户名</label>
            <input id="st-qbit-user" v-model="form.qbit_user" type="text" autocomplete="off" />
          </div>
        </div>
        <div class="field-row">
          <div class="field">
            <label for="st-qbit-pass">密码{{ info?.qbit_password_set ? '（留空保持不变）' : '' }}</label>
            <input id="st-qbit-pass" v-model="form.qbit_password" type="password" autocomplete="new-password" />
          </div>
          <div class="field">
            <label for="st-qbit-save">保存目录（qBittorrent 侧路径）</label>
            <input id="st-qbit-save" v-model="form.qbit_save_path" type="text" spellcheck="false" />
          </div>
        </div>
        <div class="field">
          <label for="st-qbit-local">映射到本程序的路径</label>
          <input id="st-qbit-local" v-model="form.qbit_local_dir" type="text" spellcheck="false" placeholder="两侧相同时可留空" />
        </div>
      </div>
      <div v-else-if="form.download_mode === 'transmission'" key="tr" class="mode-panel">
        <div class="field-row">
          <div class="field">
            <label for="st-tr-url">Transmission RPC 地址</label>
            <input id="st-tr-url" v-model="form.transmission_url" type="text" spellcheck="false" placeholder="http://127.0.0.1:9091" />
          </div>
          <div class="field">
            <label for="st-tr-user">用户名</label>
            <input id="st-tr-user" v-model="form.transmission_user" type="text" autocomplete="off" />
          </div>
        </div>
        <div class="field-row">
          <div class="field">
            <label for="st-tr-pass">密码{{ info?.transmission_password_set ? '（留空保持不变）' : '' }}</label>
            <input id="st-tr-pass" v-model="form.transmission_password" type="password" autocomplete="new-password" />
          </div>
          <div class="field">
            <label for="st-tr-dir">下载目录（Transmission 侧路径）</label>
            <input id="st-tr-dir" v-model="form.transmission_download_dir" type="text" spellcheck="false" />
          </div>
        </div>
        <div class="field">
          <label for="st-tr-local">映射到本程序的路径</label>
          <input id="st-tr-local" v-model="form.transmission_local_dir" type="text" spellcheck="false" placeholder="两侧相同时可留空" />
        </div>
      </div>
      <div v-else key="direct" class="mode-panel">
        <p class="field-hint">磁力链接与种子文件需要配置 BT 下载器。</p>
      </div>
    </Transition>
  </SettingsSection>
</template>

<style scoped>
.mode-options {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  min-width: 0;
  padding: 0;
  margin: 0 0 12px;
  border: 0;
}
.mode-options legend {
  margin-bottom: 8px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-2);
}
.mode-option {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 48px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: var(--r-sm);
  background: var(--bg-2);
  color: var(--text-2);
  font-size: 13.5px;
  cursor: pointer;
  overflow-wrap: anywhere;
  transition: border-color var(--t-fast), background var(--t-fast);
}
.mode-option:hover { border-color: var(--line-strong); }
.mode-option.active {
  border-color: var(--accent-border);
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 650;
}
.mode-option input {
  flex: none;
  width: 16px;
  height: 16px;
  margin: 0;
  accent-color: var(--accent);
  cursor: inherit;
}
.mode-option:has(:disabled) { cursor: not-allowed; }
.mode-description { margin-bottom: 18px; overflow-wrap: anywhere; }
.mode-panel { min-height: 24px; }
.mode-panel .field-hint { overflow-wrap: anywhere; }
@media (max-width: 380px) {
  .mode-options { grid-template-columns: minmax(0, 1fr); }
}
</style>
