"""Inspection-only validation of a pinned same-boot transport trial predecessor.

This authenticates archived qualification, not current liveness or reuse alone.
The caller must also check exact current unit epochs and quiescent resources.
"""
import json
import re
from runtime_doctor_checkpoint import unique_object
from runtime_diagnostic_bundle_checkpoint import qualify_timing


def qualify(data, boot, revision, archive_sha256, parent_invocation, workload_invocation):
    bad = ValueError('prior transport trial is not qualified for same-boot reuse')
    if (type(data) is not bytes or not 0 < len(data) <= 1 << 20 or
            any(type(v) is not str for v in [boot, revision, archive_sha256, parent_invocation, workload_invocation]) or
            not re.fullmatch(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}', boot) or
            not re.fullmatch('[a-f0-9]{7,40}', revision) or not re.fullmatch('[a-f0-9]{64}', archive_sha256) or
            any(not re.fullmatch('[a-f0-9]{32}', v) for v in [parent_invocation, workload_invocation])):
        raise bad
    try:
        prior = json.loads(data, object_pairs_hook=unique_object)
        if (prior['boot_id'] != boot or prior['harness_source_commit'] != revision or
                prior['source_archive_sha256'] != archive_sha256 or prior['parent_scope']['InvocationID'] != parent_invocation or
                prior['pressure_invocation'] != workload_invocation or prior['pressure_scope']['InvocationID'] != workload_invocation or
                prior['pressure_scope']['MainPID'] != '0' or prior['pressure_scope']['SubState'] != 'exited' or
                prior['pressure_scope']['Result'] != 'success' or prior['pressure_scope']['ExecMainStatus'] != '0'):
            raise bad
        for name, value in [('candidate_quiescent', True), ('cold_restore_required', True), ('original_restored', False), ('primary_unchanged', True)]:
            if prior[name] is not value:
                raise bad
        if (type(prior['unexpected_taint_bits']) is not int or prior['unexpected_taint_bits'] != 0 or
                prior['candidate_sha256'] != '74e813765d8551d086a603e047337b27ad0cce28dd4cc5de7e983a6e10a1f69f' or
                prior['primary_before'] != prior['primary_after'] or not prior['primary_after']['file_sha256'] or
                not prior['primary_after']['service_main_pids']):
            raise bad
        timing = prior['diagnostic_checkpoint_timings']
        if (timing['version'] != 1 or timing['scope'] != 'receipt-bound-doctor-bundle-checkpoint-timings' or
                timing['boot_id'] != boot or timing['invocation_id'] != workload_invocation or
                set(timing['timings']) != {'while-full', 'while-image-ready', 'after-remove'}):
            raise bad
        frames = {phase:qualify_timing(json.dumps(frame).encode(), phase) for phase,frame in timing['timings'].items()}
        if timing['maximum_elapsed_ns'] != max(v['elapsed_ns'] for v in frames.values()):
            raise bad
    except (KeyError, TypeError, ValueError, AttributeError):
        raise bad from None
    return prior
