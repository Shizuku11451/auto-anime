<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { FolderTree, Globe, HardDrive, Save, UserRound } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import { useAppStore } from '@/stores/app'
import { MODE_LABEL, type Settings, type SettingsInput } from '@/api/types'

const store = useAppStore()
const narrow = useMediaQuery('(max-width: 600px)')

const MODES = ['direct', 'aria2', 'qbittorrent', 'transmission'] as const
const MODE_DESC: Record<string, string> = {
  direct: 'HTTP/HTTPS 直链由本程序直接下载，磁力与种子需要外部下载器',
  aria2: '通过 JSON-RPC 提交任务，完成后从映射路径复制归档',
  qbittorrent: '通过 WebUI 接口提交任务并按标签跟踪进度',
  transmission: '通过 RPC 提交任务并轮询完成状态',
}

const form = reactive({
  web_user: '',
  web_password: '',
  download_mode: 'direct',
  max_concurrent_downloads: 2,
  aria2_url: '',
  aria2_token: '',
  aria2_dir: '',
  aria2_local_dir: '',
  qbit_url: '',
  qbit_user: '',
  qbit_password: '',
  qbit_save_path: '',
  qbit_local_dir: '',
  transmission_url: '',
  transmission_user: '',
  transmission_password: '',
  transmission_download_dir: '',
  transmission_local_dir: '',
  proxy_url: '',
  proxy_clear_credentials: false,
  poll_minutes: 30,
  discover_search_template: '',
  library_dir: '',
  auto_cleanup_empty_anime: false,
})

const info = ref<Settings | null>(null)
const loading = ref(true)
const saving = ref(false)
const msg = ref('')
const loadErr = ref('')

const modeIndex = computed(() => Math.max(0, MODES.indexOf(form.download_mode as (typeof MODES)[number])))

