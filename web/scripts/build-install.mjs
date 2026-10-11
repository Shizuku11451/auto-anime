import { readFile, writeFile } from 'node:fs/promises'
const read = path => readFile(new URL(path, import.meta.url), 'utf8')
const template = await read('../src/install/template.html')
const tokens = await read('../src/styles/tokens.css')
const script = await read('../src/install/generator.js')
const html = template.replace('/* DESIGN_TOKENS */', () => tokens).replace('/* INSTALL_SCRIPT */', () => script)
await writeFile(new URL('../../deploy/fnos/install.html', import.meta.url), html, 'utf8')
// 同一份页面随应用分发：容器已启动但还没填好挂载时，可直接访问 /install 重新生成配置。
await writeFile(new URL('../../backend/static/install.html', import.meta.url), html, 'utf8')
console.log('已生成离线飞牛安装配置页 deploy/fnos/install.html 与 backend/static/install.html')
