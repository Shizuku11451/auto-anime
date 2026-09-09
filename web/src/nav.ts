import type { Component } from 'vue'
import { Clapperboard, Compass, Inbox, Rss, Settings } from '@lucide/vue'

export interface NavItem {
  to: string
  name: string
  label: string
  icon: Component
}

export const NAV_ITEMS: NavItem[] = [
  { to: '/library', name: 'library', label: '番剧库', icon: Clapperboard },
  { to: '/items', name: 'items', label: '最近条目', icon: Inbox },
  { to: '/feeds', name: 'feeds', label: '订阅管理', icon: Rss },
  { to: '/discover', name: 'discover', label: '找番', icon: Compass },
  { to: '/settings', name: 'settings', label: '设置', icon: Settings },
]
