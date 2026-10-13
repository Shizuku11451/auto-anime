"""Normalize official fnpack output without extracting untrusted archive paths."""

import argparse
import hashlib
import io
from pathlib import Path, PurePosixPath
import re
import tarfile


def members(archive):
    seen = set()
    result = archive.getmembers()
    for member in result:
        path = PurePosixPath(member.name)
        if path.is_absolute() or '..' in path.parts or '\\' in member.name:
            raise ValueError('Unsafe FPK path: ' + member.name)
        if member.name in seen or not (member.isfile() or member.isdir()):
            raise ValueError('Duplicate or unsupported FPK entry: ' + member.name)
        seen.add(member.name)
    return result


def normalized(source, entries, replacements=None, executable_cmd=False):
    replacements = replacements or {}
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode='w:gz') as target:
        for member in entries:
            member.uid = member.gid = 0
            member.uname = member.gname = ''
            member.mode = 0o755 if member.isdir() or (executable_cmd and member.name.startswith('cmd/')) else 0o644
            data = replacements.get(member.name)
            if member.isfile() and data is None:
                data = source.extractfile(member).read()
            if data is not None:
                member.size = len(data)
            target.addfile(member, io.BytesIO(data) if data is not None else None)
    return output.getvalue()


def normalize(source_path, target_path):
    with tarfile.open(source_path, 'r:gz') as source:
        entries = members(source)
        app_data = source.extractfile('app.tgz').read()
        manifest = source.extractfile('manifest').read().decode('utf-8', errors='strict')
        if manifest.startswith('\ufeff') or '\ufffd' in manifest:
            raise ValueError('Manifest must be UTF-8 without BOM or replacement characters')
        original = re.search(r'(?m)^checksum\s*=\s*([0-9a-fA-F]{32})\s*$', manifest)
        if original is None or original.group(1).lower() != hashlib.md5(app_data).hexdigest():
            raise ValueError('Official fnpack app.tgz checksum mismatch')
        for member in entries:
            if member.isfile() and member.name.startswith(('cmd/', 'wizard/', 'config/')):
                data = source.extractfile(member).read()
                text = data.decode('utf-8', errors='strict')
                if '\r' in text or '\ufffd' in text or text.startswith('\ufeff'):
                    raise ValueError('Invalid UTF-8/LF package file: ' + member.name)
        with tarfile.open(fileobj=io.BytesIO(app_data), mode='r:gz') as app:
            app_data = normalized(app, members(app))
        manifest = manifest.replace('\r\n', '\n')
        manifest = re.sub(r'(?m)^checksum\s*=.*$', 'checksum                   = ' + hashlib.md5(app_data).hexdigest(), manifest)
        output = normalized(source, entries, {'app.tgz': app_data, 'manifest': manifest.encode('utf-8')}, executable_cmd=True)
    Path(target_path).write_bytes(output)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source')
    parser.add_argument('target')
    args = parser.parse_args()
    normalize(args.source, args.target)
