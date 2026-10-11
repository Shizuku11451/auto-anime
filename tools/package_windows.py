"""Build the Windows portable distribution; never executes the installer.

The pinned official installer hash is also published in microsoft/winget-pkgs:
manifests/q/qBittorrent/qBittorrent/5.2.4/qBittorrent.qBittorrent.installer.yaml
Requires 7z. Network is needed at packaging time only.
"""
import hashlib
import pathlib
import shutil
import subprocess
import sys
import tempfile
import urllib.request
import time

VERSION = "5.2.4"
SHA256 = "ef6efe581d80efeb0a77df281d93f93021fbdcc48e4d8f7b341dc864f53c30dd"
URL = ("https://downloads.sourceforge.net/project/qbittorrent/qbittorrent-win32/"
       f"qbittorrent-{VERSION}/qbittorrent_{VERSION}_x64_setup.exe")


def download(url, destination):
    urls = [url]
    if url == URL:
        urls.extend(url.replace("downloads.sourceforge.net", mirror + ".dl.sourceforge.net")
                    for mirror in ["phoenixnap", "netix", "pilotfiber"])
    failure = None
    for attempt, candidate in enumerate(urls):
        try:
            with urllib.request.urlopen(candidate, timeout=30) as response:
                with destination.open("wb") as output:
                    shutil.copyfileobj(response, output)
            return
        except (OSError, urllib.error.URLError) as error:
            failure = error
            if attempt + 1 < len(urls):
                time.sleep(1)
    raise RuntimeError(f"Download failed: {url}: {failure}")


def main():
    binary = pathlib.Path(sys.argv[1]).resolve(strict=True)
    seven_zip = shutil.which("7z") or shutil.which("7zz")
    if not seven_zip:
        raise SystemExit("7z is required to extract the official qBittorrent installer")
    with tempfile.TemporaryDirectory(prefix="auto-anime-package-") as temporary:
        work = pathlib.Path(temporary)
        installer = work / "qbittorrent-setup.exe"
        download(URL, installer)
        if hashlib.sha256(installer.read_bytes()).hexdigest() != SHA256:
            raise SystemExit("qBittorrent installer SHA-256 mismatch")
        extracted = work / "extracted"
        subprocess.run([seven_zip, "x", str(installer), "-o" + str(extracted), "-y"], check=True)
        candidates = list(extracted.rglob("qbittorrent.exe"))
        if len(candidates) != 1:
            raise SystemExit("Expected exactly one qbittorrent.exe in official installer")
        package = work / "Auto-Anime"
        bundle = package / "qbittorrent"
        bundle.mkdir(parents=True)
        # Preserve DLLs, Qt plugins, translations and license files alongside
        # the executable. NSIS installer-only pseudo directories are excluded.
        for child in candidates[0].parent.iterdir():
            if (child.name.startswith("$") or child.name.lower().startswith("uninst")
                    or child.suffix.lower() == ".pdb"):
                continue
            target = bundle / child.name
            if child.is_dir():
                shutil.copytree(child, target)
            else:
                shutil.copy2(child, target)
        shutil.copy2(binary, package / "Auto-Anime.exe")
        notices = package / "licenses"
        notices.mkdir()
        repo = pathlib.Path(__file__).resolve().parent.parent
        shutil.copy2(repo / "docs" / "WINDOWS.md", package / "Windows使用说明.md")
        # Ship the upstream source archive and GPL text with the redistributed
        # downloader; dependencies and build instructions remain in that source.
        download(f"https://codeload.github.com/qbittorrent/qBittorrent/tar.gz/refs/tags/release-{VERSION}",
                 notices / f"qbittorrent-{VERSION}-source.tar.gz")
        download(f"https://raw.githubusercontent.com/qbittorrent/qBittorrent/release-{VERSION}/COPYING",
                 notices / "qBittorrent-COPYING")
        (notices / "THIRD-PARTY.txt").write_text(
            f"qBittorrent {VERSION}: https://www.qbittorrent.org/ (GPL-2.0-or-later)\n"
            "Official installer and license files are preserved in qbittorrent/.\n"
            "Source and build instructions: included source tar.gz; https://github.com/qbittorrent/qBittorrent\n"
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
