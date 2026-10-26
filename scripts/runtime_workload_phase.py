"""Bounded workload-phase wait with a durable outcome before teardown."""
import hashlib
import json
import time


class PhaseFailure(RuntimeError):
    pass


def wait_for_phase(process, stdout, stderr, required_stdout, required_stderr,
                   deadline, checkpoint, invocation_id, boot_id, phase):
    started = time.monotonic_ns()
    while True:
        samples = {}
        data = {}
        oversized = False
        for name, path in [('stdout', stdout), ('stderr', stderr)]:
            with path.open('rb') as file:
                value = file.read(131073)
            oversized |= len(value) > 131072
            data[name] = value
            samples[name] = dict(sample_bytes=len(value), sha256=hashlib.sha256(value).hexdigest(),
                                 sample_complete=len(value) <= 131072)
        returncode = process.poll()
        observed = time.monotonic_ns()
        if oversized:
            outcome = 'output-limit'
        elif returncode is not None:
            outcome = 'process-exited'
        elif time.monotonic() >= deadline:
            outcome = 'deadline-exceeded'
        elif all(marker in data['stdout'] for marker in required_stdout) and all(marker in data['stderr'] for marker in required_stderr):
            outcome = 'reached'
        else:
            time.sleep(min(.2, max(0, deadline - time.monotonic())))
            continue
        record = dict(schema_version=1, invocation_id=invocation_id, boot_id=boot_id,
                      phase=phase, outcome=outcome, process_pid=process.pid,
                      process_returncode=returncode, started_monotonic_ns=started,
                      observed_monotonic_ns=observed, output=samples, cleanup_qualified=False)
        checkpoint.write_text(json.dumps(record, indent=2) + '\n')
        if outcome != 'reached':
            raise PhaseFailure('workload phase ' + phase + ' failed: ' + outcome)
        return record
