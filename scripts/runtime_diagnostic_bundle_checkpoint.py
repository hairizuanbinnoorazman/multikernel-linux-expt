"""Strict bundle acceptance against independently qualified observations."""
import hashlib,json,re
from runtime_doctor_checkpoint import unique_object
from runtime_component_build_checkpoint import qualify,qualify_unsupported
from runtime_operational_doctor_checkpoint import UNOBSERVED
from runtime_operational_doctor_checkpoint import qualify as qualify_operational

SCOPE='operational-and-component-observation-snapshot'
EXCLUDED=['raw-logs','guest-output','credentials','environment','configuration-files','state-files','process-command-lines','kernel-console','service-readiness','executable-attestation','kernel-attestation','containment']

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
