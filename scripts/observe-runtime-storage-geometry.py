#!/usr/bin/env python3
"""Read-only geometry checks for an explicitly reviewed whole-disk ext4 grow.

No cloud command, filesystem write, binding update or service restart exists in
this observer. Mount identity remains enforced by the separately pinned guard.
Only the fixed 1024-byte primary superblock metadata is read from the device.
"""
import argparse,hashlib,json,os
from pathlib import Path
import stat,struct,sys,uuid

MOUNT_GUARD_SHA='53c7ec25cb72e6d10e52dde8487c2634d808c80254e4178c5bee4617c9212422'
PHASES=('before','disk-grown','filesystem-grown')


def parse_superblock(data):
    if type(data) is not bytes or len(data)!=1024 or struct.unpack_from('<H',data,0x38)[0]!=0xef53:
        raise ValueError('invalid ext4 superblock metadata')
    word=lambda offset:struct.unpack_from('<I',data,offset)[0]
    shift=word(0x18)
    if shift>6:raise ValueError('invalid ext4 block geometry')
    high,high_reserved=word(0x150),word(0x154)
    if not word(0x60)&0x80 and (high or high_reserved):
        raise ValueError('unqualified ext4 high block counts')
    blocks=word(4)+(high<<32);reserved=word(8)+(high_reserved<<32)
    if not 0<blocks<1<<63 or not 0<=reserved<=blocks:
        raise ValueError('invalid ext4 block counts')
    try:label=data[0x78:0x88].split(b'\0',1)[0].decode('ascii')
    except UnicodeDecodeError:raise ValueError('invalid ext4 label metadata') from None
    return dict(block_size=1024<<shift,blocks=blocks,reserved_blocks=reserved,
                uuid=str(uuid.UUID(bytes=data[0x68:0x78])),label=label)


def qualify(geometry,phase,device_bytes,old_bytes,new_bytes,expected_uuid,expected_label):
    values=[device_bytes,old_bytes,new_bytes]
    if (phase not in PHASES or any(type(v) is not int or not 0<v<=1<<63 or v%512 for v in values)
            or not old_bytes<new_bytes):raise ValueError('invalid storage growth authority')
    if geometry['uuid']!=expected_uuid or geometry['label']!=expected_label:
        raise ValueError('filesystem identity differs from binding')
    required_device=old_bytes if phase=='before' else new_bytes
    required_fs=new_bytes if phase=='filesystem-grown' else old_bytes
    filesystem_bytes=geometry['blocks']*geometry['block_size']
    if device_bytes!=required_device or filesystem_bytes!=required_fs or filesystem_bytes>device_bytes:
        raise ValueError('filesystem and device growth phase differ')
    # Five percent with less than one block of integer rounding. Do not lower
    # the reserve to make a staging admission pass.
    if abs(geometry['reserved_blocks']*20-geometry['blocks'])>=20:
        raise ValueError('filesystem five-percent reserve differs')
    return dict(phase=phase,device_bytes=device_bytes,filesystem_bytes=filesystem_bytes,
                block_size=geometry['block_size'],blocks=geometry['blocks'],reserved_blocks=geometry['reserved_blocks'],
                binding_identity_matches=True,five_percent_reserve_verified=True,
                metadata_only=True,growth_performed=False)


def observe(args):
    if os.geteuid()!=0 or args.phase not in PHASES:raise ValueError('invalid geometry observation authority')
    p=Path(__file__).with_name('validate-runtime-storage-mount.py');data=p.read_bytes()
    if hashlib.sha256(data).hexdigest()!=MOUNT_GUARD_SHA:raise ValueError('mount guard differs from pin')
    ns={'__name__':'pinned_mount_guard','__file__':str(p)};exec(compile(data,str(p),'exec'),ns)
    required_device=args.old_bytes if args.phase=='before' else args.new_bytes
    binding=argparse.Namespace(mountpoint=args.mountpoint,device=args.device,serial=args.serial,
                               uuid=args.uuid,label=args.label,bytes=required_device)
    ns['validate'](binding)
    path=Path(args.device).resolve(strict=True)
    fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK|os.O_CLOEXEC)
    try:
        info=os.fstat(fd)
        if not stat.S_ISBLK(info.st_mode) or info.st_uid!=0 or info.st_rdev!=Path(args.mountpoint).stat().st_dev:
            raise ValueError('geometry device differs from mounted binding')
        result=qualify(parse_superblock(os.pread(fd,1024,1024)),args.phase,required_device,args.old_bytes,args.new_bytes,args.uuid,args.label)
        ns['validate'](binding)
        after=qualify(parse_superblock(os.pread(fd,1024,1024)),args.phase,required_device,args.old_bytes,args.new_bytes,args.uuid,args.label)
        if result!=after or Path(args.device).resolve(strict=True)!=path or Path(args.mountpoint).stat().st_dev!=info.st_rdev:
            raise ValueError('storage geometry changed during observation')
        return result
    finally:os.close(fd)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ['mountpoint','device','serial','uuid','label']:parser.add_argument('--'+name,required=True)
    parser.add_argument('--phase',choices=PHASES,required=True)
    for name in ['old-bytes','new-bytes']:parser.add_argument('--'+name,type=int,required=True)
    args=parser.parse_args()
    try:result=observe(args)
    except Exception:
        print('runtime storage geometry refused',file=sys.stderr);return 1
    print(json.dumps(result,sort_keys=True));return 0


if __name__=='__main__':raise SystemExit(main())
