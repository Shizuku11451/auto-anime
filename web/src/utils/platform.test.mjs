import { test } from 'node:test'
import assert from 'node:assert/strict'
import { isAbsoluteStoragePath, storageDirExample } from './platform.ts'

test('路径示例是容器或 NAS 挂载路径', () => {
  assert.equal(storageDirExample, '/vol1/1000/番剧')
})

test('容器/NAS 接受 NAS 挂载路径与盘符', () => {
  assert.equal(isAbsoluteStoragePath('/vol1/1000/番剧'), true)
  assert.equal(isAbsoluteStoragePath('D:\\番剧'), true)
  assert.equal(isAbsoluteStoragePath('番剧'), false)
})
