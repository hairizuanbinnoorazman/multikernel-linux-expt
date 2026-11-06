"""Private lab stage aliases: metadata-only retention, exact detach/rebind.

No guest contents, credentials or image data are opened. UUID plus persistent
inode identities survive device renumbering; boot identity never grants cleanup.
The production storage-mount validator remains unchanged.
"""
import ctypes
import hashlib
import json
import os
from pathlib import Path
import re
import select
import signal
import stat
import struct

UUID = re.compile(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}')
MS_BIND, MS_REC, MS_PRIVATE, CLONE_NEWNS, UMOUNT_NOFOLLOW = 4096, 16384, 1 << 18, 0x20000, 8
libc = ctypes.CDLL(None, use_errno=True)
libc.mount.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p, ctypes.c_ulong, ctypes.c_void_p]
libc.umount2.argtypes = [ctypes.c_char_p, ctypes.c_int]
libc.unshare.argtypes = [ctypes.c_int]


def refuse():
    return ValueError('stage binding identity or state is unqualified')


def canonical(value):
    if type(value) is not str or not value.startswith('/') or len(value) > 4096 or os.path.normpath(value) != value:
        raise refuse()
    return Path(value)


def directory(path, private=False):
    path = canonical(os.fspath(path))
    for part in [path, *path.parents]:
        info = part.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise refuse()
    if private and stat.S_IMODE(path.stat().st_mode) != 0o700:
        raise refuse()
    return path.stat()


def entries():
    result = []
    for line in Path('/proc/self/mountinfo').read_text().splitlines():
        left, separator, right = line.partition(' - ')
        fields, tail = left.split(), right.split()
        if not separator or len(fields) < 6 or len(tail) < 3:
            raise refuse()
        def unescape(value):
            for a,b in [('\\040',' '),('\\011','\t'),('\\012','\n'),('\\134','\\')]:
                value = value.replace(a,b)
            return value
        result.append(dict(device=fields[2], root=unescape(fields[3]), path=unescape(fields[4]),
                           options=set(fields[5].split(',')), kind=tail[0]))
    return result


def validate_record(value):
    keys = {'version','capture_boot','volume','uuid','backing','alias','backing_inode','underlay_inode','alias_parent_inode'}
    if (type(value) is not dict or set(value) != keys or type(value['version']) is not int or value['version'] != 1 or
            any(type(value[k]) is not int or value[k] <= 0 for k in ['backing_inode','underlay_inode','alias_parent_inode']) or
            type(value['capture_boot']) is not str or not UUID.fullmatch(value['capture_boot']) or
            type(value['uuid']) is not str or not UUID.fullmatch(value['uuid'])):
        raise refuse()
    volume, backing, alias = [canonical(value[k]) for k in ['volume','backing','alias']]
    if (backing.parent != volume or not backing.name.startswith('g8-stage-') or
            alias.parent != Path('/opt/mkruntime') or not re.fullmatch(r'g8-host-[a-z0-9-]{1,64}', alias.name) or
            alias == backing or alias in backing.parents or backing in alias.parents):
        raise refuse()
    return value


def topology(record):
    validate_record(record)
    volume, backing, alias = [Path(record[k]) for k in ['volume','backing','alias']]
    vol = directory(volume)
    source = directory(backing, True)
    target = directory(alias, True)
    if alias.parent.stat().st_ino != record['alias_parent_inode'] or source.st_ino != record['backing_inode'] or source.st_dev != vol.st_dev:
        raise refuse()
    link = Path('/dev/disk/by-uuid')/record['uuid']
    directory(link.parent)
    info = link.lstat()
    if not stat.S_ISLNK(info.st_mode) or info.st_uid != 0:
        raise refuse()
    device = link.resolve(strict=True).stat()
    if not stat.S_ISBLK(device.st_mode) or device.st_rdev != vol.st_dev or vol.st_dev == Path('/').stat().st_dev:
        raise refuse()
    identity = f'{os.major(vol.st_dev)}:{os.minor(vol.st_dev)}'
    mounts = entries()
    selected = [m for m in mounts if m['device'] == identity]
    base = [m for m in selected if m['path'] == str(volume)]
    bound = [m for m in selected if m['path'] == str(alias)]
    if (len(base) != 1 or base[0]['root'] != '/' or base[0]['kind'] != 'ext4' or
            not {'rw','nosuid','nodev'} <= base[0]['options'] or
            len(bound) > 1 or len(selected) != 1+len(bound) or
            any(m['path'].startswith(str(alias)+'/') for m in mounts)):
        raise refuse()
    if bound:
        if ((target.st_dev, target.st_ino) != (source.st_dev, source.st_ino) or bound[0]['root'] != '/'+backing.name or
                bound[0]['kind'] != 'ext4' or not {'rw','nosuid','nodev'} <= bound[0]['options']):
            raise refuse()
    elif (target.st_ino != record['underlay_inode'] or list(alias.iterdir()) or
          any(m['path'] == str(alias) for m in mounts)):
        raise refuse()
    return bool(bound)


