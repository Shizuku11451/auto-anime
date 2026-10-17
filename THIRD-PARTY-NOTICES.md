# 第三方软件与许可

Auto-Anime 自身以 MIT 许可发布。下列第三方组件随程序或 Windows 桌面包分发，各自保留原许可。

## 内置 BT 下载器

Windows 默认下载器使用 [github.com/cenkalti/rain](https://github.com/cenkalti/rain) v1.13.0（MIT）。

运行时源码以本地补丁版保存在 `backend/third_party/rain/`，通过 Go `replace` 构建，不增加新的外部库。补丁用于保留源文件的会话卸载/释放、端口复用、源文件缺失或大小变化时撤销完成状态，以及取消文件分配时释放句柄。原始许可与补丁范围见该目录的 `LICENSE` 和 `AUTO_ANIME_PATCHES.md`。

```text
The MIT License (MIT)

Copyright (c) 2013 Cenk Alti

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

rain 的间接依赖及其许可见 `go.mod`/`go.sum` 与各模块源码仓库；发布源码中保留原始许可文件。

## Windows 桌面包可选的 qBittorrent 回退

`-desktop.zip` 可能包含 qBittorrent 官方安装包中提取的程序文件（GPL-2.0-or-later）。对应版本的源码归档与 `COPYING` 随压缩包放在 `licenses/` 下，未修改上游代码，也未执行安装器或写入系统关联。单文件 `Auto-Anime.exe` 不包含 qBittorrent。

## 其他

WebView2 Runtime 由 Microsoft 单独提供。`github.com/jchv/go-webview2`（MIT）与 `github.com/jchv/go-winloader`（ISC）的许可文件随桌面包放在 `licenses/` 下。
