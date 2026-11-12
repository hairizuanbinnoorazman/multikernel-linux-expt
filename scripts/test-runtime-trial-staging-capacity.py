#!/usr/bin/env python3
"""Real archives and retained-floor boundary controls; never extract or launch."""
import copy
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import stat
import tarfile
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import runtime_trial_staging_capacity as subject

REV = 'a'*40
sha = lambda data: hashlib.sha256(data).hexdigest()


class StagingCapacity(unittest.TestCase):
    def setUp(self):
        self.umask = os.umask(0o077)

    def tearDown(self):
        os.umask(self.umask)

    def member(self, name, data=b'code', directory=False):
        info = tarfile.TarInfo(name)
        info.type = tarfile.DIRTYPE if directory else tarfile.REGTYPE
        info.mode = 0o755 if directory else 0o644
        info.size = 0 if directory else len(data)
        return info, None if directory else data

    def packet(self, root, variant='valid'):
        entries = [self.member('runtime', directory=True), self.member('runtime/code.go')]
        if variant.startswith('source-'):
            node = entries[-1][0]
            if variant == 'source-alias': node.type = tarfile.SYMTYPE; node.linkname = '/private'
            elif variant == 'source-owner': node.uid = 1000
            elif variant == 'source-mode': node.mode = 0o664
            elif variant == 'source-path': node.name = 'runtime/../private'
            elif variant == 'source-duplicate': entries.append(copy.deepcopy(entries[-1]))
            elif variant == 'source-empty': entries = entries[:1]
        if variant == 'source-pax-total':
            entries = entries[:1]+[self.member('runtime/code-'+str(i)+'.go') for i in range(300)]
            for node, _ in entries[1:]:node.pax_headers={'comment':'x'*60000}
        elif variant.startswith('source-pax-'):
            node=entries[-1][0]
            if variant == 'source-pax-large':node.pax_headers={'comment':'x'*70000}
            elif variant == 'source-pax-sparse':node.pax_headers={'GNU.sparse.map':'0,'+str(node.size)}
            elif variant == 'source-pax-size':node.pax_headers={'size':str(node.size)}
            elif variant == 'source-pax-unknown':node.pax_headers={'arbitrary-extension':'synthetic'}
            elif variant == 'source-pax-supported':node.pax_headers={'path':node.name,'comment':'normal fixture','mtime':'123.25'}
        source = io.BytesIO()
        with tarfile.open(fileobj=source, mode='w', format=tarfile.PAX_FORMAT,
                          pax_headers={'comment': 'b'*40 if variant == 'source-commit' else REV}) as archive:
            for node, data in entries:
                archive.addfile(node, io.BytesIO(data) if data is not None else None)
        def prefix(duplicate=False):
            payload=b'11 mtime=0\n'+(b'11 mtime=1\n' if duplicate else b'')
            node=tarfile.TarInfo('normalized-global-metadata');node.type=tarfile.XGLTYPE;node.size=len(payload)
            return node.tobuf()+payload+bytes((-len(payload))%512)
        if variant == 'source-pax-chain':source=io.BytesIO(prefix()*9+source.getvalue())
        if variant == 'source-pax-duplicate':source=io.BytesIO(prefix(True)+source.getvalue())
        binaries = {name: ('synthetic-'+name).encode() for name in subject.BINARIES}
        manifest = dict(schema_version=1, source_commit=REV, source_archive_sha256=sha(source.getvalue()),
            binaries={name: sha(data) for name, data in binaries.items()}, built_from_committed_snapshot=True,
            installed=False, full_trial_started=False)
        if variant == 'binary-pin': manifest['binaries']['mkruntimed'] = '0'*64
        if variant == 'manifest-source': manifest['source_commit'] = 'b'*40
        if variant == 'manifest-number': manifest['schema_version'] = True
        if variant == 'installed': manifest['installed'] = True
        outer = [self.member('source.tar', source.getvalue()), self.member('build.json', json.dumps(manifest).encode()),
                 self.member('bin', directory=True), self.member('tmp', directory=True)]
        outer.extend(self.member('bin/'+name, data) for name, data in binaries.items())
        for node, _ in outer:
            node.mode = 0o700 if node.isdir() else 0o500 if node.name.startswith('bin/') else 0o600
        node = outer[0][0]
        if variant == 'outer-mode': node.mode = 0o664
        elif variant == 'outer-owner': node.gid = 1000
        elif variant == 'outer-path': node.name = '../source.tar'
        elif variant == 'outer-alias': node.type = tarfile.SYMTYPE; node.linkname = '/private'
        elif variant == 'outer-duplicate': outer.append(copy.deepcopy(outer[0]))
        elif variant == 'outer-missing': outer = outer[:-1]
        elif variant == 'outer-special': node.type = tarfile.FIFOTYPE
        if variant.startswith('outer-pax-'):
            node=outer[4][0]
            if variant == 'outer-pax-large':node.pax_headers={'comment':'x'*70000}
            elif variant == 'outer-pax-sparse':node.pax_headers={'GNU.sparse.map':'0,'+str(node.size)}
            elif variant == 'outer-pax-size':node.pax_headers={'size':str(node.size)}
            elif variant == 'outer-pax-unknown':node.pax_headers={'arbitrary-extension':'synthetic'}
            elif variant == 'outer-pax-supported':node.pax_headers={'comment':'normal fixture','mtime':'123.25'}
        out = root/'stage.tar.gz'
        with tarfile.open(out, 'w:gz') as archive:
            for node, data in outer:
                archive.addfile(node, io.BytesIO(data) if data is not None else None)
        if variant == 'outer-pax-chain':out.write_bytes(gzip.compress(prefix()*9+gzip.decompress(out.read_bytes())))
        if variant == 'outer-pax-duplicate':out.write_bytes(gzip.compress(prefix(True)+gzip.decompress(out.read_bytes())))
        out.chmod(0o600)
        return out, sha(out.read_bytes())

    def test_valid_packet_is_measured_without_extraction_or_modification(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); packet, digest = self.packet(root)
            before = packet.stat()
            plan = subject.measure(packet, digest, REV)
            self.assertEqual(packet.stat(), before)
            self.assertEqual(list(root.iterdir()), [packet])
            self.assertEqual(plan['packet_entries'], 12)
            self.assertEqual(plan['source_entries'], 2)
            self.assertGreater(plan['staging_budget_bytes'], packet.stat().st_size)
            self.assertGreater(plan['storage_required_bytes'], subject.STORAGE_FLOOR)
            self.assertIs(plan['capacity_reserved'], False)
            self.assertIs(plan['full_builder_admission_claimed'], False)
            self.assertNotIn(directory, json.dumps(plan))

    def test_extended_header_limits_and_sparse_overrides_refuse_with_recomputed_pin(self):
        variants=['source-pax-large','outer-pax-large','source-pax-total','source-pax-sparse','outer-pax-sparse',
                  'source-pax-size','outer-pax-size','source-pax-unknown','outer-pax-unknown',
                  'source-pax-chain','outer-pax-chain','source-pax-duplicate','outer-pax-duplicate']
        for variant in variants:
            with self.subTest(variant=variant),tempfile.TemporaryDirectory() as directory:
                packet,digest=self.packet(Path(directory),variant);before=packet.stat()
                with self.assertRaises(ValueError):subject.measure(packet,digest,REV)
                self.assertEqual(packet.stat(),before)

    def test_oversize_metadata_refuses_before_library_metadata_processing(self):
        original=tarfile.TarInfo._proc_pax
        def guarded_library(node,archive):
            if node.size>65536:raise AssertionError('library received oversized metadata before admission')
            return original(node,archive)
        for variant in ['source-pax-large','outer-pax-large']:
            with self.subTest(variant=variant),tempfile.TemporaryDirectory() as directory:
                packet,digest=self.packet(Path(directory),variant)
                with patch.object(tarfile.TarInfo,'_proc_pax',guarded_library),self.assertRaises(ValueError):subject.measure(packet,digest,REV)

    def test_supported_normalized_metadata_remains_measurable(self):
        for variant in ['source-pax-supported','outer-pax-supported']:
            with self.subTest(variant=variant),tempfile.TemporaryDirectory() as directory:
                packet,digest=self.packet(Path(directory),variant)
                plan=subject.measure(packet,digest,REV)
                self.assertEqual(plan['source_entries'],2)
                self.assertEqual(plan['storage_floor_bytes'],8<<30)

    def test_recomputed_packet_hash_cannot_hide_unsafe_members_or_manifest(self):
        variants = ['source-alias', 'source-owner', 'source-mode', 'source-path', 'source-duplicate', 'source-empty',
            'source-commit', 'binary-pin', 'manifest-source', 'manifest-number', 'installed', 'outer-mode',
            'outer-owner', 'outer-path', 'outer-alias', 'outer-duplicate', 'outer-missing', 'outer-special']
        for variant in variants:
            with self.subTest(variant=variant), tempfile.TemporaryDirectory() as directory:
                packet, digest = self.packet(Path(directory), variant)
                with self.assertRaises(ValueError): subject.measure(packet, digest, REV)

    def test_caller_pin_file_alias_shared_mode_and_oversized_header_refuse(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); packet, digest = self.packet(root)
            with self.assertRaises(ValueError): subject.measure(packet, '0'*64, REV)
            alias = root/'alias'; alias.symlink_to(packet)
            with self.assertRaises(OSError): subject.measure(alias, digest, REV)
            alias.unlink(); os.link(packet, alias)
            with self.assertRaises(ValueError): subject.measure(packet, digest, REV)
            alias.unlink(); packet.chmod(0o664)
            with self.assertRaises(ValueError): subject.measure(packet, digest, REV)
            packet.chmod(0o600)
            header = tarfile.TarInfo('source.tar'); header.mode = 0o600; header.size = subject.SOURCE_LIMIT+1
            raw = gzip.compress(header.tobuf()+bytes(1024))
            packet.write_bytes(raw)
            with self.assertRaises(ValueError): subject.measure(packet, sha(raw), REV)

    def test_real_held_filesystem_observation_keeps_full_floor_and_inodes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); packet, digest = self.packet(root); plan = subject.measure(packet, digest, REV)
            before = packet.stat()
            for byte_delta, inode_delta, passed in [(0, 0, True), (-1, 0, False), (0, -1, False)]:
                sample = SimpleNamespace(f_bavail=plan['storage_required_bytes']+byte_delta, f_frsize=1,
                                         f_favail=plan['staging_budget_inodes']+inode_delta)
                with patch.object(subject.os, 'fstatvfs', return_value=sample):
                    report = subject.observe(root, plan)
                self.assertIs(report['staging_capacity_available'], passed)
                self.assertIs(report['packet_verified_on_guest'], False)
                self.assertIs(report['new_child_started'], False)
                self.assertIs(report['full_builder_admission_claimed'], False)
            self.assertEqual(packet.stat(), before)
            self.assertEqual(list(root.iterdir()), [packet])
            # Enough bytes for the payload alone cannot consume the retained
            # eight-GiB floor and be admitted as a complete staging operation.
            sample = SimpleNamespace(f_bavail=subject.STORAGE_FLOOR, f_frsize=1, f_favail=100000)
            with patch.object(subject.os, 'fstatvfs', return_value=sample):
                self.assertEqual(subject.observe(root, plan)['refusal_codes'], ['STAGING_BYTES_AND_RETAINED_FLOOR'])

    def test_numeric_aliases_forged_claims_and_symlinked_storage_refuse(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); packet, digest = self.packet(root); plan = subject.measure(packet, digest, REV)
            for key, value in [('version', True), ('staging_budget_bytes', 1.0), ('staging_budget_inodes', True),
                ('storage_floor_bytes', 1), ('storage_required_bytes', 1), ('capacity_reserved', True),
                ('source_commit', 'bad'), ('packet_sha256', 'bad'), ('full_builder_admission_claimed', True)]:
                with self.subTest(key=key), self.assertRaises(ValueError):
                    subject.observe(root, dict(plan, **{key: value}))
            actual = root/'actual'; actual.mkdir(); link = root/'link'; link.symlink_to(actual, target_is_directory=True)
            with self.assertRaises(OSError): subject.observe(link, plan)
            for name in ['relative', '/tmp/../tmp', '//tmp']:
                with self.assertRaises(ValueError): subject.observe(name, plan)


if __name__ == '__main__':
    unittest.main()
