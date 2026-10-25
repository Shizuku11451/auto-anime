import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
const source = parse(readFileSync(new URL('./LibraryView.vue', import.meta.url), 'utf8')).descriptor.scriptSetup.content
const compiled = compile(source + '\nexport { weekdayList, weekdayIndicator, chooseDay, date, clearFilters };')
const schedule = { exports: {} }
runInNewContext(compile(readFileSync(new URL('../components/library/schedule.ts', import.meta.url), 'utf8')), schedule)

test('星期滑块跟随按钮、日期和清除筛选，尺寸变化与数据刷新后重新对齐', async t => {
  let resize
  const store = { animes: vue.ref([]), loaded: vue.ref(false) }
  const modules = {
    vue,
    '@vueuse/core': { useResizeObserver(_target, callback) { resize = callback } },
    'vue-router': { useRouter: () => ({}) },
    '@/stores/app': { useAppStore: () => store },
    '@/stores/dialogs': { useDialogs: () => ({}) },
    '@/components/library/schedule': schedule.exports,
  }
  const context = { exports: {}, require: id => {
    assert.ok(id in modules, `unexpected import: ${id}`)
    return modules[id]
  } }
  const scope = vue.effectScope()
  scope.run(() => runInNewContext(compiled, context))
  t.after(() => scope.stop())
  const h = context.exports
  let button = { offsetLeft: 3, offsetWidth: 90, offsetHeight: 41 }
  h.weekdayList.value = { querySelector: () => button }
  await vue.nextTick()
  const check = () => {
    assert.equal(h.weekdayIndicator.value.transform, `translateX(${button.offsetLeft}px)`)
    assert.equal(h.weekdayIndicator.value.width, `${button.offsetWidth}px`)
    assert.equal(h.weekdayIndicator.value.height, `${button.offsetHeight}px`)
  }
  check()
  button = { offsetLeft: 280, offsetWidth: 105, offsetHeight: 41 }
  h.chooseDay(3)
  await vue.nextTick()
  check()
  button.offsetLeft = 480
  h.date.value = '2026-10-25'
  await vue.nextTick()
  check()
  button.offsetWidth = 72
  resize()
  check()
  button.offsetWidth = 80
  store.loaded.value = true
  await vue.nextTick()
  check()
  button.offsetLeft = 3
  h.clearFilters()
  await vue.nextTick()
  check()
})
