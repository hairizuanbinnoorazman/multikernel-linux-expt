#!/usr/bin/env python3
"""Retain/check/release/restore one private lab stage alias; no guest cleanup."""
import argparse,json,os,sys
from pathlib import Path
from runtime_stage_binding import capture,publish,load,topology,change


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('operation',choices=['capture','check','release','restore'])
    p.add_argument('--record',type=Path,required=True)
    p.add_argument('--record-sha256')
    for name in ['volume','uuid','backing','alias']:p.add_argument('--'+name)
    a=p.parse_args()
    try:
        if os.geteuid()!=0:raise ValueError('root required')
        if a.operation=='capture':
            if a.record_sha256 is not None or any(getattr(a,n) is None for n in ['volume','uuid','backing','alias']):raise ValueError('capture authority missing')
            value=capture(a.volume,a.uuid,a.backing,a.alias);publish(a.record,value);changed=True
        else:
            if any(getattr(a,n) is not None for n in ['volume','uuid','backing','alias']):raise ValueError('unexpected capture authority')
            value=load(a.record,a.record_sha256);changed=False if a.operation=='check' else change(value,a.operation=='restore')
        print(json.dumps(dict(schema=1,operation=a.operation,changed=changed,alias_bound=topology(value),guest_cleanup_qualified=False,cold_restoration_qualified=False),sort_keys=True))
        return 0
    except Exception:
        print('Stage binding operation refused; preserve private record and inspect state',file=sys.stderr)
        return 1

if __name__=='__main__':sys.exit(main())
