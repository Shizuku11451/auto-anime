import { defineConfig } from 'vitepress'

const repo = 'https://github.com/Shizuku11451/auto-anime'
// 官网部署在 https://autoanime.top 根路径；部署到子路径时设置 SITE_BASE，如 /auto-anime/
const base = process.env.SITE_BASE ?? '/'

// 与 GitHub 一致的标题锚点，docs/ 中为 GitHub 编写的锚点链接在站内同样有效
const githubSlugify = (s: string) =>
  s.trim().toLowerCase().replace(/[^\p{L}\p{M}\p{N}\p{Pc}\- ]/gu, '').replace(/ /g, '-')

export default defineConfig({
  lang: 'zh-CN',
  title: 'Auto-Anime',
  description: '为 NAS 打造的自动追番服务：番剧搜索、订阅追更、内置下载、季集归档、番剧库与播放。',
  base,
  cleanUrls: true,
  lastUpdated: true,
  markdown: {
    anchor: { slugify: githubSlugify },
  },
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}favicon.svg` }],
    ['meta', { name: 'theme-color', content: '#668fff' }],
  ],
  themeConfig: {
    logo: '/favicon.svg',
    nav: [
      { text: '指南', link: '/guide/introduction', activeMatch: '^/(guide|install)/' },
      { text: '配置', link: '/guide/configuration' },
      { text: '开发', link: '/develop/contributing', activeMatch: '^/(develop|advanced)/' },
      { text: '下载', link: `${repo}/releases/latest` },
    ],
    sidebar: [
      {
        text: '开始',
        items: [
          { text: '介绍', link: '/guide/introduction' },
          { text: '快速开始', link: '/guide/quick-start' },
        ],
      },
      {
        text: '安装',
        items: [
          { text: '飞牛 fnOS 应用包', link: '/install/fnos-app' },
          { text: '飞牛 Compose 安装', link: '/install/fnos-compose' },
          { text: 'Docker / Linux', link: '/install/docker' },
          { text: 'Windows 桌面版', link: '/install/windows' },
        ],
      },
      {
        text: '使用',
        items: [
          { text: '使用指南', link: '/guide/usage' },
          { text: '配置参考', link: '/guide/configuration' },
          { text: '维护与备份', link: '/guide/maintenance' },
          { text: '功能边界', link: '/guide/features' },
        ],
      },
      {
        text: '进阶',
        items: [{ text: '自建聚合搜索服务', link: '/advanced/search-server' }],
      },
      {
        text: '开发',
        items: [
          { text: '参与开发', link: '/develop/contributing' },
          { text: '发布流程', link: '/develop/releasing' },
        ],
      },
    ],
    socialLinks: [{ icon: 'github', link: repo }],
    editLink: {
      pattern: `${repo}/edit/main/site/:path`,
      text: '在 GitHub 上编辑此页',
    },
    search: {
      provider: 'local',
      options: {
        translations: {
          button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
          modal: {
            noResultsText: '没有找到结果',
            resetButtonTitle: '清除查询',
            footer: { selectText: '选择', navigateText: '切换', closeText: '关闭' },
          },
        },
      },
    },
    outline: { level: [2, 3], label: '本页目录' },
    docFooter: { prev: '上一页', next: '下一页' },
    lastUpdated: { text: '最后更新' },
    returnToTopLabel: '返回顶部',
    sidebarMenuLabel: '菜单',
    darkModeSwitchLabel: '主题',
    lightModeSwitchTitle: '切换到浅色模式',
    darkModeSwitchTitle: '切换到深色模式',
    footer: {
      message: '基于 MIT 许可发布 · 请仅下载你有权访问和保存的内容',
      copyright: `<a href="${repo}">Auto-Anime</a> 贡献者`,
    },
  },
})
