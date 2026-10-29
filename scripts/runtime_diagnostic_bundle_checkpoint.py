"""Strict bundle acceptance against independently qualified observations."""
import hashlib,json,os,re,stat
from runtime_doctor_checkpoint import unique_object
from runtime_component_build_checkpoint import qualify,qualify_unsupported
from runtime_operational_doctor_checkpoint import UNOBSERVED
from runtime_operational_doctor_checkpoint import qualify as qualify_operational

SCOPE='operational-and-component-observation-snapshot'
EXCLUDED=['raw-logs','guest-output','credentials','environment','configuration-files','state-files','process-command-lines','kernel-console','service-readiness','executable-attestation','kernel-attestation','containment']

def read_fixture_artifact(parent,name,limit):
 """Read one bounded immutable file from an existing private fixture parent."""
 parent=os.fspath(parent)
 if not os.path.isabs(parent) or os.path.normpath(parent)!=parent or os.path.basename(name)!=name or name in ['','.','..'] or type(limit) is not int or not 0<limit<=1<<20:raise ValueError('invalid fixture artifact request')
 directory=os.open('/',os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC)
 try:
  for component in parent.strip('/').split('/'):
   child=os.open(component,os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC|os.O_NOFOLLOW,dir_fd=directory)
   os.close(directory);directory=child
  info=os.fstat(directory)
  if info.st_uid!=os.geteuid() or stat.S_IMODE(info.st_mode)!=0o700:raise ValueError('fixture artifact parent is not private and owned')
  descriptor=os.open(name,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=directory)
  with os.fdopen(descriptor,'rb') as source:
   before=os.fstat(source.fileno())
   if not stat.S_ISREG(before.st_mode) or before.st_uid!=os.geteuid() or before.st_nlink!=1 or before.st_mode&0o022 or not 0<before.st_size<=limit:raise ValueError('unsafe fixture artifact file')
   data=source.read(limit+1);after=os.fstat(source.fileno());named=os.stat(name,dir_fd=directory,follow_symlinks=False)
   for observed in [after,named]:
    if any(getattr(before,k)!=getattr(observed,k) for k in ['st_dev','st_ino','st_mode','st_uid','st_nlink','st_size','st_mtime_ns','st_ctime_ns']):raise ValueError('fixture artifact identity changed')
   if len(data)!=before.st_size:raise ValueError('fixture artifact size changed')
   return data
 finally:os.close(directory)

def decode(data,limit):
 if not isinstance(data,bytes) or not 0<len(data)<=limit:raise ValueError('bundle checkpoint exceeds its bound')
 return json.loads(data,object_pairs_hook=unique_object)

def equal(a,b):
 if json.dumps(a,sort_keys=True)!=json.dumps(b,sort_keys=True):raise ValueError('bundle differs from the qualified observation contract')

