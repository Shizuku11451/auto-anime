import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

// Execute the component's actual setup with Vue reactivity and controlled HTTP
// responses, including late responses from a request whose signal was aborted.
const source = readFileSync(new URL('./FeedDialog.vue', import.meta.url), 'utf8')
const setup = parse(source).descriptor.scriptSetup.content
const compiled = ts.transpileModule(setup + '\nexport { form, save, runPreview, runAvailability, close, btLinks, availability, availabilityRunning, availabilityDone, availabilitySummary, preview, storagePreview, defaultBase, seasonConfirmed, previewReady };', {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
const flush = () => new Promise((resolve) => setImmediate(resolve))
const item = (link, title = link) => ({ title, link, published: '', description: '' })
const result = (url, seeds = 3) => ({ url, verdict: 'available', seeds })

function dialog(t, initiallyOpen = true, status = null, saveResponse = null) {
  const state = vue.reactive({ feed: { open: initiallyOpen, feed: {
    from_discover: true, name: '测试版本', url: 'https://feed.example/rss', include_words: '',
  } } })
  const statusRef = vue.ref(status)
  const requests = []
  const previews = []
  const errors = []
  let items = []
  let previewError
  const api = async (url, options) => {
    if (url === '/api/preview') {
      previews.push(options.json)
      if (previewError) throw previewError
      return { items, count: items.length, total: items.length, pages: 1, history: false }
    }
    assert.equal(url, '/api/discover/availability')
    return new Promise((resolve, reject) => requests.push({ ...options, resolve, reject }))
  }
  const saved = []
  const http = {
    post: async (url, options) => {
      saved.push({ url, body: options })
      // 保存订阅后会自动同步一次；这里只关心订阅本身的响应。
      return url.startsWith('/api/sync/') ? { added: 0 } : (saveResponse ?? {})
    },
    put: async (url, options) => {
      saved.push({ url, body: options })
      return saveResponse ?? {}
    },
  }
  const modules = {
    vue,
    '@/stores/dialogs': { useDialogs: () => ({ state }) },
    '@/stores/app': { useAppStore: () => ({ status: statusRef, refresh: async () => {} }) },
    '@/api/client': { api, http, errorMessage: (e) => e.message },
    '@/stores/toast': { toast: { error: (message) => errors.push(message), info: () => {}, success: () => {} } },
  }
  const context = { exports: {}, URL, AbortController, require: (id) => {
    assert.ok(id in modules, `unmocked setup import: ${id}`)
    return modules[id]
  } }
  const scope = vue.effectScope()
  scope.run(() => runInNewContext(compiled, context, { filename: 'FeedDialog.vue' }))
  const component = context.exports
  t.after(async () => {
    state.feed.open = false
    await vue.nextTick()
    scope.stop()
  })
  return {
    state, component, requests, previews, errors, saved, status: statusRef,
    setItems: (value) => { items = value },
    failPreview: () => { previewError = new Error('RSS unavailable') },
    async preview(value, include = '') {
      items = value
      component.form.url = state.feed.feed.url
      component.form.include = include
      await vue.nextTick()
      await component.runPreview()
    },
  }
}

test('季数条件进入预览和保存，修改条件后旧预览不能授权保存', async (t) => {
  const h = dialog(t, false)
  Object.assign(h.state.feed.feed, { season_filter: 1, allow_unknown_season: false, target_title: '测试动画' })
  h.setItems([item('https://source.example/video.mp4', '测试动画 S1')])
  h.state.feed.open = true
  await flush()
  assert.equal(h.previews.at(-1).season_filter, 1)
  assert.equal(h.component.previewReady.value, true)
  h.component.form.allowUnknown = true
  assert.equal(h.component.previewReady.value, false)
  await h.component.save()
  assert.equal(h.saved.length, 0)
  await h.component.runPreview()
  await h.component.save()
  assert.equal(h.saved[0].body.season_filter, 1)
  assert.equal(h.saved[0].body.allow_unknown_season, true)
})

test('季数未标明必须人工确认，不允许仅凭成功预览保存', async (t) => {
  const h = dialog(t, false)
  Object.assign(h.state.feed.feed, { season_filter: -1, requires_season_confirmation: true, target_title: '测试动画', auto_download: false })
  h.setItems([item('https://source.example/video.mp4', '测试动画 [01]')])
  h.state.feed.open = true
  await flush()
  await h.component.save()
  assert.equal(h.saved.length, 0)
  h.component.seasonConfirmed.value = true
  await h.component.save()
  assert.equal(h.saved[0].body.season_filter, -1)
  assert.equal(h.saved[0].body.auto_download, false)
})

test('打开找番弹窗自动检测带参数的种子链接', async (t) => {
  const h = dialog(t, false)
  const link = 'https://source.example/EP01.TORRENT?token=x'
  h.setItems([item(link)])
  h.state.feed.open = true
  await flush()
  assert.equal(h.requests.length, 1)
  assert.deepEqual(Array.from(h.requests[0].json.urls), [link])
})

test('按 URL pathname 检测种子，保留磁力，过滤非法地址和其他协议', async (t) => {
  const h = dialog(t)
  const links = [
    'https://source.example/a.torrent?token=x#download',
    'http://source.example/EP01.TORRENT',
    'magnet:?xt=urn:btih:7135d830b60eda66853eb780a94e53aec0528d83',
    'ftp://source.example/a.torrent', 'not a URL', 'https://source.example/a.mp4',
  ]
  await h.preview(links.map((link) => item(link)))
  assert.deepEqual(Array.from(h.requests[0].json.urls), links.slice(0, 3))
})

test('重新预览立即检测新列表，旧成功响应不能覆盖或结束新检测', async (t) => {
  const h = dialog(t)
  const a = 'https://source.example/a.torrent'
  const b = 'https://source.example/b.torrent'
  await h.preview([item(a)], '版本 A')
  await h.preview([item(b)], '版本 B')
  assert.equal(h.requests.length, 2)
  assert.equal(h.requests[0].signal.aborted, true)
  assert.equal(h.requests[1].signal.aborted, false)
  h.requests[0].resolve({ results: [result(a)] })
  await flush()
  assert.equal(h.component.availabilityRunning.value, true)
  assert.equal(h.component.availabilityDone.value, false)
  assert.deepEqual(Object.keys(h.component.availability.value), [])
  h.requests[1].resolve({ results: [result(b, 7)] })
  await flush()
  assert.deepEqual(Object.keys(h.component.availability.value), [b])
  assert.equal(h.component.availability.value[b].seeds, 7)
  assert.equal(h.component.availabilityRunning.value, false)
  assert.equal(h.component.availabilityDone.value, true)
  assert.match(h.component.availabilitySummary.value, /1 条有做种者/)
})

test('过期请求的错误不弹通知也不覆盖新结果', async (t) => {
  const h = dialog(t)
  const a = 'https://source.example/a.torrent'
  const b = 'https://source.example/b.torrent'
  await h.preview([item(a)])
  await h.preview([item(b)])
  h.requests[1].resolve({ results: [result(b)] })
  await flush()
  h.requests[0].reject(new Error('old request failed'))
  await flush()
  assert.deepEqual(h.errors, [])
  assert.deepEqual(Object.keys(h.component.availability.value), [b])
  assert.equal(h.component.availabilityDone.value, true)
})

test('修改规则或关闭弹窗取消检测，晚到结果不再展示', async (t) => {
  const h = dialog(t)
  const a = 'https://source.example/a.torrent'
  await h.preview([item(a)])
  h.component.form.exclude = '不同版本'
  await vue.nextTick()
  assert.equal(h.requests[0].signal.aborted, true)
  assert.equal(h.component.btLinks.value.length, 0)
  await h.component.runAvailability()
  assert.equal(h.requests.length, 1)
  await h.preview([item(a)])
  h.component.close()
  await vue.nextTick()
  assert.equal(h.requests[1].signal.aborted, true)
  for (const request of h.requests) request.resolve({ results: [result(a)] })
  await flush()
  assert.equal(h.component.availabilityDone.value, false)
  assert.deepEqual(Object.keys(h.component.availability.value), [])
})

test('新预览失败时清除旧检测内容', async (t) => {
  const h = dialog(t)
  const link = 'https://source.example/a.torrent'
  await h.preview([item(link)])
  h.failPreview()
  await h.component.runPreview()
  h.requests[0].resolve({ results: [result(link)] })
  await flush()
  assert.equal(h.requests[0].signal.aborted, true)
  assert.equal(h.component.btLinks.value.length, 0)
  assert.equal(h.component.availabilityDone.value, false)
  assert.match(h.component.preview.value, /预览失败/)
})

test('保存位置留空时预览默认番剧文件夹', async (t) => {
  const h = dialog(t, true, { library_dir: '/vol1/1000/番剧/', download_dir: '/downloads' })
  assert.equal(h.component.form.storage, '')
  assert.equal(h.component.defaultBase.value, '/vol1/1000/番剧')
  h.component.form.target = '葬送的芙莉莲'
  await vue.nextTick()
  assert.equal(h.component.storagePreview.value, '/vol1/1000/番剧/葬送的芙莉莲')
})

test('没有媒体库目录时按下载目录预览，填写保存位置后改用所选目录', async (t) => {
  const h = dialog(t, true, { library_dir: '', download_dir: '/downloads' })
  assert.equal(h.component.defaultBase.value, '/downloads')
  h.component.form.target = '夏日动画'
  await vue.nextTick()
  assert.equal(h.component.storagePreview.value, '/downloads/夏日动画')
  h.component.form.storage = '/vol1/1000/新番/'
  await vue.nextTick()
  assert.equal(h.component.storagePreview.value, '/vol1/1000/新番/夏日动画')
})

test('番剧名里的路径分隔符不会出现在预览目录中', async (t) => {
  const h = dialog(t, true, { library_dir: '/vol1/1000/番剧', download_dir: '/downloads' })
  h.component.form.target = '夏日动画 / Summer'
  await vue.nextTick()
  assert.equal(h.component.storagePreview.value, '/vol1/1000/番剧/夏日动画 _ Summer')
})

test('打开编辑订阅时回填已保存的保存位置', async (t) => {
  const h = dialog(t, false)
  h.state.feed.feed = { ...h.state.feed.feed, id: 7, storage_dir: '/vol1/1000/番剧', target_title: '夏日动画' }
  h.state.feed.open = true
  await flush()
  assert.equal(h.component.form.storage, '/vol1/1000/番剧')
  assert.equal(h.component.storagePreview.value, '/vol1/1000/番剧/夏日动画')
})

test('后端报告保存位置未映射时，订阅仍保存并提示确认', async (t) => {
  const h = dialog(t, true, null, {
    ok: true,
    id: 9,
    storage_path: '/vol1/1000/番剧/夏日动画',
    storage_warning: '/vol1/1000/番剧 没有映射进容器：归档会写进容器自身的文件系统',
  })
  h.state.feed.feed = { name: '夏日动画', url: 'https://feed.example/rss', target_title: '夏日动画' }
  h.state.feed.open = true
  await flush()
  h.component.form.name = '夏日动画'
  h.component.form.url = 'https://feed.example/rss'
  h.component.form.storage = '/vol1/1000/番剧'
  await vue.nextTick()

  await h.component.save()

  assert.deepEqual(h.saved.map((request) => request.url), ['/api/feeds', '/api/sync/9'])
  assert.match(h.errors.join('\n'), /保存位置需要确认.*没有映射进容器/s)
})
