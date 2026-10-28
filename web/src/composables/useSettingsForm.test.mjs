import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import * as vue from 'vue'

const compiled = ts.transpileModule(readFileSync(new URL('./useSettingsForm.ts', import.meta.url), 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText

function setup() {
  const writes = []
  const focused = []
  let fail = false
  let put = async (_url, body) => { Object.assign(data, body); return { ok: true, reauth_required: !!body.web_password } }
  let allowConfirm = false
  let probe = async () => ({ path: '/temporary/library', writable: true, mount_status: 'mapped', message: '已映射，可以写入' })
  const data = { download_mode: 'direct', poll_minutes: 30, library_dir: '/temporary/library' }
  const location = { href: '' }
  const BS = String.fromCharCode(92)
  const modules = {
    vue: { ...vue, onBeforeUnmount: () => {} },
    '@/utils/platform': {
      isAbsoluteStoragePath: (value, os) => (os === 'windows'
        ? /^[a-zA-Z]:/.test(value) || value.startsWith(BS + BS) || value.startsWith('//')
        : value.startsWith('/') || value.startsWith(BS) || /^[a-zA-Z]:/.test(value)),
    },
    '@/api/client': { api: (...args) => probe(...args), http: {
      get: async () => { if (fail) throw new Error('模拟读取失败'); return { ...data } },
      put: async (url, body) => { writes.push(body); return put(url, body) },
    }, errorMessage: e => e.message },
    '@/stores/app': { useAppStore: () => ({ refresh: async () => {} }) },
    '@/stores/toast': { toast: { success() {}, error() {}, info() {} } },
    '@/stores/confirm': { confirmDialog: async () => allowConfirm },
  }
  const context = { exports: {}, document: { getElementById: id => ({ focus: () => focused.push(id) }) }, location, AbortController, require: id => { assert.ok(id in modules, id); return modules[id] } }
  runInNewContext(compiled, context)
  const state = context.exports.useSettingsForm()
  return { state, data, writes, focused, location, put: value => { put = value }, probe: value => { probe = value }, fail: value => { fail = value }, allow: value => { allowConfirm = value } }
}

test('设置未读取或读取失败时不得提交默认值，重试成功后才允许保存', async () => {
  const h = setup()
  h.state.active.value = 'network'
  h.state.form.poll_minutes = 60
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

test('保存只提交当前分类字段，其他分类的未保存修改不受影响', async () => {
  const h = setup()
  await h.state.load()
  h.state.active.value = 'library'
  h.state.form.keep_source_files = false
  await h.state.save()
  assert.deepEqual(Object.keys(h.writes[0]).sort(), ['auto_cleanup_empty_anime', 'keep_source_files', 'library_dir'])
  assert.equal(h.writes[0].keep_source_files, false)
  assert.equal(h.state.form.keep_source_files, false)
  // 未在当前分类的修改即使存在也不会随保存提交。
  h.state.form.aria2_token = 'test-only-token'
  h.state.form.library_dir = '/temporary/other'
  await h.state.save('library')
  assert.equal(h.writes[1].aria2_token, undefined)
  assert.equal(h.state.isDirty('download'), true)
  assert.equal(h.state.form.aria2_token, 'test-only-token')
})

test('敏感字段留空不提交，填写后才提交并在保存后清空', async () => {
  const h = setup()
  await h.state.load()
  h.state.form.aria2_url = 'http://127.0.0.1:6800/jsonrpc'
  await h.state.save('download')
  assert.equal('aria2_token' in h.writes[0], false)
  h.state.form.aria2_token = 'test-only-token'
  await h.state.save('download')
  assert.equal(h.writes[1].aria2_token, 'test-only-token')
  assert.equal(h.state.form.aria2_token, '')
  assert.equal(h.state.isDirty('download'), false)
})

test('切换分类时有未保存更改需确认，取消保留输入，确认恢复本分类', async () => {
  const h = setup()
  await h.state.load()
  h.state.active.value = 'library'
  h.state.form.library_dir = '/temporary/new-library'
  assert.equal(await h.state.selectSection('network'), false)
  assert.equal(h.state.active.value, 'library')
  assert.equal(h.state.form.library_dir, '/temporary/new-library')
  h.allow(true)
  assert.equal(await h.state.selectSection('network'), true)
  assert.equal(h.state.active.value, 'network')
  assert.equal(h.state.form.library_dir, '/temporary/library')
  assert.equal(h.state.dirty.value, false)
  assert.equal(h.writes.length, 0)
})

test('无更改时直接切换分类，离开时未保存需确认，保存中阻止离开', async () => {
  const h = setup()
  await h.state.load()
  assert.equal(await h.state.selectSection('account'), true)
  assert.equal(await h.state.canLeave(), true)
  h.state.form.web_user = 'test-admin'
  assert.equal(await h.state.canLeave(), false)
  h.allow(true)
  assert.equal(await h.state.canLeave(), true)
  h.state.saving.value = true
  assert.equal(await h.state.canLeave(), false)
  assert.equal(await h.state.selectSection('download'), false)
})

test('撤销只恢复当前分类且需要确认，不发起保存', async () => {
  const h = setup()
  await h.state.load()
  h.state.form.aria2_token = 'test-only-token'
  h.state.form.library_dir = '/temporary/edited'
  h.state.form.poll_minutes = 45
  assert.equal(h.state.changedSections.value.join(','), 'download,library,network')
  h.state.form.poll_minutes = 30
  assert.equal(h.state.changedSections.value.join(','), 'download,library')
  await h.state.reset('library')
  assert.equal(h.state.form.library_dir, '/temporary/edited')
  h.allow(true)
  await h.state.reset('library')
  assert.equal(h.state.form.library_dir, '/temporary/library')
  assert.equal(h.state.changedSections.value.join(','), 'download')
  assert.equal(h.state.form.aria2_token, 'test-only-token')
  assert.equal(h.writes.length, 0)
})

test('内置 BT 与列表外旧下载模式读取后原样保留', async () => {
  const h = setup()
  h.data.download_mode = 'native'
  await h.state.load()
  assert.equal(h.state.form.download_mode, 'native')
  h.data.download_mode = 'legacy-mode'
  await h.state.load()
  assert.equal(h.state.form.download_mode, 'legacy-mode')
  assert.equal(h.state.dirty.value, false)
})

test('修改密码保存成功后跳转登录，不误报尚有未保存的更改', async () => {
  const h = setup()
  await h.state.load()
  h.state.active.value = 'account'
  h.state.form.web_user = 'test-admin'
  h.state.form.web_password = 'test-only-password'
  await h.state.save()
  assert.deepEqual(Object.keys(h.writes[0]).sort(), ['web_password', 'web_user'])
  assert.equal(h.location.href, '/login')
  assert.equal(h.state.dirty.value, false)
})

test('同步周期不能静默回退或截断，校验失败定位字段且修改后清除错误', async () => {
  const h = setup()
  await h.state.load()
  h.state.active.value = 'network'
  for (const value of ['', 0, 4, 1441, 5.5, NaN]) {
    h.state.form.poll_minutes = value
    await h.state.save()
    assert.equal(h.focused.at(-1), 'st-poll')
    assert.ok(h.state.fieldError('poll_minutes'))
  }
  assert.equal(h.writes.length, 0)
  h.state.form.poll_minutes = 1440
  assert.equal(h.state.fieldError('poll_minutes'), undefined)
  await h.state.save()
  assert.equal(h.writes[0].poll_minutes, 1440)
})

test('其他分类的校验错误不阻止当前分类保存', async () => {
  const h = setup()
  await h.state.load()
  h.state.form.poll_minutes = 0
  h.state.active.value = 'library'
  h.state.form.keep_source_files = false
  await h.state.save()
  assert.equal(h.writes.length, 1)
  assert.equal(h.state.form.poll_minutes, 0)
  assert.equal(h.state.isDirty('network'), true)
})

test('目录与账户校验，密码长度按 Unicode 字符计算', async () => {
  const h = setup()
  h.data.web_user = 'existing-admin'
  h.data.os = 'windows'
  h.data.library_dir = 'D:/temporary/library'
  await h.state.load()
  h.state.active.value = 'library'
  h.state.form.library_dir = 'relative/library'
  await h.state.save()
  assert.equal(h.focused.at(-1), 'st-library')
  h.state.form.library_dir = 'D:/temporary/library'
  h.state.active.value = 'account'
  h.state.form.web_user = 'renamed-admin'
  await h.state.save()
  assert.equal(h.focused.at(-1), 'st-pass')
  h.state.form.web_password = '😀😀😀'
  await h.state.save()
  assert.equal(h.writes.length, 0)
  h.state.form.web_password = '测试密码六字符'
  h.state.form.web_user = ' '
  await h.state.save()
  assert.equal(h.focused.at(-1), 'st-user')
  h.state.form.web_user = 'renamed-admin'
  await h.state.save()
  assert.equal(h.writes.length, 1)
  assert.equal(h.location.href, '/login')
})

test('保存失败保留输入并允许重试，保存中不能重复提交或撤销', async () => {
  const h = setup()
  await h.state.load()
  h.state.form.qbit_password = 'test-only-secret'
  h.put(async () => { throw new Error('模拟保存失败') })
  await h.state.save('download')
  assert.equal(h.state.saveError.value, '模拟保存失败')
  assert.equal(h.state.form.qbit_password, 'test-only-secret')
  assert.equal(h.state.dirty.value, true)
  h.state.form.qbit_url = 'http://127.0.0.1:8080'
  assert.equal(h.state.saveError.value, '')
  let resolve
  h.put(() => new Promise(done => { resolve = done }))
  const pending = h.state.save('download')
  await h.state.save('download')
  h.allow(true)
  await h.state.reset('download')
  assert.equal(h.writes.length, 2)
  assert.equal(h.state.form.qbit_url, 'http://127.0.0.1:8080')
  resolve({ ok: true })
  await pending
  assert.equal(h.state.form.qbit_password, '')
  assert.equal(h.state.dirty.value, false)
})

test('保存成功但回读失败时不保留敏感输入或误报未保存', async () => {
  const h = setup()
  await h.state.load()
  h.state.form.aria2_token = 'test-only-token'
  h.fail(true)
  await h.state.save('download')
  assert.equal(h.writes.length, 1)
  assert.equal(h.state.form.aria2_token, '')
  assert.equal(h.state.dirty.value, false)
  assert.equal(h.state.loadErr.value, '模拟读取失败')
  h.state.form.aria2_token = 'again'
  await h.state.save('download')
  assert.equal(h.writes.length, 1)
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

test('Ctrl+S 保存当前可编辑分类，只读分类不拦截', async () => {
  const h = setup()
  await h.state.load()
  h.state.active.value = 'network'
  h.state.form.poll_minutes = 60
  let prevented = false
  h.state.onKeydown({ ctrlKey: true, key: 's', preventDefault: () => { prevented = true } })
  assert.equal(prevented, true)
  await new Promise(done => setTimeout(done, 0))
  assert.equal(h.writes.length, 1)
  h.state.active.value = 'update'
  prevented = false
  h.state.onKeydown({ ctrlKey: true, key: 's', preventDefault: () => { prevented = true } })
  assert.equal(prevented, false)
})
