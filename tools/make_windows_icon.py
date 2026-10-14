"""由飞牛应用图标生成 Windows 多尺寸 ico（标题栏 16、任务栏 32、资源管理器/Alt+Tab 48/256）。

缺少对应尺寸时 Windows 会自行缩放，界面与任务栏图标会发虚。需要 Pillow：

    python tools/make_windows_icon.py

生成后如需更新可执行文件内嵌资源，在 backend/ 执行 `go generate ./...`（需要联网下载
go-winres，见 docs/WINDOWS.md）。
"""

import pathlib

from PIL import Image

ROOT = pathlib.Path(__file__).resolve().parent.parent
SOURCE = ROOT / "deploy" / "fnos" / "fpk" / "ICON_256.PNG"
TARGET = ROOT / "backend" / "assets" / "auto-anime.ico"
SIZES = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]


def main() -> None:
    image = Image.open(SOURCE).convert("RGBA")
    TARGET.parent.mkdir(parents=True, exist_ok=True)
    image.save(TARGET, format="ICO", sizes=SIZES)
    written = sorted(Image.open(TARGET).info.get("sizes", []))
    listed = ", ".join(f"{width}x{height}" for width, height in written)
    print(f"{TARGET.relative_to(ROOT)}: {listed}")


if __name__ == "__main__":
    main()
