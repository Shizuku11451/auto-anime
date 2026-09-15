import { test } from 'node:test'
import assert from 'node:assert/strict'
import { compareVersions, isDevVersion, noteBlocks, versionParts } from './update.ts'

test('versionParts 解析常见写法', () => {
  assert.deepEqual(versionParts('1.2.3'), [1, 2, 3])
  assert.deepEqual(versionParts('v1.2'), [1, 2])
  assert.deepEqual(versionParts('1.2.0-rc1'), [1, 2, 0])
  assert.deepEqual(versionParts('1.2.0+build9'), [1, 2, 0])
  assert.equal(versionParts('dev'), null)
  assert.equal(versionParts(''), null)
  assert.equal(versionParts('1.2.x'), null)
})

test('compareVersions 判断新版本', () => {
  assert.equal(compareVersions('1.2.0', '1.3.0'), -1)
  assert.equal(compareVersions('1.3.0', '1.2.0'), 1)
  assert.equal(compareVersions('v1.2.0', '1.2.0'), 0)
  assert.equal(compareVersions('1.2', '1.2.0'), 0)
  assert.equal(compareVersions('1.2.0', '1.2.0.1'), -1)
  assert.equal(compareVersions('2.0.0', '10.0.0'), -1)
  // 无法解析时不当作更新
  assert.equal(compareVersions('dev', '1.2.0'), 0)
})

test('isDevVersion 识别本地构建', () => {
  assert.equal(isDevVersion('dev'), true)
  assert.equal(isDevVersion(''), true)
  assert.equal(isDevVersion('1.4.0'), false)
})

test('noteBlocks 整理更新说明', () => {
  const notes = ['## 新增', '- 一键更新', '1. 版本提醒', '', '普通文本', '• 项目符号'].join('\n')
  assert.deepEqual(noteBlocks(notes), [
    { kind: 'heading', text: '新增' },
    { kind: 'bullet', text: '一键更新' },
    { kind: 'bullet', text: '版本提醒' },
    { kind: 'text', text: '普通文本' },
    { kind: 'bullet', text: '项目符号' },
  ])
  assert.deepEqual(noteBlocks(''), [])
  assert.deepEqual(noteBlocks('   \n\n'), [])
})
