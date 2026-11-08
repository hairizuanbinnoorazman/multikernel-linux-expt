"""Explicit private artifact retention across a cold boot; never workload authority.

Callers choose fixed control artifacts, separately verify real boot/module/mount
state, and keep manifests private. No directory discovery, image inspection,
process signaling, recovery or prior-completion qualification is performed.
"""
import hashlib,json,os,re,stat
from pathlib import Path

SCOPE='explicit-private-trial-artifact-retention'
MAX_FILES=256
MAX_FILE_BYTES=128<<20
BOOT=re.compile(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}')
FIELDS={'uid','gid','mode','bytes','inode','sha256'}

def fail():raise ValueError('trial retention observation refused')
def path_parts(path):
    if type(path) is not str or not path.startswith('/') or path.startswith('//') or str(Path(path))!=path or any(c in path for c in ['\n','\0']) or '..' in Path(path).parts:fail()
    return Path(path).parts[1:]

def parent(path):
    parts=path_parts(path)
    if not parts:fail()
    fd=os.open('/',os.O_PATH|os.O_DIRECTORY|os.O_CLOEXEC)
    try:
        for part in parts[:-1]:
            new=os.open(part,os.O_PATH|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=fd)
            os.close(fd);fd=new;info=os.fstat(fd)
            sticky=info.st_uid==0 and bool(info.st_mode&stat.S_ISVTX)
            if info.st_uid not in {0,os.geteuid()} or info.st_mode&0o022 and not sticky:fail()
        return fd,parts[-1]
    except BaseException:
        os.close(fd);raise

def identity(info):
    return (info.st_dev,info.st_ino,info.st_uid,info.st_gid,info.st_mode,info.st_nlink,info.st_size,info.st_mtime_ns,info.st_ctime_ns)

def read_artifact(path,limit=MAX_FILE_BYTES,keep=False):
    fd,name=parent(path)
    try:
        opened=os.open(name,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK|os.O_CLOEXEC,dir_fd=fd)
        with os.fdopen(opened,'rb') as source:
            before=os.fstat(source.fileno())
            if (not stat.S_ISREG(before.st_mode) or before.st_uid!=os.geteuid() or before.st_nlink!=1 or before.st_mode&0o7022 or not 0<=before.st_size<=limit):fail()
            digest=hashlib.sha256();total=0;chunks=[]
            while chunk:=source.read(1<<20):
                total+=len(chunk)
                if total>limit:fail()
                digest.update(chunk)
                if keep:chunks.append(chunk)
            if total!=before.st_size or identity(before)!=identity(os.fstat(source.fileno())) or identity(before)!=identity(os.stat(name,dir_fd=fd,follow_symlinks=False)):fail()
            record=dict(uid=before.st_uid,gid=before.st_gid,mode=stat.S_IMODE(before.st_mode),bytes=total,inode=before.st_ino,sha256=digest.hexdigest())
            return record,b''.join(chunks) if keep else None
    finally:os.close(fd)

def absent(paths):
    for path in paths:
        fd,name=parent(path)
        try:
            try:os.stat(name,dir_fd=fd,follow_symlinks=False)
            except FileNotFoundError:continue
            fail()
        finally:os.close(fd)

def capture(paths,boot,absent_paths=None):
    if type(boot) is not str or not BOOT.fullmatch(boot) or type(paths) is not list or not 0<len(paths)<=MAX_FILES or any(type(p) is not str for p in paths) or len(set(paths))!=len(paths):fail()
    absent_paths=[] if absent_paths is None else absent_paths
    if type(absent_paths) is not list or any(type(p) is not str for p in absent_paths) or len(set(absent_paths))!=len(absent_paths) or len(paths)+len(absent_paths)>MAX_FILES or set(paths)&set(absent_paths):fail()
    absent(absent_paths)
    records={p:read_artifact(p)[0] for p in sorted(paths)}
    # Reobserve the whole explicit set, so a later read cannot hide drift in an
    # earlier file. These samples do not claim atomicity or exclude all ABA.
    if {p:read_artifact(p)[0] for p in sorted(paths)}!=records:fail()
    absent(absent_paths)
    return dict(version=1,scope=SCOPE,previous_boot_id=boot,artifacts=records,absent_artifacts=sorted(absent_paths))

def validate(manifest):
    if type(manifest) is not dict or set(manifest)!={'version','scope','previous_boot_id','artifacts','absent_artifacts'} or type(manifest['version']) is not int or manifest['version']!=1 or manifest['scope']!=SCOPE:fail()
    if type(manifest['previous_boot_id']) is not str or not BOOT.fullmatch(manifest['previous_boot_id']):fail()
    records=manifest['artifacts']
    if type(records) is not dict or not 0<len(records)<=MAX_FILES:fail()
    missing=manifest['absent_artifacts']
    if type(missing) is not list or any(type(p) is not str for p in missing) or len(set(missing))!=len(missing) or len(records)+len(missing)>MAX_FILES or set(records)&set(missing):fail()
    for path in missing:path_parts(path)
    for path,record in records.items():
        path_parts(path)
        if type(record) is not dict or set(record)!=FIELDS:fail()
        for key in FIELDS-{'sha256'}:
            if type(record[key]) is not int or record[key]<0:fail()
        if record['uid']!=os.geteuid() or record['inode']==0 or record['mode']&0o7022 or record['mode']>0o7777 or record['bytes']>MAX_FILE_BYTES or type(record['sha256']) is not str or not re.fullmatch('[a-f0-9]{64}',record['sha256']):fail()
    return manifest

def publish(path,manifest):
    validate(manifest)
    data=(json.dumps(manifest,sort_keys=True)+'\n').encode()
    if len(data)>1<<20:fail()
    fd,name=parent(path)
    try:
        info=os.fstat(fd)
        if info.st_uid!=os.geteuid() or stat.S_IMODE(info.st_mode)!=0o700:fail()
        # Reopen the same anchored directory for fsync; O_PATH cannot be synced.
        syncfd=os.open('.',os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC,dir_fd=fd)
        try:
            opened=os.open(name,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW|os.O_CLOEXEC,0o600,dir_fd=fd)
            with os.fdopen(opened,'wb') as target:
                os.fchmod(target.fileno(),0o600);target.write(data);target.flush();os.fsync(target.fileno())
            os.fsync(syncfd)
        finally:os.close(syncfd)
    finally:os.close(fd)

def unique(pairs):
    result={}
    for key,value in pairs:
        if key in result:fail()
        result[key]=value
    return result

def load(path):
    metadata,data=read_artifact(path,1<<20,True)
    if metadata['mode']!=0o600:fail()
    return validate(json.loads(data,object_pairs_hook=unique))

def verify_after_boot(manifest,current_boot):
    validate(manifest)
    if type(current_boot) is not str or not BOOT.fullmatch(current_boot) or current_boot==manifest['previous_boot_id']:fail()
    observed=capture(list(manifest['artifacts']),current_boot,manifest['absent_artifacts'])
    if observed['artifacts']!=manifest['artifacts']:fail()
    return dict(version=1,scope=SCOPE,previous_boot_id=manifest['previous_boot_id'],verified_boot_id=current_boot,artifact_count=len(observed['artifacts']),absent_artifact_count=len(manifest['absent_artifacts']),artifacts_preserved=True,prior_completion_accepted=False,live_process_authority_granted=False,cold_restoration_qualified=False)
