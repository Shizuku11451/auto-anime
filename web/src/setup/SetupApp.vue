<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ArrowLeft, ArrowRight, Check, FolderOpen, Globe, HardDrive, UserRound } from '@lucide/vue'
import AuthShell from '@/components/auth/AuthShell.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { confirmDialog } from '@/stores/confirm'
import { MODE_LABEL } from '@/api/types'
import type { SetupResult, SetupStatus, SetupStorageResult } from '@/api/types'
import { isWindows } from '@/utils/platform'

const STEPS = [
  { key: 'account', title: '管理员账户', icon: UserRound, hint: '建议创建账户，保护订阅与下载管理；这是 Auto-Anime 的账户，与 NAS 账户无关。' },
  { key: 'download', title: '下载方式', icon: HardDrive, hint: '默认使用内置 BT 下载器，也可连接已有下载器或选择本地直链。' },
  { key: 'storage', title: '保存目录', icon: FolderOpen, hint: '选择整理好的番剧放在哪里，飞牛影视、Emby 或 Jellyfin 扫描这个目录。' },
  { key: 'network', title: '确认并开始', icon: Globe, hint: '已沿用部署配置；未配置时默认直连、每 30 分钟同步。确认后即可开始添加订阅。' },
] as const

/* 运行平台来自 /api/setup/status：Windows 桌面版没有 Docker 卷映射，文案不同。 */
const platformOS = ref('')
const isWindowsOS = computed(() => isWindows(platformOS.value))
/* 下载方式步骤的说明按平台切换；其余步骤的静态说明直接沿用。 */
const currentHint = computed(() => {
  if (current.value.key === 'download') {
    return isWindowsOS.value
      ? 'Windows 默认使用内置 BT 下载器，也可连接已有下载器或选择本地直链。'
      : 'Docker 默认使用内置 BT 下载器，也可连接已有下载器或选择本地直链。'
  }
  return current.value.hint
})
const ALL_MODES = ['native', 'direct', 'aria2', 'qbittorrent', 'transmission'] as const
const availableModes = computed(() => {
  const modes: string[] = [...ALL_MODES]
  if (form.mode && !modes.includes(form.mode)) modes.push(form.mode)
  return modes
})

const step = ref(0)
const dir = ref<1 | -1>(1)
const msg = ref('')
const busy = ref(false)
const confirming = ref(false)
const statusLoaded = ref(false)
const statusLoading = ref(false)
const statusError = ref('')
const defaultDownloadDir = ref('')
const deploymentRoot = ref('')
const storageResult = ref<SetupStorageResult | null>(null)
const storageLoading = ref(false)
const storageError = ref('')
let storageController: AbortController | null = null
let storageVersion = 0
const deploymentProxyEnabled = ref(false)
const completed = ref(false)
const libraryWarning = ref('')
const loginFailed = ref(false)
const locked = computed(() => busy.value || confirming.value || !statusLoaded.value)
let statusController: AbortController | null = null
let statusVersion = 0

const form = reactive({
  user: '',
  pass: '',
  pass2: '',
  mode: 'direct' as (typeof ALL_MODES)[number],
  library_dir: '',
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
  poll_minutes: 30,
  keep_source_files: true,
})

const current = computed(() => STEPS[step.value]!)
const isLast = computed(() => step.value === STEPS.length - 1)
const archiveRoot = computed(() => form.library_dir.trim() || defaultDownloadDir.value)
const canCheckStorage = computed(() => archiveRoot.value === deploymentRoot.value)
watch(archiveRoot, () => {
  storageVersion++
  storageController?.abort()
  storageResult.value = null
  storageError.value = ''
  storageLoading.value = false
})
const proxyLabel = computed(() => form.proxy_url.trim() ? '使用出站代理' : deploymentProxyEnabled.value ? '沿用部署代理' : '直连，无需代理')

