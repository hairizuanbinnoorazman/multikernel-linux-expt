#!/usr/bin/env python3
"""Exercise actual CPU initiators: pre-effect claim and sticky uncertainty.

Models sets, pending ACKs, repark and sends; not real CPU/peer/lifecycle proof.
"""
import argparse,ast
from pathlib import Path
import subprocess,tempfile
p=argparse.ArgumentParser(description=__doc__);p.add_argument('source',type=Path);a=p.parse_args();s=a.source.read_text()
tree=ast.parse((Path(__file__).parent/'test-kernel-cpu-move-ownership.py').read_text())
assignment=next(n for n in tree.body if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='code' for t in n.targets))
v=assignment.value
while isinstance(v,ast.BinOp):v=v.left
assert isinstance(v,ast.Constant) and isinstance(v.value,str);code=v.value
code=code.replace('int id,state;', 'int id,state,cpu_transfer_state;')
code=code.replace('struct mk_cpu_resource_payload {','''#define MK_CPU_TRANSFER_STABLE 0
#define MK_CPU_TRANSFER_MOVING 1
#define MK_CPU_TRANSFER_UNCERTAIN 2
#define MK_CPU_TRANSFER_RELEASING 3
#define READ_ONCE(x) __atomic_load_n(&(x),__ATOMIC_SEQ_CST)
#define WRITE_ONCE(x,v) __atomic_store_n(&(x),(v),__ATOMIC_SEQ_CST)
#define cmpxchg(p,o,n) ({int e=(o);__atomic_compare_exchange_n((p),&e,(n),0,__ATOMIC_SEQ_CST,__ATOMIC_SEQ_CST);e;})
#define ERR_PTR(e) ((void *)(intptr_t)(e))
#define PTR_ERR(p) ((long)(intptr_t)(p))
#define IS_ERR(p) ((uintptr_t)(p)>=(uintptr_t)-4095)
struct mk_request_id {unsigned long long epoch,sequence;};
struct mk_cpu_resource_payload {struct mk_request_id request_id;''')
code=code.replace('int operation,mk_phys_cpu_t id)\n{CHECK(cls==MK_MSG_RESOURCE','int operation,mk_phys_cpu_t id,struct mk_request_id *request)\n{request->epoch=7;request->sequence=9;CHECK(cls==MK_MSG_RESOURCE')
code=code.replace('if(fail_pending)return NULL;', 'if(fail_pending)return ERR_PTR(-ENOMEM);')
code=code.replace('CHECK(!mk_cpu_set_add(instance->cpus,cpu));CHECK(mk_cpu_set_del(mk_cpu_pool,cpu));', 'ret=mk_cpu_set_add(instance->cpus,cpu);if(ret)return ret;CHECK(mk_cpu_set_del(mk_cpu_pool,cpu));')
code=code.replace('CHECK(!mk_cpu_set_add(mk_cpu_pool,cpu));CHECK(mk_cpu_set_del(instance->cpus,cpu));', 'ret=mk_cpu_set_add(mk_cpu_pool,cpu);if(ret)return -EUCLEAN;CHECK(mk_cpu_set_del(instance->cpus,cpu));')
code=code.replace('effects++;repark_peer++;', 'CHECK(mk_cpu_set_contains(&target_set,cpu)&&!mk_cpu_set_contains(&pool,cpu)&&pending_live);effects++;repark_peer++;')
code=code.replace('effects++;sends++;', 'CHECK(target.cpu_transfer_state==MK_CPU_TRANSFER_MOVING&&mk_cpu_set_contains(&target_set,cpu)&&!mk_cpu_set_contains(&pool,cpu));effects++;sends++;')
def function(name):
 start=s.index('int '+name+'(');return s[start:s.index('\n}',start)+2]+'\n'
