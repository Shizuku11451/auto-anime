import { test } from 'node:test'
import assert from 'node:assert/strict'
import { episodeSummary, groupReleases, releaseEpisodes, subscriptionTitle, subscriptionWords } from './discover.ts'

const item = (episode, group = '目标组', quality = '1080P', language = '简中') => ({
  title: `[${group}] 夏日动画 / Summer [${episode}] [${quality}][${language}]`,
  anime_title: `夏日动画 / Summer [${quality}][${language}]`,
  fansub_group: group, quality, episode,
  link: `https://example.test/${episode}`, torrent: '', published: '',
})

test('同版多集合并，不混入其他字幕组、清晰度和语言', () => {
  const groups = groupReleases([item('07'), item('08'), item('07', '其他组'), item('07', '目标组', '720P'), item('07', '目标组', '1080P', '繁中')])
  assert.equal(groups.length, 4)
  assert.equal(groups[0].items.length, 2)
  assert.equal(groups[0].title, '夏日动画')
})

test('所选版本过滤后包含后续集数，拒绝其他版本和字幕组子串', () => {
  const words = subscriptionWords(item('07'), '夏日')
  const allowed = (it) => words.split(',').every((w) => it.title.toLowerCase().includes(w.toLowerCase()))
  assert.equal(allowed(item('08')), true)
  assert.equal(allowed(item('08', '另一个目标组')), false)
  assert.equal(allowed(item('08', '目标组', '720P')), false)
  assert.equal(allowed(item('08', '目标组', '1080P', '繁中')), false)
})

test('番名中的斜线与季标记保留', () => {
  assert.equal(subscriptionTitle({ ...item('07'), anime_title: 'Fate/stay night [1080P]' }), 'Fate/stay night')
  assert.equal(subscriptionTitle({ ...item('07'), anime_title: '夏日动画 [S02] [1080P]' }), '夏日动画 [S02]')
})

test('资源条数与集数区分，补入历史页后按集数排列', () => {
  const releases = groupReleases(['07', '08', '06', '05', '04', '03', '02', '01', '008'].map((ep) => item(ep)))
  assert.equal(releases[0].items.length, 9)
  assert.deepEqual(releases[0].episodes, [1, 2, 3, 4, 5, 6, 7, 8])
  assert.equal(episodeSummary(releases[0].episodes), '01–08')
})

test('缺集与未识别集数不展示为连续全集', () => {
  const episodes = releaseEpisodes(['01', '2', '05', '07', '8', '', '合集', '01-12'].map((ep) => item(ep)))
  assert.deepEqual(episodes, [1, 2, 5, 7, 8])
  assert.equal(episodeSummary(episodes), '01–02、05、07–08')
  assert.equal(episodeSummary(releaseEpisodes([item('合集')])), '')
})
