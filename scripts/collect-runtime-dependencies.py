#!/usr/bin/env python3
"""Read-only G7 dependency identities. No config values, environment or state export.

This is an inventory/review input, not a vulnerability scanner or admission gate.
Unknown licenses, package owners and provenance remain explicit review gaps.
"""
from __future__ import annotations
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import traceback

MAX_FILE = 1 << 30
PUBLIC_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.+:()~ -]{0,255}\Z")
NATIVE_TOOLS = (
    'bash', 'env', 'python3', 'busybox', 'jq', 'cpio', 'gzip', 'find', 'sort',
    'stat', 'readlink', 'realpath', 'install', 'cp', 'mv', 'rm', 'mkdir', 'mktemp',
    'chmod', 'chown', 'touch', 'cmp', 'cat', 'wc', 'head', 'tail', 'dd', 'du',
    'truncate', 'mount', 'umount', 'findmnt', 'mountpoint', 'unshare', 'nsenter',
    'sfdisk', 'mkfs.ext4', 'e2fsck', 'debugfs', 'dumpe2fs', 'blkid', 'losetup',
    'udevadm', 'ip', 'iptables', 'ip6tables', 'modprobe', 'modinfo', 'insmod',
    'rmmod', 'systemctl', 'docker', 'ctr', 'containerd', 'runc', 'gcc', 'ld',
)

def public(value):
    return value if isinstance(value, str) and PUBLIC_NAME.fullmatch(value) else None

def capture(args, *, input_data=None, timeout=60):
    p = subprocess.run(list(map(str, args)), input=input_data, capture_output=True,
                       timeout=timeout)
    # Raw stderr can carry unrelated local configuration; do not include it.
    if p.returncode:
        raise ValueError('metadata command failed')
    if len(p.stdout) > 32 << 20:
        raise ValueError('metadata output exceeds bound')
    return p.stdout

def parent_policy(path, uid=0):
    for p in (path, *path.parents):
        s = p.lstat()
        if not stat.S_ISDIR(s.st_mode) or s.st_uid != uid or s.st_mode & 0o022:
            return False
    return True

def named_path_policy(path, uid=0):
    # Include original symlink names/ancestors, not just their resolved targets.
    for p in (path, *path.parents):
        st = p.lstat()
        if st.st_uid != uid:
            return False
        if not stat.S_ISLNK(st.st_mode) and st.st_mode & 0o022:
            return False
    return True

