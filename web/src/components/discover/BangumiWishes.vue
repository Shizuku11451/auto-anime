<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { http, errorMessage } from '@/api/client'
import BaseButton from '@/components/ui/BaseButton.vue'
import { confirmDialog } from '@/stores/confirm'

interface Wish { id: number; name: string; name_cn: string }
interface State { username: string; last_sync: string; error: string; items: Wish[] }
const props = defineProps<{ settings?: boolean }>()
const emit = defineEmits<{ search: [keyword: string] }>()
const state = ref<State | null>(null)
const token = ref('')
const busy = ref(false)
const error = ref('')

async function load() {
  try { state.value = await http.get<State>('/api/bangumi/wishes') }
  catch (e) { error.value = errorMessage(e) }
}
async function sync() {
  busy.value = true
  error.value = ''
  try { await http.post('/api/bangumi/wishes/sync') }
  catch (e) { error.value = errorMessage(e) }
  finally { await load(); busy.value = false }
}
async function connect() {
  if (!token.value.trim()) return
  busy.value = true
  error.value = ''
  try {
    await http.post('/api/bangumi/wishes', { token: token.value.trim() })
    token.value = ''
    await sync()
  } catch (e) { error.value = errorMessage(e) }
  finally { busy.value = false; await load() }
}
async function disconnect() {
  if (!await confirmDialog('解除 Bangumi 连接', '清除本地令牌和想看列表？不会修改 Bangumi 收藏或已有订阅。', { danger: true, okText: '解除连接' })) return
  busy.value = true
  error.value = ''
  try { await http.delete('/api/bangumi/wishes'); await load() }
  catch (e) { error.value = errorMessage(e) }
  finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <section class="bangumi card" :aria-label="props.settings ? 'Bangumi 同步' : 'Bangumi 想看'">
    <h2>{{ props.settings ? 'Bangumi 同步' : 'Bangumi 想看' }}</h2>
    <p v-if="!state && !error">正在读取…</p>
    <template v-if="props.settings">
      <p>连接账号后，可在「番剧库」查看 Bangumi 的动画想看列表。列表每 30 分钟更新一次，也可以手动同步。</p>
      <p>令牌保存在本机数据库中，不包含在 JSON 备份里，请妥善保管数据库。<a href="https://bgm.tv/dev/app" target="_blank" rel="noopener noreferrer">获取个人令牌</a></p>
      <label for="bangumi-token">个人 Access Token{{ state?.username ? '（输入新令牌可更换账户）' : '' }}</label>
      <input id="bangumi-token" v-model="token" type="password" autocomplete="off" spellcheck="false" />
      <div class="actions">
        <BaseButton :loading="busy" :disabled="!token.trim()" @click="connect">连接并同步</BaseButton>
        <BaseButton v-if="state?.username" :disabled="busy" @click="disconnect">解除连接</BaseButton>
      </div>
    </template>
    <template v-else-if="state && !state.username">
      <p>连接 Bangumi 后可在这里查看想看的动画。</p>
      <RouterLink to="/settings">前往设置，在「网络与同步」中连接账户</RouterLink>
    </template>
    <template v-if="state?.username">
      <p>已连接：{{ state.username }}<span v-if="state.last_sync"> · 上次同步：{{ new Date(state.last_sync).toLocaleString() }}</span></p>
      <p v-if="state.error" class="error" role="alert">上次同步失败：{{ state.error }}；保留上次成功的列表。</p>
      <template v-if="!props.settings">
        <p>想看列表与本地番剧分开显示，不会自动创建订阅。</p>
        <div class="actions"><BaseButton size="sm" :loading="busy" @click="sync">立即同步</BaseButton></div>
        <p v-if="!state.last_sync">尚未完成同步。</p>
        <p v-else-if="!state.items.length">没有想看的动画。</p>
        <ul v-else class="wishes">
          <li v-for="item in state.items" :key="item.id">
            <div><strong>{{ item.name_cn || item.name }}</strong><p v-if="item.name_cn && item.name !== item.name_cn">{{ item.name }}</p></div>
            <BaseButton size="sm" @click="emit('search', item.name_cn || item.name)">找资源</BaseButton>
          </li>
        </ul>
      </template>
    </template>
    <p v-if="error" class="error" role="alert">{{ error }} <BaseButton v-if="!state" size="sm" @click="load">重试</BaseButton></p>
  </section>
</template>

<style scoped>
.bangumi { padding: 20px; margin-bottom: 16px; }
h2 { font-size: 16px; margin-bottom: 8px; }
p { color: var(--text-2); font-size: 13px; margin: 6px 0; overflow-wrap: anywhere; }
label { display: block; margin: 12px 0 6px; }
input { width: 100%; }
.actions { display: flex; gap: 8px; margin: 10px 0; flex-wrap: wrap; }
.error { color: var(--red); }
.wishes { list-style: none; padding: 0; margin: 10px 0 0; max-height: 440px; overflow-y: auto; }
.wishes li { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 0; border-top: 1px solid var(--line); }
.wishes li div { min-width: 0; overflow-wrap: anywhere; }
.wishes li button { flex: none; }
</style>
