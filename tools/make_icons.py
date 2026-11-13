"""由统一原图生成飞牛 FPK 使用的 PNG 图标。

需要 Pillow：

    python tools/make_icons.py
"""

import pathlib

from PIL import Image

ROOT = pathlib.Path(__file__).resolve().parent.parent
SOURCE = ROOT / "backend" / "assets" / "app-icon.png"
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


if __name__ == "__main__":
    main()
