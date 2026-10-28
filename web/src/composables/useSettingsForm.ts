/* 设置页状态：按分类独立编辑、校验与保存；每次保存只提交当前分类的字段。 */
import { computed, inject, nextTick, onBeforeUnmount, provide, reactive, ref, watch, type InjectionKey } from 'vue'
import { api, http, errorMessage } from '@/api/client'
import { confirmDialog } from '@/stores/confirm'
import { toast } from '@/stores/toast'
import { useAppStore } from '@/stores/app'
import type { Settings, SettingsInput, SetupStorageResult, StorageCheckInput } from '@/api/types'
import { isAbsoluteStoragePath } from '@/utils/platform'

function emptyForm() {
  return {
    web_user: '',
    web_password: '',
    download_mode: 'direct',
    aria2_url: '',
    aria2_token: '',
    aria2_dir: '',
    aria2_local_dir: '',
    qbit_url: '',
    qbit_user: '',
    qbit_password: '',
    qbit_save_path: '',
    qbit_local_dir: '',
    transmission_url: '',
    transmission_user: '',
    transmission_password: '',
    transmission_download_dir: '',
    transmission_local_dir: '',
    proxy_url: '',
    proxy_clear_credentials: false,
    poll_minutes: 30 as number,
    library_dir: '',
    auto_cleanup_empty_anime: false,
    keep_source_files: true,
  }
}
export type SettingsForm = ReturnType<typeof emptyForm>
export type SettingsField = keyof SettingsForm
export type SectionId = 'download' | 'library' | 'network' | 'account' | 'bangumi' | 'update'

export const SECTIONS: { id: SectionId; title: string; fields: SettingsField[] }[] = [
  { id: 'download', title: '下载方式', fields: ['download_mode', 'aria2_url', 'aria2_token', 'aria2_dir', 'aria2_local_dir', 'qbit_url', 'qbit_user', 'qbit_password', 'qbit_save_path', 'qbit_local_dir', 'transmission_url', 'transmission_user', 'transmission_password', 'transmission_download_dir', 'transmission_local_dir'] },
  { id: 'library', title: '媒体库', fields: ['library_dir', 'keep_source_files', 'auto_cleanup_empty_anime'] },
  { id: 'network', title: '网络与同步', fields: ['proxy_url', 'proxy_clear_credentials', 'poll_minutes'] },
  { id: 'account', title: '管理员账户', fields: ['web_user', 'web_password'] },
  { id: 'bangumi', title: 'Bangumi', fields: [] },
  { id: 'update', title: '版本更新', fields: [] },
]
/* 只写字段：留空表示保持不变，保存成功后清空。 */
const SECRET_FIELDS: SettingsField[] = ['web_password', 'aria2_token', 'qbit_password', 'transmission_password']

export function isSectionId(value: unknown): value is SectionId {
  return SECTIONS.some(section => section.id === value)
}
export function sectionTitle(id: SectionId) {
  return SECTIONS.find(section => section.id === id)?.title || ''
}
function sectionFields(id: SectionId) {
  return SECTIONS.find(section => section.id === id)?.fields || []
}
export function isEditableSection(id: SectionId) {
  return sectionFields(id).length > 0
}

function formFromSettings(s: Settings): SettingsForm {
  return {
    web_user: s.web_user || '',
    web_password: '',
    download_mode: s.download_mode || 'direct',
    aria2_url: s.aria2_url || '',
    aria2_token: '',
    aria2_dir: s.aria2_dir || '',
    aria2_local_dir: s.aria2_local_dir || '',
    qbit_url: s.qbit_url || '',
    qbit_user: s.qbit_user || '',
    qbit_password: '',
    qbit_save_path: s.qbit_save_path || '',
    qbit_local_dir: s.qbit_local_dir || '',
    transmission_url: s.transmission_url || '',
    transmission_user: s.transmission_user || '',
    transmission_password: '',
    transmission_download_dir: s.transmission_download_dir || '',
    transmission_local_dir: s.transmission_local_dir || '',
    proxy_url: s.proxy_url || '',
    proxy_clear_credentials: false,
    poll_minutes: s.poll_minutes || 30,
    library_dir: s.library_dir || '',
    auto_cleanup_empty_anime: s.auto_cleanup_empty_anime === true,
    keep_source_files: s.keep_source_files !== false,
  }
}

