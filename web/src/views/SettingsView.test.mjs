import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

const descriptor = parse(readFileSync(new URL('./SettingsView.vue', import.meta.url), 'utf8')).descriptor
const compiled = ts.transpileModule(compileScript(descriptor, { id: 'settings-test' }).content, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText

function setup() {
  const writes = []
  let fail = false
  let allowLeave = false
  let leave
  let beforeUnload
  let probe = async () => ({ path: '/temporary/library', writable: true, mount_status: 'mapped', message: '已映射，可以写入' })
  const data = { download_mode: 'direct', poll_minutes: 30, library_dir: '/temporary/library' }
  const location = { href: '' }
  const stub = { render: () => null }
  const BS = String.fromCharCode(92)
  const platform = {
    isWindows: os => os === 'windows',
    storageDirExample: os => (os === 'windows' ? `D:${BS}番剧` : '/vol1/1000/番剧'),
    isAbsoluteStoragePath: (value, os) => (os === 'windows'
      ? /^[a-zA-Z]:/.test(value) || value.startsWith(BS + BS) || value.startsWith('//')
      : value.startsWith('/') || value.startsWith(BS) || /^[a-zA-Z]:/.test(value)),
    isWindowsPlatform: value => !!value && value.split('/')[0] === 'windows',
  }
  const modules = {
    vue: { ...vue, onMounted: () => {}, onBeforeUnmount: () => {} },
    'vue-router': { onBeforeRouteLeave: fn => { leave = fn } },
    '@vueuse/core': { useMediaQuery: () => vue.ref(false), useEventListener: (_target, _event, fn) => { beforeUnload = fn } },
    '@lucide/vue': new Proxy({}, { get: () => stub }),
    '@/components/ui/PageHeader.vue': stub,
    '@/components/ui/BaseButton.vue': stub,
    '@/components/discover/BangumiWishes.vue': stub,
    '@/api/types': { MODE_LABEL: {} },
    '@/utils/platform': platform,
    '@/api/client': { api: (...args) => probe(...args), http: {
      get: async () => { if (fail) throw new Error('模拟读取失败'); return data },
      put: async (_url, body) => { writes.push(body); Object.assign(data, body); return { ok: true, reauth_required: !!body.web_password } },
    }, errorMessage: e => e.message },
    '@/stores/app': { useAppStore: () => ({ refresh: async () => {} }) },
    '@/stores/dialogs': { useDialogs: () => ({ openUpdate: () => {} }) },
    '@/stores/update': { useUpdateStore: () => ({ checkNow: async () => {} }) },
    '@/stores/toast': { toast: { success() {}, error() {}, info() {} } },
    '@/stores/confirm': { confirmDialog: async () => allowLeave },
  }
  const context = { exports: {}, window: {}, location, AbortController, require: id => { assert.ok(id in modules, id); return modules[id] } }
  runInNewContext(compiled, context)
  const state = context.exports.default.setup({}, { expose() {} })
  return { state, data, writes, location, probe: value => { probe = value }, fail: value => { fail = value }, allow: value => { allowLeave = value }, leave: () => leave(), unload: e => beforeUnload(e) }
}

test('内置 BT 下载器在所有平台都可选，已移除的内置 qBittorrent 不再列出，库里存的列表外旧值也不丢', async () => {
  const h = setup()
  await h.state.load()
  // 容器镜像默认 DOWNLOAD_MODE=native，Linux 上同样要能选到 native。
  assert.equal(h.state.availableModes.value.includes('native'), true)
  assert.equal(h.state.availableModes.value.includes('builtin'), false)
  h.data.os = 'windows'
  h.data.download_mode = 'native'
  await h.state.load()
  assert.equal(h.state.form.download_mode, 'native')
  // 界面列表里没有的旧值必须保留，不能在保存时被悄悄改掉。
  h.data.download_mode = 'legacy-mode'
  await h.state.load()
  assert.equal(h.state.form.download_mode, 'legacy-mode')
  assert.equal(h.state.availableModes.value.includes('legacy-mode'), true)
})

test('设置未读取或读取失败时不得提交默认值，重试成功后才允许保存', async () => {
  const h = setup()
  await h.state.save()
  assert.equal(h.writes.length, 0)
  h.fail(true)
  await h.state.load()
  h.state.form.poll_minutes = 60
  await h.state.save()
  assert.equal(h.writes.length, 0)
  assert.equal(h.state.loadErr.value, '模拟读取失败')
  h.fail(false)
  await h.state.load()
  assert.equal(h.state.form.poll_minutes, 30)
  h.state.form.poll_minutes = 45
  await h.state.save()
  assert.equal(h.writes.length, 1)
  assert.equal(h.writes[0].poll_minutes, 45)
  assert.equal(h.state.dirty.value, false)
})

test('分类切换保留输入，取消离开保留更改，明确放弃才允许离开', async () => {
  const h = setup()
  await h.state.load()
  assert.equal(await h.leave(), true)
  h.state.form.library_dir = '/temporary/new-library'
  h.state.section.value = 'network'
  assert.equal(h.state.form.library_dir, '/temporary/new-library')
  assert.equal(h.state.dirty.value, true)
  assert.equal(await h.leave(), false)
  h.allow(true)
  assert.equal(await h.leave(), true)
  assert.equal(h.writes.length, 0)
})

test('做种保留默认开启，关闭保存后刷新仍保留 false', async () => {
  const h = setup()
  await h.state.load()
  assert.equal(h.state.form.keep_source_files, true)
  h.state.form.keep_source_files = false
  await h.state.save()
  assert.equal(h.writes[0].keep_source_files, false)
  assert.equal(h.state.form.keep_source_files, false)
})

test('未更改时不重复保存，未保存输入触发刷新保护，保存中阻止离开', async () => {
  const h = setup()
  await h.state.load()
  await h.state.save()
  assert.equal(h.writes.length, 0)
  let prevented = false
  h.state.form.poll_minutes = 90
  h.unload({ preventDefault: () => { prevented = true } })
  assert.equal(prevented, true)
  h.state.saving.value = true
  assert.equal(await h.leave(), false)
})

test('修改密码保存成功后跳转登录，不误报尚有未保存的更改', async () => {
  const h = setup()
  await h.state.load()
  h.state.form.web_user = 'test-admin'
  h.state.form.web_password = 'test-only-password'
  await h.state.save()
  assert.equal(h.writes.length, 1)
  assert.equal(h.location.href, '/login')
  assert.equal(h.state.dirty.value, false)
  let prevented = false
  h.unload({ preventDefault: () => { prevented = true } })
  assert.equal(prevented, false)
})

test('目录检测失败可重试，更换目录取消请求且旧结果不得覆盖新输入', async () => {
  const h = setup()
  await h.state.load()
  h.probe(async () => { throw new Error('没有写入权限') })
  await h.state.checkStorage()
  assert.equal(h.state.storageError.value, '没有写入权限')
  let resolve
  let signal
  h.probe((_url, options) => { signal = options.signal; return new Promise(done => { resolve = done }) })
  const pending = h.state.checkStorage()
  h.state.form.library_dir = '/temporary/new-library'
  assert.equal(signal.aborted, true)
  resolve({ path: '/temporary/library', writable: true, mount_status: 'mapped', message: '旧结果' })
  await pending
  assert.equal(h.state.storageResult.value, null)
  h.probe(async () => ({ path: '/temporary/new-library', writable: true, mount_status: 'mapped', message: '已映射，可以写入' }))
  await h.state.checkStorage()
  assert.equal(h.state.storageResult.value.path, '/temporary/new-library')
  assert.equal(h.writes.length, 0)
})
