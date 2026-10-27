import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

const { descriptor, errors } = parse(readFileSync(new URL('./SubjectSearch.vue', import.meta.url), 'utf8'))
assert.deepEqual(errors, [])
const compiled = ts.transpileModule(descriptor.scriptSetup.content + '\nexport { filters, filterOptions, filteredItems, filtersActive, resetFilters };', {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
const items = [
  { title: '[A] 动画 [简日][HEVC][1080p]', fansub_group: 'A', quality: '1080P', sources: ['mikan', 'nyaasi'] },
  { title: '[B] 动画 [繁日][720p]', fansub_group: 'B', quality: '720P', sources: ['dmhy'] },
  { title: '动画 合集', fansub_group: '', quality: '', sources: ['mikan'] },
]

function fixture(t, resources = items) {
  const state = vue.reactive({ resources: { items: resources, warnings: ['读取上限警告'] } })
  const modules = {
    vue: { ...vue, onMounted() {}, onUnmounted() {} },
    'vue-router': { useRoute: () => ({ query: {} }) },
    '@/stores/subjectSearch': { useSubjectSearch: () => ({ state }) },
    '@/stores/dialogs': { useDialogs: () => ({}) },
    '@/stores/app': { useAppStore: () => ({}) },
  }
  const context = { exports: {}, require: id => modules[id] ?? {} }
  const scope = vue.effectScope()
  scope.run(() => runInNewContext(compiled, context))
  t.after(() => scope.stop())
  return { ...context.exports, state }
}

test('默认显示全部资源；选项从完整结果去重，保留未识别项', t => {
  const h = fixture(t)
  assert.equal(h.filteredItems.value.length, 3)
  assert.equal(h.filtersActive.value, false)
  assert.deepEqual(Array.from(h.filterOptions.value.sources), ['dmhy', 'mikan', 'nyaasi'])
  assert.deepEqual(Array.from(h.filterOptions.value.groups), ['', 'A', 'B'])
  assert.ok(h.filterOptions.value.qualities.includes(''))
  assert.deepEqual(Array.from(h.state.resources.warnings), ['读取上限警告'])
})

test('关键词忽略大小写和全角差异，四项组合取交集，多来源均可匹配', t => {
  const h = fixture(t)
  Object.assign(h.filters, { keyword: '  ｈｅｖｃ  ', source: 'nyaasi', group: 'A', quality: '1080P' })
  assert.equal(h.filteredItems.value.length, 1)
  assert.equal(h.filteredItems.value[0].title, items[0].title)
  h.filters.source = 'mikan'
  assert.equal(h.filteredItems.value.length, 1)
  h.filters.group = 'B'
  assert.equal(h.filteredItems.value.length, 0)
  assert.equal(h.filterOptions.value.groups.length, 3)
  assert.equal(h.state.resources.items.length, 3)
  assert.equal(h.filtersActive.value, true)
})

test('未识别与全部不同；清空、切换条目及重新加载重置筛选', t => {
  const h = fixture(t)
  Object.assign(h.filters, { group: '', quality: '' })
  assert.equal(h.filteredItems.value.length, 1)
  assert.equal(h.filteredItems.value[0].title, items[2].title)
  h.resetFilters()
  assert.equal(h.filteredItems.value.length, 3)
  assert.equal(h.filtersActive.value, false)
  h.filters.keyword = '不存在'
  h.state.resources = null
  assert.equal(h.filtersActive.value, false)
  assert.equal(h.filteredItems.value.length, 0)
  h.state.resources = { items: [items[1]], warnings: [] }
  assert.equal(h.filteredItems.value.length, 1)
  assert.deepEqual(Array.from(h.filterOptions.value.sources), ['dmhy'])
})

test('空资源可筛选和重置，模板使用筛选结果并保留警告与无匹配提示', t => {
  const h = fixture(t, [])
  h.filters.keyword = 'HEVC'
  assert.equal(h.filteredItems.value.length, 0)
  h.resetFilters()
  assert.equal(h.filterOptions.value.groups.length, 0)
  const template = descriptor.template.content
  assert.match(template, /v-for="\(item, index\) in filteredItems"/)
  assert.match(template, /v-for="warning in state.resources.warnings"/)
  assert.match(template, /没有符合条件的资源/)
  assert.match(template, /aria-label="筛选候选资源"/)
})
