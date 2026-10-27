import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

const descriptor = parse(readFileSync(new URL('./SetupApp.vue', import.meta.url), 'utf8')).descriptor
const compiled = ts.transpileModule(compileScript(descriptor, { id: 'setup-test' }).content, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText

function setup() {
  const writes = []
  const redirects = []
  const confirmations = []
  let handler = async () => response({ required: true, download_mode: 'native', library_dir: '/media', download_dir: '/data/downloads', keep_source_files: true })
  let save = async () => response({ ok: true })
  let login = async () => response({ ok: true })
  let storage = async () => response({ path: '/media', mount_status: 'mapped', writable: true, message: '媒体目录已映射，可以写入。' })
  let confirm = async () => true
  let unmount
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
    vue: { ...vue, onMounted() {}, onBeforeUnmount(fn) { unmount = fn } },
    '@lucide/vue': new Proxy({}, { get: () => stub }),
    '@/components/auth/AuthShell.vue': stub,
    '@/components/ui/BaseButton.vue': stub,
    '@/components/ui/ConfirmHost.vue': stub,
    '@/stores/confirm': { confirmDialog: async (...args) => { confirmations.push(args); return confirm() } },
    '@/api/types': { MODE_LABEL: { native: '内置 BT 下载器', direct: '本地直链' } },
    '@/utils/platform': platform,
  }
  const context = { exports: {}, URL, AbortController, setTimeout, clearTimeout, location: { replace: value => redirects.push(value) },
    fetch: async (url, init) => {
      if (url === '/api/setup/status') return handler(init)
      if (url === '/api/setup/storage') return storage(init)
      if (url === '/api/setup') { writes.push(JSON.parse(init.body)); return save() }
      if (url === '/api/login') return login()
      throw new Error('unexpected URL ' + url)
    }, require: id => { assert.ok(id in modules, id); return modules[id] } }
  runInNewContext(compiled, context)
  return { state: context.exports.default.setup({}, { expose() {} }), writes, redirects, confirmations,
    status: fn => { handler = fn }, storage: fn => { storage = fn }, save: fn => { save = fn }, login: fn => { login = fn }, confirm: fn => { confirm = fn }, unmount: () => unmount() }
}
function response(body, ok = true) { return { ok, json: async () => body, text: async () => '模拟失败' } }
function account(h) { Object.assign(h.state.form, { user: '测试用户', pass: 'test-password', pass2: 'test-password' }) }

test('首次向导在容器上接受并显示内置 BT 下载器，不再列出已移除的内置 qBittorrent', async () => {
  const h = setup()
  h.status(async () => response({
    required: true,
    download_mode: 'native',
    os: 'linux',
    library_dir: '/media',
    download_dir: '/data/downloads',
    keep_source_files: true,
  }))
  await h.state.loadStatus()
  assert.equal(h.state.form.mode, 'native')
  assert.equal(h.state.availableModes.value.includes('native'), true)
  assert.equal(h.state.availableModes.value.includes('builtin'), false)
})

test('未读取或读取失败不能推进和保存，重试沿用实际下载方式与媒体目录', async () => {
  const h = setup()
  h.state.go(1)
  await h.state.finish()
  await h.state.skip()
  assert.equal(h.state.step.value, 0)
  assert.equal(h.writes.length, 0)
  h.status(async () => response({}, false))
  await h.state.loadStatus()
  assert.ok(h.state.statusError.value)
  assert.equal(h.state.statusLoaded.value, false)
  h.status(async () => response({ required: true, download_mode: 'direct', library_dir: '/media', download_dir: '/data/downloads' }))
  await h.state.loadStatus()
  assert.equal(h.state.form.mode, 'direct')
  assert.equal(h.state.archiveRoot.value, '/media')
  h.state.form.library_dir = ''
  assert.equal(h.state.archiveRoot.value, '/data/downloads')
})

test('重试和卸载隔离旧状态响应，已初始化不会显示安装表单', async () => {
  const h = setup()
  let resolveOld
  h.status(() => new Promise(resolve => { resolveOld = resolve }))
  const old = h.state.loadStatus()
  h.status(async () => response({ required: true, download_mode: 'direct', library_dir: '/media' }))
  await h.state.loadStatus()
  resolveOld(response({ required: true, download_mode: 'direct', library_dir: '/old' }))
  await old
  assert.equal(h.state.form.library_dir, '/media')
  h.status(() => new Promise(resolve => { resolveOld = resolve }))
  const closing = h.state.loadStatus()
  h.unmount()
  resolveOld(response({ required: true, download_mode: 'direct', library_dir: '/late' }))
  await closing
  assert.equal(h.state.form.library_dir, '/media')
  const initialized = setup()
  initialized.status(async () => response({ required: false, download_mode: 'direct' }))
  await initialized.state.loadStatus()
  assert.deepEqual(initialized.redirects, ['/'])
  assert.equal(initialized.state.statusLoaded.value, false)
})

test('外部下载器缺少有效地址不能进入下一步或完成，目录与默认直连正确提交', async () => {
  const h = setup()
  await h.state.loadStatus()
  account(h)
  h.state.go(1)
  h.state.form.mode = 'qbittorrent'
  h.state.go(1)
  assert.equal(h.state.step.value, 1)
  assert.ok(h.state.msg.value.includes('地址'))
  await h.state.finish()
  assert.equal(h.writes.length, 0)
  h.state.form.mode = 'native'
  await h.state.finish()
  assert.equal(h.writes[0].library_dir, '/media')
  assert.equal(h.writes[0].proxy_url, undefined)
  assert.equal(h.writes[0].poll_minutes, 30)
  assert.equal(h.writes[0].keep_source_files, true)
  assert.equal(h.state.completed.value, true)
  assert.equal(h.redirects.length, 0)
  h.state.enterPanel()
  assert.deepEqual(h.redirects, ['/#/discover'])
})