async function loadStatus() {
  const version = ++statusVersion
  statusController?.abort()
  statusController = new AbortController()
  const requestController = statusController
  const timeout = setTimeout(() => requestController.abort(), 15000)
  statusLoaded.value = false
  statusLoading.value = true
  statusError.value = ''
  try {
    const r = await fetch('/api/setup/status', { signal: statusController.signal })
    if (!r.ok) throw new Error('读取失败')
    const s = (await r.json()) as SetupStatus
    if (version !== statusVersion) return
    if (typeof s.required !== 'boolean' || !(ALL_MODES as readonly string[]).includes(s.download_mode)) throw new Error('配置不完整')
    if (!s.required) { location.replace('/'); return }
    form.mode = s.download_mode
    form.library_dir = s.library_dir ?? ''
    defaultDownloadDir.value = s.download_dir ?? ''
    deploymentRoot.value = s.library_dir?.trim() || defaultDownloadDir.value
    form.poll_minutes = s.poll_minutes ?? 30
    deploymentProxyEnabled.value = s.proxy_enabled ?? false
    form.keep_source_files = s.keep_source_files !== false
    platformOS.value = s.os || ''
    statusLoaded.value = true
  } catch {
    if (version === statusVersion) statusError.value = '无法读取安装配置，请检查服务是否已启动，然后重试。'
  } finally {
    clearTimeout(timeout)
    if (version === statusVersion) statusLoading.value = false
  }
}

onMounted(loadStatus)
onBeforeUnmount(() => { statusVersion++; statusController?.abort(); storageVersion++; storageController?.abort() })

async function checkStorage() {
  if (!canCheckStorage.value || storageLoading.value || completed.value) return
  const version = ++storageVersion
  const root = archiveRoot.value
  storageController?.abort()
  const controller = new AbortController()
  storageController = controller
  const timeout = setTimeout(() => controller.abort(), 15000)
  storageLoading.value = true
  storageError.value = ''
  storageResult.value = null
  try {
    const response = await fetch('/api/setup/storage', { method: 'POST', signal: controller.signal })
    if (!response.ok) throw new Error('检测失败')
    const result = await response.json() as SetupStorageResult
    if (version !== storageVersion || root !== archiveRoot.value) return
    if (result.path !== root || typeof result.writable !== 'boolean' || !['mapped', 'unmapped', 'unknown', 'not_required'].includes(result.mount_status) || typeof result.message !== 'string') throw new Error('检测结果不完整')
    storageResult.value = result
  } catch {
    if (version === storageVersion) storageError.value = '未能完成目录检测，请检查服务后重试。'
  } finally {
    clearTimeout(timeout)
    if (version === storageVersion) storageLoading.value = false
  }
}

function validateAccount(): string {
  const u = form.user.trim()
  if ((u || form.pass) && (!u || !form.pass)) return '用户名和密码需要一起填写，或都留空跳过'
  if (form.pass && form.pass.length < 6) return '密码至少 6 位'
  if (form.pass && form.pass !== form.pass2) return '两次输入的密码不一致'
  return ''
}

function validateNetwork(): string {
  const n = Number(form.poll_minutes)
  if (!Number.isInteger(n) || n < 5 || n > 1440) return '同步周期需在 5 到 1440 分钟之间'
  return ''
}

function validateDownloader(): string {
  const address = form.mode === 'aria2' ? form.aria2_url : form.mode === 'qbittorrent' ? form.qbit_url : form.mode === 'transmission' ? form.transmission_url : null
  if (address === null) return ''
  try {
    const url = new URL(address.trim())
    if (url.protocol === 'http:' || url.protocol === 'https:') return ''
  } catch { /* 显示统一的连接地址提示 */ }
  return isWindowsOS.value
    ? '请填写下载器的 HTTP 或 HTTPS 地址；下载器在其他设备时请使用其 IP 或主机名。'
    : '请填写下载器的 HTTP 或 HTTPS 地址；另一个容器的地址请使用 NAS 局域网 IP 或服务名。'
}

function go(delta: 1 | -1) {
  if (locked.value) return
  msg.value = ''
  if (delta > 0) {
    const err = step.value === 0 ? validateAccount() : step.value === 1 ? validateDownloader() : ''
    if (err) {
      msg.value = err
      return
    }
  }
  dir.value = delta
  step.value = Math.min(STEPS.length - 1, Math.max(0, step.value + delta))
}

