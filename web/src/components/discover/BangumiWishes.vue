<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { http, errorMessage } from '@/api/client'
import BaseButton from '@/components/ui/BaseButton.vue'
import { confirmDialog } from '@/stores/confirm'
import type { BangumiCollectionState } from '@/api/types'

const state = ref<BangumiCollectionState | null>(null)
const token = ref('')
const busy = ref(false)
const error = ref('')

async function load() {
  try { state.value = await http.get<BangumiCollectionState>('/api/bangumi/wishes') }
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
  if (!await confirmDialog('解除 Bangumi 连接', '清除本地令牌和收藏缓存？不会修改 Bangumi 收藏、已有订阅或媒体。', { danger: true, okText: '解除连接' })) return
  busy.value = true
  error.value = ''
  try { await http.delete('/api/bangumi/wishes'); await load() }
  catch (e) { error.value = errorMessage(e) }
  finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <section class="bangumi card" aria-label="Bangumi 同步">
    <h2>Bangumi 同步</h2>
    <p v-if="!state && !error">正在读取…</p>
    <p>连接账号后，Bangumi 的动画收藏（想看、在看、看过、搁置、抛弃）会直接显示在番剧库中。仅从 Bangumi 读取，不会回写状态，也不会自动订阅、停订或删除媒体。列表每 30 分钟更新一次，也可以手动同步。</p>
    <p>令牌保存在本机数据库中，不包含在 JSON 备份里，请妥善保管数据库。<a href="https://bgm.tv/dev/app" target="_blank" rel="noopener noreferrer">获取个人令牌</a></p>
    <label for="bangumi-token">个人 Access Token{{ state?.username ? '（输入新令牌可更换账户）' : '' }}</label>
    <input id="bangumi-token" v-model="token" type="password" autocomplete="off" spellcheck="false" />
    <div class="actions">
      <BaseButton :loading="busy" :disabled="!token.trim()" @click="connect">连接并同步</BaseButton>
      <BaseButton v-if="state?.username" :loading="busy" @click="sync">立即同步</BaseButton>
      <BaseButton v-if="state?.username" :disabled="busy" @click="disconnect">解除连接</BaseButton>
    </div>
    <template v-if="state?.username">
      <p>已连接：{{ state.username }}<span v-if="state.last_sync"> · 上次同步：{{ new Date(state.last_sync).toLocaleString() }}</span></p>
      <p v-if="state.error" class="error" role="alert">上次同步失败：{{ state.error }}；保留上次成功的列表。</p>
      <p v-if="!state.last_sync">尚未完成同步。</p>
      <p v-else-if="!state.items.length">没有收藏的动画。</p>
      <p v-else>已同步 {{ state.items.length }} 部收藏的动画，可在番剧库查看并找资源。</p>
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
</style>
