#!/usr/bin/env python3
"""Model post-SMP inventory synchronization and the real strict pool claim."""
import argparse,ast,sys,subprocess,tempfile
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__);p.add_argument('directory',type=Path);p.add_argument('--old',action='store_true');a=p.parse_args();w=a.directory
fixture=Path(__file__).with_name('test-kernel-cpu-pool-serialization.py');tree=ast.parse(fixture.read_text());body=tree.body[:next(i for i,n in enumerate(tree.body) if isinstance(n,ast.With))];argv=sys.argv;sys.argv=[str(fixture),str(w)];ns={'__name__':'inventory_fixture'}
try:exec(compile(ast.Module(body=body,type_ignores=[]),str(fixture),'exec'),ns)
finally:sys.argv=argv
code=ns['code'];code=code[:code.index('static int effects,backend_error,tracking_error;')]
code=code.replace('#define GFP_KERNEL 0', '#define GFP_KERNEL 0\n#define CONFIG_X86 1\n#define IS_ENABLED(x) (x)\n#define __init\n#define late_initcall(fn)')
at=code.index('DEFINE_MUTEX(mk_cpu_ownership_mutex);');code=code[:at]+r'''
#include <setjmp.h>
static unsigned long online_mask;static bool sparse;static int read_lock;static jmp_buf panic_target;static bool panicked;
static void cpus_read_lock(void){CHECK(!held&&!read_lock);read_lock=1;}
static void cpus_read_unlock(void){CHECK(!held&&read_lock);read_lock=0;}
#define for_each_online_cpu(c) for((c)=0;(c)<16;(c)++)if(online_mask&(1UL<<(c)))
static mk_phys_cpu_t arch_cpu_physical_id(int cpu){CHECK(read_lock&&held);return sparse?0x100000000ULL+(unsigned)cpu*2:(unsigned)cpu;}
static void panic(const char *fmt,...) __attribute__((noreturn));
static void panic(const char *fmt,...){CHECK(fmt);panicked=true;longjmp(panic_target,1);}
'''+code[at:]
# Match the kernel build's inline-address warning behavior for the shared fixture macro.
code+=r'''
static struct mk_cpu_set host={0};
static void reset(unsigned long mask,bool high){CHECK(!held&&!read_lock);online_mask=mask;sparse=high;panicked=false;alloc_fail=0;root.cpus=&host;mutex_lock(&mk_cpu_ownership_mutex);host.nr=pool.nr=0;while(mk_cpu_pool_transitions.next!=&mk_cpu_pool_transitions){struct mk_cpu_pool_transition *op=list_entry(mk_cpu_pool_transitions.next,struct mk_cpu_pool_transition,list);list_del(&op->list);free(op);}mutex_unlock(&mk_cpu_ownership_mutex);mk_pool_park_enable();}
static void synchronize(void){
'''+('/* Predecessor has no post-SMP synchronization call. */' if a.old else 'CHECK(!mk_root_cpu_ownership_init());')+r'''
}
int main(void){
 reset(0xffff,false);mutex_lock(&mk_cpu_ownership_mutex);CHECK(!mk_cpu_set_add(root.cpus,0));mutex_unlock(&mk_cpu_ownership_mutex);CHECK(mk_cpu_pool_begin(8,false)==-EBUSY);synchronize();CHECK(!held&&!read_lock);for(int cpu=8;cpu<16;cpu++){CHECK(!mk_cpu_pool_begin((mk_phys_cpu_t)cpu,false));}CHECK(host.nr==16);puts("ROOT_POST_SMP_SIXTEEN_ONLINE_CPUS_ENABLE_STRICT_EIGHT_CPU_POOL_CLAIMS_PASS");
 reset((1UL<<2)|(1UL<<7),true);mutex_lock(&mk_cpu_ownership_mutex);CHECK(!mk_cpu_set_add(root.cpus,0x100000004ULL));mutex_unlock(&mk_cpu_ownership_mutex);synchronize();CHECK(host.nr==2&&!held&&!read_lock);CHECK(!mk_cpu_pool_begin(0x10000000eULL,false));CHECK(mk_cpu_pool_begin(0x100000010ULL,false)==-EBUSY);puts("ROOT_SPARSE_PHYSICAL_IDS_ONLY_ONLINE_CPUS_ADMITTED_PASS");
 reset(0xffff,false);mutex_lock(&mk_cpu_ownership_mutex);free(host.ids);host=(struct mk_cpu_set){0};mutex_unlock(&mk_cpu_ownership_mutex);alloc_fail=1;if(!setjmp(panic_target)){mk_root_cpu_ownership_init();CHECK(false);}CHECK(panicked&&!held&&!read_lock&&!host.nr);root.cpus=NULL;panicked=false;if(!setjmp(panic_target)){mk_root_cpu_ownership_init();CHECK(false);}CHECK(panicked&&!held&&!read_lock);root.cpus=&host;puts("ROOT_MISSING_POLICY_AND_ALLOCATION_FAILURE_REFUSE_INCOMPLETE_BOOT_PASS");free(host.ids);free(pool.ids);return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-root-cpu-sync-') as td:
 source=Path(td)/'test.c';exe=Path(td)/'test';source.write_text(code)
 subprocess.run(['gcc','-std=gnu11','-pthread','-Wall','-Wextra','-Werror','-Wno-unused-function','-Wno-unused-parameter','-Wno-unused-variable','-fsanitize=undefined','-fno-sanitize-recover=all',str(source),'-o',str(exe)],check=True)
 print('ROOT_CPU_SYNC_AND_STRICT_POOL_CLAIM_ACTUAL_FUNCTIONS_COMPILED',flush=True);subprocess.run([str(exe)],check=True)
