#!/usr/bin/env python3
"""Exercise actual CPU initiators with modeled sets, allocation, repark and ACK.

Checks pre-effect ownership/capacity and failure bookkeeping; not live hotplug,
peer authentication, concurrency or physical rollback proof.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('source', type=Path)
p.add_argument('--section',choices=['all','ownership','capacity','tracking'],default='all')
a = p.parse_args()
s = a.source.read_text()


def function(name):
    start = s.index('int ' + name + '(')
    return s[start:s.index('\n}', start) + 2] + '\n'


code = r'''
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
typedef unsigned long long mk_phys_cpu_t;typedef uint32_t u32;
#define MK_STATE_ACTIVE 1
#define MK_MSG_RESOURCE 2
#define MK_RES_CPU_REMOVE 3
#define MK_RES_CPU_ADD 4
#define CHECK(c) do{if(!(c)){fprintf(stderr,"CPU ownership line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
struct mk_cpu_set {unsigned int nr,cap;mk_phys_cpu_t *ids;};
struct mk_instance {int id,state;unsigned long long generation;struct mk_cpu_set *cpus;};
struct mk_cpu_resource_payload {mk_phys_cpu_t cpu_id;u32 numa_node,flags;int sender_instance_id;};
struct mk_pending_msg {int unused;};
static mk_phys_cpu_t cpu=8,pool_ids[8],target_ids[8];
static struct mk_cpu_set pool,target_set,*mk_cpu_pool;
static struct mk_instance root,target,*root_instance;
static struct mk_pending_msg pending;
static int refs,finds,effects,repark_peer,repark_host,sends,core_calls,self_calls;
static int target_reserves,pool_reserves,fail_target_reserve,fail_pool_reserve;
static int fail_target_track,fail_pool_track,fail_pending,fail_send,ack_error,fail_peer,fail_host,pending_live;
static void logger(const char *format,...){(void)format;}
#define pr_err logger
#define pr_warn logger
static bool mk_cpu_set_contains(const struct mk_cpu_set *set,mk_phys_cpu_t id)
{if(!set)return false;for(unsigned i=0;i<set->nr;i++)if(set->ids[i]==id)return true;return false;}
static int mk_cpu_set_reserve(struct mk_cpu_set *set,unsigned int extra)
{CHECK(set);if(set==&target_set){target_reserves++;if(fail_target_reserve)return -ENOMEM;}else{CHECK(set==&pool);pool_reserves++;if(fail_pool_reserve)return -ENOMEM;}if(set->cap<set->nr+extra)set->cap=set->nr+extra;CHECK(set->cap<=8);return 0;}
static int mk_cpu_set_add(struct mk_cpu_set *set,mk_phys_cpu_t id)
{CHECK(set);if((set==&target_set&&fail_target_track)||(set==&pool&&fail_pool_track))return -ENOMEM;if(mk_cpu_set_contains(set,id))return 0;if(set->nr==set->cap){int ret=mk_cpu_set_reserve(set,1);if(ret)return ret;}set->ids[set->nr++]=id;return 0;}
static bool mk_cpu_set_del(struct mk_cpu_set *set,mk_phys_cpu_t id)
{CHECK(set);for(unsigned i=0;i<set->nr;i++)if(set->ids[i]==id){memmove(&set->ids[i],&set->ids[i+1],(--set->nr-i)*sizeof(id));return true;}return false;}
static struct mk_instance *mk_instance_find(int id){finds++;if(id!=7)return NULL;refs++;return &target;}
static void mk_instance_put(struct mk_instance *instance){CHECK(instance==&target&&refs>0);refs--;}
static int mk_pool_cpu_add(mk_phys_cpu_t id){CHECK(id==cpu);self_calls++;return 0;}
static int mk_pool_cpu_remove(mk_phys_cpu_t id,u32 node,u32 flags){CHECK(id==cpu&&node==2&&flags==16);self_calls++;return 0;}
static int mk_do_cpu_remove(mk_phys_cpu_t id){CHECK(id==cpu);self_calls++;return 0;}
static int mk_do_cpu_add(mk_phys_cpu_t id,u32 node,u32 flags){CHECK(id==cpu&&node==2&&flags==16);self_calls++;return 0;}
static int mk_repark_cpu_to_instance(struct mk_instance *instance,mk_phys_cpu_t id)
{CHECK(instance==&target&&id==cpu);effects++;repark_peer++;return fail_peer?-EIO:0;}
static int mk_repark_cpu_to_host(struct mk_instance *instance,mk_phys_cpu_t id)
{CHECK(instance==&target&&id==cpu);effects++;repark_host++;return fail_host?-EIO:0;}
static int mk_instance_transfer_cpus(struct mk_instance *instance,const struct mk_cpu_set *cpus)
{CHECK(instance==&target&&cpus->nr==1&&cpus->ids[0]==cpu);core_calls++;if(!mk_cpu_set_contains(mk_cpu_pool,cpu))return -EBUSY;int ret=mk_cpu_set_reserve(instance->cpus,1);if(ret)return ret;CHECK(!mk_cpu_set_add(instance->cpus,cpu));CHECK(mk_cpu_set_del(mk_cpu_pool,cpu));return 0;}
static int mk_instance_return_cpus(struct mk_instance *instance,const struct mk_cpu_set *cpus)
{CHECK(instance==&target&&cpus->nr==1&&cpus->ids[0]==cpu);core_calls++;if(!mk_cpu_set_contains(instance->cpus,cpu))return -EINVAL;int ret=mk_cpu_set_reserve(mk_cpu_pool,1);if(ret)return ret;CHECK(!mk_cpu_set_add(mk_cpu_pool,cpu));CHECK(mk_cpu_set_del(instance->cpus,cpu));return 0;}
static struct mk_pending_msg *mk_msg_pending_add(int cls,int operation,mk_phys_cpu_t id)
{CHECK(cls==MK_MSG_RESOURCE&&(operation==MK_RES_CPU_ADD||operation==MK_RES_CPU_REMOVE)&&id==cpu);if(fail_pending)return NULL;CHECK(!pending_live);pending_live=1;return &pending;}
static int mk_msg_pending_wait(struct mk_pending_msg *p,int timeout)
{CHECK(p==&pending&&pending_live&&(timeout==0||timeout==10000));pending_live=0;return timeout?ack_error:0;}
static int mk_send_message(int id,int cls,int operation,const void *data,size_t size)
{const struct mk_cpu_resource_payload *p=data;CHECK(id==7&&cls==MK_MSG_RESOURCE&&size==sizeof(*p)&&p->cpu_id==cpu&&p->sender_instance_id==0);if(operation==MK_RES_CPU_ADD)CHECK(p->numa_node==2&&p->flags==16);else CHECK(operation==MK_RES_CPU_REMOVE&&!p->numa_node&&!p->flags);effects++;sends++;return fail_send?-EIO:0;}
''' + function('mk_send_cpu_add') + function('mk_send_cpu_remove') + r'''
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
static void refused(int ret,int expected){CHECK(ret==expected&&!effects&&!core_calls&&!self_calls);}
static void ownership(void)
{for(int state=0;state<=1;state++){reset(false,state);target_set.nr=1;refused(call(false,7),-EINVAL);}puts("CPU_WRONG_OWNER_REFUSED_BEFORE_REPARK_PASS");}
static void capacity(void)
{reset(true,MK_STATE_ACTIVE);fail_target_reserve=1;refused(call(true,7),-ENOMEM);for(int state=0;state<=1;state++)for(int add=0;add<2;add++){reset(add,state);if(add)fail_target_reserve=1;else fail_pool_reserve=1;refused(call(add,7),-ENOMEM);}puts("CPU_DESTINATION_CAPACITY_BEFORE_CONTEXT_OR_IPI_PASS");}
static void tracking(void)
{
 reset(true,MK_STATE_ACTIVE);fail_target_track=1;CHECK(call(true,7)==-EUCLEAN&&!mk_cpu_set_contains(&pool,cpu)&&!mk_cpu_set_contains(&target_set,cpu)&&sends==1&&!repark_host);
 reset(false,MK_STATE_ACTIVE);fail_pool_track=1;CHECK(call(false,7)==-EUCLEAN&&!mk_cpu_set_contains(&pool,cpu)&&mk_cpu_set_contains(&target_set,cpu)&&repark_host==1);
 puts("CPU_TRACKING_FAILURE_NO_FALSE_SUCCESS_OR_FREE_POOL_ALIAS_PASS");
}
static int all(void)
{
 for(int state=0;state<=1;state++)for(int add=0;add<2;add++){
  reset(add,state);root_instance=NULL;refused(call(add,7),-ENODEV);CHECK(!finds);
  reset(add,state);refused(call(add,99),-ENODEV);
  reset(add,state);mk_cpu_pool=NULL;refused(call(add,7),-ENODEV);
  reset(add,state);target.cpus=NULL;refused(call(add,7),-ENODEV);
  reset(add,state);if(add)pool.nr=0;else target_set.nr=1;refused(call(add,7),add?-EBUSY:-EINVAL);
  reset(add,state);if(add)target_ids[target_set.nr++]=cpu;else{pool.nr=pool.cap=1;pool_ids[0]=cpu;}refused(call(add,7),-EUCLEAN);
  reset(add,state);if(add)fail_target_reserve=1;else fail_pool_reserve=1;
  refused(call(add,7),-ENOMEM);
  reset(add,state);CHECK(!call(add,7)&&mk_cpu_set_contains(&pool,cpu)==!add&&mk_cpu_set_contains(&target_set,cpu)==add);
  if(state==MK_STATE_ACTIVE)CHECK(sends==1);else CHECK(core_calls==1&&!sends);
  reset(add,state);CHECK(!call(add,0)&&self_calls==1&&!finds);
  reset(add,state);mk_cpu_pool=NULL;CHECK(!call(add,0)&&self_calls==1&&!finds);
 }
 reset(true,MK_STATE_ACTIVE);fail_target_track=1;CHECK(call(true,7)==-EUCLEAN&&!mk_cpu_set_contains(&pool,cpu)&&!mk_cpu_set_contains(&target_set,cpu)&&sends==1&&!repark_host);
 reset(false,MK_STATE_ACTIVE);fail_pool_track=1;CHECK(call(false,7)==-EUCLEAN&&!mk_cpu_set_contains(&pool,cpu)&&mk_cpu_set_contains(&target_set,cpu)&&repark_host==1);
 for(int add=0;add<2;add++)for(int fault=0;fault<4;fault++){
  reset(add,MK_STATE_ACTIVE);if(fault==0)fail_pending=1;if(fault==1)fail_send=1;if(fault==2)ack_error=-ETIMEDOUT;if(fault==3){if(add)fail_peer=1;else fail_host=1;}
  CHECK(call(add,7)<0&&mk_cpu_set_contains(&pool,cpu)==add&&mk_cpu_set_contains(&target_set,cpu)==!add);
 }
 cpu=(1ULL<<40)|8;reset(true,MK_STATE_ACTIVE);CHECK(!call(true,7)&&mk_cpu_set_contains(&target_set,cpu));reset(false,MK_STATE_ACTIVE);CHECK(!call(false,7)&&mk_cpu_set_contains(&pool,cpu));
 puts("CPU_MOVE_OWNERSHIP_AND_PREALLOCATED_TRACKING_PASS active_ready_self_routes=true wrong_owner_no_repark=true allocation_failure_before_effects=true unexpected_tracking_requires_reconciliation=true backend_failure_accounting_preserved=true");
 return 0;
}
int main(int argc,char **argv){CHECK(argc==2);if(!strcmp(argv[1],"ownership"))ownership();else if(!strcmp(argv[1],"capacity"))capacity();else if(!strcmp(argv[1],"tracking"))tracking();else return all();return 0;}
'''
# Support the correlated API while retaining historical candidate controls.
if '&payload.request_id' in s:
    code = code.replace('struct mk_cpu_resource_payload {',
                        'struct mk_request_id {unsigned long long epoch,sequence;};\n'
                        '#define ERR_PTR(e) ((void *)(intptr_t)(e))\n'
                        '#define PTR_ERR(p) ((long)(intptr_t)(p))\n'
                        '#define IS_ERR(p) ((uintptr_t)(p)>=(uintptr_t)-4095)\n'
                        'struct mk_cpu_resource_payload {struct mk_request_id request_id;')
    code = code.replace('int operation,mk_phys_cpu_t id)\n{CHECK(cls==MK_MSG_RESOURCE',
                        'int operation,mk_phys_cpu_t id,struct mk_request_id *request)\n'
                        '{request->epoch=7;request->sequence=9;CHECK(cls==MK_MSG_RESOURCE')
    code = code.replace('if(fail_pending)return NULL;', 'if(fail_pending)return ERR_PTR(-ENOMEM);')

with tempfile.TemporaryDirectory(prefix='mk-cpu-owners-') as directory:
    root = Path(directory)
    (root/'probe.c').write_text(code)
    subprocess.run(['cc','-std=gnu11','-Wall','-Wextra','-Werror',
                    '-fsanitize=undefined','-fno-sanitize-recover=all',
                    str(root/'probe.c'),'-o',str(root/'probe')],check=True)
    subprocess.run([str(root/'probe'),a.section],check=True)