function collect() {
  const body: Record<string, unknown> = {
    download_mode: form.mode,
    library_dir: form.library_dir.trim(),
    poll_minutes: Number.parseInt(String(form.poll_minutes), 10) || 30,
    keep_source_files: form.keep_source_files,
  }
  // Empty input preserves a deployment proxy without exposing its address or
  // credentials through the anonymous setup status endpoint.
  if (form.proxy_url.trim()) body.proxy_url = form.proxy_url.trim()
  const u = form.user.trim()
  if (u || form.pass) {
    body.web_user = u
    body.web_password = form.pass
  }
  if (form.mode === 'aria2') {
    body.aria2_url = form.aria2_url.trim()
    body.aria2_token = form.aria2_token
    body.aria2_dir = form.aria2_dir.trim()
    body.aria2_local_dir = form.aria2_local_dir.trim()
  } else if (form.mode === 'qbittorrent') {
    body.qbit_url = form.qbit_url.trim()
    body.qbit_user = form.qbit_user.trim()
    body.qbit_password = form.qbit_password
    body.qbit_save_path = form.qbit_save_path.trim()
    body.qbit_local_dir = form.qbit_local_dir.trim()
  } else if (form.mode === 'transmission') {
    body.transmission_url = form.transmission_url.trim()
    body.transmission_user = form.transmission_user.trim()
    body.transmission_password = form.transmission_password
    body.transmission_download_dir = form.transmission_download_dir.trim()
    body.transmission_local_dir = form.transmission_local_dir.trim()
  }
  return body
}

async function submit(body: Record<string, unknown>) {
  busy.value = true
  try {
    const res = await fetch('/api/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) {
      msg.value = (await res.text()).trim() || '保存失败'
      return
    }
    // The configuration is committed; never offer to submit setup again if
    // login fails or a mount warning requires the user's attention.
    completed.value = true
    try {
      const result = (await res.json()) as SetupResult
      libraryWarning.value = result.library_warning ?? ''
    } catch {
      libraryWarning.value = '设置已保存，但未能读取目录检查结果。请进入设置页确认媒体库目录。'
    }
    if (body.web_user && body.web_password) {
      const login = await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: body.web_user, password: body.web_password }),
      })
      loginFailed.value = !login.ok
    }
  } catch {
    if (completed.value) loginFailed.value = true
    else msg.value = '网络错误，请稍后重试'
  } finally {
    busy.value = false
  }
}

async function finish() {
  if (locked.value || completed.value) return
  msg.value = ''
  const err = validateAccount() || validateDownloader() || validateNetwork()
  if (err) {
    msg.value = err
    return
  }
  const body = collect()
  if (!body.web_user) {
    confirming.value = true
    let ok = false
    try {
      ok = await confirmDialog('尚未设置管理员账户', '能访问此服务的人都可以管理订阅、下载和设置。仍要在不启用登录的情况下继续吗？', { okText: '仍要继续', danger: true })
    } finally { confirming.value = false }
    if (!ok) return
  }
  await submit(body)
}

async function skip() {
  if (locked.value || completed.value) return
  msg.value = ''
  confirming.value = true
  let ok = false
  try {
    ok = await confirmDialog('使用部署默认配置', '不保存当前填写的内容，也不启用登录。能访问此服务的人都可以管理订阅与下载；之后可在设置页创建账户。', { okText: '使用默认配置', danger: true })
  } finally { confirming.value = false }
  if (!ok) return
  await submit({})
}

function enterPanel() {
  location.replace(loginFailed.value ? '/login' : libraryWarning.value ? '/#/settings' : '/#/discover')
}
</script>

