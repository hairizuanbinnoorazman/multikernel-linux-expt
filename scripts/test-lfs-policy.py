#!/usr/bin/env python3
"""Exercise automatic LFS conversion against immutable, partially staged bytes."""
import hashlib,os,pathlib,subprocess,sys,tempfile,shlex
SCRIPT=pathlib.Path(sys.argv[1] if len(sys.argv)>1 else pathlib.Path(__file__).with_name('auto-track-lfs.py')).resolve()
def run(*args,input=None,expected=0):
 r=subprocess.run(args,input=input,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 assert r.returncode==expected,(args,r.returncode,r.stdout,r.stderr)
 return r.stdout
with tempfile.TemporaryDirectory(prefix='mk-lfs-policy-') as temporary:
 os.chdir(temporary)
 run('git','init','-q');run('git','config','user.name','LFS policy test');run('git','config','user.email','lfs-policy@example.invalid')
 run('git','lfs','install','--local')
 pathlib.Path('.gitattributes').write_text('evidence/** filter=lfs diff=lfs merge=lfs -text\n')
 run('git','add','.gitattributes');run('git','commit','-qm','Fixture')
 pathlib.Path('evidence').mkdir();pathlib.Path('evidence/note.md').write_text('small evidence\n');run('git','add','evidence/note.md')
 assert run('git','show',':evidence/note.md').startswith(b'version https://git-lfs.github.com/spec/v1\n')
 pathlib.Path('evidence/empty.log').touch();run('git','add','evidence/empty.log')
 run(sys.executable,str(SCRIPT),'--check')
 name='payload [x]*?.bin';data=b'a'*(1<<20)+b'b';path=pathlib.Path(name);path.write_bytes(data);path.chmod(0o755);run('git','add','--',name)
 path.write_text('unstaged changes\n');path.chmod(0o644)
 run(sys.executable,str(SCRIPT))
 pointer=run('git','show',':'+name)
 assert ('oid sha256:'+hashlib.sha256(data).hexdigest()).encode() in pointer
 assert path.read_text()=='unstaged changes\n'
 assert run('git','ls-files','--stage','--',name).startswith(b'100755 ')
 assert run('git','lfs','smudge','--',name,input=pointer)==data
 run(sys.executable,str(SCRIPT),'--check')
 pathlib.Path('.gitattributes').write_text(pathlib.Path('.gitattributes').read_text()+'# unstaged user note\n')
 pathlib.Path('second.bin').write_bytes(data);run('git','add','second.bin')
 before=run('git','show',':second.bin');attributes=pathlib.Path('.gitattributes').read_bytes()
 run(sys.executable,str(SCRIPT),expected=1)
 assert run('git','show',':second.bin')==before and pathlib.Path('.gitattributes').read_bytes()==attributes
 run('git','add','.gitattributes');run(sys.executable,str(SCRIPT));run(sys.executable,str(SCRIPT),'--check')
 # Simulate a caller bypassing the clean filter for a small evidence file.
 raw=b'raw index evidence\n';oid=run('git','hash-object','-w','--stdin',input=raw).strip().decode()
 run('git','update-index','--add','--cacheinfo','100644',oid,'evidence/raw.txt')
 run(sys.executable,str(SCRIPT),'--check',expected=1);run(sys.executable,str(SCRIPT));run(sys.executable,str(SCRIPT),'--check')
 assert b'size 19\n'in run('git','show',':evidence/raw.txt')
 # Exercise the real Git pre-commit handoff, not just direct script calls.
 hook=pathlib.Path('.git/hooks/pre-commit')
 hook.write_text('#!/bin/sh\nexec '+shlex.quote(sys.executable)+' '+shlex.quote(str(SCRIPT))+'\n');hook.chmod(0o755)
 pathlib.Path('third.bin').write_bytes(data);run('git','add','third.bin')
 run('git','commit','-qm','Automatic hook conversion')
 assert b'oid sha256:'+hashlib.sha256(data).hexdigest().encode() in run('git','show','HEAD:third.bin')
 run(sys.executable,str(SCRIPT),'--check')
print('LFS policy tests: PASS (evidence, large files, literal paths, partial staging, modes, attribute safety, bypass detection)')
