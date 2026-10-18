#!/usr/bin/env python3
"""Exercise extracted CPU-policy preparation with modeled topology and masks.

Both off-stack pointer and inline-array cpumask_var_t forms are compiled. This
is not a CPU hotplug or physical topology test.
"""
from pathlib import Path
import argparse,subprocess,tempfile
p=argparse.ArgumentParser(description=__doc__);p.add_argument('source',type=Path);a=p.parse_args()
s=a.source.read_text();at=s.index('static int __init mk_prepare_instance_cpus(');actual=s[at:s.index('\n}',at)+2]
code=r'''
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>
#include <string.h>
#include <errno.h>
#define __init
#define GFP_KERNEL 0
#define CHECK(x) do{if(!(x)){fprintf(stderr,"CPU policy line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
typedef uint64_t mk_phys_cpu_t;
struct mk_cpu_set {unsigned n;uint64_t ids[4];};
struct mk_dt_config {struct mk_cpu_set *cpus;};
struct cpumask {unsigned bits;};
#if OFFSTACK
typedef struct cpumask *cpumask_var_t;
#else
typedef struct cpumask cpumask_var_t[1];
#endif
static struct cpumask __cpu_present_mask={15};
static unsigned nr_cpu_ids=8,boot_cpu;
static int fail_alloc,alloc_calls,live;
static void log_stub(const char *fmt,...){(void)fmt;}
#define pr_err log_stub
static bool mk_cpu_set_empty(const struct mk_cpu_set *p){return !p||!p->n;}
#define mk_cpu_set_for_each(i,p,set) for(i=0;i<(set)->n&&((p=(set)->ids[i]),1);i++)
static bool alloc_cpumask_var(cpumask_var_t *p,int flags){(void)flags;alloc_calls++;if(fail_alloc)return false;
#if OFFSTACK
 *p=malloc(sizeof(**p));CHECK(*p);live++;
#endif
 memset(*p,0,sizeof(struct cpumask));return true;}
static void free_cpumask_var(cpumask_var_t p){
#if OFFSTACK
 free(p);CHECK(live==1);live--;
#else
 (void)p;
#endif
}
static void cpumask_clear(struct cpumask *p){p->bits=0;}
static bool cpumask_test_cpu(unsigned cpu,const struct cpumask *p){CHECK(cpu<8);return !!(p->bits&(1u<<cpu));}
static void cpumask_set_cpu(unsigned cpu,struct cpumask *p){CHECK(cpu<8);p->bits|=1u<<cpu;}
static int arch_cpu_from_physical_id(uint64_t p){switch(p){case 8:return 0;case 10:return 1;case 12:return 0;case 14:return 8;case 16:return 4;case 18:return -2;case 22:return 7;default:return -1;}}
static bool cpu_present(unsigned cpu){return cpumask_test_cpu(cpu,&__cpu_present_mask);}
static unsigned raw_smp_processor_id(void){return boot_cpu;}
'''+actual+r'''
static void check(struct mk_dt_config *config,int expected,unsigned mask){cpumask_var_t prepared;unsigned old=__cpu_present_mask.bits;CHECK(mk_prepare_instance_cpus(config,&prepared)==expected);CHECK(__cpu_present_mask.bits==old);if(!expected){CHECK(prepared->bits==mask);free_cpumask_var(prepared);}CHECK(!live);}
int main(void){
 struct mk_cpu_set cpus={2,{8,10}};struct mk_dt_config config={&cpus};
 check(NULL,-EINVAL,0);struct mk_dt_config missing={NULL};check(&missing,-EINVAL,0);cpus.n=0;check(&config,-EINVAL,0);CHECK(!alloc_calls);
 cpus.n=2;fail_alloc=1;check(&config,-ENOMEM,0);fail_alloc=0;check(&config,0,3);
 const uint64_t bad[]={11,14,16,18,UINT64_MAX,12,8};
 for(unsigned i=0;i<sizeof(bad)/sizeof(bad[0]);i++){cpus.ids[1]=bad[i];check(&config,-EINVAL,0);}
 cpus.n=1;cpus.ids[0]=10;check(&config,-EINVAL,0);cpus.ids[0]=8;check(&config,0,1);boot_cpu=1;check(&config,-EINVAL,0);
 boot_cpu=0;cpus.n=2;cpus.ids[1]=22;__cpu_present_mask.bits=255;check(&config,0,129);
 printf("BOOT_CPU_POLICY_VALIDATION_AND_NO_PREPARATION_SIDE_EFFECTS_PASS offstack=%d\n",OFFSTACK);return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-boot-cpus-') as tmp:
    tmp=Path(tmp);(tmp/'test.c').write_text(code)
    for offstack in [0,1]:
        subprocess.run(['gcc','-std=gnu11','-O1','-Wall','-Wextra','-Werror','-fsanitize=undefined','-fno-sanitize-recover=all',f'-DOFFSTACK={offstack}',str(tmp/'test.c'),'-o',str(tmp/'test')],check=True)
        subprocess.run([str(tmp/'test')],check=True)
