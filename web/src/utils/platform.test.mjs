import { test } from 'node:test'
import assert from 'node:assert/strict'
import { isAbsoluteStoragePath, isWindows, isWindowsPlatform, storageDirExample } from './platform.ts'

test('平台判断只认后端返回的 windows', () => {
  assert.equal(isWindows('windows'), true)
  assert.equal(isWindows('linux'), false)
  assert.equal(isWindows(undefined), false)
  assert.equal(isWindowsPlatform('windows/amd64'), true)
  assert.equal(isWindowsPlatform('linux/arm64'), false)
  assert.equal(isWindowsPlatform(''), false)
})

test('路径示例按平台区分', () => {
  assert.match(storageDirExample('windows'), /D:\\/)
  assert.equal(storageDirExample('linux'), '/vol1/1000/番剧')
  assert.equal(storageDirExample(undefined), '/vol1/1000/番剧')
})

test('Windows 只接受盘符或 UNC 绝对路径', () => {
  assert.equal(isAbsoluteStoragePath('D:\\番剧', 'windows'), true)
  assert.equal(isAbsoluteStoragePath('D:/番剧', 'windows'), true)
  assert.equal(isAbsoluteStoragePath('\\\\NAS\\共享\\番剧', 'windows'), true)
  assert.equal(isAbsoluteStoragePath('//NAS/共享/番剧', 'windows'), true)
  // "/番剧"、"\番剧" 会按当前盘解析，不能当作完整路径。
  assert.equal(isAbsoluteStoragePath('/番剧', 'windows'), false)
  assert.equal(isAbsoluteStoragePath('\\番剧', 'windows'), false)
  assert.equal(isAbsoluteStoragePath('番剧', 'windows'), false)
})

test('容器/Linux 接受 NAS 挂载路径与盘符', () => {
  assert.equal(isAbsoluteStoragePath('/vol1/1000/番剧', 'linux'), true)
  assert.equal(isAbsoluteStoragePath('D:\\番剧', 'linux'), true)
  assert.equal(isAbsoluteStoragePath('番剧', 'linux'), false)
})
