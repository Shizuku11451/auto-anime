function generateCompose(options) {
  const custom = options.custom;
  const path = options.path.trim();
  if (custom && (!path.startsWith('/') || path === '/' || path.length > 4096 || /[\x00-\x1f\x7f\\]/.test(path) || path.split('/').some(part => part === '.' || part === '..'))) {
    throw new Error('请填写 NAS 上的绝对目录，例如 /vol1/1000/ani；不能使用根目录、相对路径或 ..。');
  }
  const port = Number(options.port);
  const bt = Number(options.bt);
  if (![port, bt].every(n => Number.isInteger(n) && n >= 1 && n <= 65535) || port === bt) {
    throw new Error('端口须为 1 到 65535 的整数，网页与 BT 端口不能相同。');
  }
  // Compose performs environment interpolation even inside quoted YAML.
  const source = JSON.stringify((custom ? path : './media').replace(/\$/g, '$$$$'));
  return `# 飞牛 Docker → Compose → 创建项目：粘贴全部内容。
# data 保存配置和做种源文件；/media 保存整理后的番剧。
services:
  auto-anime:
    image: ghcr.io/shizuku11451/auto-anime:latest
    restart: unless-stopped
    stop_grace_period: 45s
    ports:
      - "${port}:8000"
      - "${bt}:${bt}/tcp"
      - "${bt}:${bt}/udp"
    environment:
      DB_PATH: /data/auto-anime.db
      DOWNLOAD_DIR: /data/downloads
      LIBRARY_DIR: /media
      DOWNLOAD_MODE: builtin
      QBITTORRENT_DATA_DIR: /data/qbittorrent
      QBITTORRENT_BT_PORT: "${bt}"
      KEEP_SOURCE_FILES: "${options.keepSource !== false}"
    volumes:
      - ./data:/data
      - type: bind
        source: ${source}
        target: /media
`;
}

if (typeof document !== 'undefined') {
  const form = document.querySelector('form');
  const output = document.querySelector('#output');
  const message = document.querySelector('#message');
  const copy = document.querySelector('#copy');
  const download = document.querySelector('#download');
  let current = '';
  function render() {
    const custom = document.querySelector('#custom').checked;
    document.querySelector('#path-field').hidden = !custom;
    message.textContent = '';
    try {
      current = generateCompose({ custom, path: form.elements.path.value, port: form.elements.port.value, bt: form.elements.bt.value, keepSource: form.elements.keepSource.checked });
      output.value = current;
      copy.disabled = download.disabled = false;
      document.querySelector('#destination').textContent = custom ? form.elements.path.value.trim() : '项目目录下的 media 文件夹';
    } catch (error) {
      current = '';
      output.value = '';
      copy.disabled = download.disabled = true;
      message.textContent = error.message;
    }
  }
  form.addEventListener('input', render);
  form.addEventListener('submit', event => event.preventDefault());
  copy.addEventListener('click', async () => {
    const text = current;
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      if (current === text) message.textContent = '已复制，前往飞牛创建 Compose 项目并粘贴。';
    } catch {
      if (current !== text) return;
      output.focus(); output.select();
      message.textContent = '浏览器未允许自动复制，配置已选中，请按 Ctrl+C（Mac 使用 Command+C）。';
    }
  });
  download.addEventListener('click', () => {
    if (!current) return;
    const url = URL.createObjectURL(new Blob([current], { type: 'text/yaml;charset=utf-8' }));
    const link = document.createElement('a');
    link.href = url; link.download = 'docker-compose.yml'; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  });
  document.querySelector('#theme').addEventListener('click', () => {
    document.documentElement.dataset.theme = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
  });
  render();
}
