#!/usr/bin/env python3
"""Read-only preparation of the reviewed immutable storage binding clone.

The pinned manager's preparation path is used unchanged. Its persistence
entry point is replaced before calling it, and exits through a private sentinel.
No directory creation, selection, disk operation or service control is invoked.
CLI output contains no deployment IDs, hashes, file inventory or environment.
"""
import argparse,hashlib,json,os
from pathlib import Path
import stat,sys

MANAGER_SHA='89f4707af9be5198e0666b8941abbe10bd4139285400ddbdbc51132ed9202402'


def load_manager(path):
    data=path.read_bytes()
    if hashlib.sha256(data).hexdigest()!=MANAGER_SHA:raise ValueError('storage planner manager differs from pin')
    ns={'__name__':'readonly_storage_planner','__file__':str(path)}
    exec(compile(data,str(path),'exec'),ns)
    return ns


def prepare(root,old_bytes,new_bytes):
    if not root.is_absolute() or str(root)!=os.path.normpath(root):raise ValueError('invalid storage planner root')
    ns=load_manager(Path(__file__).with_name('manage-runtime-deployment.py'))
    captured={}
    class Prepared(Exception):pass
    def stop_before_persistence(selected_root,files,*,modes,expected_active):
        if selected_root!=root:raise ValueError('storage planner root changed')
        current=ns['rooted'](root,ns['BASE']/'current')
        if os.readlink(current)!=expected_active:raise ValueError('storage planner selector changed')
        previous=expected_active.split('/',1)[1];manifest=ns['verify_deployment'](root,previous)
        changed=[name for name in files if ns['digest'](files[name])!=manifest['files'][name]]
        if changed!=['runtime.env']:raise ValueError('storage planner changes unexpected assets')
        directory=ns['rooted'](root,ns['BASE']/expected_active)
        if set(modes)!=set(files) or any(stat.S_IMODE((directory/name).lstat().st_mode)!=mode for name,mode in modes.items()):
            raise ValueError('storage planner asset modes changed')
        candidate=ns['identifier'](files)
        if candidate==previous:raise ValueError('storage planner has no binding change')
        prospective=ns['rooted'](root,ns['BASE']/'deployments'/candidate)
        if os.path.lexists(prospective):raise ValueError('storage planner candidate already exists')
        captured.update(previous=previous,candidate=candidate,old_bytes=old_bytes,new_bytes=new_bytes,
                        changed_files=changed,modes=modes,manager_sha256=MANAGER_SHA,
                        deployment_selected=False,disk_resize_performed=False,service_restart_performed=False)
        raise Prepared()
    ns['install_files']=stop_before_persistence
    try:ns['update_storage_size'](root,old_bytes,new_bytes)
    except Prepared:pass
    else:raise ValueError('storage planner persistence interception was not reached')
    if not captured:raise ValueError('storage planner produced no prospective clone')
    ns['preflight_links'](root)
    current=ns['rooted'](root,ns['BASE']/'current')
    if os.readlink(current)!='deployments/'+captured['previous']:raise ValueError('storage planner selector changed')
    ns['verify_deployment'](root,captured['previous'])
    return captured


class PrivateParser(argparse.ArgumentParser):
    def error(self,message):raise ValueError('invalid storage planner arguments')


def main():
    parser=PrivateParser(description=__doc__);parser.add_argument('--root',type=Path,default=Path('/'))
    parser.add_argument('old_bytes',type=int);parser.add_argument('new_bytes',type=int)
    try:
        args=parser.parse_args();plan=prepare(args.root,args.old_bytes,args.new_bytes)
    except Exception:
        print('runtime storage binding plan refused',file=sys.stderr);return 1
    print(json.dumps(dict(scope='readonly-storage-binding-clone-plan',old_bytes=plan['old_bytes'],new_bytes=plan['new_bytes'],
                         only_runtime_env_change=True,asset_modes_preserved=True,manager_pin_verified=True,
                         disk_resize_performed=False,deployment_selected=False,service_restart_performed=False),sort_keys=True));return 0


if __name__=='__main__':raise SystemExit(main())
