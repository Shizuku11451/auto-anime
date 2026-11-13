import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

const source = readFileSync(new URL('./AnimeEditDialog.vue', import.meta.url), 'utf8')
const setup = parse(source).descriptor.scriptSetup.content
const compiled = ts.transpileModule(setup + '\nexport { form, moveFiles, save, restoreAuto, metaState };', {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText

function editDialog(options = {}) {
  const anime = {
    id: 1, title: '测试动画', search_name: '', season: 0, update_weekday: 0, storage_dir: '', include_rules: '', exclude_rules: '',
    bangumi_id: options.binding ?? 0, meta_subject_id: options.subject ?? 0, meta_name_cn: '', meta_name: '', saved_count: options.saved ?? 0,
  }
  const state = vue.reactive({
    animeEdit: { open: false, anime, tab: 'basic' },
    animeDetail: { data: null },
    relocation: { animeId: 0, status: null },
  })
  const calls = [], successes = [], errors = [], confirmations = [], relocations = []
  const modules = {
    vue,
    '@lucide/vue': {},
    '@/components/ui/BaseDialog.vue': {},
    '@/components/ui/BaseButton.vue': {},
    '@/stores/dialogs': { useDialogs: () => ({ state, reloadAnimeDetail: async () => {}, openAnimeManage: () => {}, openBind: () => {} }) },
    '@/stores/app': { useAppStore: () => ({ animes: vue.ref([anime]), refresh: async () => {} }) },
    '@/api/client': {
      http: {
        put: async (url, body) => { calls.push(['PUT', url, body]); return {} },
        post: async (url, body) => {
          calls.push(['POST', url, body])
          if (options.error) throw new Error(options.error)
          return {}
        },
      },
      errorMessage: (e) => e.message,
    },
    '@/stores/toast': { toast: { success: (m) => successes.push(m), error: (m) => errors.push(m) } },
    '@/stores/confirm': { confirmDialog: async (...args) => {
      confirmations.push(args)
      return options.confirm ? options.confirm() : true
    } },
    '@/composables/relocation': { startRelocation: async (id) => relocations.push(id) },
    '@/utils/platform': { storageDirExample: '/vol1/1000/番剧' },
    '@/components/library/schedule': { localDate: () => '2026-11-04' },
  }
  const context = { exports: {}, setTimeout, clearTimeout, require: (id) => {
    assert.ok(id in modules, `unmocked setup import: ${id}`)
    return modules[id]
  } }
  runInNewContext(compiled, context, { filename: 'AnimeEditDialog.vue' })
  return { state, anime, component: context.exports, calls, successes, errors, confirmations, relocations }
}

async function opened(options) {
  const h = editDialog(options)
  h.state.animeEdit.open = true
  await vue.nextTick()
  return h
}

test('自动识别时重新识别走 rematch 接口', async () => {
  const h = await opened({ subject: 5 })
  assert.equal(h.component.metaState.value.label, '自动识别')
  await h.component.restoreAuto()
  assert.deepEqual(h.calls.map(c => c[1]), ['/api/animes/1/rematch'])
  assert.equal(h.successes.length, 1)
})

test('手动绑定时恢复自动识别会解除绑定', async () => {
  const h = await opened({ binding: 42 })
  assert.equal(h.component.metaState.value.label, '手动绑定')
  await h.component.restoreAuto()
  assert.deepEqual(h.calls.map(c => [c[0], c[1], JSON.stringify(c[2])]), [['PUT', '/api/animes/1/metadata', '{"bangumi_id":0}']])
})

test('取消确认或确认期间关闭弹窗不发送请求', async () => {
  const cancelled = await opened({ confirm: () => false })
  await cancelled.component.restoreAuto()
  assert.equal(cancelled.calls.length, 0)

  let resolve
  const h = await opened({ confirm: () => new Promise(r => { resolve = r }) })
  const op = h.component.restoreAuto()
  h.state.animeEdit.open = false
  resolve(true)
  await op
  assert.equal(h.calls.length, 0)
})

test('识别失败显示错误', async () => {
  const h = await opened({ error: '元数据队列繁忙' })
  await h.component.restoreAuto()
  assert.deepEqual(h.errors, ['元数据队列繁忙'])
})

test('季号 0 保持自动，位置未改时不发送 storage_dir', async () => {
  const h = await opened()
  assert.equal(h.component.form.season, 0)
  await h.component.save()
  const [, , body] = h.calls.find(c => c[0] === 'PUT')
  assert.equal(body.season, 0)
  assert.ok(!('storage_dir' in body))
  assert.equal(h.relocations.length, 0)
})

test('有已保存文件时改季号默认移动文件，可选只影响之后的下载', async () => {
  const h = await opened({ saved: 3 })
  h.component.form.season = 2
  await h.component.save()
  assert.deepEqual(h.relocations, [1])

  const keep = await opened({ saved: 3 })
  keep.component.form.season = 2
  keep.component.moveFiles.value = false
  await keep.component.save()
  assert.equal(keep.relocations.length, 0)
})