def file_record(path, *, maximum=MAX_FILE):
    path = Path(path)
    target = path.resolve(strict=True)
    fd = os.open(target, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_size > maximum:
            raise ValueError('not a bounded regular file')
        h = hashlib.sha256(); count = 0
        while True:
            block = os.read(fd, 1 << 20)
            if not block: break
            count += len(block)
            if count > maximum: raise ValueError('file exceeds bound')
            h.update(block)
        after = os.fstat(fd)
        identity = lambda s: (s.st_dev,s.st_ino,s.st_size,s.st_mtime_ns,s.st_ctime_ns)
        if identity(before) != identity(after) or count != before.st_size:
            raise ValueError('file changed while hashing')
        protected = (before.st_uid == 0 and not before.st_mode & 0o022 and
                     parent_policy(target.parent) and named_path_policy(path.absolute()))
        return {'path':str(path), 'resolved':str(target), 'sha256':h.hexdigest(),
                'size_bytes':count, 'uid':before.st_uid, 'gid':before.st_gid,
                'mode':oct(stat.S_IMODE(before.st_mode)), 'protected_root_owned':protected}
    finally:
        os.close(fd)

def fixed_json(path):
    def pairs(items):
        d = {}
        for k,v in items:
            if k in d: raise ValueError('duplicate key in fixed metadata')
            d[k] = v
        return d
    p = Path(path)
    info = file_record(p, maximum=1 << 20)
    if not info['protected_root_owned']: raise ValueError('unsafe fixed metadata')
    data = p.read_bytes()
    if hashlib.sha256(data).hexdigest() != info['sha256']:
        raise ValueError('metadata changed')
    return json.loads(data, object_pairs_hook=pairs), info

def locate(name):
    for d in ['/usr/sbin','/usr/bin','/sbin','/bin']:
        p = Path(d)/name
        if p.is_file(): return p
    return None

def package_records():
    raw = capture(['/usr/bin/dpkg-query','-W','-f=${binary:Package}\t${Version}\t${source:Package}\t${source:Version}\t${db:Status-Status}\n'])
    records = {}
    for line in raw.decode().splitlines():
        parts = line.split('\t')
        if len(parts) != 5 or parts[4] != 'installed': continue
        if not all(public(v) for v in parts[:4]): raise ValueError('invalid package metadata')
        name, version, source, source_version, _ = parts
        records[name] = {'package':name,'version':version,'source_package':source,'source_version':source_version}
    return records

def owners(path, packages):
    resolved = str(Path(path).resolve())
    candidates = [resolved]
    for base in ['/bin/','/sbin/','/lib/']:
        if resolved.startswith('/usr'+base): candidates.append(resolved[4:])
    result = set()
    for p in candidates:
        r = subprocess.run(['/usr/bin/dpkg-query','-S',p],capture_output=True,text=True,timeout=30)
        for line in r.stdout.splitlines():
            names, sep, matched = line.partition(': ')
            if sep and matched == p:
                result.update(n for n in names.split(', ') if n in packages)
    return sorted(result)

def package_license(name):
    p = Path('/usr/share/doc')/name.split(':',1)[0]/'copyright'
    try:
        record = file_record(p, maximum=4 << 20)
        data = p.read_bytes()
        if hashlib.sha256(data).hexdigest() != record['sha256']: raise ValueError('changed license')
        declared = sorted({s.partition(':')[2].strip() for s in data.decode(errors='replace').splitlines() if s.startswith('License:')})
        values = [public(x) for x in declared]
        return {'copyright':record,'declared_license_labels':[v for v in values if v],
                'status':'declared_labels' if values and all(values) else 'copyright_present_requires_review',
                'scope':'Installed copyright metadata, not a legal interpretation or package integrity attestation.'}
    except (OSError,ValueError):
        return {'status':'copyright_unavailable_or_unsafe','declared_license_labels':[]}

PYTHON_METADATA = r'''
import importlib.metadata as m,json,re
rows=[]
valid=re.compile(r"[A-Za-z0-9][A-Za-z0-9_.+()~ -]{0,255}\Z")
for d in m.distributions():
 name=d.metadata.get('Name');version=d.version
 if not isinstance(name,str) or not valid.fullmatch(name) or not valid.fullmatch(version):
  raise SystemExit('invalid distribution metadata')
 expr=d.metadata.get('License-Expression');legacy=d.metadata.get('License')
 value=expr or legacy
 if isinstance(value,str) and value.strip().lower()in ['','unknown','none','noassertion','unspecified','not specified']:value=None
 declared=value if isinstance(value,str) and valid.fullmatch(value) else None
 classifiers=[v for v in d.metadata.get_all('Classifier',[]) if v.startswith('License :: ') and len(v)<256]
 # Requirements may include authenticated source URLs. Export names/counts only.
 requirements=[];direct=0
 for v in d.requires or []:
  n=re.match(r'[A-Za-z0-9][A-Za-z0-9_.-]*',v)
  if n:requirements.append(n.group())
  if '@' in v:direct+=1
 rows.append({'name':name,'version':version,'declared_license':declared,
  'license_classifiers':classifiers,'license_status':'declared' if declared or classifiers else 'metadata_requires_review',
  'requires_names':requirements,'direct_reference_count':direct})
print(json.dumps(rows))
'''

def collect(buildinfo):
    packages = package_records(); artifacts = []; gaps = []
    def add(family, label, path, expected=None):
        try:
            row = file_record(path)
            row.update(family=family,label=label)
            if expected:
                row['expected_sha256']=expected;row['matches_expected']=row['sha256']==expected
                if not row['matches_expected']:gaps.append({'family':family,'label':label,'reason':'artifact_hash_mismatch'})
            if not row['protected_root_owned']:gaps.append({'family':family,'label':label,'reason':'ownership_or_write_policy'})
            artifacts.append(row)
            return row
        except (OSError,ValueError):
            gaps.append({'family':family,'label':label,'reason':'artifact_unavailable_unstable_or_not_regular'})
            return None
    release, release_record = fixed_json('/usr/local/lib/multikernel/current/release-manifest.json')
    if not re.fullmatch('[0-9a-f]{40}',release.get('revision','')):raise ValueError('invalid release revision')
    required={'containerd-shim-multikernel-v2','mk-agent','mk-agentctl','mk-cni','mk-host-check','mknetd','mkruntimed'}
    if len(release['components'])!=7 or {c['name']for c in release['components']}!=required:
        raise ValueError('runtime component inventory is incomplete')
    for c in release['components']:
        if not re.fullmatch('[a-z0-9.-]+',c['name']) or not re.fullmatch('[0-9a-f]{64}',c['sha256']):raise ValueError('invalid release component')
        add('runtime-go',c['name'],Path('/usr/local/lib/multikernel/current/bin')/c['name'],c['sha256'])
    profile, profile_record = fixed_json('/etc/mkruntime/kernels/gce-mk2.json')
    compatibility = {k:public(profile['compatibility'].get(k)) for k in ['multikernel_revision','kerf_version','kerf_revision']}
    for k in ['kernel','initramfs','agent','relay']:
        a=profile[k];add('kernel-profile',k,a['path'],a['sha256'])
    a=profile['transport']['module'];add('kernel-profile','transport-module',a['path'],a['sha256'])
    for i,a in enumerate(profile.get('modules',[])):add('kernel-profile','support-module-'+str(i),a['path'],a['sha256'])
    add('native-helper','mkvsock-nbd','/usr/local/libexec/multikernel/mkvsock-nbd')
    add('kerf','entrypoint','/opt/mkruntime/kerf-venv/bin/kerf')
    add('python','kerf-interpreter','/opt/mkruntime/kerf-venv/bin/python')
    for name in NATIVE_TOOLS:
        p=locate(name)
        if p:add('native-tool',name,p)
        else:gaps.append({'family':'native-tool','label':name,'reason':'tool_not_installed'})
    for p in sorted(Path('/opt/cni/bin').glob('*')):
        if p.is_file():add('cni',p.name,p)
    # Record the running server, rather than mistaking the distribution CLI for it.
    services=[]
    for name in ['mkruntimed','mknetd','containerd','docker']:
        pid=int(capture(['/usr/bin/systemctl','show',name,'-p','MainPID','--value']).strip())
        if pid<=0:raise ValueError('service not running')
        p=Path('/proc')/str(pid)/'exe';target=p.resolve(strict=True)
        row=add('running-service',name,target)
        if row:
            if file_record(p)['sha256']!=row['sha256']:raise ValueError('running service binary differs')
            services.append({'service':name,'pid':pid,'sha256':row['sha256'],'resolved':row['resolved']})
    # Entire selected support generation: content hashes only, including config/env.
    support, support_record = fixed_json('/etc/multikernel/current/deployment-manifest.json')
    if support.get('schema_version')!=1 or not re.fullmatch('[0-9a-f]{64}',support.get('deployment','')):
        raise ValueError('invalid support manifest')
    support_rows=[]
    for name,expected in support['files'].items():
        if (Path(name).is_absolute() or '..' in Path(name).parts or not re.fullmatch('[a-zA-Z0-9_. /-]+',name)):
            raise ValueError('invalid support path')
        p=Path('/etc/multikernel/current')/name
        item=file_record(p,maximum=4<<20)
        support_rows.append({'name':name,'sha256':item['sha256'],'matches_expected':item['sha256']==expected,'protected_root_owned':item['protected_root_owned']})
        if item['sha256']!=expected or not item['protected_root_owned']:
            gaps.append({'family':'deployment','label':name,'reason':'hash_or_ownership_mismatch'})
    # Metadata inspection reads files; it never executes the target binaries.
    inspector=file_record(buildinfo)
    if not inspector['protected_root_owned']:raise ValueError('unsafe metadata inspector')
    unique=sorted({r['resolved']for r in artifacts})
    binary_metadata=json.loads(capture([buildinfo],input_data=json.dumps(unique).encode(),timeout=180))
    if [r['path']for r in binary_metadata]!=unique:raise ValueError('inspector response path mismatch')
    for r in artifacts:
        if file_record(r['resolved'])['sha256']!=r['sha256']:raise ValueError('artifact changed during metadata inspection')
    # A library name is not an identity. Resolve and hash its actual loader-cache path.
    cache={}
    for line in capture(['/usr/sbin/ldconfig','-p']).decode().splitlines():
        fields=line.split()
        if '=>'in fields and len(fields)>=4 and fields[-1].startswith('/'):
            cache.setdefault(fields[0],[]).append(fields[-1])
    inspected=set(unique)
    pending=binary_metadata[:]
    while pending:
        added=[]
        for name in sorted({n for r in pending for n in r.get('needed_libraries',[])}):
            candidates=cache.get(name,[])
            if not candidates:gaps.append({'family':'native-library','label':name,'reason':'loader_cache_entry_absent'})
            for path in sorted(set(candidates)):
                resolved=str(Path(path).resolve())
                if resolved in inspected:continue
                inspected.add(resolved)
                item=add('native-library',name,path)
                if item:added.append(item['resolved'])
        if len(inspected)>512:raise ValueError('library dependency closure exceeds bound')
        if not added:break
        pending=json.loads(capture([buildinfo],input_data=json.dumps(sorted(added)).encode(),timeout=180))
        if [r['path']for r in pending]!=sorted(added):raise ValueError('library inspector response mismatch')
        binary_metadata.extend(pending)
    for name in ['LICENSE','COPYING','NOTICE','pyproject.toml']:
        p=Path('/opt/mkruntime/kerf')/name
        if p.is_file():add('kerf-source',name,p)
    used=set()
    for r in artifacts:
        r['package_owners']=owners(r['resolved'],packages);used.update(r['package_owners'])
        if not r['package_owners']:r['package_owner_status']='not_owned_by_installed_package'
    package_licenses={n:package_license(n) for n in sorted(used)}
    py=json.loads(capture(['/opt/mkruntime/kerf-venv/bin/python','-I','-c',PYTHON_METADATA]))
    kerf_revision=capture(['/usr/bin/git','-C','/opt/mkruntime/kerf','rev-parse','HEAD']).decode().strip()
    if not re.fullmatch('[0-9a-f]{40}',kerf_revision):raise ValueError('invalid Kerf revision')
    kerf_status={'revision':kerf_revision,'matches_profile':kerf_revision==compatibility['kerf_revision']}
    if not kerf_status['matches_profile']:gaps.append({'family':'kerf','reason':'checkout_revision_mismatch'})
    # Only public source identity fields; do not export git diff/remote URLs.
    diff=subprocess.run(['/usr/bin/git','-C','/opt/mkruntime/kerf','diff','--quiet','HEAD'],capture_output=True,timeout=30)
    if diff.returncode not in [0,1]:raise ValueError('Kerf tracked-tree metadata unavailable')
    kerf_status['tracked_tree_modified']=diff.returncode==1
    return {'schema_version':1,'collected_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),
      'scope':'Read-only explicit dependency identities/declared-license metadata. No vulnerability, license approval, trusted source or ABI completeness claim.',
      'review_status':'REQUIRES_REVIEW','boot_id':Path('/proc/sys/kernel/random/boot_id').read_text().strip(),
      'host_kernel':os.uname().release,'selected_kernel_release':public(profile['kernel_release']),
      'runtime_revision':release['revision'],'release_manifest':release_record,'profile_metadata':{'sha256':profile_record['sha256'],'protected_root_owned':profile_record['protected_root_owned']},
      'compatibility_pins':compatibility,'artifacts':artifacts,'binary_metadata':binary_metadata,
      'services':services,'support':{'id':support['deployment'],'manifest_sha256':support_record['sha256'],'files':support_rows},
      'installed_packages':list(packages.values()),'selected_package_licenses':package_licenses,
      'python_distributions':py,'kerf_checkout':kerf_status,'collection_gaps':gaps,
      'coverage_limits':['Embedded Go module metadata can omit vendored versions; missing metadata is not a clean scan.',
        'Recursive DT_NEEDED and all matching loader-cache paths are inventory candidates, not a complete dlopen, script-import or loader-policy proof.',
        'Declared copyright/license metadata is not legal approval; local project licensing remains a separate gap.',
        'Package metadata does not prove distro backports, source provenance or executable/package equality.',
        'Custom kernel/module vulnerability and host/child ABI assessment require separate pinned evidence.',
        'Tracked Kerf diff excludes untracked files; checkout revision alone does not attest installed Python code.',
        'No network or vulnerability database queries; attach exact-artifact advisory triage separately.']}

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--buildinfo',type=Path,required=True,help='root-owned mk-dependency-buildinfo executable')
    p.add_argument('--output',type=Path,required=True,help='new private JSON report; never overwrites an existing file')
    args=p.parse_args()
    if os.geteuid()!=0:p.error('host inventory requires root to inspect protected selected artifacts')
    result=collect(args.buildinfo)
    data=(json.dumps(result,indent=2)+'\n').encode()
    fd=os.open(args.output,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
    with os.fdopen(fd,'wb')as f:f.write(data);f.flush();os.fsync(f.fileno())
    print(json.dumps({'status':'INVENTORY_COLLECTED_REVIEW_REQUIRED','artifacts':len(result['artifacts']),
                      'installed_packages':len(result['installed_packages']),'python_distributions':len(result['python_distributions']),
                      'collection_gaps':len(result['collection_gaps']),'sha256':hashlib.sha256(data).hexdigest()}))
    return 0

if __name__=='__main__':
    try:raise SystemExit(main())
    except (OSError,ValueError,KeyError,subprocess.SubprocessError) as error:
        # Avoid reflecting package/config data or command stderr into collection logs.
        frame=traceback.extract_tb(error.__traceback__)[-1]
        print(json.dumps({'status':'COLLECTION_FAILED_NO_COMPLETE_INVENTORY','error_type':type(error).__name__,'function':frame.name,'line':frame.lineno}),file=sys.stderr)
        raise SystemExit(1)
