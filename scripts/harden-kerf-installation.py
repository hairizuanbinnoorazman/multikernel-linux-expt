#!/usr/bin/env python3
"""Check or remove group/other write access from a root-owned Kerf code tree.

No ownership/content changes, symlink following during traversal, state copies,
or service operations. A complete private plan is optional; stdout is aggregate.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat
import sys

LIMIT_FILES=100000
LIMIT_BYTES=2<<30

def identity(s):return (s.st_dev,s.st_ino,s.st_size,s.st_mtime_ns,s.st_ctime_ns)

def ancestors(path,uid):
    for p in path.parents:
        s=p.lstat()
        if not stat.S_ISDIR(s.st_mode) or s.st_uid not in [0,uid]:raise ValueError('unsafe code-tree ancestor')
        if s.st_mode&0o022 and not(s.st_uid==0 and s.st_mode&stat.S_ISVTX):raise ValueError('writable code-tree ancestor')

def target_trusted(path,uid,tree):
    # Inspect each symlink hop, including intermediate directory aliases. Merely
    # checking the final resolved file would trust a replaceable external alias.
    seen=set()
    def visit(path):
        path=Path(path).absolute()
        key=str(path)
        if key in seen:return
        seen.add(key)
        if len(seen)>128:raise ValueError('symlink validation exceeds bound')
        for p in (path,*path.parents):
            st=p.lstat()
            if st.st_uid not in [0,uid]:raise ValueError('untrusted symlink target owner')
            if stat.S_ISLNK(st.st_mode):
                visit(p.parent/os.readlink(p))
                continue
            pending=Path(os.path.normpath(p)).is_relative_to(tree)
            if st.st_mode&0o022 and not pending:
                if not(p!=path and st.st_uid==0 and st.st_mode&stat.S_ISVTX):raise ValueError('writable symlink target path')
        resolved=path.resolve(strict=True)
        if resolved!=path:visit(resolved)
    visit(path)

def inspect(tree,uid=0):
    tree=Path(tree).absolute();ancestors(tree,uid)
    initial=tree.lstat()
    if not stat.S_ISDIR(initial.st_mode) or initial.st_uid!=uid:raise ValueError('code-tree root must be an owned real directory')
    entries=[];total=0
    def walk_error(error):raise error
    for directory,dirs,files in os.walk(tree,followlinks=False,onerror=walk_error):
        for p in [Path(directory),*[Path(directory)/n for n in files],*[Path(directory)/n for n in dirs if (Path(directory)/n).is_symlink()]]:
            s=p.lstat()
            if s.st_uid!=uid:raise ValueError('code-tree entry not owned by required uid')
            kind='directory' if stat.S_ISDIR(s.st_mode) else 'file' if stat.S_ISREG(s.st_mode) else 'symlink' if stat.S_ISLNK(s.st_mode) else None
            if kind is None:raise ValueError('special code-tree entry')
            row={'name':str(p.relative_to(tree)),'kind':kind,'uid':s.st_uid,'gid':s.st_gid,'mode':stat.S_IMODE(s.st_mode),'identity':identity(s)}
            if kind=='file':
                if s.st_nlink!=1:raise ValueError('multiply linked code-tree file')
                total+=s.st_size
                if total>LIMIT_BYTES:raise ValueError('code tree exceeds byte bound')
                fd=os.open(p,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NONBLOCK)
                try:
                    if identity(os.fstat(fd))!=identity(s):raise ValueError('code file changed before open')
                    h=hashlib.sha256();size=0
                    while True:
                        block=os.read(fd,1<<20)
                        if not block:break
                        size+=len(block)
                        if size>s.st_size:raise ValueError('code file grew during hashing')
                        h.update(block)
                    if size!=s.st_size or identity(os.fstat(fd))!=identity(s):raise ValueError('code file changed during hashing')
                    row['sha256']=h.hexdigest()
                finally:os.close(fd)
            if kind=='symlink':
                # Internal targets are locked by this plan; external ones must
                # already be protected. Do not chmod any target through a link.
                target_trusted(p.parent/os.readlink(p),uid,tree)
                row['target_sha256']=hashlib.sha256(os.readlink(p).encode()).hexdigest()
            entries.append(row)
            if len(entries)>LIMIT_FILES:raise ValueError('code tree exceeds file bound')
    if not entries or entries[0]['kind']!='directory':raise ValueError('missing regular code-tree root')
    by_name={r['name']:r for r in entries}
    if len(by_name)!=len(entries):raise ValueError('duplicate code-tree entry')
    for r in entries:
        for parent in Path(r['name']).parents:
            if str(parent) not in by_name or by_name[str(parent)]['kind']!='directory':
                raise ValueError('code entry has an uninspected or symlink parent')
    return sorted(entries,key=lambda r:r['name'])

def content_digest(rows):
    values=[{k:r[k]for k in ['name','kind','uid','gid','sha256','target_sha256']if k in r}for r in rows]
    return hashlib.sha256(json.dumps(values,sort_keys=True,separators=(',',':')).encode()).hexdigest()

def apply(tree,before,uid=0):
    # Protect all directories before touching regular files. Any mutation of the
    # inspected code during locking is detected by the final content comparison.
    planned=[r for r in before if r['kind']!='symlink'and r['mode']&0o022]
    planned.sort(key=lambda r:(r['kind']!='directory',len(Path(r['name']).parts),r['name']))
    for r in planned:
        p=Path(tree)/r['name'];flags=os.O_RDONLY|os.O_NOFOLLOW|os.O_CLOEXEC|os.O_NONBLOCK
        if r['kind']=='directory':flags|=os.O_DIRECTORY
        fd=os.open(p,flags)
        try:
            s=os.fstat(fd)
            # Directory ctime can change while protecting its children; compare
            # stable object identity plus exact permission/owner before chmod.
            if (s.st_dev,s.st_ino)!=(r['identity'][0],r['identity'][1])or s.st_uid!=uid or stat.S_IMODE(s.st_mode)!=r['mode']:raise ValueError('planned code entry changed')
            if r['kind']=='file'and identity(s)!=tuple(r['identity']):raise ValueError('code file changed before chmod')
            os.fchmod(fd,r['mode']&~0o022)
            if stat.S_IMODE(os.fstat(fd).st_mode)!=r['mode']&~0o022:raise ValueError('permission change did not match plan')
        finally:os.close(fd)
    after=inspect(tree,uid)
    if content_digest(before)!=content_digest(after):raise ValueError('code tree contents or owners changed')
    if any(r['mode']&0o022 for r in after if r['kind']!='symlink'):raise ValueError('writable code entry remains')
    return after,len(planned)

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('tree',type=Path);m=p.add_mutually_exclusive_group(required=True);m.add_argument('--check',action='store_true');m.add_argument('--apply',action='store_true');p.add_argument('--private-plan',type=Path)
    a=p.parse_args()
    if os.geteuid()!=0:p.error('production Kerf code-tree checks require root')
    before=inspect(a.tree)
    if a.private_plan:
        fd=os.open(a.private_plan,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
        with os.fdopen(fd,'w')as f:json.dump(before,f);f.flush();os.fsync(f.fileno())
    count=sum(r['kind']!='symlink'and bool(r['mode']&0o022)for r in before)
    after,changed=apply(a.tree,before)if a.apply else (before,0)
    result={'status':'PROTECTED'if a.apply or not count else 'WRITABLE_CODE_TREE_REFUSED','entries':len(before),'writable_before':count,'changed_permissions':changed,'contents_owners_unchanged':content_digest(before)==content_digest(after),'before_content_sha256':content_digest(before),'after_content_sha256':content_digest(after),'scope':'Code tree permissions only; no service/state/ownership/content changes or external symlink-target writes.'}
    print(json.dumps(result));return 0 if a.apply or not count else 1
if __name__=='__main__':
    try:raise SystemExit(main())
    except (OSError,ValueError,RuntimeError):
        print('Kerf code-tree trust check refused; no execution authorized',file=sys.stderr);raise SystemExit(1)
