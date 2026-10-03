import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'
import * as discoverUtils from '../utils/discover.ts'

const source = parse(readFileSync(new URL('./DiscoverView.vue', import.meta.url), 'utf8')).descriptor.scriptSetup.content
const compiled = ts.transpileModule(source + '\nexport { subscribe, filters, filtered };', {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText

test('输入框和来源修改后，订阅仍对应展示结果；新结果到达后清除旧筛选', async (t) => {
  const item = { title: '[字幕组] 夏日动画 [01][1080P][简中]', anime_title: '夏日动画', fansub_group: '字幕组', quality: '1080P', episode: '01', published: '', torrent: '', link: '' }
  const state = vue.reactive({ sources: [], keyword: '尚未搜索的番名', source: 'new-source', result: { keyword: '夏日', feed_url: 'https://example.test/old-feed', items: [item] } })
  const feeds = vue.ref([{ id: 7, name: '旧订阅', url: state.result.feed_url, storage_dir: '/data/anime', enabled: false, auto_download: false }])
  let opened
  const modules = {
    vue: { ...vue, onMounted() {}, onUnmounted() {} },
    '@/stores/discover': { useDiscover: () => ({ state }) },
    '@/stores/dialogs': { useDialogs: () => ({ openFeed: feed => { opened = feed } }) },
    '@/stores/app': { useAppStore: () => ({ feeds }) },
    '@/utils/discover': discoverUtils,
  }
  const context = { exports: {}, require: id => modules[id] }
  const scope = vue.effectScope()
  scope.run(() => runInNewContext(compiled, context))
  t.after(() => scope.stop())
  context.exports.subscribe(item)
  assert.equal(opened.url, 'https://example.test/old-feed')
  assert.ok(opened.include_words.startsWith('夏日,'))
  assert.equal(opened.include_words.includes('尚未搜索'), false)
  assert.equal(opened.id, 7)
  assert.equal(opened.storage_dir, '/data/anime')
  assert.equal(opened.enabled, false)
  assert.equal(opened.auto_download, false)
  context.exports.filters.group = '不会匹配的字幕组'
  assert.equal(context.exports.filtered.value.length, 0)
  state.result = { keyword: '新搜索', feed_url: 'https://example.test/new-feed', items: [item] }
  await vue.nextTick()
  assert.equal(context.exports.filtered.value.length, 1)
})
