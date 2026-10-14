/* 运行平台由后端状态接口提供（/api/status、/api/settings、/api/setup/status）。
   Windows 桌面版把归档放在程序同级的 media\，没有 Docker 卷映射，界面文案也不同。 */

export function isWindows(os?: string | null): boolean {
  return os === 'windows'
}

/* 给用户看的完整绝对路径示例：Windows 是盘符或 UNC 共享，容器/Linux 是 NAS 挂载路径。 */
export function storageDirExample(os?: string | null): string {
  return isWindows(os) ? 'D:\\番剧 或 \\\\NAS\\共享\\番剧' : '/vol1/1000/番剧'
}

/* 与后端校验规则一致：Windows 要求盘符路径或 UNC 共享，其它平台接受 / 开头或盘符。 */
export function isAbsoluteStoragePath(value: string, os?: string | null): boolean {
  if (isWindows(os)) return /^([a-zA-Z]:[\\/]|[/\\]{2})/.test(value)
  return /^([/\\]|[a-zA-Z]:[\\/])/.test(value)
}

/* 更新接口的 platform 形如 "windows/amd64"。 */
export function isWindowsPlatform(platform?: string | null): boolean {
  return !!platform && platform.split('/')[0] === 'windows'
}
