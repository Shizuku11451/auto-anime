/* 打开中的弹窗栈（模块级单例）：Esc 只关闭最上层；有弹窗打开时锁定页面滚动。 */
const stack: symbol[] = []

export function pushDialog(id: symbol) {
  if (!stack.includes(id)) stack.push(id)
  sync()
}

export function popDialog(id: symbol) {
  const i = stack.indexOf(id)
  if (i >= 0) stack.splice(i, 1)
  sync()
}

export function isTopDialog(id: symbol): boolean {
  return stack[stack.length - 1] === id
}

function sync() {
  document.body.style.overflow = stack.length ? 'hidden' : ''
}