code+=function('mk_send_cpu_add')+function('mk_send_cpu_remove')+r'''
static void reset(bool add,int state)
{
 CHECK(!refs&&!pending_live);memset(pool_ids,0,sizeof(pool_ids));memset(target_ids,0,sizeof(target_ids));
 pool=(struct mk_cpu_set){.nr=add?1:0,.cap=add?1:0,.ids=pool_ids};pool_ids[0]=cpu;
 target_set=(struct mk_cpu_set){.nr=add?1:2,.cap=add?1:2,.ids=target_ids};target_ids[0]=9;if(!add)target_ids[1]=cpu;
 root=(struct mk_instance){.id=0};target=(struct mk_instance){.id=7,.state=state,.generation=17,.cpus=&target_set};root_instance=&root;mk_cpu_pool=&pool;
 finds=effects=repark_peer=repark_host=sends=core_calls=self_calls=target_reserves=pool_reserves=0;
 fail_target_reserve=fail_pool_reserve=fail_target_track=fail_pool_track=fail_pending=fail_send=ack_error=fail_peer=fail_host=0;
}
static int call(bool add,int id){int ret=add?mk_send_cpu_add(id,cpu,2,16):mk_send_cpu_remove(id,cpu);CHECK(!refs&&!pending_live);return ret;}
static void stable(void){CHECK(target.cpu_transfer_state==MK_CPU_TRANSFER_STABLE);}
static void uncertain(bool add)
{CHECK(target.cpu_transfer_state==MK_CPU_TRANSFER_UNCERTAIN&&mk_cpu_set_contains(&target_set,cpu)&&!mk_cpu_set_contains(&pool,cpu));int old=effects;CHECK(call(add,7)==-EUCLEAN&&effects==old);}
int main(void)
{
 for(int state=0;state<=1;state++)for(int add=0;add<2;add++){
  reset(add,state);CHECK(!call(add,7));stable();CHECK(mk_cpu_set_contains(&pool,cpu)==!add&&mk_cpu_set_contains(&target_set,cpu)==add);
  reset(add,state);root_instance=NULL;CHECK(call(add,7)==-ENODEV&&!effects);stable();
  reset(add,state);CHECK(call(add,99)==-ENODEV&&!effects);stable();
  reset(add,state);if(add)fail_target_reserve=1;else fail_pool_reserve=1;CHECK(call(add,7)==-ENOMEM&&!effects);stable();
  for(int gate=1;gate<=3;gate++){reset(add,state);target.cpu_transfer_state=gate;CHECK(call(add,7)==-EUCLEAN&&!effects&&!core_calls&&target.cpu_transfer_state==gate);}
 }
 puts("CPU_TRANSFER_GATE_AND_CLEAN_SUCCESS_ROUTES_PASS");
 for(int add=0;add<2;add++){
  reset(add,MK_STATE_ACTIVE);fail_pending=1;CHECK(call(add,7)==-ENOMEM&&!effects&&mk_cpu_set_contains(&pool,cpu)==add);stable();
  for(int fault=0;fault<3;fault++){
   reset(add,MK_STATE_ACTIVE);if(fault==0)fail_send=1;if(fault==1)ack_error=-ETIMEDOUT;if(fault==2)ack_error=-EIO;
   CHECK(call(add,7)==-EUCLEAN);uncertain(add);CHECK(repark_host==0);
  }
 }
 reset(true,MK_STATE_ACTIVE);fail_peer=1;CHECK(call(true,7)==-EUCLEAN&&!sends&&!repark_host);uncertain(true);
 reset(true,MK_STATE_ACTIVE);fail_target_track=1;CHECK(call(true,7)==-ENOMEM&&!effects&&mk_cpu_set_contains(&pool,cpu));stable();
 reset(false,MK_STATE_ACTIVE);fail_host=1;CHECK(call(false,7)==-EUCLEAN);uncertain(false);
 reset(false,MK_STATE_ACTIVE);fail_pool_track=1;CHECK(call(false,7)==-EUCLEAN);uncertain(false);
 reset(false,0);fail_host=1;CHECK(call(false,7)==-EUCLEAN);uncertain(false);
 puts("CPU_TIMEOUT_REPARK_SEND_AND_TRACKING_FAILURE_RETAINS_OWNER_PASS");
 cpu=(1ULL<<40)|8;reset(true,MK_STATE_ACTIVE);CHECK(!call(true,7));stable();reset(false,MK_STATE_ACTIVE);CHECK(!call(false,7));stable();
 puts("CPU_64BIT_RESOURCE_IDS_AND_NO_SPECULATIVE_TIMEOUT_INVERSE_PASS");return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-cpu-uncertainty-') as td:
 f=Path(td)/'test.c';exe=Path(td)/'test';f.write_text(code)
 subprocess.run(['gcc','-std=gnu11','-Wall','-Wextra','-Werror','-Wno-unused-function','-fsanitize=undefined','-fno-sanitize-recover=all',str(f),'-o',str(exe)],check=True)
 print('CPU_UNCERTAINTY_ACTUAL_INITIATORS_COMPILED',flush=True);subprocess.run([str(exe)],check=True)
