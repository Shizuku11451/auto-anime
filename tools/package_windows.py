"""Build the Windows portable distribution.

Bundles the Windows executable with usage docs and the desktop wrapper's
third-party license files. Network is needed at packaging time only (license
downloads); the package no longer bundles any downloader binaries.
"""
import pathlib
import shutil
import sys
import tempfile
import urllib.request


def download(url, destination):
    with urllib.request.urlopen(url, timeout=30) as response:
        with destination.open("wb") as output:
            shutil.copyfileobj(response, output)


def main():
    binary = pathlib.Path(sys.argv[1]).resolve(strict=True)
    with tempfile.TemporaryDirectory(prefix="auto-anime-package-") as temporary:
        work = pathlib.Path(temporary)
        package = work / "Auto-Anime"
        package.mkdir(parents=True)
        shutil.copy2(binary, package / "Auto-Anime.exe")
        notices = package / "licenses"
        notices.mkdir()
        repo = pathlib.Path(__file__).resolve().parent.parent
        shutil.copy2(repo / "docs" / "WINDOWS.md", package / "Windows使用说明.md")
        shutil.copy2(repo / "THIRD-PARTY-NOTICES.md", package / "THIRD-PARTY-NOTICES.md")
        (notices / "THIRD-PARTY.txt").write_text(
            "Built-in BT downloader: github.com/cenkalti/rain (MIT); see THIRD-PARTY-NOTICES.md.\n"
            "Desktop wrapper: github.com/jchv/go-webview2 (MIT), github.com/jchv/go-winloader (ISC).\n"
            "Microsoft Edge WebView2 Runtime is provided separately by Microsoft.\n", encoding="utf-8")
        for module, revision, filename in [("go-webview2", "56598839c808a2340edee99204db479f410e9bf4", "LICENSE"),
                                           ("go-winloader", "c1995be93bd1", "LICENSE.md")]:
            download(f"https://raw.githubusercontent.com/jchv/{module}/{revision}/{filename}",
                     notices / (module + "-LICENSE"))
        archive = shutil.make_archive(str(binary.with_suffix("")) + "-desktop", "zip", work, "Auto-Anime")
        print(archive)


if __name__ == "__main__":
    main()