<template>
  <AuthShell subtitle="首次安装 · 完成基本设置即可开始使用" wide>
    <section v-if="completed" class="completion" aria-live="polite">
      <h2>安装设置已保存</h2>
      <p v-if="loginFailed">自动登录未成功，请使用刚设置的账户登录，无需重新安装。</p>
      <template v-else>
        <p v-if="libraryWarning" class="storage-warning" role="alert">{{ libraryWarning }}</p>
        <p v-if="libraryWarning">请在设置页确认媒体库目录。{{ isWindowsOS ? '目录要存在且当前用户可以写入，再开始下载。' : '容器内目录需要映射到 NAS 文件夹，再开始下载。' }}</p>
        <p v-else>下一步在「找番」搜索番名，预览资源并添加订阅。使用媒体服务器时，扫描{{ isWindowsOS ? '上面显示的媒体库目录' : 'Compose 中映射的 NAS 媒体文件夹' }}。</p>
      </template>
      <BaseButton variant="primary" :icon="ArrowRight" :disabled="busy" @click="enterPanel">{{ loginFailed ? '前往登录' : libraryWarning ? '检查媒体库目录' : '开始添加订阅' }}</BaseButton>
    </section>
    <section v-else-if="!statusLoaded" aria-live="polite">
      <p v-if="statusLoading">正在读取部署配置…</p>
      <template v-else>
        <p role="alert">{{ statusError }}</p>
        <BaseButton :loading="statusLoading" @click="loadStatus">重新读取配置</BaseButton>
      </template>
    </section>
    <template v-else>
    <ol class="steps" aria-label="设置步骤">
      <li
        v-for="(st, i) in STEPS"
        :key="st.key"
        class="step"
        :class="{ active: i === step, done: i < step }"
        :aria-current="i === step ? 'step' : undefined"
      >
        <span class="step-dot">
          <Check v-if="i < step" aria-hidden="true" />
          <template v-else>{{ i + 1 }}</template>
        </span>
        <span class="step-label">{{ st.title }}</span>
        <span v-if="i < STEPS.length - 1" class="step-line" aria-hidden="true" />
      </li>
    </ol>

    <div class="stage">
      <Transition :name="dir > 0 ? 'step-next' : 'step-prev'" mode="out-in">
        <section :key="current.key" class="panel">
          <header class="panel-head">
            <span class="panel-ico"><component :is="current.icon" aria-hidden="true" /></span>
            <div>
              <h2>{{ current.title }}</h2>
              <p>{{ currentHint }}</p>
            </div>
          </header>

          <template v-if="current.key === 'account'">
            <div class="field">
              <label for="su-user">用户名</label>
              <input id="su-user" v-model="form.user" type="text" autocomplete="username" autofocus />
            </div>
            <div class="field-row">
              <div class="field">
                <label for="su-pass">密码（至少 6 位）</label>
                <input id="su-pass" v-model="form.pass" type="password" autocomplete="new-password" />
              </div>
              <div class="field">
                <label for="su-pass2">确认密码</label>
                <input id="su-pass2" v-model="form.pass2" type="password" autocomplete="new-password" />
              </div>
            </div>
          </template>

          <template v-else-if="current.key === 'download'">
            <div class="field">
              <label for="su-mode">下载模式</label>
              <select id="su-mode" v-model="form.mode">
                <option v-for="m in availableModes" :key="m" :value="m">{{ MODE_LABEL[m] }}</option>
              </select>
            </div>
            <Transition name="fade" mode="out-in">
              <div v-if="form.mode === 'aria2'" key="aria2">
                <div class="field">
                  <label for="su-aria2-url">Aria2 RPC 地址（如 http://NAS地址:6800/jsonrpc）</label>
                  <input id="su-aria2-url" v-model="form.aria2_url" type="text" placeholder="http://127.0.0.1:6800/jsonrpc" spellcheck="false" />
                </div>
                <div class="field-row">
                  <div class="field">
                    <label for="su-aria2-token">RPC 密钥（可选）</label>
                    <input id="su-aria2-token" v-model="form.aria2_token" type="password" autocomplete="off" />
                  </div>
                  <div class="field">
                    <label for="su-aria2-dir">Aria2 下载目录（{{ isWindowsOS ? 'Aria2 侧路径' : 'Aria2 容器内路径' }}）</label>
                    <input id="su-aria2-dir" v-model="form.aria2_dir" type="text" placeholder="/downloads" spellcheck="false" />
                  </div>
                </div>
                <div class="field">
                  <label for="su-aria2-local">同一目录映射到本程序的路径（两侧相同时可留空）</label>
                  <input id="su-aria2-local" v-model="form.aria2_local_dir" type="text" placeholder="/downloads" spellcheck="false" />
                </div>
              </div>
              <div v-else-if="form.mode === 'qbittorrent'" key="qbit">
                <div class="field">
                  <label for="su-qbit-url">qBittorrent WebUI 地址</label>
                  <input id="su-qbit-url" v-model="form.qbit_url" type="text" placeholder="http://127.0.0.1:8080" spellcheck="false" />
                </div>
                <div class="field-row">
                  <div class="field">
                    <label for="su-qbit-user">用户名</label>
                    <input id="su-qbit-user" v-model="form.qbit_user" type="text" autocomplete="off" />
                  </div>
                  <div class="field">
                    <label for="su-qbit-pass">密码</label>
                    <input id="su-qbit-pass" v-model="form.qbit_password" type="password" autocomplete="new-password" />
                  </div>
                </div>
                <div class="field-row">
                  <div class="field">
                    <label for="su-qbit-save">保存目录（qBittorrent 内路径）</label>
                    <input id="su-qbit-save" v-model="form.qbit_save_path" type="text" spellcheck="false" />
                  </div>
                  <div class="field">
                    <label for="su-qbit-local">映射到本程序的路径</label>
                    <input id="su-qbit-local" v-model="form.qbit_local_dir" type="text" spellcheck="false" />
                  </div>
                </div>
              </div>
              <div v-else-if="form.mode === 'transmission'" key="tr">
                <div class="field">
                  <label for="su-tr-url">Transmission RPC 地址</label>
                  <input id="su-tr-url" v-model="form.transmission_url" type="text" placeholder="http://127.0.0.1:9091" spellcheck="false" />
                </div>
                <div class="field-row">
                  <div class="field">
                    <label for="su-tr-user">用户名</label>
                    <input id="su-tr-user" v-model="form.transmission_user" type="text" autocomplete="off" />
                  </div>
                  <div class="field">
                    <label for="su-tr-pass">密码</label>
                    <input id="su-tr-pass" v-model="form.transmission_password" type="password" autocomplete="new-password" />
                  </div>
                </div>
                <div class="field-row">
                  <div class="field">
                    <label for="su-tr-dir">下载目录（Transmission 内路径）</label>
                    <input id="su-tr-dir" v-model="form.transmission_download_dir" type="text" spellcheck="false" />
                  </div>
                  <div class="field">
                    <label for="su-tr-local">映射到本程序的路径</label>
                    <input id="su-tr-local" v-model="form.transmission_local_dir" type="text" spellcheck="false" />
                  </div>
                </div>
              </div>
              <p v-if="form.mode === 'native'" key="native" class="field-hint direct-hint">
                程序内置 BT 引擎，无需安装 qBittorrent；磁力与种子由本程序下载，任务和续传数据保存在数据目录的 bt 子目录，重启后继续。容器部署需已映射 BT 端口段（默认 6881-6890）。
              </p>
              <p v-else key="direct" class="field-hint direct-hint">
                本地直链会直接下载 HTTP/HTTPS 资源；磁力链接与种子在没有下载器时会停在「等待下载器」，之后可在设置页补充。
              </p>
            </Transition>
          </template>

          <template v-else-if="current.key === 'storage'">
            <div class="storage-note">
              <p>整理好的番剧将保存到安装时配置的媒体文件夹，当前目录为 <strong class="storage-path">{{ archiveRoot }}</strong>。</p>
              <p v-if="isWindowsOS">媒体库目录可以是盘符路径（如 D:\番剧）或 UNC 共享（如 \\NAS\共享\番剧），归档按番剧文件夹与 Season 目录存放。</p>
              <template v-else>
                <p>使用飞牛模板时，无需重复填写 NAS 路径。飞牛影视选择 Compose 中映射的 NAS 文件夹。</p>
                <p>想换到其他 NAS 文件夹或端口，打开<a href="/install" target="_blank" rel="noopener noreferrer">部署配置页</a>生成新的 Compose，再在飞牛重建项目；重建时保留原来的 data 映射。</p>
              </template>
              <p>订阅的「保存位置」留空时沿用这里。BT 原文件保留供做种，整理后的番剧另存一份。</p>
            </div>
            <div class="storage-check">
              <BaseButton :icon="FolderOpen" :loading="storageLoading" :disabled="!canCheckStorage || locked" @click="checkStorage">检测保存目录</BaseButton>
              <p class="field-hint">仅检测部署时配置的目录，创建、写入并删除一个临时文件。</p>
              <p v-if="!canCheckStorage" class="field-hint">{{ isWindowsOS ? '当前选择已不同于部署目录。修改后请确认新目录存在且可写；这里的安装检测只针对部署目录。' : '当前选择已不同于部署目录。请先核对 Docker 映射；此安装检测仅适用于部署目录。' }}</p>
              <p v-if="storageError" class="storage-warning" role="alert">{{ storageError }}</p>
              <p v-if="storageResult" :class="{ 'storage-warning': !storageResult.writable || storageResult.mount_status === 'unknown' }" role="status">{{ storageResult.message }}</p>
            </div>
            <div class="field">
              <label class="check" for="su-keep-source">
                <input id="su-keep-source" v-model="form.keep_source_files" type="checkbox" />
                保留下载器原文件做种
              </label>
              <p class="field-hint">默认开启。关闭后，归档成功并写入数据库才停止对应任务、清理源文件；失败保留原文件。归档时仍需预留两份文件空间。外部下载器须配置明确的目录映射。</p>
            </div>
            <details>
              <summary>{{ isWindowsOS ? '高级：修改媒体库目录' : '高级：修改容器内媒体目录' }}</summary>
            <div class="field">
              <label for="su-library">媒体库目录（整理好的番剧）</label>
              <input id="su-library" v-model="form.library_dir" type="text" :placeholder="`留空使用 ${defaultDownloadDir}`" spellcheck="false" />
              <p class="field-hint">已按部署配置填写；留空沿用下载目录。修改这里不会搬迁已有文件。</p>
            </div>
              <p>预计保存到：<strong class="storage-path">{{ archiveRoot }}/番剧名/Season XX/</strong></p>
              <p v-if="isWindowsOS">填写盘符路径或 UNC 共享即可，例如 <code>D:\番剧</code>；留空沿用下载目录。</p>
              <p v-else>Docker 中填写容器内路径。例如 Compose 将 NAS 文件夹映射为 <code>/media</code>，这里就填 <code>/media</code>，飞牛影视扫描对应的 NAS 文件夹。</p>
            </details>
          </template>

          <template v-else>
            <dl class="setup-summary">
              <dt>登录账户</dt><dd>{{ form.user.trim() || '未启用登录' }}</dd>
              <dt>下载方式</dt><dd>{{ MODE_LABEL[form.mode] }}</dd>
              <dt>媒体库目录</dt><dd class="storage-path">{{ archiveRoot }}</dd>
              <dt>原文件做种</dt><dd>{{ form.keep_source_files ? '保留并继续做种' : '归档后删除' }}</dd>
              <dt>网络与同步</dt><dd>{{ proxyLabel }} · 每 {{ form.poll_minutes }} 分钟</dd>
            </dl>
            <details>
              <summary>调整代理与同步周期（可选）</summary>
            <div class="field">
              <label for="su-proxy">出站代理（可选，支持 http、socks5）</label>
              <input id="su-proxy" v-model="form.proxy_url" type="text" placeholder="例如 http://192.168.1.10:7890 或 socks5://…" spellcheck="false" />
              <p class="field-hint">留空沿用部署配置，未配置代理时直连。需要关闭已有代理时，可在安装后的设置页修改。</p>
            </div>
            <div class="field">
              <label for="su-poll">RSS 同步周期（分钟，最小 5）</label>
              <input id="su-poll" v-model.number="form.poll_minutes" type="number" min="5" max="1440" />
            </div>
            </details>
          </template>
        </section>
      </Transition>
    </div>

    <p class="form-msg" role="alert">{{ msg }}</p>

    <div class="actions">
      <BaseButton variant="ghost" :disabled="locked" @click="skip">使用默认配置</BaseButton>
      <span class="spacer" />
      <BaseButton v-if="step > 0" :icon="ArrowLeft" :disabled="locked" @click="go(-1)">上一步</BaseButton>
      <BaseButton v-if="!isLast" variant="primary" :icon="ArrowRight" :disabled="locked" @click="go(1)">下一步</BaseButton>
      <BaseButton v-else variant="primary" :icon="Check" :disabled="locked" :loading="busy" @click="finish">完成设置</BaseButton>
    </div>
    </template>
  </AuthShell>
  <ConfirmHost />