function sectionBody(id: SectionId, form: SettingsForm): SettingsInput {
  let body: SettingsInput = {}
  if (id === 'download') {
    body = {
      download_mode: form.download_mode,
      aria2_url: form.aria2_url.trim(),
      aria2_dir: form.aria2_dir.trim(),
      aria2_local_dir: form.aria2_local_dir.trim(),
      qbit_url: form.qbit_url.trim(),
      qbit_user: form.qbit_user.trim(),
      qbit_save_path: form.qbit_save_path.trim(),
      qbit_local_dir: form.qbit_local_dir.trim(),
      transmission_url: form.transmission_url.trim(),
      transmission_user: form.transmission_user.trim(),
      transmission_download_dir: form.transmission_download_dir.trim(),
      transmission_local_dir: form.transmission_local_dir.trim(),
    }
  } else if (id === 'library') {
    body = { library_dir: form.library_dir.trim(), keep_source_files: form.keep_source_files, auto_cleanup_empty_anime: form.auto_cleanup_empty_anime }
  } else if (id === 'network') {
    body = { proxy_url: form.proxy_url.trim(), proxy_clear_credentials: form.proxy_clear_credentials, poll_minutes: form.poll_minutes }
  } else if (id === 'account') {
    body = { web_user: form.web_user.trim() }
  }
  for (const field of SECRET_FIELDS) {
    const value = form[field]
    if (sectionFields(id).includes(field) && typeof value === 'string' && value) Object.assign(body, { [field]: value })
  }
  return body
}

interface SaveResult { ok: boolean; reauth_required: boolean; cleaned_anime: number; cleanup_warning: string; library_warning: string }
interface FieldError { section: SectionId; field: SettingsField; id: string; message: string }

