import { createRouter, createWebHashHistory } from 'vue-router'
import LibraryView from './views/LibraryView.vue'
import ItemsView from './views/ItemsView.vue'
import FeedsView from './views/FeedsView.vue'
import DiscoverView from './views/DiscoverView.vue'
import SettingsView from './views/SettingsView.vue'

// hash 路由：Go 服务端只把 / 映射到 index.html，不需要为前端路由做回退配置。
export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/library' },
    { path: '/library', name: 'library', component: LibraryView, meta: { title: '番剧库' } },
    { path: '/items', name: 'items', component: ItemsView, meta: { title: '最近条目' } },
    { path: '/feeds', name: 'feeds', component: FeedsView, meta: { title: '订阅管理' } },
    { path: '/discover', name: 'discover', component: DiscoverView, meta: { title: '找番' } },
    { path: '/settings', name: 'settings', component: SettingsView, meta: { title: '设置' } },
    { path: '/:pathMatch(.*)*', redirect: '/library' },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

router.afterEach((to) => {
  const title = to.meta.title as string | undefined
  document.title = title ? `${title} · 自动追番` : '自动追番'
})
