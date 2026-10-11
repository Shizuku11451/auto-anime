import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'

const source = readFileSync(new URL('./generator.js', import.meta.url), 'utf8')
const generate = runInNewContext(source + '\n generateCompose')
const defaults = { custom: false, path: '', port: '8000', bt: '6881' }

test('默认配置保留独立数据目录，网页端口只映射到 8000，BT 三处一致', () => {
  const yaml = generate({ ...defaults, port: '18000', bt: '16881' })
  assert.ok(yaml.includes('source: "./media"'))
  assert.ok(yaml.includes('./data:/data'))
  assert.ok(yaml.includes('"18000:8000"'))
  assert.ok(yaml.includes('"16881:16881/tcp"'))
  assert.ok(yaml.includes('"16881:16881/udp"'))
  assert.ok(yaml.includes('QBITTORRENT_BT_PORT: "16881"'))
  assert.ok(yaml.includes('KEEP_SOURCE_FILES: "true"'))
})

test('自定义路径保留中文、空格、冒号与引号，并转义 Compose 环境变量', () => {
  const path = '/vol1/1000/番剧 : "${ANI}"'
  const yaml = generate({ ...defaults, custom: true, path })
  const source = yaml.match(/source: (.*)/)[1]
  assert.equal(JSON.parse(source), path.replace(/\$/g, '$$$$'))
  assert.ok(yaml.includes('LIBRARY_DIR: /media'))
  assert.ok(yaml.includes('target: /media'))
})

test('无效路径和端口无法生成可部署配置', () => {
  for (const path of ['', '/', '我的文件/ani', '/vol1/../ani', '/vol1/./ani', '/ani\nservices:', '/ani\u0000', 'C:\\ani']) {
    assert.throws(() => generate({ ...defaults, custom: true, path }))
  }
  for (const port of ['', '0', '65536', '12.5', '8000:80', '6881']) {
    assert.throws(() => generate({ ...defaults, port }))
  }
})

test('可以关闭原文件保留，默认开启', () => {
  assert.ok(generate({ ...defaults, keepSource: false }).includes('KEEP_SOURCE_FILES: "false"'))
  assert.ok(generate(defaults).includes('KEEP_SOURCE_FILES: "true"'))
})