test('沿用部署同步周期与已有代理，不读取或清空代理凭据', async () => {
  const h = setup()
  h.status(async () => response({ required: true, download_mode: 'native', library_dir: '/media', poll_minutes: 15, proxy_enabled: true }))
  await h.state.loadStatus()
  account(h)
  assert.equal(h.state.proxyLabel.value, '沿用部署代理')
  await h.state.finish()
  assert.equal(h.writes[0].poll_minutes, 15)
  assert.equal(h.writes[0].proxy_url, undefined)
})

test('部署可关闭原文件保留，向导回填并按用户选择提交', async () => {
  const h = setup()
  h.status(async () => response({ required: true, download_mode: 'native', library_dir: '/media', keep_source_files: false }))
  await h.state.loadStatus()
  account(h)
  assert.equal(h.state.form.keep_source_files, false)
  await h.state.finish()
  assert.equal(h.writes[0].keep_source_files, false)
})

test('目录映射警告保留在完成页并引导设置，保存成功不会重复提交', async () => {
  const h = setup()
  await h.state.loadStatus()
  account(h)
  h.save(async () => response({ ok: true, library_warning: '目录未映射' }))
  await h.state.finish()
  assert.equal(h.state.libraryWarning.value, '目录未映射')
  await h.state.finish()
  assert.equal(h.writes.length, 1)
  h.state.enterPanel()
  assert.deepEqual(h.redirects, ['/#/settings'])
})

test('自动登录失败保留已安装结果，保存失败保留输入供重试', async () => {
  const h = setup()
  await h.state.loadStatus()
  account(h)
  h.login(async () => response({}, false))
  await h.state.finish()
  assert.equal(h.state.completed.value, true)
  assert.equal(h.state.loginFailed.value, true)
  h.state.enterPanel()
  assert.deepEqual(h.redirects, ['/login'])
  const failed = setup()
  await failed.state.loadStatus()
  account(failed)
  failed.save(async () => response({}, false))
  await failed.state.finish()
  assert.equal(failed.state.completed.value, false)
  assert.equal(failed.state.form.library_dir, '/media')
  assert.equal(failed.state.msg.value, '模拟失败')
})

test('无账户的内置 BT 模式也需要明确确认，确认和保存期间防止重复操作', async () => {
  const h = setup()
  await h.state.loadStatus()
  let resolveConfirm
  h.confirm(() => new Promise(resolve => { resolveConfirm = resolve }))
  const pending = h.state.finish()
  await h.state.finish()
  await h.state.skip()
  assert.equal(h.confirmations.length, 1)
  assert.equal(h.writes.length, 0)
  resolveConfirm(false)
  await pending
  assert.equal(h.writes.length, 0)
  h.confirm(async () => true)
  let resolveSave
  h.save(() => new Promise(resolve => { resolveSave = resolve }))
  const saving = h.state.finish()
  await new Promise(resolve => setImmediate(resolve))
  await h.state.finish()
  assert.equal(h.writes.length, 1)
  resolveSave(response({ ok: true }))
  await saving
})

test('目录检测支持重试，旧响应不覆盖修改后的路径或卸载后的状态', async () => {
  const h = setup()
  await h.state.loadStatus()
  await vue.nextTick()
  await h.state.checkStorage()
  assert.equal(h.state.storageResult.value.mount_status, 'mapped')
  h.storage(async () => response({}, false))
  await h.state.checkStorage()
  assert.ok(h.state.storageError.value)
  assert.equal(h.state.storageResult.value, null)
  let resolveOld
  h.storage(() => new Promise(resolve => { resolveOld = resolve }))
  const pending = h.state.checkStorage()
  h.state.form.library_dir = '/different'
  await vue.nextTick()
  resolveOld(response({ path: '/media', mount_status: 'mapped', writable: true, message: '旧结果' }))
  await pending
  assert.equal(h.state.canCheckStorage.value, false)
  assert.equal(h.state.storageResult.value, null)
  assert.equal(h.state.storageLoading.value, false)
  h.state.form.library_dir = '/media'
  await vue.nextTick()
  const closing = h.state.checkStorage()
  h.unmount()
  resolveOld(response({ path: '/media', mount_status: 'mapped', writable: true, message: '迟到结果' }))
  await closing
  assert.equal(h.state.storageResult.value, null)
})

test('安装向导直接从保存目录进入确认，不再配置或提交 RSS 搜索模板', async () => {
  const h = setup()
  await h.state.loadStatus()
  account(h)
  h.state.go(1); h.state.go(1)
  assert.equal(h.state.current.value.key, 'storage')
  h.state.go(1)
  assert.equal(h.state.current.value.key, 'network')
  assert.equal(h.state.isLast.value, true)
  await h.state.finish()
  assert.equal(Object.hasOwn(h.writes[0], 'discover_search_template'), false)
  assert.equal(h.state.completed.value, true)
  const skipped = setup()
  await skipped.state.loadStatus()
  await skipped.state.skip()
  assert.deepEqual(Object.keys(skipped.writes[0]), [])
  assert.equal(skipped.state.completed.value, true)
})
