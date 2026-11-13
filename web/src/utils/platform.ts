/* 给用户看的完整绝对路径示例：容器或飞牛 NAS 的挂载路径。 */
export const storageDirExample = '/vol1/1000/番剧'

/* 与后端校验规则一致：接受 / 开头或盘符。 */
export function isAbsoluteStoragePath(value: string): boolean {
  return /^([/\\]|[a-zA-Z]:[\\/])/.test(value)
}
