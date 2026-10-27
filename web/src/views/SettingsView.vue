<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useEventListener, useMediaQuery } from '@vueuse/core'
import { onBeforeRouteLeave } from 'vue-router'
import { confirmDialog } from '@/stores/confirm'
import { FolderTree, Globe, HardDrive, RefreshCw, Rocket, Save, UserRound } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import BangumiWishes from '@/components/discover/BangumiWishes.vue'
import { api, http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import { useAppStore } from '@/stores/app'
import { useDialogs } from '@/stores/dialogs'
import { useUpdateStore } from '@/stores/update'
import { MODE_LABEL, type Settings, type SettingsInput, type SetupStorageResult, type StorageCheckInput } from '@/api/types'
import { isWindows, storageDirExample } from '@/utils/platform'

const store = useAppStore()
const update = useUpdateStore()
const { openUpdate } = useDialogs()
const narrow = useMediaQuery('(max-width: 1000px)')
/* 运行平台（/api/settings）：Windows 桌面版没有 Docker 卷映射，文案与路径示例不同。 */
const isWindowsOS = computed(() => isWindows(info.value?.os))
const pathExample = computed(() => storageDirExample(info.value?.os))

const ALL_MODES = ['native', 'direct', 'aria2', 'qbittorrent', 'transmission'] as const
const MODE_DESC: Record<string, string> = {
  native: '程序内置 BT 引擎下载磁力与种子，完成后归档；普通直链仍由本程序下载，无需安装 qBittorrent',
  direct: 'HTTP/HTTPS 直链由本程序直接下载，磁力与种子需要外部下载器',
  aria2: '通过 JSON-RPC 提交任务，完成后从映射路径复制归档',
  qbittorrent: '通过 WebUI 接口提交任务并按标签跟踪进度',
  transmission: '通过 RPC 提交任务并轮询完成状态',
}
const availableModes = computed(() => {
  const modes: string[] = [...ALL_MODES]
  if (form.download_mode && !modes.includes(form.download_mode)) modes.push(form.download_mode)
  return modes
})

const form = reactive({
  web_user: '',
  web_password: '',
  download_mode: 'direct',
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
  keep_source_files: true,
})

const info = ref<Settings | null>(null)
const loading = ref(true)
const saving = ref(false)
const storageLoading = ref(false)
const storageResult = ref<SetupStorageResult | null>(null)
const storageError = ref('')
let storageController: AbortController | null = null
let storageVersion = 0
function clearStorageCheck() {
  storageVersion++
  storageController?.abort()
  storageLoading.value = false
  storageResult.value = null
  storageError.value = ''
}
watch(() => form.library_dir, clearStorageCheck, { flush: 'sync' })
onBeforeUnmount(clearStorageCheck)
async function checkStorage() {
  if (loading.value || saving.value || storageLoading.value || loadErr.value) return
  const path = form.library_dir.trim()
  const version = ++storageVersion
  storageController?.abort()
  const controller = new AbortController()
  storageController = controller
  storageLoading.value = true
  storageError.value = ''
  storageResult.value = null
  try {
    const body: StorageCheckInput = { path }
    const result = await api<SetupStorageResult>('/api/settings/storage', { method: 'POST', json: body, signal: controller.signal })
    if (version !== storageVersion || path !== form.library_dir.trim()) return
    if (result.path !== (path || info.value?.download_dir) || typeof result.writable !== 'boolean' || typeof result.message !== 'string' || !['mapped', 'unmapped', 'unknown', 'not_required'].includes(result.mount_status)) throw new Error('目录检测结果不完整')
    storageResult.value = result
  } catch (e) {
    if (version === storageVersion) storageError.value = errorMessage(e)
  } finally {
    if (version === storageVersion) storageLoading.value = false
  }
}
const msg = ref('')
const loadErr = ref('')
const section = ref('download')
const sections = [{ id: 'download', title: '下载方式', icon: HardDrive }, { id: 'library', title: '媒体库', icon: FolderTree }, { id: 'network', title: '网络与同步', icon: Globe }, { id: 'account', title: '管理员账户', icon: UserRound }, { id: 'update', title: '版本更新', icon: Rocket }]
const savedForm = ref('')
const dirty = computed(() => !!savedForm.value && JSON.stringify(form) !== savedForm.value)
onBeforeRouteLeave(async () => {
  if (saving.value) return false
  return !dirty.value || await confirmDialog('放弃未保存的更改？', '离开后，本次设置修改不会保存。', { okText: '放弃更改', cancelText: '继续编辑' })
})
useEventListener(window, 'beforeunload', (event: BeforeUnloadEvent) => {
  if (dirty.value) { event.preventDefault(); event.returnValue = '' }
})

const modeIndex = computed(() => Math.max(0, availableModes.value.indexOf(form.download_mode)))

async function load() {
  loading.value = true
  loadErr.value = ''
  try {
    const s = await http.get<Settings>('/api/settings')
    info.value = s
    form.web_user = s.web_user || ''
    form.web_password = ''
    form.download_mode = s.download_mode || 'direct'
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
    form.keep_source_files = s.keep_source_files !== false
    savedForm.value = JSON.stringify(form)
  } catch (e) {
    loadErr.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
  void update.checkNow()
})

async function save() {
  if (loading.value || saving.value || loadErr.value || !dirty.value) return
  msg.value = ''
  const body: SettingsInput = {
    web_user: form.web_user.trim(),
    download_mode: form.download_mode,
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
    keep_source_files: form.keep_source_files,
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
    const r = await http.put<{ ok: boolean; reauth_required: boolean; cleaned_anime: number; cleanup_warning: string; library_warning: string }>('/api/settings', body)
    if (r.reauth_required) {
      form.web_password = ''
      savedForm.value = JSON.stringify(form)
      toast.info('密码已更新，请重新登录')
      location.href = '/login'
      return
    }
    if (r.library_warning) toast.error('设置已保存，但媒体库目录需要确认：' + r.library_warning)
    else if (r.cleanup_warning) toast.error(r.cleanup_warning)
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
    <PageHeader title="设置" description="让追番服务按照你的习惯运行" />

    <div v-if="loadErr" class="form-msg" role="alert">读取设置失败：{{ loadErr }} <BaseButton @click="load">重新加载</BaseButton></div>
    <nav class="settings-nav" aria-label="设置分类">
      <button v-for="tab in sections" :key="tab.id" type="button" :aria-current="section === tab.id ? 'true' : undefined" :class="{ active: section === tab.id }" @click="section = tab.id"><component :is="tab.icon" aria-hidden="true" />{{ tab.title }}</button>
    </nav>

    <fieldset class="groups" :class="{ loading }" :disabled="loading || saving || !!loadErr" :aria-busy="loading">
      <!-- 管理员账户 -->
      <section v-show="section === 'account'" class="group card" style="--i: 0">
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
      <section v-show="section === 'download'" class="group card" style="--i: 1">
        <header class="group-head">
          <span class="group-ico"><HardDrive aria-hidden="true" /></span>
          <div>
            <h2>下载方式</h2>
            <p>{{ MODE_DESC[form.download_mode] }}</p>
          </div>
        </header>

        <select v-if="narrow" v-model="form.download_mode" class="mode-select" aria-label="下载模式">
          <option v-for="m in availableModes" :key="m" :value="m">{{ MODE_LABEL[m] }}</option>
        </select>
        <div v-else class="seg" role="radiogroup" aria-label="下载模式" :style="{ '--active': modeIndex, '--n': availableModes.length }">
          <span class="seg-indicator" aria-hidden="true" />
          <button
            v-for="m in availableModes"
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
          <div v-if="form.download_mode === 'native'" key="native" class="mode-panel direct-note">
            <p class="field-hint">程序内置 BT 引擎，无需安装 qBittorrent。磁力与种子由本程序下载，普通直链仍走本地下载；任务和续传数据保存在数据目录的 <code>bt</code> 子目录，重启后继续。</p>
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

          <div v-else key="direct" class="mode-panel direct-note">
            <p class="field-hint">磁力链接与种子文件需要配置 BT 下载器。</p>
          </div>
        </Transition>
      </section>

      <!-- 媒体库 -->
      <section v-show="section === 'library'" class="group card" style="--i: 2">
        <header class="group-head">
          <span class="group-ico"><FolderTree aria-hidden="true" /></span>
          <div>
            <h2>媒体库</h2>
            <p>一部番一个文件夹，按 Season 01 / 番名 - S01E07 组织，适配飞牛影视、Emby、Jellyfin</p>
          </div>
        </header>
        <div class="field">
          <label for="st-library">媒体库目录（番剧文件夹的根目录；留空使用下载目录）</label>
          <input id="st-library" v-model="form.library_dir" type="text" spellcheck="false" :placeholder="`例如 ${pathExample}`" />
          <BaseButton :icon="FolderTree" :loading="storageLoading" :disabled="loading || saving || !!loadErr" @click="checkStorage">检测保存目录</BaseButton>
          <p v-if="storageResult" class="field-hint" role="status">{{ storageResult.message }}</p>
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
      </section>

      <!-- 版本更新 -->
      <section v-show="section === 'update'" class="group card" style="--i: 3">
        <header class="group-head">
          <span class="group-ico"><Rocket aria-hidden="true" /></span>
          <div>
            <h2>版本更新</h2>
            <p>启动后自动检查新版本；有新版本时侧边栏会出现提醒，可一键更新并重启服务</p>
          </div>
        </header>
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
          <BaseButton v-if="update.updateAvailable.value" variant="primary" :icon="Rocket" @click="openUpdate()">
            查看更新内容
          </BaseButton>
          <BaseButton v-if="update.skipped.value" variant="ghost" size="sm" @click="update.restoreSkipped()">
            恢复 {{ update.skipped.value }} 的提醒
          </BaseButton>
        </div>
      </section>

      <!-- 网络与同步 -->
      <section v-show="section === 'network'" class="group card" style="--i: 4">
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
          <label for="st-discover">找番搜索源（RSS 地址模板，含 {keyword} 占位符；留空不启用资源搜索）</label>
          <input
            id="st-discover"
            v-model="form.discover_search_template"
            type="text"
            spellcheck="false"
            placeholder="例如 https://example.com/search/rss?term={keyword}"
          />
          <p class="field-hint">应用不内置或推荐资源来源。请自行确认来源的使用条款与内容授权；保存仅校验格式，不检测连通性。留空仍可手动添加 RSS，已有订阅不受影响。</p>
        </div>
      </section>
    </fieldset>

    <BangumiWishes v-if="section === 'network'" />
    <div class="save-bar">
      <span class="save-status" role="status">{{ loading ? '正在读取设置…' : loadErr ? '读取失败后无法保存' : dirty ? '有未保存的更改' : '设置已与服务同步' }}</span>
      <Transition name="fade">
        <p v-if="msg" class="form-msg" role="alert">{{ msg }}</p>
      </Transition>
      <BaseButton variant="primary" :icon="Save" :loading="saving" :disabled="loading || !!loadErr || !dirty" @click="save">保存设置</BaseButton>
    </div>
  </div>
</template>

<style scoped>
.settings {
  max-width: 900px;
}
.groups {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
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
.update-meta {
  margin-bottom: 6px;
}
.update-meta code {
  font-family: var(--mono);
  font-size: 11.5px;
}
.field-hint.hot {
  color: var(--accent);
  font-weight: 550;
}
.update-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin: 12px 0 16px;
}

.save-bar {
  position: sticky;
  bottom: 18px;
  padding: 16px;
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: var(--shadow-sm);
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 14px;
  margin-top: 18px;
}
.save-status { margin-right: auto; color: var(--text-2); font-size: 13px; }
.settings-nav { display: flex; gap: 6px; margin-bottom: 24px; overflow-x: auto; padding-bottom: 6px; }
.settings-nav button { display: flex; align-items: center; gap: 8px; padding: 11px 14px; white-space: nowrap; background: transparent; border: 1px solid transparent; border-radius: var(--r-sm); color: var(--text-2); cursor: pointer; }
.settings-nav button:hover { background: var(--surface-2); }
.settings-nav button.active { background: var(--accent-soft); color: var(--accent); }
.settings-nav svg { width: 16px; height: 16px; }
@media (max-width: 900px) { .save-bar { bottom: calc(84px + env(safe-area-inset-bottom)); } }
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
