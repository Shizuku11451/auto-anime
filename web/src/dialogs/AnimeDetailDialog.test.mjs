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
const compiled = compile(setup + '\nexport { remove, resourceState, downloadingCount, findResources, metaBits, subject, rating, summary, cover };')
const busyCompiled = compile(readFileSync(new URL('../composables/useBusy.ts', import.meta.url), 'utf8'))
const typesContext = { exports: {} }, statusContext = { exports: {} }
runInNewContext(compile(readFileSync(new URL('../api/types.ts', import.meta.url), 'utf8')), typesContext)
runInNewContext(compile(readFileSync(new URL('../utils/status.ts', import.meta.url), 'utf8')), statusContext)

function detailDialog(options = {}) {
  const state = vue.reactive({ relocation: { animeId: 0, status: null }, animeDetail: { open: true, id: 1, loading: false, error: '', data: {
    id: 1, title: '测试动画', bangumi_id: options.binding ?? 0, meta_source: options.source ?? 'bangumi', item_count: 0, saved_count: 0, seasons: [],
  } } })
  const posts = [], deletes = [], successes = [], errors = [], confirmations = [], refreshed = [], routes = []
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
    'vue-router': { useRouter: () => ({ push: async route => { routes.push(route); return options.navigationFailure } }) },
    '@/api/types': typesContext.exports,
    '@/utils/status': statusContext.exports,
    '@vueuse/core': { useDocumentVisibility: () => vue.ref('visible'), useIntervalFn: () => {} },
    '@/stores/dialogs': { useDialogs: () => ({ state, reloadAnimeDetail: async () => refreshed.push('detail') }) },
    '@/stores/app': { useAppStore: () => ({ platformOS: vue.ref(''), refresh: async () => refreshed.push('list') }) },
    '@/utils/platform': platform,
    '@/api/client': { http: { post: async (url) => {
      posts.push(url)
      if (options.error) throw new Error(options.error)
      return { ok: true, queued: true }
    }, delete: async (url) => {
      deletes.push(url)
      return options.deleteResult ?? {}
    } }, errorMessage: (e) => e.message },
    '@/composables/relocation': { relocationSummary: () => '' },
    '@/stores/toast': { toast: { success: (message) => successes.push(message), error: (message) => errors.push(message) } },
    '@/stores/confirm': { confirmDialog: async (...args) => {
      confirmations.push(args)
      return options.confirm ? options.confirm(args) : true
    }, lastConfirmChecked: () => !!options.checked },
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
  return { state, component: evaluate(compiled), posts, deletes, successes, errors, confirmations, refreshed, routes }
}

test('展示当前绑定的 Bangumi 评分和简介，资料不覆盖本地资源或错误绑定', () => {
  const h = detailDialog({ binding: 101 })
  h.state.animeDetail.data.rating = 7
  h.state.animeDetail.data.summary = '本地缓存简介'
  h.state.animeDetail.data.cover = '/local.jpg'
  h.state.animeDetail.subject = { id: 101, rating: 8.4, rating_total: 1000, summary: 'Bangumi 简介', cover: '/remote.jpg' }
  assert.equal(h.component.rating.value, 8.4)
  assert.equal(h.component.summary.value, 'Bangumi 简介')
  assert.equal(h.component.cover.value, '/local.jpg')
  assert.equal(h.state.animeDetail.data.item_count, 0)
  h.state.animeDetail.subject.id = 202
  assert.equal(h.component.subject.value, null)
  assert.equal(h.component.rating.value, 7)
  assert.equal(h.component.summary.value, '本地缓存简介')
})

test('详情区分资源记录、已保存与下载中，不把收藏状态当成本地进度', () => {
  const h = detailDialog()
  const s = h.state.animeDetail
  assert.match(h.component.resourceState.value, /暂无资源记录/)
  s.data.item_count = 2
  assert.match(h.component.resourceState.value, /尚无已保存媒体/)
  s.data.saved_count = 1
  assert.match(h.component.resourceState.value, /本地已有 1 条已保存资源/)
  s.data.seasons = [{ episodes: [{ items: [{ status: '下载中' }, { status: '已保存' }] }], loose: [{ status: '已发送到 qBittorrent' }] }]
  assert.equal(h.component.downloadingCount.value, 2)
  s.data.id = -101
  s.data.season = 1
  assert.ok(!h.component.metaBits.value.some(bit => /识别名|默认第/.test(bit)))
  assert.match(h.component.resourceState.value, /未找到关联/)
  s.loading = true
  assert.match(h.component.resourceState.value, /正在核对/)
  s.loading = false
  s.error = '读取失败'
  assert.match(h.component.resourceState.value, /暂无法确认/)
})

test('先打开详情，用户明确选择找资源后才跳转，核对中或失败不跳转', async () => {
  const h = detailDialog()
  const s = h.state.animeDetail
  s.data.id = -101
  s.data.meta_name_cn = '收藏动画'
  assert.equal(h.routes.length, 0)
  s.loading = true
  await h.component.findResources()
  s.loading = false
  s.error = '查询失败'
  await h.component.findResources()
  assert.equal(h.routes.length, 0)
  s.error = ''
  await h.component.findResources()
  assert.equal(h.routes[0].path, '/discover')
  assert.equal(h.routes[0].query.keyword, '收藏动画')
  assert.equal(s.open, false)
  assert.equal(h.posts.length, 0)
})

test('路由被未保存更改保护拦截时保留详情', async () => {
  const h = detailDialog({ navigationFailure: { type: 4 } })
  await h.component.findResources()
  assert.equal(h.state.animeDetail.open, true)
})

const savedItem = { id: 7, title: '第 1 集', status: '已保存', file_path: '/media/a.mkv' }

test('移除记录默认保留文件，删除文件需要勾选并再次确认', async () => {
  const keep = detailDialog()
  await keep.component.remove(savedItem)
  assert.deepEqual(keep.deletes, ['/api/items/7'])
  assert.equal(keep.confirmations.length, 1)
  assert.equal(keep.confirmations[0][2].checkLabel, '同时删除硬盘上的文件')

  const drop = detailDialog({ checked: true })
  await drop.component.remove(savedItem)
  assert.deepEqual(drop.deletes, ['/api/items/7?delete_files=1'])
  assert.equal(drop.confirmations[1][0], '删除文件')

  const cancelled = detailDialog({ checked: true, confirm: (args) => args[0] !== '删除文件' })
  await cancelled.component.remove(savedItem)
  assert.equal(cancelled.deletes.length, 0)
})

test('未保存的记录不提供删除文件选项', async () => {
  const h = detailDialog({ checked: true })
  await h.component.remove({ id: 8, title: '第 2 集', status: '下载中', file_path: '' })
  assert.deepEqual(h.deletes, ['/api/items/8'])
  assert.equal(h.confirmations[0][2].checkLabel, undefined)
})

test('文件删除失败时说明记录已移除', async () => {
  const h = detailDialog({ checked: true, deleteResult: { deleted_files: 0, file_errors: ['a.mkv：不在媒体目录内，已保留'] } })
  await h.component.remove(savedItem)
  assert.match(h.errors[0], /记录已移除，文件未删除/)
})
