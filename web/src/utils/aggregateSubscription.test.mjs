import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const context = { exports: {}, URLSearchParams }
runInNewContext(ts.transpileModule(readFileSync(new URL('./aggregateSubscription.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText, context)
const { aggregateSubscription } = context.exports
const subject = { id: 123, name_cn: '动画 第二季', name: 'Anime S2' }
const item = { title: '[字幕组] 动画 S2 [01][1080p][简日]', anime_title: '动画', fansub_group: '字幕组', quality: '1080P', season: 2, magnet: `magnet:?xt=urn:btih:${'a'.repeat(40)}` }

test('所选版本生成持久聚合订阅，不依赖 RSS 或当前集号', () => {
  const feed = aggregateSubscription(subject, item, [])
  const url = new URL(feed.url)
  assert.equal(url.protocol, 'aggregate:')
  assert.equal(url.pathname, '/123')
  assert.equal(url.searchParams.get('group'), '字幕组')
  assert.equal(url.searchParams.get('season'), '2')
  assert.equal(url.searchParams.get('language'), '简日')
  assert.equal(feed.target_title, '动画 第二季')
  assert.equal(feed.episode_offset, 0)
  assert.equal(feed.season_filter, 2)
  assert.equal(feed.auto_download, true)
  assert.equal(feed.requires_season_confirmation, true)
  assert.equal(aggregateSubscription(subject, { ...item, episode: '02' }, []).url, feed.url)
  assert.notEqual(aggregateSubscription(subject, { ...item, title: item.title.replace('简日', '繁日') }, []).url, feed.url)
})

test('重复选择编辑已有订阅且保留用户设置；缺少标题或非法磁力不能订阅', () => {
  const feed = aggregateSubscription(subject, item, [])
  const existing = { ...feed, id: 7, auto_download: false, target_title: '用户命名', storage_dir: '/media', enabled: false }
  const edit = aggregateSubscription(subject, item, [existing])
  assert.equal(edit.id, 7)
  assert.equal(edit.auto_download, false)
  assert.equal(edit.target_title, '用户命名')
  assert.equal(edit.storage_dir, '/media')
  assert.equal(edit.enabled, false)
  assert.equal(aggregateSubscription(subject, { ...item, anime_title: '' }, []), null)
  assert.equal(aggregateSubscription(subject, { ...item, magnet: 'https://example.org' }, []), null)
  assert.equal(aggregateSubscription(subject, { ...item, season: 0 }, []).season_filter, -1)
  const punctuation = { ...item, anime_title: '动画 ~' }
  const goEncoded = aggregateSubscription(subject, punctuation, [])
  goEncoded.url = goEncoded.url.replace('%7E', '~')
  assert.equal(aggregateSubscription(subject, punctuation, [{ ...goEncoded, id: 9 }]).id, 9)
})