</template>

<style scoped>
.completion p { margin: 12px 0; color: var(--text-2); line-height: 1.7; }
.storage-warning { color: var(--amber) !important; }
.storage-check { margin: 16px 0; font-size: 13px; line-height: 1.7; }
.storage-note { padding: 14px; border-radius: var(--r-md); background: var(--surface-2); color: var(--text-2); font-size: 13px; line-height: 1.7; }
.storage-note p + p { margin-top: 8px; }
.storage-path { overflow-wrap: anywhere; }
.setup-summary { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 10px 18px; margin-bottom: 22px; font-size: 13px; }
.setup-summary dt { color: var(--text-2); }
.setup-summary dd { margin: 0; }
summary { cursor: pointer; color: var(--text-2); margin-bottom: 16px; }
.steps {
  list-style: none;
  margin: 0 0 24px;
  padding: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}
.step {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
  color: var(--text-3);
  font-size: 12.5px;
  font-weight: 600;
  transition: color var(--t-med) var(--ease-out);
}
.step:last-child {
  flex: none;
}
.step.active {
  color: var(--text);
}
.step.done {
  color: var(--accent);
}
.step-dot {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  flex: none;
  background: var(--surface-3);
  color: var(--text-2);
  font-size: 12px;
  font-weight: 700;
  transition:
    background var(--t-med) var(--ease-out),
    color var(--t-med) var(--ease-out),
    box-shadow var(--t-med) var(--ease-out),
    transform var(--t-med) var(--ease-spring);
}
.step.active .step-dot {
  background: var(--accent);
  color: var(--accent-contrast);
  box-shadow: 0 0 0 4px var(--accent-soft);
  transform: scale(1.08);
}
.step.done .step-dot {
  background: var(--accent-soft);
  color: var(--accent);
}
.step-dot svg {
  width: 14px;
  height: 14px;
}
.step-label {
  white-space: nowrap;
}
.step-line {
  flex: 1;
  height: 2px;
  min-width: 12px;
  border-radius: 2px;
  background: var(--line-strong);
  position: relative;
  overflow: hidden;
}
.step-line::after {
  content: '';
  position: absolute;
  inset: 0;
  background: var(--accent);
  transform: scaleX(0);
  transform-origin: left;
  transition: transform var(--t-slow) var(--ease-out);
}
.step.done .step-line::after {
  transform: scaleX(1);
}

