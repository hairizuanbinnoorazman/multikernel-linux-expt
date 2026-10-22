#!/usr/bin/env python3
"""Verify the offline production-profile proof image as bounded archive data."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import tarfile


def digest(data):
    return hashlib.sha256(data).hexdigest()


def verify(data, busybox_sha256):
    assert len(data) <= 16 << 20, 'saved archive exceeds qualification limit'
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:') as archive:
        entries = archive.getmembers()
        assert len(entries) <= 1024
        assert len({e.name for e in entries}) == len(entries), 'duplicate archive names'

        def read(name):
            entry = archive.getmember(name)
            assert entry.isfile() and 0 <= entry.size <= 8 << 20
            return archive.extractfile(entry).read()

        manifests = json.loads(read('manifest.json'))
        assert len(manifests) == 1
        manifest = manifests[0]
        assert manifest['RepoTags'] == ['g8-proof:latest']
        configuration = read(manifest['Config'])
        config = json.loads(configuration)
        assert config['os'] == 'linux' and config['architecture'] == 'amd64'
        assert config['config']['Cmd'] == ['/bin/busybox', 'cat', '/result']
        layers = manifest['Layers']
        diff_ids = config['rootfs']['diff_ids']
        assert len(layers) == len(diff_ids) and 1 <= len(layers) <= 8
        files = {}
        for name, diff_id in zip(layers, diff_ids):
            raw = read(name)
            if raw[:2] == b'\x1f\x8b':
                with gzip.GzipFile(fileobj=io.BytesIO(raw)) as compressed:
                    raw = compressed.read((8 << 20) + 1)
            assert len(raw) <= 8 << 20
            assert diff_id == 'sha256:' + digest(raw), 'layer digest mismatch'
            with tarfile.open(fileobj=io.BytesIO(raw), mode='r:') as layer:
                members = layer.getmembers()
                assert len(members) <= 128
                for entry in members:
                    path = entry.name.removeprefix('./').rstrip('/')
                    assert path in {'bin', 'bin/busybox', 'result'}, path
                    if entry.isdir():
                        assert path == 'bin'
                        continue
                    assert entry.isfile() and path not in files
                    assert 0 <= entry.size <= 4 << 20
                    files[path] = (entry.mode, layer.extractfile(entry).read())
        assert set(files) == {'bin/busybox', 'result'}
        assert files['result'] == (0o644, b'multikernel-production-docker-build\n')
        mode, busybox = files['bin/busybox']
        assert mode == 0o755 and busybox.startswith(b'\x7fELF')
        assert digest(busybox) == busybox_sha256, 'copied build executable differs'
        return {'saved_image_sha256': digest(data),
                'image_id': 'sha256:' + digest(configuration),
                'layer_count': len(layers), 'layer_diff_ids_verified': True,
                'result_content_verified': True, 'static_input_verified': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('archive', type=Path)
    parser.add_argument('busybox_sha256')
    args = parser.parse_args()
    assert args.archive.stat().st_size <= 16 << 20
    print(json.dumps(verify(args.archive.read_bytes(), args.busybox_sha256), indent=2))
