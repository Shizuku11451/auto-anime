import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

const source = readFileSync(new URL('./AnimeDetailDialog.vue', import.meta.url), 'utf8')
const setup = parse(source).descriptor.scriptSetup.content
const compile = (text) => ts.transpileModule(text, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
const compiled = compile(setup + '\nexport { rematchMetadata };')
const busyCompiled = compile(readFileSync(new URL('../composables/useBusy.ts', import.meta.url), 'utf8'))

function detailDialog(options = {}) {
  const state = vue.reactive({ animeDetail: { open: true, id: 1, loading: false, error: '', data: {
    id: 1, title: '测试动画', bangumi_id: options.binding ?? 0, meta_source: options.source ?? 'bangumi', seasons: [],
  } } })
  const posts = [], successes = [], errors = [], confirmations = [], refreshed = []
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
    vue,
    '@vueuse/core': { useDocumentVisibility: () => vue.ref('visible'), useIntervalFn: () => {} },
    '@/stores/dialogs': { useDialogs: () => ({ state, reloadAnimeDetail: async () => refreshed.push('detail') }) },
    '@/stores/app': { useAppStore: () => ({ platformOS: vue.ref(''), refresh: async () => refreshed.push('list') }) },
    '@/utils/platform': platform,
    '@/api/client': { http: { post: async (url) => {
      posts.push(url)
      if (options.error) throw new Error(options.error)
      return { ok: true, queued: true }
    } }, errorMessage: (e) => e.message },
    '@/stores/toast': { toast: { success: (message) => successes.push(message), error: (message) => errors.push(message) } },
    '@/stores/confirm': { confirmDialog: async (...args) => {
      confirmations.push(args)
      return options.confirm ? options.confirm() : true
    } },
  }
  const evaluate = (code) => {
    const context = { exports: {}, require: (id) => {
      assert.ok(id in modules, `unmocked setup import: ${id}`)
      return modules[id]
    } }
    runInNewContext(code, context, { filename: 'AnimeDetailDialog.vue' })
    return context.exports
  }
  modules['@/composables/useBusy'] = evaluate(busyCompiled)
  return { state, component: evaluate(compiled), posts, successes, errors, confirmations, refreshed }
}

test('确认重新识别后调用当前番剧接口，并刷新详情与列表', async () => {
  const h = detailDialog()
  await h.component.rematchMetadata()
  assert.deepEqual(h.posts, ['/api/animes/1/rematch'])
  assert.deepEqual(h.refreshed.sort(), ['detail', 'list'])
  assert.equal(h.confirmations[0][0], '重新识别元数据')
  assert.equal(h.successes.length, 1)
  assert.equal(h.errors.length, 0)
})

test('取消确认保留当前匹配', async () => {
  const h = detailDialog({ confirm: () => false })
  await h.component.rematchMetadata()
  assert.equal(h.posts.length, 0)
  assert.equal(h.refreshed.length, 0)
})

test('手动绑定与旧 manual 记录都提示先解绑', async () => {
  for (const options of [{ binding: 42 }, { source: 'manual' }]) {
    const h = detailDialog(options)
    await h.component.rematchMetadata()
    assert.equal(h.posts.length, 0)
    assert.equal(h.confirmations.length, 0)
    assert.match(h.errors[0], /解除绑定/)
  }
})

test('接口失败显示中文错误且不报告已入队', async () => {
  const h = detailDialog({ error: '元数据队列繁忙，请稍后重试' })
  await h.component.rematchMetadata()
  assert.deepEqual(h.errors, ['元数据队列繁忙，请稍后重试'])
  assert.equal(h.successes.length, 0)
  assert.equal(h.refreshed.length, 0)
})

test('确认期间详情已关闭或切换时不发送旧操作', async () => {
  let confirm
  const h = detailDialog({ confirm: () => new Promise((resolve) => { confirm = resolve }) })
  const operation = h.component.rematchMetadata()
  h.state.animeDetail.open = false
  confirm(true)
  await operation
  assert.equal(h.posts.length, 0)
})
