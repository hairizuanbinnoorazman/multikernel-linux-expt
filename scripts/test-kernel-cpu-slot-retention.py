#!/usr/bin/env python3
"""Run actual per-CPU/bulk/boot park publication with modeled wake results."""
import argparse
from pathlib import Path
import subprocess,tempfile
p=argparse.ArgumentParser(description=__doc__);p.add_argument('source',type=Path);a=p.parse_args();s=a.source.read_text()
def block(prefix):
 at=s.index(prefix);return s[at:s.index('\n}',at)+2]+'\n'
code=r'''
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
typedef unsigned long long u64;typedef uint32_t u32;typedef u64 mk_phys_cpu_t;
#define CHECK(c) do{if(!(c)){fprintf(stderr,"CPU slot retention line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
#define MK_CPU_TRANSFER_MOVING 1
#define MK_CPU_TRANSFER_RELEASING 3
#define READ_ONCE(x) (x)
#define MK_CPU_TRANSFER_STABLE 0
#define MK_CPU_TRANSFER_UNCERTAIN 2
#define MK_SPAWN_F_REPARK 4
#define WRITE_ONCE(x,v) ((x)=(v))
#define guard(name) model_guard
#define lockdep_assert_held(p) ((void)(p))
static int mk_park_mutex;
static void model_guard(void *p){CHECK(p==&mk_park_mutex);}
struct mk_cpu_set {unsigned nr,cap;u64 ids[8];};
struct mk_spawn_context {unsigned long park_phys,park_cr3,identity_cr3,kernel_entry,trampoline_phys,self_phys,repark_park_phys,repark_park_cr3,repark_slot_phys;u32 flags;};
struct mk_instance {int id,cpu_transfer_state;char *name;void *ident_pgt,*trampoline_va,*park_va;u64 spawn_ctx_phys;struct mk_spawn_context *spawn_ctx;struct mk_cpu_set *cpus,*cpus_on_slot;};
static struct {struct mk_spawn_context *slot;unsigned long park_phys,cr3,slot_phys;} mk_host_park;
static unsigned mk_cpu_set_count(struct mk_cpu_set *s){return s?s->nr:0;}
#define mk_cpu_set_for_each(i,id,s) for((i)=0;(s)&&(i)<(s)->nr&&(((id)=(s)->ids[i]),1);(i)++)
static bool mk_cpu_set_contains(struct mk_cpu_set *s,u64 id){if(!s)return false;for(unsigned i=0;i<s->nr;i++)if(s->ids[i]==id)return true;return false;}
static int fail_reserve,fail_add,effects,fail_cpu;
static struct mk_cpu_set *mk_cpu_set_alloc(void){return calloc(1,sizeof(struct mk_cpu_set));}
static int mk_cpu_set_reserve(struct mk_cpu_set *s,unsigned n){if(fail_reserve)return -ENOMEM;if(s->nr+n>s->cap)s->cap=s->nr+n;CHECK(s->cap<=8);return 0;}
static int mk_cpu_set_add(struct mk_cpu_set *s,u64 id){CHECK(s);if(fail_add)return -ENOMEM;if(mk_cpu_set_contains(s,id))return 0;CHECK(s->nr<s->cap);s->ids[s->nr++]=id;return 0;}
static unsigned long virt_to_phys(void *p){CHECK(p);return 4096;}
static u32 cpu_map[2]={8,10};
#define x86_cpu_to_apicid cpu_map
#define per_cpu(map,cpu) ((map)[cpu])
static int confirm_error,arch_frees;
static bool mk_cpu_set_empty(struct mk_cpu_set *set){return !set||!set->nr;}
static void mk_cpu_set_free(struct mk_cpu_set *set){free(set);arch_frees++;}
static void mk_cpu_set_del(struct mk_cpu_set *set,u64 id){for(unsigned i=0;set&&i<set->nr;i++)if(set->ids[i]==id){memmove(&set->ids[i],&set->ids[i+1],(--set->nr-i)*sizeof(id));return;}}
static void mk_cpu_set_format(char *out,size_t n,struct mk_cpu_set *set){CHECK(n>1&&set);out[0]=0;}
static void mk_free_identity_pgtable(void *p){CHECK(p);arch_frees++;}
static int mk_instance_confirm_parked(struct mk_instance *i){CHECK(i);return confirm_error;}
static struct mk_instance instance;
static int mk_slot_wake(struct mk_spawn_context *ctx,u32 cpu,const char *what)
{CHECK(ctx&&what);CHECK(mk_cpu_set_contains(instance.cpus_on_slot,cpu));effects++;return (int)cpu==fail_cpu?-ETIMEDOUT:0;}
static void logger(const char *format,...){(void)format;}
#define pr_err logger
'''
for name,prefix in [('mk_slot_set','static struct mk_cpu_set *'),('mk_publish_repark','static int '),('mk_repark_to_instance','static int '),('mk_spawn_cpu','int '),('mk_repark_cpu_to_instance','int ')]:code+=block(prefix+name+'(')
code+=block('int mk_repark_instance_to_host(')+block('int mk_arch_release_instance(')
code+=r'''
static struct mk_spawn_context host,child;
static struct mk_cpu_set owner;
static void reset(void)
{free(instance.cpus_on_slot);memset(&instance,0,sizeof(instance));host=(struct mk_spawn_context){.park_phys=8192};child=(struct mk_spawn_context){.park_phys=16384,.park_cr3=24576};owner=(struct mk_cpu_set){.nr=2,.cap=8,.ids={8,10}};instance=(struct mk_instance){.id=7,.spawn_ctx_phys=4096,.spawn_ctx=&child,.cpus=&owner};mk_host_park.slot=&host;mk_host_park.park_phys=8192;mk_host_park.cr3=24576;mk_host_park.slot_phys=32768;confirm_error=arch_frees=0;fail_reserve=fail_add=effects=0;fail_cpu=-1;}
int main(void)
{
 reset();fail_cpu=12;CHECK(mk_repark_cpu_to_instance(&instance,12)==-ETIMEDOUT&&effects==1&&mk_cpu_set_contains(instance.cpus_on_slot,12)&&instance.cpu_transfer_state==MK_CPU_TRANSFER_UNCERTAIN);
 reset();CHECK(!mk_repark_cpu_to_instance(&instance,12)&&effects==1&&mk_cpu_set_contains(instance.cpus_on_slot,12));
 reset();fail_reserve=1;CHECK(mk_repark_cpu_to_instance(&instance,12)==-ENOMEM&&!effects);
 reset();fail_add=1;CHECK(mk_repark_cpu_to_instance(&instance,12)==-ENOMEM&&!effects);
 puts("CPU_SLOT_HOTADD_TRACKED_BEFORE_EFFECT_AND_RETAINED_ON_TIMEOUT_PASS");
 reset();owner.nr=3;owner.ids[2]=12;fail_cpu=12;CHECK(mk_repark_to_instance(&instance,&child,8)==-ETIMEDOUT&&effects==2&&mk_cpu_set_contains(instance.cpus_on_slot,10)&&mk_cpu_set_contains(instance.cpus_on_slot,12)&&instance.cpu_transfer_state==MK_CPU_TRANSFER_UNCERTAIN);
 reset();fail_cpu=8;CHECK(mk_spawn_cpu(&instance,0,&child)==-ETIMEDOUT&&effects==2&&mk_cpu_set_contains(instance.cpus_on_slot,8)&&mk_cpu_set_contains(instance.cpus_on_slot,10)&&instance.cpu_transfer_state==MK_CPU_TRANSFER_UNCERTAIN);
 reset();CHECK(!mk_spawn_cpu(&instance,0,&child)&&effects==2&&mk_cpu_set_contains(instance.cpus_on_slot,8)&&mk_cpu_set_contains(instance.cpus_on_slot,10));
 fail_cpu=8;CHECK(mk_spawn_cpu(&instance,0,&child)==-ETIMEDOUT&&instance.cpu_transfer_state==MK_CPU_TRANSFER_UNCERTAIN);
 free(instance.cpus_on_slot);instance.cpus_on_slot=NULL;puts("CPU_SLOT_BULK_BOOT_AND_RESPAWN_PUBLICATION_RETENTION_PASS");
 reset();CHECK(!mk_spawn_cpu(&instance,0,&child));instance.ident_pgt=(void *)1;instance.trampoline_va=(void *)2;instance.park_va=(void *)3;instance.cpu_transfer_state=MK_CPU_TRANSFER_UNCERTAIN;
 confirm_error=-EBUSY;int old=effects;CHECK(mk_arch_release_instance(&instance)==-EBUSY&&effects==old&&!arch_frees&&instance.ident_pgt&&instance.spawn_ctx);
 confirm_error=0;fail_cpu=10;CHECK(mk_arch_release_instance(&instance)==-ETIMEDOUT&&!arch_frees&&instance.spawn_ctx&&instance.ident_pgt&&mk_cpu_set_contains(instance.cpus_on_slot,10));
 mk_host_park.slot=NULL;CHECK(mk_arch_release_instance(&instance)==-ENODEV&&!arch_frees&&instance.spawn_ctx);mk_host_park.slot=&host;
 fail_cpu=-1;instance.cpu_transfer_state=MK_CPU_TRANSFER_RELEASING;CHECK(!mk_arch_release_instance(&instance)&&arch_frees==2&&!instance.cpus_on_slot&&!instance.spawn_ctx&&!instance.ident_pgt&&instance.cpu_transfer_state==MK_CPU_TRANSFER_RELEASING);
 puts("CPU_ARCH_RELEASE_PROOF_PARTIAL_FAILURE_AND_MISSING_CONTEXT_RETENTION_PASS");return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-cpu-slot-') as td:
 f=Path(td)/'test.c';exe=Path(td)/'test';f.write_text(code)
 subprocess.run(['gcc','-std=gnu11','-Wall','-Wextra','-Werror','-Wno-unused-function','-fsanitize=undefined','-fno-sanitize-recover=all',str(f),'-o',str(exe)],check=True)
 print('CPU_SLOT_ACTUAL_PUBLISHERS_COMPILED',flush=True);subprocess.run([str(exe)],check=True)
