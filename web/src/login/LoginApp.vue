<script setup lang="ts">
import { ref } from 'vue'
import { LogIn } from '@lucide/vue'
import AuthShell from '@/components/auth/AuthShell.vue'
import BaseButton from '@/components/ui/BaseButton.vue'

const params = new URLSearchParams(location.search)
let next = params.get('next') || '/'
if (!next.startsWith('/') || next.startsWith('//') || next.includes('\\')) next = '/'
/* 服务端重定向时看不到 hash 路由，浏览器会把它保留在登录页地址上；登录后带回去 */
if (location.hash && !next.includes('#')) next += location.hash

const user = ref('')
const pass = ref('')
const msg = ref('')
const busy = ref(false)
const shake = ref(false)

function fail(text: string) {
  msg.value = text
  shake.value = false
  requestAnimationFrame(() => (shake.value = true))
}

async function submit() {
  const u = user.value.trim()
  if (!u || !pass.value) {
    fail('请输入用户名和密码')
    return
  }
  msg.value = ''
  busy.value = true
  try {
    const res = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: u, password: pass.value }),
    })
    if (res.ok) {
      location.replace(next)
      return
    }
    fail((await res.text()).trim() || `登录失败 HTTP ${res.status}`)
  } catch {
    fail('网络错误，请稍后重试')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthShell>
    <h1 class="title">欢迎回来</h1>
    <p class="desc">登录后管理订阅、番剧库与下载任务</p>
    <form class="form" :class="{ shake }" @submit.prevent="submit" @animationend="shake = false">
      <div class="field">
        <label for="l-user">用户名</label>
        <input id="l-user" v-model="user" type="text" autocomplete="username" autofocus />
      </div>
      <div class="field">
        <label for="l-pass">密码</label>
        <input id="l-pass" v-model="pass" type="password" autocomplete="current-password" />
      </div>
      <p class="form-msg" role="alert">{{ msg }}</p>
      <BaseButton type="submit" variant="primary" size="lg" block :icon="LogIn" :loading="busy">登 录</BaseButton>
    </form>
  </AuthShell>
</template>

<style scoped>
.title {
  font-size: 22px;
  font-weight: 750;
  letter-spacing: -0.02em;
}
.desc {
  margin: 4px 0 22px;
  color: var(--text-2);
  font-size: 13.5px;
}
.form :deep(input) {
  height: 42px;
}
.shake {
  animation: shake 420ms var(--ease-out);
}
@keyframes shake {
  10%,
  90% {
    transform: translateX(-1px);
  }
  20%,
  80% {
    transform: translateX(2px);
  }
  30%,
  50%,
  70% {
    transform: translateX(-4px);
  }
  40%,
  60% {
    transform: translateX(4px);
  }
}
</style>
