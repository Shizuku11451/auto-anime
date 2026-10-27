import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'

test('搜索页只挂载 Bangumi 与聚合资源搜索，不加载旧 RSS 搜索', () => {
  const source = readFileSync(new URL('./DiscoverView.vue', import.meta.url), 'utf8')
  const { descriptor, errors } = parse(source)
  assert.deepEqual(errors, [])
  assert.match(descriptor.template.content, /title="搜索"/)
  assert.match(descriptor.template.content, /<SubjectSearch \/>/)
  assert.doesNotMatch(source, /useDiscover|fallback|RSS|search-modes/)
  const subject = readFileSync(new URL('../components/discover/SubjectSearch.vue', import.meta.url), 'utf8')
  assert.doesNotMatch(subject, /fallback|RSS 搜索|搜索 RSS/)
})
