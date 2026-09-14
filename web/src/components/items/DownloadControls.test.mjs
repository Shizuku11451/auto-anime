import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

const compile = (source) => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
const source = parse(readFileSync(new URL('./DownloadControls.vue', import.meta.url), 'utf8')).descriptor.scriptSetup.content
const compiled = compile(source + '\nexport { limit, dirty, saving, error, saveLimit, active, queued, paused };')
const status = { exports: {} }
runInNewContext(compile(readFileSync(new URL('../../utils/status.ts', import.meta.url), 'utf8')), status)

function controls(t, initialLimit = 2) {
  const calls = []
  const store = {
    status: vue.ref(initialLimit === null ? null : { max_concurrent_downloads: initialLimit }),
    counts: vue.ref({ '下载中': 1, '已发送到 qBittorrent': 1, '等待下载': 2, '等待继续': 3, '已暂停': 4 }),
  }
  let save = async () => ({ ok: true })
  const modules = {
    vue,
    '@/stores/app': { useAppStore: () => store },
    '@/api/client': { http: { put: async (...args) => { calls.push(args); return save() } }, errorMessage: (e) => e.message },
    '@/stores/toast': { toast: { success() {} } },
    '@/utils/status': status.exports,
  }
  const context = { exports: {}, require: (id) => {
    assert.ok(id in modules, `unexpected setup import: ${id}`)
    return modules[id]
  } }
  const scope = vue.effectScope()
  scope.run(() => runInNewContext(compiled, context))
  t.after(() => scope.stop())
  return { component: context.exports, store, calls, setSave: (fn) => { save = fn } }
}

test('并发控制读取真实值，轮询保留未保存输入，保存只提交并发字段', async (t) => {
  const h = controls(t, 7)
  assert.equal(h.component.limit.value, 7)
  h.component.limit.value = 8
  h.component.dirty.value = true
  h.store.status.value = { max_concurrent_downloads: 3 }
  await vue.nextTick()
  assert.equal(h.component.limit.value, 8)
  await h.component.saveLimit()
  assert.equal(h.calls.length, 1)
  assert.equal(h.calls[0][0], '/api/settings')
  assert.deepEqual(JSON.parse(JSON.stringify(h.calls[0][1])), { max_concurrent_downloads: 8 })
  assert.equal(h.store.status.value.max_concurrent_downloads, 8)
  assert.equal(h.component.dirty.value, false)
})

test('非法并发数量不提交，保存失败保留原配置和用户输入', async (t) => {
  const h = controls(t, 2)
  for (const value of ['', 0, -1, 33, 1.5, 'invalid']) {
    h.component.limit.value = value
    await h.component.saveLimit()
    assert.match(h.component.error.value, /1–32/)
  }
  assert.equal(h.calls.length, 0)
  h.component.limit.value = 5
  h.component.dirty.value = true
  h.setSave(async () => { throw new Error('保存失败') })
  await h.component.saveLimit()
  assert.equal(h.store.status.value.max_concurrent_downloads, 2)
  assert.equal(h.component.limit.value, 5)
  assert.equal(h.component.dirty.value, true)
  assert.equal(h.component.error.value, '保存失败')
})

test('保存期间阻止重复提交；下载、排队和暂停数量分开统计', async (t) => {
  const h = controls(t)
  let finish
  h.setSave(() => new Promise(resolve => { finish = resolve }))
  h.component.limit.value = 4
  const pending = h.component.saveLimit()
  await h.component.saveLimit()
  assert.equal(h.calls.length, 1)
  assert.equal(h.component.saving.value, true)
  finish({ ok: true })
  await pending
  assert.equal(h.component.saving.value, false)
  assert.equal(h.component.active.value, 2)
  assert.equal(h.component.queued.value, 5)
  assert.equal(h.component.paused.value, 4)
})

test('配置加载前不能用默认值覆盖服务器设置', async (t) => {
  const h = controls(t, null)
  h.component.limit.value = 2
  await h.component.saveLimit()
  assert.equal(h.calls.length, 0)
})
