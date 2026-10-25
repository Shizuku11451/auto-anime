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
  const traditional = { ...item,
    title: '[綠茶字幕組] 無職轉生 第三季 ～到了異世界就拿出真本事～ / Mushoku Tensei S3 [14][WebRip][1080p][繁日內嵌]',
    anime_title: '無職轉生 ～到了異世界就拿出真本事～', fansub_group: '綠茶字幕組', quality: '1080P',
  }
  state.result = { keyword: '无职转生', feed_url: 'https://example.test/traditional-feed', items: [traditional] }
  context.exports.subscribe(traditional)
  assert.equal(opened.url, state.result.feed_url)
  assert.equal(opened.id, undefined)
  assert.equal(opened.target_title, traditional.anime_title)
  assert.ok(opened.include_words.split(',').every(word => word === '无职转生' || traditional.title.toLowerCase().includes(word.toLowerCase())))
  assert.ok(opened.include_words.includes('無職轉生'))
  assert.equal(opened.include_words.includes('无职转生'), true)
  const expanded = { ...item, season: 1, feed_url: 'https://example.test/S01', query: '夏日动画 S01' }
  state.result = { keyword: '夏日动画 第一季', base_keyword: '夏日动画', requested_season: 1, feed_url: 'https://example.test/original', items: [expanded] }
  await vue.nextTick()
  const remounted = { exports: {}, require: id => modules[id] }
  scope.run(() => runInNewContext(compiled, remounted))
  assert.equal(remounted.exports.filtered.value.length, 1)
  state.result.items.push({ ...expanded, season: 3 })
  assert.equal(remounted.exports.filtered.value.length, 1)
  context.exports.subscribe(expanded)
  assert.equal(opened.url, expanded.feed_url)
  assert.equal(opened.season_filter, 1)
  assert.equal(opened.allow_unknown_season, false)
  assert.equal(opened.include_words.includes('第一季'), false)
  context.exports.subscribe({ ...expanded, season: 0 })
  assert.equal(opened.season_filter, -1)
  assert.equal(opened.auto_download, false)
  assert.equal(opened.requires_season_confirmation, true)
  const inferred = { ...expanded, season: 0, inferred_season: 1 }
  state.result.items.push(inferred)
  assert.equal(remounted.exports.filtered.value.length, 2)
  context.exports.subscribe(inferred)
  assert.equal(opened.season_filter, -1)
  assert.equal(opened.allow_unknown_season, false)
  assert.equal(opened.auto_download, false)
  assert.equal(opened.requires_season_confirmation, true)
  assert.ok(opened.name.includes('推测第 1 季'))
  const previous = opened
  context.exports.subscribe({ ...expanded, season: -1 })
  assert.equal(opened, previous)
})
