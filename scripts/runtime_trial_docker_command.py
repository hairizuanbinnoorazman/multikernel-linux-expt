"""Bounded native-trial CLI streams; private warnings never become stdout."""
from contextlib import ExitStack
import json,os,re,stat,subprocess
from runtime_bounded_command import bounded_command,CommandOutputLimit


def capture(argv, private, index, deadline, env):
    if (type(index) is not int or not 0<=index<100 or not argv or
            not argv[0].startswith('/') or any(type(v) is not str or '\x00' in v for v in argv) or
            any(not re.fullmatch('[A-Z_][A-Z_0-9]*',k) or type(v) is not str or '\x00' in v for k,v in env.items())):
        raise ValueError('invalid native Docker command')
    info=private.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid!=os.geteuid() or info.st_mode&0o077:
        raise ValueError('unprotected native Docker command evidence')
    paths=[private/(f'command-{index:02d}.'+suffix) for suffix in ['stdout','stderr','json']]
    if any(p.exists() or p.is_symlink() for p in paths):raise ValueError('native Docker command evidence already exists')
    with ExitStack() as stack:
        files=[stack.enter_context(p.open('xb')) for p in paths]
        for f in files:os.fchmod(f.fileno(),0o600)
        stdout=stderr=b'';status=None;failure=None
        try:
            result=bounded_command(['/usr/bin/env','-i',*[k+'='+v for k,v in sorted(env.items())],*argv],deadline,stdout_limit=65536,stderr_limit=4096)
            stdout,stderr,status=result.stdout,result.stderr,result.returncode
            if status:failure='exit-status'
        except subprocess.TimeoutExpired as error:
            stdout,stderr=error.stdout or b'',error.stderr or b'';failure='deadline'
        except CommandOutputLimit as error:
            stdout,stderr=error.stdout,error.stderr;failure='stream-limit'
        files[0].write(stdout);files[1].write(stderr)
        files[2].write((json.dumps(dict(exit_status=status,failure=failure,stdout_bytes=len(stdout),stderr_bytes=len(stderr)),sort_keys=True)+'\n').encode())
        for f in files:f.flush();os.fsync(f.fileno())
    if failure:raise ValueError('private Docker trial command failed')
    return stdout
