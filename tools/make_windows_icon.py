"""由统一原图生成飞牛 PNG 和 Windows 多尺寸 ico。

缺少对应尺寸时 Windows 会自行缩放，界面与任务栏图标会发虚。需要 Pillow：

    python tools/make_windows_icon.py

生成后如需更新可执行文件内嵌资源，在 backend/ 执行 `go generate ./...`（需要联网下载
go-winres，见 docs/WINDOWS.md）。
"""

import pathlib

from PIL import Image

ROOT = pathlib.Path(__file__).resolve().parent.parent
SOURCE = ROOT / "backend" / "assets" / "app-icon.png"
TARGET = ROOT / "backend" / "assets" / "auto-anime.ico"
SIZES = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
FPK = ROOT / "deploy" / "fnos" / "fpk"
PNG_TARGETS = {
    FPK / "ICON.PNG": 64,
    FPK / "ICON_256.PNG": 256,
    FPK / "app" / "ui" / "images" / "icon_64.png": 64,
    FPK / "app" / "ui" / "images" / "icon_256.png": 256,
}


def main() -> None:
    with Image.open(SOURCE) as source:
        image = source.convert("RGBA")
    if image.width != image.height or image.width < 256:
        raise ValueError("App icon source must be square and at least 256 pixels")
    for path, size in PNG_TARGETS.items():
        path.parent.mkdir(parents=True, exist_ok=True)
        image.resize((size, size), Image.Resampling.LANCZOS).save(path, format="PNG")
        print(f"{path.relative_to(ROOT)}: {size}x{size}")
    TARGET.parent.mkdir(parents=True, exist_ok=True)
    image.save(TARGET, format="ICO", sizes=SIZES)
    written = sorted(Image.open(TARGET).info.get("sizes", []))
    listed = ", ".join(f"{width}x{height}" for width, height in written)
    print(f"{TARGET.relative_to(ROOT)}: {listed}")


if __name__ == "__main__":
    main()
