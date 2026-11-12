"""Bounded, exclusive private command evidence for owned Docker trials."""
from contextlib import ExitStack
import json,os,re,stat,subprocess
from runtime_bounded_command import bounded_trial_command,CommandOutputLimit

LIMIT=131071


def capture(arguments,private,label,success,deadline):
    if type(label) is not str or not re.fullmatch('[a-z][a-z0-9-]{0,95}',label):
        raise ValueError('invalid trial command label')
    info=private.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid!=os.geteuid() or info.st_mode&0o077:
        raise ValueError('unprotected trial command evidence')
    paths={suffix:private/(label+suffix) for suffix in ['.stdout','.stderr','.exit.json','-timeout.json','-failure.json']}
    if any(p.exists() or p.is_symlink() for p in paths.values()):
        raise ValueError('trial command evidence already exists')
    with ExitStack() as stack:
        streams={key:stack.enter_context(paths[key].open('xb')) for key in ['.stdout','.stderr']}
        for stream in streams.values():os.fchmod(stream.fileno(),0o600)
        def record(suffix,value):
            with paths[suffix].open('xb') as f:
                os.fchmod(f.fileno(),0o600);f.write((json.dumps(value,sort_keys=True)+'\n').encode());f.flush();os.fsync(f.fileno())
        def retain(stdout,stderr):
            for suffix,data in [('.stdout',stdout),('.stderr',stderr)]:
                f=streams[suffix];f.write(data[:LIMIT]);f.flush();os.fsync(f.fileno())
        failure=None
        try:
            result=bounded_trial_command([str(x) for x in arguments],deadline)
        except (subprocess.TimeoutExpired,CommandOutputLimit) as error:
            stdout,stderr=error.stdout or b'',error.stderr or b'';retain(stdout,stderr)
            record('-timeout.json' if isinstance(error,subprocess.TimeoutExpired) else '-failure.json',dict(
                timed_out=isinstance(error,subprocess.TimeoutExpired),output_refused=isinstance(error,CommandOutputLimit),
                stdout_truncated=len(stdout)>LIMIT,stderr_truncated=len(stderr)>LIMIT,
                deadline_monotonic_ns=deadline,qualified=False))
            failure='timeout' if isinstance(error,subprocess.TimeoutExpired) else 'output'
        # Raise outside the handler so even __context__ cannot retain the
        # original command arguments or guest bytes in the public exception.
        if failure=='timeout':raise subprocess.TimeoutExpired(['owned-trial-command'],0)
        if failure=='output':raise CommandOutputLimit(b'',b'')
        retain(result.stdout,result.stderr);record('.exit.json',dict(returncode=result.returncode))
        if success and result.returncode:
            raise ValueError('trial command failed; private evidence retained')
        return result