async function load() {
  loading.value = true
  loadErr.value = ''
  try {
    const s = await http.get<Settings>('/api/settings')
    info.value = s
    form.web_user = s.web_user || ''
    form.web_password = ''
    form.download_mode = s.download_mode || 'direct'
    form.max_concurrent_downloads = s.max_concurrent_downloads ?? 2
    form.aria2_url = s.aria2_url || ''
    form.aria2_token = ''
    form.aria2_dir = s.aria2_dir || ''
    form.aria2_local_dir = s.aria2_local_dir || ''
    form.qbit_url = s.qbit_url || ''
    form.qbit_user = s.qbit_user || ''
    form.qbit_password = ''
    form.qbit_save_path = s.qbit_save_path || ''
    form.qbit_local_dir = s.qbit_local_dir || ''
    form.transmission_url = s.transmission_url || ''
    form.transmission_user = s.transmission_user || ''
    form.transmission_password = ''
    form.transmission_download_dir = s.transmission_download_dir || ''
    form.transmission_local_dir = s.transmission_local_dir || ''
    form.proxy_url = s.proxy_url || ''
    form.proxy_clear_credentials = false
    form.poll_minutes = s.poll_minutes || 30
    form.discover_search_template = s.discover_search_template || ''
    form.library_dir = s.library_dir || ''
    form.auto_cleanup_empty_anime = s.auto_cleanup_empty_anime === true
  } catch (e) {
    loadErr.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}

onMounted(load)

async function save() {
  msg.value = ''
  if (!Number.isInteger(form.max_concurrent_downloads) || form.max_concurrent_downloads < 1 || form.max_concurrent_downloads > 32) {
    msg.value = '同时下载数量必须是 1-32 的整数'
    return
  }
  const body: SettingsInput = {
    web_user: form.web_user.trim(),
    download_mode: form.download_mode,
    max_concurrent_downloads: form.max_concurrent_downloads,
    aria2_url: form.aria2_url.trim(),
    aria2_dir: form.aria2_dir.trim(),
    aria2_local_dir: form.aria2_local_dir.trim(),
    qbit_url: form.qbit_url.trim(),
    qbit_user: form.qbit_user.trim(),
    qbit_save_path: form.qbit_save_path.trim(),
    qbit_local_dir: form.qbit_local_dir.trim(),
    transmission_url: form.transmission_url.trim(),
    transmission_user: form.transmission_user.trim(),
    transmission_download_dir: form.transmission_download_dir.trim(),
    transmission_local_dir: form.transmission_local_dir.trim(),
    proxy_url: form.proxy_url.trim(),
    proxy_clear_credentials: form.proxy_clear_credentials,
    poll_minutes: Number.parseInt(String(form.poll_minutes), 10) || 30,
    discover_search_template: form.discover_search_template.trim(),
    library_dir: form.library_dir.trim(),
    auto_cleanup_empty_anime: form.auto_cleanup_empty_anime,
  }
  /* 敏感字段留空表示保持不变，只有填写时才提交 */
  if (form.aria2_token) body.aria2_token = form.aria2_token
  if (form.qbit_password) body.qbit_password = form.qbit_password
  if (form.transmission_password) body.transmission_password = form.transmission_password
  if (form.web_password) {
    if (!body.web_user) {
      msg.value = '设置密码前请先填写用户名'
      return
    }
    if (form.web_password.length < 6) {
      msg.value = '密码至少 6 位'
      return
    }
    body.web_password = form.web_password
  }
  saving.value = true
  try {
    const r = await http.put<{ ok: boolean; reauth_required: boolean; cleaned_anime: number; cleanup_warning: string }>('/api/settings', body)
    if (r.reauth_required) {
      toast.info('密码已更新，请重新登录')
      location.href = '/login'
      return
    }
    if (r.cleanup_warning) toast.error(r.cleanup_warning)
    else toast.success(r.cleaned_anime > 0 ? `设置已保存，已清理 ${r.cleaned_anime} 部空番剧` : '设置已保存并立即生效')
    await Promise.all([load(), store.refresh()])
  } catch (e) {
    msg.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page settings">
    <PageHeader title="设置" description="保存在数据库中并立即生效；未填写的项沿用环境变量或默认值">
      <BaseButton variant="primary" :icon="Save" :loading="saving" :disabled="loading" @click="save">保存设置</BaseButton>
    </PageHeader>

    <p v-if="loadErr" class="form-msg">读取设置失败：{{ loadErr }}</p>

    <div class="groups" :class="{ loading }">
      <!-- 管理员账户 -->
      <section class="group card" style="--i: 0">
        <header class="group-head">
          <span class="group-ico"><UserRound aria-hidden="true" /></span>
          <div>
            <h2>管理员账户</h2>
            <p>启用后访问面板需要登录；修改账户或密码后所有会话退出</p>
          </div>
        </header>
        <div class="field-row">
          <div class="field">
            <label for="st-user">用户名</label>
            <input id="st-user" v-model="form.web_user" type="text" autocomplete="username" />
          </div>
          <div class="field">
            <label for="st-pass">新密码{{ info?.web_password_set ? '（留空保持不变）' : '' }}</label>
            <input id="st-pass" v-model="form.web_password" type="password" autocomplete="new-password" placeholder="至少 6 位" />
          </div>
        </div>
        <p class="field-hint">普通设置无需重填密码。设置过账户后无法在页面撤销登录；修改用户名时必须同时填写新密码。</p>
      </section>

      <!-- 下载方式 -->
      <section class="group card" style="--i: 1">
        <header class="group-head">
          <span class="group-ico"><HardDrive aria-hidden="true" /></span>
          <div>
            <h2>下载方式</h2>
            <p>{{ MODE_DESC[form.download_mode] }}</p>
          </div>
        </header>

        <select v-if="narrow" v-model="form.download_mode" class="mode-select" aria-label="下载模式">
          <option v-for="m in MODES" :key="m" :value="m">{{ MODE_LABEL[m] }}</option>
        </select>
        <div v-else class="seg" role="radiogroup" aria-label="下载模式" :style="{ '--active': modeIndex, '--n': MODES.length }">
          <span class="seg-indicator" aria-hidden="true" />
          <button
            v-for="m in MODES"
            :key="m"
            type="button"
            role="radio"
            class="seg-btn"
            :class="{ active: form.download_mode === m }"
            :aria-checked="form.download_mode === m"
            @click="form.download_mode = m"
          >
            {{ MODE_LABEL[m] }}
          </button>
        </div>

        <Transition name="fade" mode="out-in">
          <div v-if="form.download_mode === 'aria2'" key="aria2" class="mode-panel">
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

          <div v-else key="direct" class="mode-panel direct-note">
            <p class="field-hint">磁力链接与种子文件需要配置 BT 下载器。</p>
          </div>
        </Transition>
        <div class="field">
          <label for="st-concurrent">同时下载数量</label>
          <input id="st-concurrent" v-model.number="form.max_concurrent_downloads" type="number" min="1" max="32" step="1" required />
          <p class="field-hint">范围 1–32。降低上限后，正在下载的任务继续完成，新任务排队。外部下载器自身的队列上限也会影响实际并发数。</p>
        </div>
      </section>

      <!-- 媒体库 -->
      <section class="group card" style="--i: 2">
        <header class="group-head">
          <span class="group-ico"><FolderTree aria-hidden="true" /></span>
          <div>
            <h2>媒体库</h2>
            <p>一部番一个文件夹，按 Season 01 / 番名 - S01E07 组织，适配飞牛影视、Emby、Jellyfin</p>
          </div>
        </header>
        <div class="field">
          <label for="st-library">媒体库目录（番剧文件夹的根目录；留空使用下载目录）</label>
          <input id="st-library" v-model="form.library_dir" type="text" spellcheck="false" placeholder="例如 /vol1/1000/番剧" />
          <p class="field-hint">
            媒体服务器扫描该目录即可。当前下载目录：<code>{{ info?.download_dir || '未知' }}</code>
          </p>
        </div>
        <div class="field">
          <label class="check" for="st-cleanup-empty">
            <input id="st-cleanup-empty" v-model="form.auto_cleanup_empty_anime" type="checkbox" />
            自动清理空番剧
          </label>
          <p class="field-hint">默认关闭。开启并保存后立即检查，之后每分钟自动检查。仅清理没有资源、没有订阅指定归属、没有手动元数据绑定或自定义搜索名、季号、存储位置、归并规则的空番剧记录。暂停的订阅也会保留其关联番剧；不会删除媒体文件。</p>
        </div>
      </section>

      <!-- 网络与同步 -->
      <section class="group card" style="--i: 3">
        <header class="group-head">
          <span class="group-ico"><Globe aria-hidden="true" /></span>
          <div>
            <h2>网络与同步</h2>
            <p>代理只作用于 RSS、找番搜索、直链下载、封面与 Webhook；下载器 RPC 始终直连</p>
          </div>
        </header>
        <div class="field-row">
          <div class="field">
            <label for="st-proxy">出站代理（留空直连；支持 http、https、socks5）</label>
            <input id="st-proxy" v-model="form.proxy_url" type="text" spellcheck="false" placeholder="http://192.168.1.10:7890" />
            <label class="check small">
              <input v-model="form.proxy_clear_credentials" type="checkbox" />
              清除代理账号密码
            </label>
            <p class="field-hint">
              {{
                info?.proxy_credentials_set
                  ? '已配置代理认证；地址不变时保存会保留，勾选上方选项可清除。'
                  : '未配置代理认证；需要时可在代理地址中填写账号密码，保存后不会回显。'
              }}
            </p>
          </div>
          <div class="field">
            <label for="st-poll">RSS 同步周期（分钟）</label>
            <input id="st-poll" v-model.number="form.poll_minutes" type="number" min="5" max="1440" />
            <p class="field-hint">修改后从下一轮同步开始生效，最小 5 分钟。</p>
          </div>
        </div>
        <div class="field">
          <label for="st-discover">找番自定义搜索源（RSS 地址模板，含 {keyword} 占位符；留空只用内置源）</label>
          <input
            id="st-discover"
            v-model="form.discover_search_template"
            type="text"
            spellcheck="false"
            placeholder="例如 https://example.com/search/rss?term={keyword}"
          />
        </div>
      </section>
    </div>

    <div class="save-bar">
      <Transition name="fade">
        <p v-if="msg" class="form-msg" role="alert">{{ msg }}</p>
      </Transition>
      <BaseButton variant="primary" :icon="Save" :loading="saving" :disabled="loading" @click="save">保存设置</BaseButton>
    </div>
  </div>
</template>

<style scoped>
.settings {
  max-width: 900px;
}
.groups {
  display: grid;
  gap: 16px;
  transition: opacity var(--t-med) var(--ease-out);
}
.groups.loading {
  opacity: 0.55;
  pointer-events: none;
}
.group {
  padding: 20px 22px 12px;
  animation: rise-in var(--t-slow) var(--ease-out) both;
  animation-delay: calc(var(--i) * 70ms + 60ms);
}
.group-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 18px;
}
.group-head h2 {
  font-size: 15.5px;
  font-weight: 700;
  letter-spacing: -0.01em;
}
.group-head p {
  margin-top: 3px;
  color: var(--text-3);
  font-size: 12.5px;
  line-height: 1.55;
}
.group-ico {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 11px;
  color: var(--accent);
  background: var(--accent-soft);
  flex: none;
}
.group-ico svg {
  width: 17px;
  height: 17px;
}

.seg {
  position: relative;
  display: grid;
  grid-template-columns: repeat(var(--n), 1fr);
  padding: 4px;
  margin-bottom: 16px;
  border-radius: var(--r-md);
  background: var(--bg-2);
  border: 1px solid var(--line);
}
.seg-indicator {
  position: absolute;
  top: 4px;
  left: 4px;
  width: calc((100% - 8px) / var(--n));
  height: calc(100% - 8px);
  border-radius: 10px;
  background: var(--surface);
  box-shadow: var(--shadow-sm), inset 0 0 0 1px var(--accent-border);
  transform: translateX(calc(var(--active) * 100%));
  transition: transform 420ms var(--ease-spring);
  pointer-events: none;
}
.seg-btn {
  position: relative;
  z-index: 1;
  height: 36px;
  border: 0;
  background: transparent;
  color: var(--text-2);
  font-weight: 550;
  font-size: 13.5px;
  border-radius: 10px;
  cursor: pointer;
  transition: color var(--t-med) var(--ease-out);
}
.seg-btn:hover {
  color: var(--text);
}
.seg-btn.active {
  color: var(--accent);
  font-weight: 650;
}
.mode-select {
  margin-bottom: 16px;
}
.mode-panel {
  min-height: 24px;
}
.direct-note {
  padding-bottom: 12px;
}
.check.small {
  font-size: 13px;
  margin: 6px 0 4px;
}

.save-bar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 14px;
  margin-top: 18px;
}
.save-bar .form-msg {
  margin: 0;
}
@media (max-width: 640px) {
  .group {
    padding: 16px 16px 8px;
  }
  .save-bar {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
