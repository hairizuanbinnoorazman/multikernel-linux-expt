"""Bind an idle journal observation to qualified Docker lifecycle ownership."""
import json
from runtime_doctor_checkpoint import qualify as qualify_storage, unique_object


def qualify(data, boot, owner, storage_report, released=False):
    qualify_storage(json.dumps(storage_report).encode(), boot, owner, released=released)
    if not isinstance(data, bytes) or not 0 < len(data) <= 4096:
        raise ValueError('journal status checkpoint exceeds its bound')
    report = json.loads(data, object_pairs_hook=unique_object)
    if (not isinstance(report, dict) or
            set(report) != {'version', 'source', 'status', 'sequence',
                            'pending_completions', 'journal_failed'} or
            type(report['version']) is not int or report['version'] != 1 or
            report['source'] != 'in-process-lifecycle-journal' or
            report['status'] != 'idle' or report['journal_failed'] is not False or
            type(report['pending_completions']) is not int or report['pending_completions'] != 0 or
            type(report['sequence']) is not int or not 0 <= report['sequence'] < 1 << 64 or
            report['sequence'] != storage_report['ownership']['sequence']):
        raise ValueError('journal observation does not match the qualified lifecycle hold')
    return report
