<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ArrowLeft, ArrowRight, Check, Globe, HardDrive, UserRound } from '@lucide/vue'
import AuthShell from '@/components/auth/AuthShell.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { confirmDialog } from '@/stores/confirm'
import { MODE_LABEL } from '@/api/types'

const STEPS = [
  { key: 'account', title: '管理员账户', icon: UserRound, hint: '设置后访问面板需要登录；留空则暂不启用登录，之后可在设置页开启。' },
  { key: 'download', title: '下载方式', icon: HardDrive, hint: 'Docker 默认使用内置 qBittorrent，也可连接已有下载器或选择本地直链。' },
  { key: 'network', title: '网络与同步', icon: Globe, hint: '代理只作用于 RSS、找番、直链下载与封面；下载器 RPC 始终直连。' },
] as const

const MODES = ['builtin', 'direct', 'aria2', 'qbittorrent', 'transmission'] as const

const step = ref(0)
const dir = ref<1 | -1>(1)
const msg = ref('')
const busy = ref(false)

const form = reactive({
  user: '',
  pass: '',
  pass2: '',
  mode: 'direct' as (typeof MODES)[number],
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
})

const current = computed(() => STEPS[step.value]!)
const isLast = computed(() => step.value === STEPS.length - 1)

onMounted(async () => {
  try {
    const r = await fetch('/api/setup/status')
    const s = (await r.json()) as { required: boolean; download_mode?: (typeof MODES)[number] }
    if (!s.required) location.replace('/')
    if (s.download_mode && MODES.includes(s.download_mode)) form.mode = s.download_mode
  } catch {
    /* 状态接口不可用时留在向导页 */
  }
})

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

function go(delta: 1 | -1) {
  msg.value = ''
  if (delta > 0) {
    const err = step.value === 0 ? validateAccount() : step.value === 2 ? validateNetwork() : ''
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
    proxy_url: form.proxy_url.trim(),
    poll_minutes: Number.parseInt(String(form.poll_minutes), 10) || 30,
  }
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
    if (body.web_user && body.web_password) {
      await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: body.web_user, password: body.web_password }),
      })
    }
    location.replace('/')
  } catch {
    msg.value = '网络错误，请稍后重试'
  } finally {
    busy.value = false
  }
}

async function finish() {
  msg.value = ''
  const err = validateAccount() || validateNetwork()
  if (err) {
    msg.value = err
    return
  }
  const body = collect()
  const hasDownloader = !!(body.aria2_url || body.qbit_url || body.transmission_url)
  if (hasDownloader && !body.web_user) {
    // 配置了下载器但没设置登录：确认一次，避免面板裸奔
    const ok = await confirmDialog('尚未设置管理员账户', '面板任何人都可访问，下载器凭据也可能被读取。仍要继续吗？', {
      okText: '仍要继续',
      danger: true,
    })
    if (!ok) return
  }
  await submit(body)
}

async function skip() {
  msg.value = ''
  const ok = await confirmDialog('跳过初始设置', '将使用默认配置启动，不启用登录；之后可以在设置页随时修改。', {
    okText: '跳过并进入',
  })
  if (!ok) return
  await submit({})
}
</script>

<template>
  <AuthShell subtitle="首次安装 · 完成基本设置即可开始使用" wide>
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
              <p>{{ current.hint }}</p>
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
                <option v-for="m in MODES" :key="m" :value="m">{{ MODE_LABEL[m] }}</option>
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
                    <label for="su-aria2-dir">Aria2 下载目录（Aria2 容器内路径）</label>
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
              <p v-else-if="form.mode === 'builtin'" key="builtin" class="field-hint direct-hint">
                镜像已包含 qBittorrent，由程序自动启动和连接，无需填写地址或密码。任务与续传数据保存在数据目录，重启后继续下载。
              </p>
              <p v-else key="direct" class="field-hint direct-hint">
                本地直链会直接下载 HTTP/HTTPS 资源；磁力链接与种子在没有下载器时会停在「等待下载器」，之后可在设置页补充。
              </p>
            </Transition>
          </template>

          <template v-else>
            <div class="field">
              <label for="su-proxy">出站代理（可选，支持 http、socks5；留空直连）</label>
              <input id="su-proxy" v-model="form.proxy_url" type="text" placeholder="例如 http://192.168.1.10:7890 或 socks5://…" spellcheck="false" />
            </div>
            <div class="field">
              <label for="su-poll">RSS 同步周期（分钟，最小 5）</label>
              <input id="su-poll" v-model.number="form.poll_minutes" type="number" min="5" max="1440" />
            </div>
          </template>
        </section>
      </Transition>
    </div>

    <p class="form-msg" role="alert">{{ msg }}</p>

    <div class="actions">
      <BaseButton variant="ghost" :disabled="busy" @click="skip">跳过，稍后设置</BaseButton>
      <span class="spacer" />
      <BaseButton v-if="step > 0" :icon="ArrowLeft" :disabled="busy" @click="go(-1)">上一步</BaseButton>
      <BaseButton v-if="!isLast" variant="primary" :icon="ArrowRight" @click="go(1)">下一步</BaseButton>
      <BaseButton v-else variant="primary" :icon="Check" :loading="busy" @click="finish">完成设置，开始使用</BaseButton>
    </div>
  </AuthShell>
  <ConfirmHost />
</template>

<style scoped>
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
