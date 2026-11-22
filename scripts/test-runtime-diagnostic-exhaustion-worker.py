#!/usr/bin/env python3
"""Qualify the explicit fault-test entry and its exclusion from go build."""
import json
import os
from pathlib import Path
import stat
import subprocess as sp
import tempfile

ROOT = Path(__file__).resolve().parent.parent
RECEIPT = 'diagnostic-exhaustion.fixture.json'
TAG = 'diagnostic_exhaustion_worker'
PACKAGE = './cmd/containerd-shim-multikernel-v2'
FIELDS = {'version', 'scope', 'source_revision', 'worker_pid', 'limit_bytes',
          'reserved_bytes', 'synthetic_preload_attempts', 'synthetic_preload_admitted',
          'allowance_closed_before_production_entry', 'instrumented_test_executable'}


def main():
    environment = {**os.environ, 'CGO_ENABLED': '0', 'GOWORK': 'off', 'GOFLAGS': ''}
    with tempfile.TemporaryDirectory(prefix='mk-diagnostic-worker-') as directory:
        root = Path(directory)
        fixture, production = root/'fixture', root/'production'
        common = ['-mod=readonly', '-tags='+TAG, '-trimpath', '-buildvcs=false',
                  '-ldflags=-X github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo.Revision=fixture']
        for command, output in [(['test', '-c'], fixture), (['build'], production)]:
            sp.run(['go', *command, *common, '-o', str(output), PACKAGE],
                   cwd=ROOT/'runtime', env=environment, check=True, timeout=180,
                   stdout=sp.PIPE, stderr=sp.PIPE)

        def run(binary, arguments, cwd):
            return sp.run([str(binary), *arguments], cwd=cwd, env=environment,
                          stdout=sp.PIPE, stderr=sp.PIPE, timeout=15)

        probe = root/'probe'; probe.mkdir(mode=0o700)
        for arguments in [['--version'], ['-info']]:
            expected, actual = run(production, arguments, probe), run(fixture, arguments, probe)
            assert expected.returncode == actual.returncode == 0
            assert expected.stdout == actual.stdout and actual.stdout
            assert not (probe/RECEIPT).exists(), 'discovery consumed worker allowance'
        refused = run(production, ['--diagnostic-exhaustion-probe'], probe)
        assert refused.returncode != 0 and not (probe/RECEIPT).exists(), 'go build exposed test entry'
        controls = run(fixture, ['-test.run=^TestDiagnosticExhaustionLatchesTailAndSkipsProducerAllocations$', '-test.v'], probe)
        assert controls.returncode == 0 and b'exhausted_producer_allocations=0' in controls.stdout
        assert not (probe/RECEIPT).exists(), 'test invocation entered production shim'
        result = run(fixture, ['--diagnostic-exhaustion-probe'], probe)
        assert result.returncode == 0 and result.stdout == b''
        receipt = probe/RECEIPT
        before = receipt.read_bytes(); value = json.loads(before)
        assert set(value) == FIELDS and value['version'] == 1
        assert value['scope'] == 'diagnostic-exhaustion-test-worker'
        assert value['source_revision'] == 'fixture' and value['worker_pid'] > 0
        assert value['limit_bytes'] == 1 << 20
        assert 0 < value['reserved_bytes'] <= 1 << 20
        assert 0 < value['synthetic_preload_admitted'] <= value['synthetic_preload_attempts'] <= 10000
        assert value['allowance_closed_before_production_entry'] is True
        assert value['instrumented_test_executable'] is True
        assert stat.S_IMODE(receipt.stat().st_mode) == 0o600 and receipt.stat().st_nlink == 1
        assert len(result.stderr) <= 1 << 20
        for line in result.stderr.splitlines():
            record = json.loads(line)
            assert record['sandbox_id'] == 'qualification' and record['process_id'] == 'synthetic-preload'
            assert record['code'] == 'OUTPUT_READ_FAILED'
        again = run(fixture, ['--diagnostic-exhaustion-probe'], probe)
        assert again.returncode == 93 and receipt.read_bytes() == before, 'existing receipt replaced'
        outside = root/'sentinel'; outside.write_bytes(b'preserve synthetic sentinel')
        for kind in ['symlink', 'hardlink']:
            path = root/kind; path.mkdir(mode=0o700)
            if kind == 'symlink': (path/RECEIPT).symlink_to(outside)
            else: os.link(outside, path/RECEIPT)
            result = run(fixture, ['--diagnostic-exhaustion-probe'], path)
            assert result.returncode == 93 and outside.read_bytes() == b'preserve synthetic sentinel'
        print('TAGGED_TEST_ENTRY_PRODUCTION_EXCLUSION_DISCOVERY_EXHAUSTION_AND_NO_REPLACEMENT_PASS')
        print('instrumented_probe_only=true actual_child_execution_qualified=false')


if __name__ == '__main__':
    main()
