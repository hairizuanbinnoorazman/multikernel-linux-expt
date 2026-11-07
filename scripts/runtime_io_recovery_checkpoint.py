"""Bound synced child writes across an intentionally dead private runtime.

Progress is inspection evidence; this never authorizes storage recovery/signals.
"""
import re
from runtime_diagnostic_bundle_checkpoint import decode,read_fixture_artifact

IO_BYTES=4<<20
IO_SHA256='bb9f8df61474d25e71fa00722318cd387396ca1736605e1248821cc0de3d3af8'


def sync_sequence(raw):
    if type(raw) is not bytes or len(raw)>131072:raise ValueError('invalid synced write stream')
    entries=re.findall(rb'^#\d+\s+\d+(?:\.\d+)?\s+G8_IO_SYNCED_SEQ=([^\r\n]*)$',raw,re.M)
    if any(not re.fullmatch(rb'[1-9][0-9]{0,6}[ \t]*',v) for v in entries):raise ValueError('noncanonical synced write progress')
    values=[int(v) for v in entries]
    if not values or any(not 1<=v<=1048576 for v in values) or any(a>=b for a,b in zip(values,values[1:])):
        raise ValueError('synced write progress is unqualified')
    return values[-1]


def inspect_synced_io(work,recovery,streams,active):
    """All metadata has already passed the common source/epoch/RUN qualifier."""
    stages=active['stages'];seq=[]
    for phase in ['before-kill','confirmed-dead','after-kill','after-adoption']:
        frame=stages[phase];raw=read_fixture_artifact(work/'integration',frame['stderr_snapshot'],1<<20)
        value=sync_sequence(raw)
        if type(frame['io_synced_seq']) is not int or frame['io_synced_seq']!=value:raise ValueError('synced write frame differs from its stream')
        seq.append(value)
    if not seq[0]<=seq[1]<seq[2]<seq[3]:raise ValueError('synced writes did not advance during outage and adoption')
    # Only executed BuildKit RUN lines count; an echoed Dockerfile is not output.
    final=re.findall(rb'^#\d+\s+\d+(?:\.\d+)?\s+G8_IO_FINAL_SEQ=([1-9][0-9]{0,6})\s*$',streams['stderr'],re.M)
    hashes=re.findall(rb'^#\d+\s+\d+(?:\.\d+)?\s+G8_IO_FINAL_SHA256=([a-f0-9]{64})\s*$',streams['stderr'],re.M)
    if len(final)!=1 or not seq[-1]<=int(final[0])<=1048576 or hashes!=[IO_SHA256.encode()]:raise ValueError('final synced write artifact is unqualified')
    if re.findall(rb'^G8_IO_IMAGE_SHA256=([a-f0-9]{64})$',streams['stdout'],re.M)!=[IO_SHA256.encode()] or streams['stdout'].splitlines().count(b'G8_IO_IMAGE_BYTES=4194304')!=1:
        raise ValueError('built image synced data differs')
    storage=decode(read_fixture_artifact(work/'integration/storage-state','state.json',1<<20),1<<20)
    exports=storage.get('exports')
    if type(exports) is not dict or len(exports)!=1:raise ValueError('synced write storage identity is ambiguous')
    lease=next(iter(exports.values()));owner=recovery['before']['ownership'][0]
    if type(lease) is not dict:raise ValueError('invalid synced write storage lease')
    if any(lease.get(k)!=owner[v] for k,v in [('sandbox_id','sandbox_id'),('sandbox_generation','generation'),('peer_cid','child_cid')]):raise ValueError('synced write storage owner differs')
    counters=lease.get('counters',{})
    if type(counters) is not dict:raise ValueError('invalid synced write counters')
    written=counters.get('written_bytes')
    if (lease.get('state')!='RELEASED' or type(lease.get('offline_check')) is not str or not re.fullmatch('e2fsck-clean-sha256:[a-f0-9]{64}',lease['offline_check']) or
            type(written) is not int or not int(final[0])*IO_BYTES<=written<1<<64):
        raise ValueError('native release counters do not cover synced child writes')
    return dict(synced_writes_while_runtime_dead_observed=True,synced_writes_after_adoption_observed=True,
                built_image_data_readback_observed=True,native_written_bytes_cover_logical_synced_bytes=True,cleanup_qualified=False)