.stage {
  position: relative;
  min-height: 280px;
}
.panel-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 18px;
}
.panel-head h2 {
  font-size: 17px;
  font-weight: 720;
  letter-spacing: -0.01em;
}
.panel-head p {
  margin-top: 3px;
  color: var(--text-2);
  font-size: 12.5px;
  line-height: 1.55;
}
.panel-ico {
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  border-radius: 12px;
  color: var(--accent);
  background: var(--accent-soft);
  flex: none;
}
.panel-ico svg {
  width: 18px;
  height: 18px;
}
.direct-hint {
  padding: 4px 0 8px;
}

.actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 6px;
}
.spacer {
  flex: 1;
}

/* 步骤切换：按方向滑入滑出 */
.step-next-enter-active,
.step-next-leave-active,
.step-prev-enter-active,
.step-prev-leave-active {
  transition:
    opacity 220ms var(--ease-out),
    transform 260ms var(--ease-out);
}
.step-next-enter-from {
  opacity: 0;
  transform: translateX(28px);
}
.step-next-leave-to {
  opacity: 0;
  transform: translateX(-28px);
}
.step-prev-enter-from {
  opacity: 0;
  transform: translateX(-28px);
}
.step-prev-leave-to {
  opacity: 0;
  transform: translateX(28px);
}

@media (max-width: 520px) {
  .step-label {
    display: none;
  }
  .step.active .step-label {
    display: inline;
  }
  .actions :deep(.btn) {
    flex: 1 1 auto;
  }
  .spacer {
    display: none;
  }
}
</style>
