import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

test('取消未保存设置的离开操作后，页面标题保留当前路由', () => {
  let afterEach
  const document = { title: '设置 · Auto-Anime' }
  const source = ts.transpileModule(readFileSync(new URL('./router.ts', import.meta.url), 'utf8'), {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText
  runInNewContext(source, {
    exports: {}, document,
    require: id => id === 'vue-router' ? {
      createWebHashHistory: () => ({}),
      createRouter: () => ({ afterEach: fn => { afterEach = fn } }),
    } : { default: {} },
  })
  afterEach({ meta: { title: '订阅管理' } }, {}, { type: 4 })
  assert.equal(document.title, '设置 · Auto-Anime')
  afterEach({ meta: { title: '订阅管理' } }, {}, undefined)
  assert.equal(document.title, '订阅管理 · Auto-Anime')
})