def underlay_inode(alias):
    """Inspect beneath a live alias in a private child namespace only."""
    read_end, write_end = os.pipe2(os.O_CLOEXEC)
    pid = os.fork()
    if pid == 0:
        os.close(read_end)
        try:
            if libc.unshare(CLONE_NEWNS) or libc.mount(None,b'/',None,MS_REC|MS_PRIVATE,None) or libc.umount2(os.fsencode(alias),UMOUNT_NOFOLLOW):
                os._exit(1)
            info = directory(alias, True)
            if list(alias.iterdir()):
                os._exit(1)
            os.write(write_end,struct.pack('>Q',info.st_ino))
            os._exit(0)
        except Exception:
            os._exit(1)
    os.close(write_end)
    try:
        if not select.select([read_end],[],[],5)[0]:
            os.kill(pid,signal.SIGKILL)
            data=b''
        else:
            data = os.read(read_end,9)
    finally:
        os.close(read_end)
    _, status = os.waitpid(pid,0)
    if status or len(data) != 8:
        raise refuse()
    return struct.unpack('>Q',data)[0]


def capture(volume, uuid, backing, alias):
    volume, backing, alias = [canonical(str(v)) for v in [volume,backing,alias]]
    source = directory(backing,True)
    current = directory(alias,True)
    mounted = (source.st_dev,source.st_ino) == (current.st_dev,current.st_ino)
    value = dict(version=1,capture_boot=Path('/proc/sys/kernel/random/boot_id').read_text().strip(),volume=str(volume),uuid=uuid,
                 backing=str(backing),alias=str(alias),backing_inode=source.st_ino,underlay_inode=current.st_ino,
                 alias_parent_inode=alias.parent.stat().st_ino)
    if topology(value) != mounted:
        raise refuse()
    if mounted:
        value['underlay_inode']=underlay_inode(alias)
        if not topology(value):
            raise refuse()
    return value


def unique(pairs):
    value = {}
    for key,item in pairs:
        if key in value:
            raise refuse()
        value[key]=item
    return value


def load(path, expected):
    if type(expected) is not str or not re.fullmatch('[a-f0-9]{64}',expected):
        raise refuse()
    directory(path.parent,True)
    fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK|os.O_CLOEXEC)
    with os.fdopen(fd,'rb') as file:
        info=os.fstat(file.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid!=0 or info.st_nlink!=1 or stat.S_IMODE(info.st_mode)!=0o600 or not 0<info.st_size<=16384:
            raise refuse()
        data=file.read(16385)
        if len(data)!=info.st_size or hashlib.sha256(data).hexdigest()!=expected:
            raise refuse()
        fields=['st_dev','st_ino','st_mode','st_uid','st_nlink','st_size','st_mtime_ns','st_ctime_ns']
        if any(getattr(info,k)!=getattr(observed,k) for observed in [os.fstat(file.fileno()),path.lstat()] for k in fields):
            raise refuse()
    return validate_record(json.loads(data,object_pairs_hook=unique))


def publish(path,value):
    validate_record(value);directory(path.parent,True)
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW|os.O_CLOEXEC,0o600)
    with os.fdopen(fd,'wb') as file:
        os.fchmod(file.fileno(),0o600)
        file.write((json.dumps(value,sort_keys=True)+'\n').encode());file.flush();os.fsync(file.fileno())
    fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC)
    try:os.fsync(fd)
    finally:os.close(fd)


def change(value,restore):
    bound=topology(value)
    if bound==restore:
        return False
    alias=Path(value['alias'])
    if restore:
        source=os.open(value['backing'],os.O_PATH|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC)
        target=os.open(alias,os.O_PATH|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC)
        try:
            source_info,target_info=os.fstat(source),os.fstat(target)
            if (source_info.st_ino!=value['backing_inode'] or source_info.st_dev!=Path(value['volume']).stat().st_dev or
                    target_info.st_ino!=value['underlay_inode'] or target_info.st_dev!=alias.parent.stat().st_dev or
                    any(info.st_uid!=0 or not stat.S_ISDIR(info.st_mode) or stat.S_IMODE(info.st_mode)!=0o700 for info in [source_info,target_info])):
                raise refuse()
            if libc.mount(f'/proc/self/fd/{source}'.encode(),f'/proc/self/fd/{target}'.encode(),None,MS_BIND,None):
                raise OSError(ctypes.get_errno(),'stage alias mount refused')
        finally:os.close(source);os.close(target)
    else:
        if underlay_inode(alias)!=value['underlay_inode'] or not topology(value):
            raise refuse()
        if libc.umount2(os.fsencode(alias),UMOUNT_NOFOLLOW):
            raise OSError(ctypes.get_errno(),'stage alias detach refused')
    if topology(value)!=restore:
        raise refuse()
    return True
