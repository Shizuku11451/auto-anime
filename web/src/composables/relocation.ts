/* 番剧文件迁移：启动后台任务并跟踪到结束，结束时刷新列表与详情并报告结果。 */
import { http, errorMessage } from '@/api/client'
import type { RelocationStatus } from '@/api/types'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { toast } from '@/stores/toast'

export function relocationSummary(st: RelocationStatus): string {
  if (st.failed.length) {
    const first = st.failed[0]!
    return `已移动 ${st.moved} 集，${st.failed.length} 集未移动（${first.title}：${first.error}），原文件保留在旧位置`
  }
  return st.moved ? `已移动 ${st.moved} 集到新位置` : ''
}

export function followRelocation(animeId: number, status: RelocationStatus) {
  const dialogs = useDialogs()
  const store = useAppStore()
  dialogs.watchRelocation(animeId, status, async (done) => {
    const text = relocationSummary(done)
    if (done.failed.length) toast.error(text)
    else if (text) toast.success(text)
    if (done.warning) toast.info(done.warning)
    await Promise.all([store.refresh(), dialogs.reloadAnimeDetail()])
  })
}

/** 按番剧当前设置移动已保存的文件；失败只提示，不影响已保存的设置。 */
export async function startRelocation(animeId: number) {
  try {
    const status = await http.post<RelocationStatus>(`/api/animes/${animeId}/relocation`)
    followRelocation(animeId, status)
  } catch (e) {
    toast.error('文件未移动：' + errorMessage(e))
  }
}