export function useSettingsForm() {
  const store = useAppStore()
  const form = reactive(emptyForm())
  const saved = ref<SettingsForm | null>(null)
  const info = ref<Settings | null>(null)
  const loading = ref(true)
  const loadErr = ref('')
  const saving = ref(false)
  const saveError = ref('')
  const active = ref<SectionId>('download')
  const validationAttempted = ref(false)
  /* 确认框打开期间不接受保存，避免与切换/撤销交错。 */
  const confirming = ref(false)
  const ready = computed(() => !loading.value && !loadErr.value && !!saved.value)

  function isDirty(id: SectionId) {
    const snapshot = saved.value
    return !!snapshot && sectionFields(id).some(field => form[field] !== snapshot[field])
  }
  const changedSections = computed(() => SECTIONS.filter(section => isDirty(section.id)).map(section => section.id))
  const dirty = computed(() => changedSections.value.length > 0)
  const activeDirty = computed(() => isDirty(active.value))

  const validationErrors = computed(() => {
    const errors: FieldError[] = []
    const os = info.value?.os
    if (!Number.isInteger(form.poll_minutes) || form.poll_minutes < 5 || form.poll_minutes > 1440) {
      errors.push({ section: 'network', field: 'poll_minutes', id: 'st-poll', message: '请输入 5–1440 之间的整数' })
    }
    if (form.library_dir.trim() && !isAbsoluteStoragePath(form.library_dir.trim(), os)) {
      errors.push({ section: 'library', field: 'library_dir', id: 'st-library', message: '请填写绝对路径' })
    }
    if ((form.web_password || info.value?.web_user) && !form.web_user.trim()) {
      errors.push({ section: 'account', field: 'web_user', id: 'st-user', message: '请填写用户名' })
    }
    if (form.web_user.trim() !== (info.value?.web_user || '') && !form.web_password) {
      errors.push({ section: 'account', field: 'web_password', id: 'st-pass', message: '修改用户名时必须同时填写新密码' })
    } else if (form.web_password && [...form.web_password].length < 6) {
      errors.push({ section: 'account', field: 'web_password', id: 'st-pass', message: '密码至少 6 位' })
    }
    return errors
  })
  function fieldError(field: SettingsField) {
    if (!validationAttempted.value) return undefined
    return validationErrors.value.find(error => error.field === field && error.section === active.value)?.message
  }
  watch(form, () => { saveError.value = '' }, { flush: 'sync' })

  /* ---- 目录检测 ---- */
  const storageLoading = ref(false)
  const storageResult = ref<SetupStorageResult | null>(null)
  const storageError = ref('')
  let storageController: AbortController | null = null
  let storageVersion = 0
  function clearStorageCheck() {
    storageVersion++
    storageController?.abort()
    storageLoading.value = false
    storageResult.value = null
    storageError.value = ''
  }
  watch(() => form.library_dir, clearStorageCheck, { flush: 'sync' })
  onBeforeUnmount(clearStorageCheck)
  async function checkStorage() {
    if (!ready.value || saving.value || storageLoading.value) return
    const path = form.library_dir.trim()
    const version = ++storageVersion
    storageController?.abort()
    const controller = new AbortController()
    storageController = controller
    storageLoading.value = true
    storageError.value = ''
    storageResult.value = null
    try {
      const body: StorageCheckInput = { path }
      const result = await api<SetupStorageResult>('/api/settings/storage', { method: 'POST', json: body, signal: controller.signal })
      if (version !== storageVersion || path !== form.library_dir.trim()) return
      if (result.path !== (path || info.value?.download_dir) || typeof result.writable !== 'boolean' || typeof result.message !== 'string' || !['mapped', 'unmapped', 'unknown', 'not_required'].includes(result.mount_status)) throw new Error('目录检测结果不完整')
      storageResult.value = result
    } catch (e) {
      if (version === storageVersion) storageError.value = errorMessage(e)
    } finally {
      if (version === storageVersion) storageLoading.value = false
    }
  }

  /* ---- 读取 / 撤销 ---- */
  async function load() {
    loading.value = true
    loadErr.value = ''
    try {
      const s = await http.get<Settings>('/api/settings')
      info.value = s
      const next = formFromSettings(s)
      // 回读只刷新已保存的值，其他分类尚未保存的输入保留。
      const previous = saved.value
      const drafts = previous ? (Object.keys(next) as SettingsField[]).filter(field => form[field] !== previous[field]) : []
      Object.assign(form, next, Object.fromEntries(drafts.map(field => [field, form[field]])))
      saved.value = next
      validationAttempted.value = false
    } catch (e) {
      loadErr.value = errorMessage(e)
    } finally {
      loading.value = false
    }
  }

  function restore(id: SectionId) {
    const snapshot = saved.value
    if (!snapshot) return
    for (const field of sectionFields(id)) Object.assign(form, { [field]: snapshot[field] })
    if (id === 'library') clearStorageCheck()
    validationAttempted.value = false
    saveError.value = ''
  }

  async function confirm(title: string, text: string, okText: string) {
    confirming.value = true
    try {
      return await confirmDialog(title, text, { okText, cancelText: '继续编辑' })
    } finally {
      confirming.value = false
    }
  }

  async function reset(id: SectionId = active.value) {
    if (!ready.value || saving.value || confirming.value || !isDirty(id)) return
    if (!await confirm(`撤销「${sectionTitle(id)}」的更改？`, '将恢复为已保存的设置。', '撤销更改')) return
    if (saving.value) return
    restore(id)
  }

  /* 切换分类：当前分类有未保存更改时确认放弃，保证同一时刻只有一个分类处于编辑中。 */
  async function selectSection(id: SectionId) {
    if (id === active.value) return true
    if (saving.value || confirming.value) return false
    const current = active.value
    if (isDirty(current)) {
      if (!await confirm(`放弃「${sectionTitle(current)}」的更改？`, '切换分类后，本分类未保存的修改不会保留。', '放弃更改')) return false
      if (saving.value || active.value !== current) return false
      restore(current)
    }
    active.value = id
    validationAttempted.value = false
    saveError.value = ''
    return true
  }

  async function canLeave() {
    if (saving.value) return false
    if (!dirty.value) return true
    return confirm('放弃未保存的更改？', '离开后，本次设置修改不会保存。', '放弃更改')
  }

  /* ---- 保存 ---- */
  async function save(id: SectionId = active.value) {
    if (!ready.value || saving.value || confirming.value || !isDirty(id)) return
    saveError.value = ''
    validationAttempted.value = true
    const invalid = validationErrors.value.find(error => error.section === id)
    if (invalid) {
      await nextTick()
      document.getElementById(invalid.id)?.focus()
      return
    }
    const fields = sectionFields(id)
    const body = sectionBody(id, form)
    saving.value = true
    try {
      const r = await http.put<SaveResult>('/api/settings', body)
      // 保存已完成：即使随后读取失败也不能重复提交密码或清理操作。
      for (const field of fields) {
        if (SECRET_FIELDS.includes(field)) Object.assign(form, { [field]: '' })
      }
      if (fields.includes('proxy_clear_credentials')) form.proxy_clear_credentials = false
      if (saved.value) {
        const snapshot = { ...saved.value }
        for (const field of fields) Object.assign(snapshot, { [field]: form[field] })
        saved.value = snapshot
      }
      validationAttempted.value = false
      if (r.reauth_required) {
        toast.info('密码已更新，请重新登录')
        location.href = '/login'
        return
      }
      if (r.library_warning) toast.error('设置已保存，但媒体库目录需要确认：' + r.library_warning)
      else if (r.cleanup_warning) toast.error(r.cleanup_warning)
      else toast.success(r.cleaned_anime > 0 ? `${sectionTitle(id)}已保存，已清理 ${r.cleaned_anime} 部空番剧` : `${sectionTitle(id)}已保存并立即生效`)
      await Promise.all([load(), store.refresh()])
    } catch (e) {
      saveError.value = errorMessage(e)
    } finally {
      saving.value = false
    }
  }

  /* Ctrl/⌘+S 保存当前分类 */
  function onKeydown(event: KeyboardEvent) {
    if (!(event.ctrlKey || event.metaKey) || event.altKey || event.shiftKey || event.key.toLowerCase() !== 's') return
    if (!isEditableSection(active.value)) return
    event.preventDefault()
    void save()
  }

  return {
    form, info, loading, loadErr, saving, saveError, active, ready, confirming,
    changedSections, dirty, activeDirty, isDirty, validationErrors, fieldError,
    storageLoading, storageResult, storageError, checkStorage,
    load, reset, save, selectSection, canLeave, onKeydown,
  }
}

export type SettingsContext = ReturnType<typeof useSettingsForm>
const SETTINGS_KEY: InjectionKey<SettingsContext> = Symbol('settings')

export function provideSettings() {
  const context = useSettingsForm()
  provide(SETTINGS_KEY, context)
  return context
}

export function useSettings() {
  const context = inject(SETTINGS_KEY)
  if (!context) throw new Error('useSettings 必须在设置页内使用')
  return context
}
