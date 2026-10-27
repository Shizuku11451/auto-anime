import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

// Execute the card's real compiled setup so the rating badge and expandable
// summary behave as the template expects, without a DOM.
const source = readFileSync(new URL('./AnimeCard.vue', import.meta.url), 'utf8')
const descriptor = parse(source).descriptor
const scriptBlock = compileScript(descriptor, { id: 'test' })
const compiled = ts.transpileModule(scriptBlock.content, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText

const stub = { render: () => null }
const scheduleContext = { exports: {} }
runInNewContext(ts.transpileModule(readFileSync(new URL('./schedule.ts', import.meta.url), 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText, scheduleContext)
const typesContext = { exports: {} }
runInNewContext(ts.transpileModule(readFileSync(new URL('../../api/types.ts', import.meta.url), 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText, typesContext)
const modules = {
  '@/components/library/schedule': scheduleContext.exports,
  vue,
  '@lucide/vue': new Proxy({}, { get: () => stub }),
  '@/components/ui/BaseButton.vue': stub,
  '@/components/ui/CoverImage.vue': stub,
  '@/api/types': typesContext.exports,
}
const context = { exports: {}, require: (id) => {
  assert.ok(id in modules, `unmocked import: ${id}`)
  return modules[id]
} }
runInNewContext(compiled, context, { filename: 'AnimeCard.vue' })
const component = context.exports.default
assert.equal(typeof component.setup, 'function')

const anime = {
  id: 7, title: '测试番剧甲', meta_name_cn: '无职转生 第三季', meta_name: '', search_name: '',
  bangumi_id: 501963, season: 1, storage_dir: '', include_rules: '', exclude_rules: '', updated_at: '',
  cover: '/api/covers/x.jpg', item_count: 8, saved_count: 8, episode_count: 8,
  rating: 7.9, summary: '这是第一行简介。\n这是第二行简介。',
}

function card(extra = {}) {
  const events = []
  const bound = component.setup({ anime: { ...anime, ...extra } }, {
    emit: (ev, ...args) => events.push([ev, ...args]),
    expose: () => {}, slots: {}, attrs: {},
  })
  return { events, bound }
}

test('编译后的卡片包含评分徽章与简介展开结构', () => {
  const template = descriptor.template.content.replace(/\r\n/g, '\n')
  assert.match(template, /v-if="anime\.rating > 0"/)
  assert.match(template, /class="rating-badge"/)
  assert.match(template, /Bangumi 评分 \$\{anime\.rating\.toFixed\(1\)\}/)
  assert.match(template, /v-if="anime\.summary" class="poster-veil" aria-hidden="true"/)
  assert.match(template, /class="veil-text"/)
  assert.match(template, /v-if="anime\.summary" type="button" class="summary-toggle"/)
  assert.match(template, /:aria-expanded="expanded"/)
  assert.match(template, /@click="expanded = !expanded"/)
  assert.match(template, /v-if="anime\.summary" v-show="expanded" class="summary-panel"/)
  assert.match(template, /\{\{ anime\.summary \}\}/)
})

test('评分与简介齐全时 setup 暴露展开状态，默认收起可切换', async () => {
  const h = card()
  assert.equal(vue.unref(h.bound.expanded), false)
  assert.equal(vue.unref(h.bound.ratio), 100)
  assert.equal(vue.unref(h.bound.displayName), '无职转生 第三季')
  h.bound.expanded.value = !h.bound.expanded.value
  await vue.nextTick()
  assert.equal(vue.unref(h.bound.expanded), true)
})

test('未绑定元数据的番剧标题回退且无展开内容', () => {
  const h = card({ rating: 0, summary: '', meta_name_cn: '', bangumi_id: 0 })
  assert.equal(vue.unref(h.bound.displayName), '测试番剧甲')
  assert.equal(vue.unref(h.bound.showSub), false)
  assert.equal(vue.unref(h.bound.ratio), 100)
})

test('模板渲染：评分徽章随评分出现，简介面板默认收起', async () => {
  const { compileTemplate } = await import('@vue/compiler-sfc')
  const { renderToString } = await import('vue/server-renderer')
  const tpl = compileTemplate({
    source: descriptor.template.content.replace(/\r\n/g, '\n'),
    filename: 'AnimeCard.vue', id: 'test',
    compilerOptions: { mode: 'function', bindingMetadata: scriptBlock.bindings },
  })
  assert.deepEqual(tpl.errors, [])
  const render = new Function('Vue', `${tpl.code}\nreturn render;`)(vue)
  const html = async (props, extra = {}) => {
    const app = vue.createSSRApp({ render: () => vue.h({ ...component, render }, { anime: props, ...extra }) })
    for (const name of ['CoverImage', 'BaseButton', 'Star', 'ChevronDown', 'CircleCheck', 'Film', 'Tv', 'Pencil', 'Merge', 'Trash2']) {
      app.component(name, stub)
    }
    return renderToString(app)
  }
  const withMeta = await html(anime)
  assert.match(withMeta, /rating-badge/)
  assert.match(withMeta, /7\.9/)
  assert.match(withMeta, /Bangumi 评分 7\.9/)
  assert.match(withMeta, /aria-expanded="false"/)
  assert.match(withMeta, /summary-panel/)
  assert.match(withMeta, /display:none/)
  assert.match(withMeta, /poster-veil/)
  assert.match(withMeta, /veil-text/)
  assert.match(withMeta, /class="veil-text">[^<]*这是第一行简介。/)
  const withoutMeta = await html({ ...anime, rating: 0, summary: '', bangumi_id: 0 })
  assert.doesNotMatch(withoutMeta, /rating-badge/)
  assert.doesNotMatch(withoutMeta, /summary-toggle/)
  assert.doesNotMatch(withoutMeta, /summary-panel/)
  assert.doesNotMatch(withoutMeta, /poster-veil/)
  for (const [type, label] of Object.entries(typesContext.exports.BANGUMI_COLLECTION_LABELS)) {
    const local = await html(anime, { collectionType: Number(type), selecting: true })
    assert.match(local, new RegExp(`>${label}</span>`))
    assert.match(local, /8 资源/)
    assert.match(local, /8 已保存/)
    assert.match(local, /type="checkbox"/)
    assert.match(local, /class="ratio"/)
    assert.match(local, /不代表本地下载或播放进度/)
  }
  const cloud = await html(anime, { collectionType: 3, cloudOnly: true, selecting: true })
  assert.match(cloud, />在看<\/span>/)
  assert.match(cloud, /找资源/)
  assert.match(cloud, /disabled/)
  assert.doesNotMatch(cloud, /type="checkbox"|class="ratio"|8 资源|8 已保存/)
  assert.doesNotMatch(withMeta, /class="collection-state"/)
})
