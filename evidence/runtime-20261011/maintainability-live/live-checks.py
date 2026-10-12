import subprocess, pathlib, sys
checks = [('feature-matrix','test-runtime-g4-g6-feature-matrix.sh',''), ('containerd-restart','test-runtime-containerd-restart-live.sh',''), ('docker-restart','test-runtime-docker-restart-live.sh',''), ('shim-death','test-runtime-shim-death-live.sh',''), ('network-isolation','test-runtime-network-isolation-live.sh',' /var/tmp/mk-maint-src'), ('clean-shim-restart','test-runtime-clean-shim-restart-live.sh','')]
for name, script, args in checks:
 print('START '+name, flush=True)
 command='cd /var/tmp/mk-maint-src && export PATH=/opt/go/bin:$PATH TMPDIR=/var/tmp GOTMPDIR=/var/tmp MK_EVIDENCE_XTRACE=0 TERM=xterm && timeout --signal=TERM --kill-after=60s 1800 bash scripts/'+script+args
 with open('/tmp/mk-maint-'+name+'-console.log','x') as output:
  result=subprocess.run([sys.executable,'scripts/capture-evidence-command.py','evidence/runtime-20261011/maintainability-live/'+name+'.log','--','gcloud','compute','ssh','mklinux-maintainability-20261011','--project=${MK_PROJECT}','--zone=asia-southeast1-b','--command='+command],stdout=output,stderr=subprocess.STDOUT)
 print('END '+name+' status='+str(result.returncode),flush=True)
 print('\n'.join(pathlib.Path('/tmp/mk-maint-'+name+'-console.log').read_text().splitlines()[-22:]),flush=True)
 if result.returncode: sys.exit(result.returncode)
