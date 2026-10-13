from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]


class ReleaseTests(unittest.TestCase):
    def test_release_waits_for_public_image_gate(self):
        source = (ROOT / '.github/workflows/release.yml').read_text(encoding='utf-8')
        release = source.split('\n  release:\n', 1)[1].split('\n  image:\n', 1)[0]
        image = source.split('\n  image:\n', 1)[1]
        self.assertIn('needs: [binaries, fpk, image]', release)
        self.assertIn('needs: [binaries, fpk]', image)
        self.assertNotIn('needs: release', image)
        self.assertLess(image.index('verify-public-image.sh'), image.index('Promote verified image to latest'))
        self.assertNotIn('--tag "$IMAGE:latest"', image)

    def test_official_package_uses_version_image_and_normalization(self):
        compose = (ROOT / 'deploy/fnos/fpk/app/docker/docker-compose.yaml').read_text(encoding='utf-8')
        self.assertIn('pull_policy: missing', compose)
        self.assertNotIn('pull_policy: never', compose)
        workflow = (ROOT / '.github/workflows/release.yml').read_text(encoding='utf-8')
        self.assertIn('export IMAGE_REPOSITORY="ghcr.io/${GITHUB_REPOSITORY,,}"', workflow)
        for file in ['deploy/fnos/build-fpk.ps1', 'deploy/fnos/build-fpk.sh']:
            source = (ROOT / file).read_text(encoding='utf-8')
            self.assertIn('normalize_fpk.py', source)
            self.assertIn('image_tag' if file.endswith('.sh') else '$ImageTag', source)
            self.assertIn('${IMAGE_REPOSITORY:-' if file.endswith('.sh') else '[string] $ImageRepository', source)

    def test_lifecycle_waits_and_reports_failure(self):
        for name in ['main', 'config_callback']:
            source = (ROOT / 'deploy/fnos/fpk/cmd' / name).read_text(encoding='utf-8')
            self.assertIn('run_compose up -d --remove-orphans --wait --wait-timeout 120', source)
        storage = (ROOT / 'deploy/fnos/fpk/cmd/storage').read_text(encoding='utf-8')
        self.assertIn('mount_error "$reason', storage)
        self.assertIn('return "$status"', storage)
        for name in ['install_callback', 'upgrade_callback']:
            source = (ROOT / 'deploy/fnos/fpk/cmd' / name).read_text(encoding='utf-8')
            self.assertIn('"$script_dir/main" start', source)

    def test_image_gate_uses_fresh_anonymous_credentials(self):
        script = (ROOT / 'scripts/verify-public-image.sh').read_text(encoding='utf-8')
        self.assertIn('export DOCKER_CONFIG="$tmp/docker"', script)
        self.assertIn('"auths":{}', script)
        self.assertIn('for arch in amd64 arm64', script)
        self.assertIn('--network none', script)
        self.assertIn('--user "$uid:$gid" --cap-drop ALL', script)
        self.assertIn('[ "$uid" -ne 0 ]', script)
        self.assertIn("pgrep -f '[q]bittorrent-nox'", script)
        self.assertNotIn('docker login', script)


if __name__ == '__main__':
    unittest.main()
