<script setup lang="ts">
import { useData, withBase } from 'vitepress'

const { isDark } = useData()
const repo = 'https://github.com/Shizuku11451/auto-anime'
const platforms = [
  { name: '飞牛 fnOS', kind: 'FPK 应用包', detail: 'x86-64 · 推荐', href: '/install/fnos-app', icon: 'server' },
  { name: 'Docker / Linux', kind: '容器与独立程序', detail: 'amd64 / arm64', href: '/install/docker', icon: 'box' },
  { name: 'Windows', kind: '桌面应用', detail: 'Windows 10 / 11 · x64', href: '/install/windows', icon: 'window' },
]
</script>

<template>
  <div class="landing">
    <a class="landing-skip" href="#main">跳到正文</a>
    <header class="landing-header landing-wrap">
      <a class="landing-brand" :href="withBase('/')" aria-label="Auto-Anime 首页">
        <img :src="withBase('/favicon.svg')" alt="" width="30" height="30" />
        Auto-Anime<span class="brand-dot">.</span>
      </a>
      <nav class="landing-nav" aria-label="主导航">
        <a :href="withBase('/guide/quick-start')">文档<span class="nav-doc-extra">与指南</span></a>
        <a :href="repo" class="nav-github">GitHub <svg class="inline-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M5 19 19 5H7m12 0v12"/></svg></a>
        <button class="theme-toggle" type="button" aria-label="切换明暗主题" @click="isDark = !isDark">
          <svg class="theme-sun" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/></svg>
          <svg class="theme-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M20.8 13.2A9 9 0 0 1 10.8 3.1a9 9 0 1 0 10 10.1Z"/></svg>
        </button>
      </nav>
    </header>

    <main id="main" class="landing-wrap">
      <section class="landing-hero" aria-labelledby="hero-title">
        <div class="hero-copy">
          <p class="landing-eyebrow"><span class="status-dot" /> 为 NAS 打造的自动追番工具</p>
          <h1 id="hero-title">你的番剧。<br /><span>自动追更。</span></h1>
          <p class="hero-description">订阅喜欢的番剧，剩下的交给 Auto-Anime。<br />自动下载，按季整理，随时开看。</p>
          <div class="hero-actions">
            <a class="landing-button" href="#install">开始使用 <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="M4 12h15m-6-6 6 6-6 6"/></svg></a>
            <a class="hero-guide" :href="withBase('/guide/quick-start')">阅读指南 <svg class="inline-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M5 19 19 5H7m12 0v12"/></svg></a>
          </div>
          <div class="hero-meta"><span>开源免费</span><span>自托管</span><span>媒体留在你的 NAS</span></div>
        </div>

        <div class="hero-art" role="img" aria-label="日落山海与番剧归档流程插画">
          <img class="landscape" :src="withBase('/landing-landscape.svg')" alt="" width="640" height="690" fetchpriority="high" />
          <div class="art-topline"><span>AUTO-ANIME / COLLECTION</span><span>自 动 收 藏</span></div>
          <div class="art-title" aria-hidden="true">订阅。<br />下载。归档。</div>
          <div class="archive-note"><span class="note-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="m5 12 4 4L19 6"/></svg></span><div><strong>从更新，到入库。</strong><span>订阅 → 下载 → 归档</span></div><span class="note-lines" aria-hidden="true"><i/><i/><i/><i/></span></div>
          <span class="art-bottomline">YOUR ANIME. YOUR SPACE.</span>
        </div>
      </section>

      <section class="workflow" aria-label="核心功能">
        <a :href="withBase('/guide/usage')"><span class="step-number">01 /</span><div><h2>订阅一次</h2><p>搜索番剧，挑选版本，自动追更新集。</p></div><svg class="step-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M5 19 19 5H7m12 0v12"/></svg></a>
        <a :href="withBase('/guide/configuration#下载模式')"><span class="step-number">02 /</span><div><h2>下载到位</h2><p>内置 BT 下载，也能连接常用下载器。</p></div><svg class="step-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M5 19 19 5H7m12 0v12"/></svg></a>
        <a :href="withBase('/guide/usage#番剧库')"><span class="step-number">03 /</span><div><h2>整齐入库</h2><p>按番名和季集归档，浏览器直接播放。</p></div><svg class="step-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M5 19 19 5H7m12 0v12"/></svg></a>
      </section>

      <section id="install" class="install-section" aria-labelledby="install-title">
        <div class="install-heading"><h2 id="install-title">选择你的平台。</h2><a :href="`${repo}/releases/latest`" class="all-releases">所有版本 <svg class="inline-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M5 19 19 5H7m12 0v12"/></svg></a></div>
        <div class="platform-grid">
          <a v-for="platform in platforms" :key="platform.name" :href="withBase(platform.href)" class="platform-link">
            <svg class="platform-icon" viewBox="0 0 32 32" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" aria-hidden="true">
              <template v-if="platform.icon === 'server'"><rect x="5" y="4" width="22" height="24" rx="3"/><path d="M5 16h22M10 10h6m-6 12h6m6-12h1m-1 12h1"/></template>
              <template v-else-if="platform.icon === 'box'"><path d="m16 3 12 7v13l-12 7L4 23V10L16 3Z M4 10l12 7 12-7M16 17v13M10 6.5l12 7"/></template>
              <template v-else><rect x="3" y="5" width="26" height="22" rx="2"/><path d="M3 11h26M8 8h1m3 0h1M16 11v16"/></template>
            </svg>
            <div><h3>{{ platform.name }}</h3><p>{{ platform.kind }}</p><span>{{ platform.detail }}</span></div>
            <svg class="platform-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M4 12h15m-6-6 6 6-6 6"/></svg>
          </a>
        </div>
        <p class="media-compat">与现有媒体库一起使用 <span>Jellyfin</span><span>Emby</span><span>飞牛影视</span></p>
      </section>
    </main>

    <footer class="landing-footer landing-wrap"><a class="footer-brand" :href="repo">Auto-Anime<span class="brand-dot">.</span></a><p>MIT 开源 · 请仅下载你有权保存的内容</p><a :href="withBase('/guide/maintenance')">维护与备份 <svg class="inline-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M5 19 19 5H7m12 0v12"/></svg></a></footer>
  </div>
