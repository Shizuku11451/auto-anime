import hashlib
import io
from pathlib import Path
import tarfile
import tempfile
import unittest

from normalize_fpk import normalize


def archive(files):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode='w:gz') as tar:
        for name, data in files.items():
            member = tarfile.TarInfo(name)
            member.size = len(data)
            member.mode = 0o666
            tar.addfile(member, io.BytesIO(data))
    return output.getvalue()


class PackageTests(unittest.TestCase):
    def test_permissions_and_checksum(self):
        app = archive({'docker/docker-compose.yaml': b'image: example:1\n'})
        manifest = ('appname = auto-anime\r\nchecksum = ' + hashlib.md5(app).hexdigest() + '\r\n').encode()
        raw = archive({'manifest': manifest, 'app.tgz': app, 'cmd/main': b'#!/bin/bash\nexit 0\n'})
        with tempfile.TemporaryDirectory() as tmp:
            source, target = Path(tmp) / 'raw.fpk', Path(tmp) / 'normalized.fpk'
            source.write_bytes(raw)
            normalize(source, target)
            with tarfile.open(target) as package:
                self.assertEqual(package.getmember('cmd/main').mode, 0o755)
                self.assertEqual(package.getmember('manifest').mode, 0o644)
                manifest = package.extractfile('manifest').read()
                app = package.extractfile('app.tgz').read()
                self.assertNotIn(b'\r', manifest)
                self.assertIn(hashlib.md5(app).hexdigest().encode(), manifest)
                with tarfile.open(fileobj=io.BytesIO(app)) as contents:
                    self.assertEqual(contents.getmember('docker/docker-compose.yaml').mode, 0o644)
                    self.assertEqual(contents.extractfile('docker/docker-compose.yaml').read(), b'image: example:1\n')

    def test_bad_checksum_or_path_does_not_write_target(self):
        for unsafe in [False, True]:
            app = archive({'../escape' if unsafe else 'ui/config': b'{}\n'})
            digest = hashlib.md5(app).hexdigest() if unsafe else '0' * 32
            raw = archive({'manifest': ('checksum = ' + digest + '\n').encode(), 'app.tgz': app})
            with tempfile.TemporaryDirectory() as tmp:
                source, target = Path(tmp) / 'raw.fpk', Path(tmp) / 'result.fpk'
                source.write_bytes(raw)
                with self.assertRaises(ValueError):
                    normalize(source, target)
                self.assertFalse(target.exists())


if __name__ == '__main__':
    unittest.main()
