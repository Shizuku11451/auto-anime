// 将仓库 docs/ 与贡献指南同步为文档站页面，保持单一文档来源。
// 生成的页面已在 site/.gitignore 中忽略，请修改 docs/ 下的原文。
import { copyFile, mkdir, readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const siteDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const repoDir = path.resolve(siteDir, '..')
const repoBlob = 'https://github.com/Shizuku11451/auto-anime/blob/main/'

// 仓库相对路径 -> 站内路由（不含 .md）
export const pages = {
  'docs/FNOS_APP.md': 'install/fnos-app',
  'docs/FNOS.md': 'install/fnos-compose',
  'docs/WINDOWS.md': 'install/windows',
  'docs/USAGE.md': 'guide/usage',
  'docs/CONFIGURATION.md': 'guide/configuration',
  'docs/MAINTENANCE.md': 'guide/maintenance',
  'docs/FEATURE_COMPARISON.md': 'guide/features',
  'docs/SEARCH_SERVER.md': 'advanced/search-server',
  'CONTRIBUTING.md': 'develop/contributing',
  'docs/RELEASING.md': 'develop/releasing',
}

// 不单独生成页面、但会被链接到的仓库文档
const aliases = {
  'README.md': 'guide/introduction',
  'docs/README.md': 'guide/introduction',
}

function resolveLink(fromFile, target) {
  if (/^([a-z]+:|#|\/\/)/i.test(target)) return target
  const [rawPath, hash] = target.split('#')
  const repoPath = path.posix.normalize(path.posix.join(path.posix.dirname(fromFile), rawPath))
  const route = pages[repoPath] ?? aliases[repoPath]
  if (route) {
    // README 中的锚点在站内页面不存在，只保留页面级跳转
    const keepHash = hash && pages[repoPath]
    return '/' + route + (keepHash ? '#' + hash : '')
  }
  return repoBlob + repoPath + (hash ? '#' + hash : '')
}

function transform(fromFile, source) {
  const lines = source.replace(/^﻿/, '').split(/\r?\n/)
  // 去掉标题下方的面包屑导航行，站点已有侧边栏
  const crumb = lines.findIndex((line, i) => i < 5 && /^\[[^\]]+\]\([^)]+\)( · \[[^\]]+\]\([^)]+\))+\s*$/.test(line))
  if (crumb !== -1) lines.splice(crumb, lines[crumb + 1] === '' ? 2 : 1)

  let inFence = false
  const out = lines.map((line) => {
    if (/^\s*(```|~~~)/.test(line)) inFence = !inFence
    if (inFence) return line
    return line.replace(/(\]\()([^)\s]+)(\))/g, (_, open, target, close) => open + resolveLink(fromFile, target) + close)
  })
  const editLink = `${repoBlob}${fromFile}`
  return `---\neditLink: false\n---\n\n<!-- 本页由 site/scripts/sync-docs.mjs 从 ${fromFile} 生成，请修改原文。 -->\n\n${out.join('\n')}\n\n---\n\n<p class="source-note">本页同步自仓库 <a href="${editLink}">${fromFile}</a></p>\n`
}

for (const [from, route] of Object.entries(pages)) {
  const source = await readFile(path.join(repoDir, from), 'utf8')
  const dest = path.join(siteDir, route + '.md')
  await mkdir(path.dirname(dest), { recursive: true })
  await writeFile(dest, transform(from, source), 'utf8')
}

await mkdir(path.join(siteDir, 'public'), { recursive: true })
for (const asset of ['logo.svg', 'favicon.svg']) {
  await copyFile(path.join(repoDir, 'web/public', asset), path.join(siteDir, 'public', asset))
}

console.log(`synced ${Object.keys(pages).length} pages`)