def envelope(data,receipt_data,status):
 bundle=decode(data,8<<20);receipt=decode(receipt_data,65536)
 equal(receipt,dict(version=1,scope=SCOPE,collection_status='captured',observation_status=status,bytes=len(data),sha256=hashlib.sha256(data).hexdigest()))
 if not isinstance(bundle,dict) or set(bundle)!={'version','scope','collection_status','started_at','finished_at','snapshot','excluded'}:raise ValueError('invalid bundle envelope')
 for key in ['started_at','finished_at']:
  if not isinstance(bundle[key],str) or not re.fullmatch(r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?Z',bundle[key]):raise ValueError('invalid bundle time')
 if not isinstance(bundle['snapshot'],dict) or set(bundle['snapshot'])!={'components','operational'}:raise ValueError('invalid bundle snapshot')
 return bundle

def bind(bundle,components,operational):
 equal(bundle,dict(version=1,scope=SCOPE,collection_status='captured',started_at=bundle['started_at'],finished_at=bundle['finished_at'],snapshot=dict(components=components,operational=operational),excluded=EXCLUDED))
 return bundle

def qualify_idle(data,receipt_data,operational_data,boot,revision,versions,revisions,unsupported=None):
 if unsupported not in [None,'RUNTIME','NETWORK']:raise ValueError('invalid idle fixture mode')
 bundle=envelope(data,receipt_data,'inconclusive')
 component=json.dumps(bundle['snapshot']['components']).encode()
 if unsupported:qualified=qualify_unsupported(component,boot,revision,unsupported)
 else:qualified=qualify(component,boot,revision,versions,revisions)
 codes={'RUNTIME':['RESOURCE_OBSERVATION_UNAVAILABLE','COMBINED_OBSERVATION_UNAVAILABLE','NATIVE_RESOURCE_IDENTITY_UNSUPPORTED'],
        'NETWORK':['RESOURCE_OBSERVATION_UNAVAILABLE','COMBINED_OBSERVATION_UNAVAILABLE','NETWORK_OWNERSHIP_UNSUPPORTED'],
        None:['RESOURCE_OBSERVATION_UNAVAILABLE','POOL_OBSERVATION_UNAVAILABLE','KERNEL_CPU_FREE_LIST_UNAVAILABLE']}[unsupported]
 expected=dict(version=7,scope='resource-network-pool-and-compiled-component-observations',status='inconclusive',revision=revision,boot_id=boot,boot_stable=False,findings=[dict(code=c) for c in codes],admission_guaranteed=False,artifact_verified=False,kernel_verified=False,unobserved=UNOBSERVED)
 equal(decode(operational_data,8<<20),expected)
 return bind(bundle,qualified,expected)

def qualify_live(data,receipt_data,boot,owner,storage_report,network_report,endpoint,
                 combined_report,pool_report,unified_report,component_report,
                 operational_report,versions,revisions,released=False):
 # Requalify all independent reference views before binding either nested
 # report. A consistent label, receipt digest, or self-consistent forged bundle
 # is insufficient. The live qualifier includes the released hold explicitly.
 bundle=envelope(data,receipt_data,'consistent')
 components=qualify(json.dumps(component_report).encode(),boot,storage_report['revision'],versions,revisions)
 operational=qualify_operational(json.dumps(operational_report).encode(),boot,owner,storage_report,
                 network_report,endpoint,combined_report,pool_report,unified_report,
                 component_report,versions,revisions,released=released)
 return bind(bundle,components,operational)

def qualify_timing(data,phase):
 if phase not in ['while-full','while-image-ready','after-remove','record-restored']:raise ValueError('invalid diagnostic checkpoint phase')
 timing=decode(data,65536)
 keys={'version','scope','phase','configured_budget_ns','started_monotonic_ns','deadline_monotonic_ns','finished_monotonic_ns','elapsed_ns','report_count'}
 if not isinstance(timing,dict) or set(timing)!=keys:raise ValueError('invalid diagnostic checkpoint timing')
 for key in keys-{'scope','phase'}:
  if type(timing[key]) is not int:raise ValueError('invalid diagnostic checkpoint timing number')
 if timing['version']!=1 or timing['scope']!='doctor-and-diagnostic-bundle-checkpoint' or timing['phase']!=phase or timing['configured_budget_ns']!=10_000_000_000 or timing['report_count']!=8:raise ValueError('invalid diagnostic checkpoint timing scope')
 start=timing['started_monotonic_ns'];end=timing['finished_monotonic_ns'];deadline=timing['deadline_monotonic_ns']
 if not 0<=start<=end<=deadline<=start+timing['configured_budget_ns'] or timing['elapsed_ns']!=end-start:raise ValueError('diagnostic checkpoint exceeded its shared budget')
 return timing

def qualify_lifecycle_timings(receipt_data,frames,boot,invocation):
 """Bind all normal-pressure timings to one completed workload receipt.

 This validates timing evidence only, not health, native isolation or cleanup.
 The caller independently owns the expected boot and systemd invocation.
 """
 phases=['while-full','while-image-ready','after-remove']
 receipt=decode(receipt_data,1<<20)
 if not re.fullmatch(r'[a-f0-9]{32}',invocation) or not re.fullmatch(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}',boot):raise ValueError('invalid expected timing identity')
 if not isinstance(receipt,dict) or type(receipt.get('schema_version')) is not int or receipt['schema_version']!=1 or receipt.get('boot_id')!=boot or receipt.get('invocation_id')!=invocation:raise ValueError('timing receipt identity mismatch')
 if receipt.get('storage_pressure_tested') is not True or receipt.get('production_profile_child_build_pass') is not True:raise ValueError('timing receipt lacks completed pressure workload')
 hashes=receipt.get('runtime_diagnostic_checkpoint_timing_sha256')
 if not isinstance(hashes,dict) or set(hashes)!=set(phases) or not isinstance(frames,dict) or set(frames)!=set(phases):raise ValueError('incomplete diagnostic lifecycle timing evidence')
 timings={}
 for phase in phases:
  raw=frames[phase]
  if not isinstance(raw,bytes) or not 0<len(raw)<=65536 or not isinstance(hashes[phase],str) or not re.fullmatch('[a-f0-9]{64}',hashes[phase]) or hashlib.sha256(raw).hexdigest()!=hashes[phase]:raise ValueError('diagnostic timing receipt digest mismatch')
  timings[phase]=qualify_timing(raw,phase)
 for earlier,later in zip(phases,phases[1:]):
  if timings[earlier]['finished_monotonic_ns']>timings[later]['started_monotonic_ns']:raise ValueError('diagnostic lifecycle timing phases overlap or reverse')
 return dict(version=1,scope='receipt-bound-doctor-bundle-checkpoint-timings',boot_id=boot,invocation_id=invocation,timings=timings,maximum_elapsed_ns=max(v['elapsed_ns'] for v in timings.values()))

def inspect_fixture_lifecycle_timings(work,boot,invocation):
 # The lifecycle receipt belongs to the trial root. Per-hold timing frames
 # belong to integration/. Never search alternate directories or reuse a
 # stale receipt from another layout when the exact root artifact is absent.
 work=os.fspath(work)
 frames={phase:read_fixture_artifact(os.path.join(work,'integration'),'diagnostic-checkpoint-timing-'+phase+'.json',65536) for phase in ['while-full','while-image-ready','after-remove']}
 return qualify_lifecycle_timings(read_fixture_artifact(work,'integration-receipt.json',1<<20),frames,boot,invocation)