</template>

<style scoped>
.landing { --paper: #f7f6f2; --ink: #272923; --muted: #686b62; --line: #dcded4; --accent: #b0442e; --card: #eeeee6; background: var(--paper); color: var(--ink); font-family: Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Microsoft YaHei', sans-serif; min-height: 100vh; }
:global(.dark .landing) { --paper: #191c1a; --ink: #eeeee5; --muted: #a5aea2; --line: #383e37; --accent: #ef9b7d; --card: #222722; }
.landing-wrap { max-width: 1320px; padding-inline: 48px; margin-inline: auto; }
.landing a { text-decoration: none; color: inherit; }
.landing a:focus-visible, .landing button:focus-visible { outline: 3px solid var(--accent); outline-offset: 6px; border-radius: 4px; }
.landing-skip { position: fixed; left: 20px; top: -80px; z-index: 50; padding: 12px 20px; background: var(--paper); }
.landing-skip:focus { top: 16px; }
.landing-header { height: 104px; display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.landing-brand { display: flex; align-items: center; font-size: 22px; font-weight: 750; letter-spacing: -.8px; }
.landing-brand img { margin-right: 9px; filter: grayscale(1) brightness(.45); }
:global(.dark .landing-brand img) { filter: grayscale(1) brightness(1.7); }
.brand-dot { color: var(--accent); }
.landing-nav { display: flex; align-items: center; gap: 32px; font-size: 13px; font-weight: 550; }
.landing-nav a:hover, .hero-guide:hover, .all-releases:hover, .landing-footer a:hover { color: var(--accent); }
.inline-arrow { width: 13px; height: 13px; display: inline-block; vertical-align: middle; margin-left: 6px; }
.theme-toggle { width: 34px; height: 34px; border: 1px solid var(--line); border-radius: 50%; display: grid; place-items: center; cursor: pointer; background: transparent; color: var(--ink); }
.theme-toggle svg { width: 17px; height: 17px; }
.theme-toggle .theme-sun { display: none; }
:global(.dark .theme-toggle .theme-sun) { display: block; }
:global(.dark .theme-toggle .theme-moon) { display: none; }
.landing-hero { display: grid; grid-template-columns: 1fr 1fr; align-items: center; gap: 48px; padding: 32px 0 64px; }
.landing-eyebrow { display: flex; align-items: center; gap: 9px; font-size: 11px; font-weight: 600; letter-spacing: 1px; line-height: 1.7; margin: 0; }
.status-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--accent); }
.hero-copy h1 { font-size: clamp(48px, 5.4vw, 76px); font-weight: 800; line-height: 1.25; letter-spacing: -3px; margin: 29px 0 25px; }
.hero-copy h1 span { color: var(--accent); }
.hero-description { color: var(--muted); font-size: 15px; line-height: 2; margin: 0; }
.hero-actions { display: flex; align-items: center; gap: 28px; margin: 34px 0 30px; }
.landing .landing-button { display: flex; align-items: center; gap: 30px; padding: 16px 23px; background: var(--ink); color: var(--paper); font-size: 14px; border-radius: 5px; font-weight: 600; transition: background .2s, transform .2s; }
.landing .landing-button:hover { background: var(--accent); transform: translateY(-2px); }
.landing-button svg { width: 20px; height: 20px; }
.hero-guide { font-size: 13px; font-weight: 550; }
.hero-meta { display: flex; flex-wrap: wrap; gap: 0; color: var(--muted); font-size: 11px; }
.hero-meta span + span::before { content: '/'; margin: 0 12px; opacity: .5; }
.hero-art { aspect-ratio: 1 / 1.04; position: relative; isolation: isolate; border-radius: 8px 8px 80px 8px; overflow: hidden; background: #153a3c; color: #fcf0d9; }
.landscape { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; z-index: -1; }
.art-topline { position: absolute; inset: 26px 27px auto; display: flex; justify-content: space-between; font-size: 8px; letter-spacing: 1.2px; opacity: .85; }
.art-title { position: absolute; top: 17%; left: 8%; font-size: clamp(32px, 3.6vw, 50px); line-height: 1.45; font-weight: 750; letter-spacing: 3px; text-shadow: 0 2px 16px #153a3c33; }
.archive-note { position: absolute; bottom: 13%; left: 8%; right: 8%; display: flex; align-items: center; gap: 13px; padding: 19px; color: #ecedde; background: #173536ed; border: 1px solid #c5d4b633; border-radius: 6px; box-shadow: 0 12px 32px #0b242833; }
.note-icon { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 50%; color: #d0e2ae; background: #d0e2ae18; }
.note-icon svg { width: 21px; height: 21px; }
.archive-note strong { display: block; font-size: 14px; font-weight: 550; }
.archive-note div > span { display: block; font-size: 10px; letter-spacing: 1px; color: #bacbbd; margin-top: 5px; }
.note-lines { margin-left: auto; display: flex; gap: 3px; align-items: end; height: 24px; }
.note-lines i { display: block; width: 4px; background: #b2c6a4; border-radius: 2px; height: 40%; }
.note-lines i:nth-child(2) { height: 65%; }.note-lines i:nth-child(3) { height: 85%; }.note-lines i:nth-child(4) { height: 100%; }
.art-bottomline { position: absolute; bottom: 5%; left: 8%; font-size: 8px; letter-spacing: 2px; color: #d0d8c4; }
.workflow { display: grid; grid-template-columns: repeat(3, 1fr); padding: 32px 0; border-block: 1px solid var(--line); }
.workflow > a { display: flex; gap: 17px; align-items: start; padding: 7px 25px; }
.workflow > a:first-child { padding-left: 0; }.workflow > a:last-child { padding-right: 0; }
.workflow > a + a { border-left: 1px solid var(--line); }
.step-number { color: var(--accent); font-size: 11px; font-family: ui-monospace, monospace; padding-top: 3px; white-space: nowrap; }
.workflow h2 { margin: 0 0 8px; font-size: 17px; line-height: 1.4; font-weight: 650; }
.workflow p { margin: 0; color: var(--muted); font-size: 12px; line-height: 1.8; }
.step-arrow { margin-left: auto; width: 16px; height: 16px; flex-shrink: 0; opacity: .55; transition: transform .2s; }
.workflow a:hover .step-arrow { transform: translate(2px, -2px); color: var(--accent); }
.install-section { padding: 76px 0 58px; scroll-margin-top: 24px; }
.install-heading { display: flex; justify-content: space-between; align-items: end; gap: 20px; margin-bottom: 28px; }
.install-heading h2 { font-size: 29px; font-weight: 700; line-height: 1.4; letter-spacing: -.8px; margin: 9px 0 0; }
.all-releases { font-size: 12px; white-space: nowrap; padding-bottom: 4px; }
.platform-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; }
.platform-link { border: 1px solid var(--line); display: flex; align-items: start; gap: 17px; padding: 25px 22px; border-radius: 6px; transition: background .2s, border-color .2s; }
.platform-link:hover { background: var(--card); border-color: var(--muted); }
.platform-icon { width: 30px; height: 30px; flex-shrink: 0; margin-top: 2px; color: var(--muted); }
.platform-link h3 { font-size: 17px; line-height: 1.5; font-weight: 650; margin: 0 0 7px; }
.platform-link p { font-size: 12px; margin: 0 0 4px; color: var(--muted); }
.platform-link div > span { font-size: 10px; color: var(--muted); }
.platform-arrow { width: 17px; height: 17px; margin: 4px 0 0 auto; flex-shrink: 0; }
.media-compat { display: flex; align-items: center; justify-content: center; gap: 22px; flex-wrap: wrap; font-size: 11px; color: var(--muted); margin: 27px 0 0; }
.media-compat span { font-size: 12px; font-weight: 600; color: var(--ink); }
.landing-footer { display: flex; align-items: center; justify-content: space-between; gap: 20px; border-top: 1px solid var(--line); min-height: 93px; font-size: 11px; color: var(--muted); }
.footer-brand { font-weight: 750; font-size: 16px; letter-spacing: -.5px; color: var(--ink) !important; }
.landing-footer p { margin: 0; }
@media (min-width: 1500px) { .landing-hero { padding-top: 48px; } }
@media (max-width: 1050px) {
  .landing-wrap { padding-inline: 32px; }.landing-hero { gap: 28px; }.hero-copy h1 { font-size: 58px; }.hero-description { font-size: 13px; }.workflow > a { padding-inline: 16px; gap: 10px; }.step-arrow { display: none; }.platform-link { padding: 22px 16px; gap: 12px; }.platform-link h3 { font-size: 15px; }
}
@media (max-width: 760px) {
  .landing-wrap { padding-inline: 24px; }.landing-header { height: 80px; }.landing-brand { font-size: 19px; }.landing-nav { gap: 20px; font-size: 12px; }.nav-doc-extra { display: none; }.landing-hero { grid-template-columns: 1fr; gap: 36px; padding: 32px 0 36px; }.hero-copy h1 { font-size: 60px; margin-top: 23px; }.hero-description { font-size: 14px; }.hero-actions { margin: 27px 0 24px; }.hero-art { aspect-ratio: 1.2; border-bottom-right-radius: 64px; }.art-title { font-size: 36px; top: 18%; line-height: 1.3; }.art-topline { inset: 22px 25px auto; }.archive-note { padding: 15px; }.workflow { grid-template-columns: 1fr; padding: 8px 0; }.workflow > a, .workflow > a:first-child, .workflow > a:last-child { padding: 22px 0; gap: 20px; }.workflow > a + a { border-left: 0; border-top: 1px solid var(--line); }.workflow h2 { font-size: 17px; margin-bottom: 5px; }.workflow p { font-size: 12px; }.step-arrow { display: block; }.install-section { padding: 44px 0 36px; }.install-heading h2 { font-size: 25px; }.platform-grid { grid-template-columns: 1fr; gap: 10px; }.platform-link { padding: 22px; gap: 20px; }.platform-link h3 { font-size: 17px; }.platform-link div { display: grid; grid-template-columns: 1fr; }.platform-icon { width: 32px; height: 32px; }.media-compat { gap: 14px; font-size: 10px; }.media-compat span { font-size: 11px; }.landing-footer { padding-block: 25px; flex-wrap: wrap; gap: 15px; }.landing-footer p { order: 3; width: 100%; font-size: 10px; }
}
@media (max-width: 380px) { .landing-wrap { padding-inline: 20px; }.landing-nav { gap: 13px; }.landing-brand { font-size: 17px; }.hero-copy h1 { font-size: 52px; }.hero-description { font-size: 12px; }.art-topline { font-size: 6px; }.art-title { font-size: 28px; }.archive-note strong { font-size: 12px; }.install-heading h2 { font-size: 22px; }.install-heading { gap: 10px; } }
@media (prefers-reduced-motion: reduce) { .landing *, .landing *::before, .landing *::after { transition: none !important; scroll-behavior: auto !important; } }
</style>
