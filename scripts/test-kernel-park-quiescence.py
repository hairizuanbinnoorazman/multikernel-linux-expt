#!/usr/bin/env python3
"""Exercise actual park setup/selection/teardown and hotplug with modeled physical backends."""
import argparse,ast,sys,subprocess,tempfile
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__);p.add_argument('directory',type=Path);p.add_argument('--old-teardown',action='store_true');p.add_argument('--old-selector',action='store_true');a=p.parse_args();w=a.directory
# Reuse the preceding fixture's small CPU-set/list/allocator definitions.
fixture=Path(__file__).with_name('test-kernel-cpu-pool-serialization.py');tree=ast.parse(fixture.read_text());body=tree.body[:next(i for i,n in enumerate(tree.body) if isinstance(n,ast.With))];argv=sys.argv;sys.argv=[str(fixture),str(w)];ns={'__name__':'park_fixture'}
try:exec(compile(ast.Module(body=body,type_ignores=[]),str(fixture),'exec'),ns)
finally:sys.argv=argv
block=ns['block'];code=ns['code'];code=code[:code.index('static void clear_ops')]
core=(w/'core.c').read_text();arch=(w/'spawn.c').read_text();hotplug=(w/'hotplug.c').read_text()
code=code.replace('#define GFP_KERNEL 0','#define GFP_KERNEL 0\n#define CONFIG_X86 1\n#define IS_ENABLED(x) (x)')
code=code.replace('static __thread int held;', 'static __thread int held;static __thread struct mutex *lock_stack[8];')
code=code.replace('CHECK(!held);CHECK(!pthread_mutex_lock(&m->value));held=1;', 'for(int i=0;i<held;i++){CHECK(lock_stack[i]!=m);}CHECK(held<8);CHECK(!pthread_mutex_lock(&m->value));lock_stack[held++]=m;')
code=code.replace('CHECK(held);held=0;CHECK(!pthread_mutex_unlock(&m->value));','CHECK(held&&lock_stack[held-1]==m);held--;CHECK(!pthread_mutex_unlock(&m->value));')
code=code.replace('#define lockdep_assert_held(p) CHECK(held && (p)==&mk_cpu_ownership_mutex)', 'static bool owns(struct mutex *m){for(int i=0;i<held;i++){if(lock_stack[i]==m)return true;}return false;}\n#define lockdep_assert_held(p) CHECK(owns(p))')
code=code.replace('#define guard(type) struct mutex *guard_lock __attribute__((cleanup(lock_release)))=lock_acquire', '#define GUARD_NAME_I(a,b) a##b\n#define GUARD_NAME(a,b) GUARD_NAME_I(a,b)\n#define guard(type) struct mutex *GUARD_NAME(guard_lock,__COUNTER__) __attribute__((cleanup(lock_release)))=lock_acquire')
code=code.replace('static void list_del(', 'static bool list_empty(struct list_head *h){return h->next==h;}\nstatic void list_del(')
code=code.replace('struct mk_instance {int id;', 'struct mk_instance {struct list_head list;int id;')
# Replace only modeled hotplug helpers; retain actual pool wrappers.
for prefix in ['static int mk_do_cpu_remove(', 'static int mk_do_cpu_add(']:
 start=code.index(prefix);end=code.index('\n',start);code=code[:start]+code[end+1:]
code=code.replace('static bool cpu_online(int cpu){return cpu==8;}', 'static bool online=true;static int cpu_read_depth;static bool cpu_online(int cpu){CHECK(cpu==8);return online;}')
code=code.replace('static void mk_set_pool_cpu(int cpu,bool b){CHECK(cpu==8);(void)b;}', 'static bool pool_flag;static void mk_set_pool_cpu(int cpu,bool b){CHECK(cpu==8);pool_flag=b;}')
code=code.replace('static int effects,backend_error,tracking_error;', '''static int effects,backend_error,tracking_error;
static int mk_do_cpu_add(mk_phys_cpu_t,u32,u32);
static int mk_do_cpu_remove(mk_phys_cpu_t);''')
code+=r'''
typedef unsigned long phys_addr_t;typedef unsigned long long u64;typedef unsigned short u16;typedef unsigned char u8;
#define PAGE_SIZE 4096
#define NUMA_NO_NODE -1
#define ALIGN(n,a) (((n)+(a)-1)&~((a)-1))
#define __va(p) ((void *)(p))
#define WRITE_ONCE(p,v) ((p)=(v))
#define per_cpu(p,c) ((p)[c])
#define this_cpu_read(p) ((p)[8])
#define WARN_ON_ONCE(c) CHECK(!(c))
#define IS_ERR(p) ((intptr_t)(p)<0 && (intptr_t)(p)>-4096)
#define PTR_ERR(p) ((int)(intptr_t)(p))
static unsigned mk_cpu_set_empty(const struct mk_cpu_set *s){return !s||!s->nr;}
DEFINE_MUTEX(mk_instance_mutex);DEFINE_MUTEX(mk_host_park_lock);DEFINE_MUTEX(mk_cpu_hotplug_mutex);DEFINE_MUTEX(mk_hotplug_mutex);
static LIST_HEAD(mk_instance_list);static LIST_HEAD(mk_retained_instances);static LIST_HEAD(mk_hotplug_ops);static int mk_retained_instances_lock;
#define spin_lock_irqsave(p,f) do{CHECK((p)==&mk_retained_instances_lock);(f)=1;}while(0)
#define spin_unlock_irqrestore(p,f) CHECK((p)==&mk_retained_instances_lock&&(f))
'''
for prefix in ['static bool mk_pool_cpus_returned_locked(', 'bool mk_pool_cpus_returned(', 'int mk_pool_park_quiesce(']:code+=block(core,prefix)
code+=r'''
struct mk_spawn_context {unsigned long self_phys;};struct mk_ident_pgtable {int unused;};
static struct {struct mk_spawn_context *slot;unsigned long slot_phys;void *park_va;unsigned long park_phys;struct mk_ident_pgtable *pgt;unsigned long cr3;} mk_host_park;
static bool mk_cpu_uses_host_park[16],fallback;
static bool mk_cpu_parkable(void){return fallback;}
static int fault_stage,free_count,alloc_count,pgt_frees,park_calls,fallback_calls;static bool empty_pool,probe_claim;
static unsigned char park_code[64];
#define mk_pool_park_start park_code
#define mk_pool_park_end (park_code+sizeof(park_code))
static struct mk_ident_pgtable *mk_build_identity_pgtable(void *p){CHECK(!p&&owns(&mk_host_park_lock));return fault_stage==1?(void *)(intptr_t)-ENOMEM:calloc(1,sizeof(struct mk_ident_pgtable));}
static unsigned long mk_get_identity_cr3(struct mk_ident_pgtable *p){CHECK(p);return 0x1000;}
static bool mk_pool_empty(void){return empty_pool;}
static int mk_host_park_map_pool(struct mk_ident_pgtable *p){CHECK(p);return fault_stage==2?-ENOMEM:0;}
static unsigned long multikernel_alloc(size_t n,int node){CHECK(node==-1&&owns(&mk_host_park_lock));alloc_count++;if((alloc_count==1&&fault_stage==3)||(alloc_count==2&&fault_stage==6))return 0;return (unsigned long)calloc(1,n);}
static void multikernel_free(unsigned long p,size_t n){CHECK(p&&n&&owns(&mk_host_park_lock));if(probe_claim){CHECK(mk_cpu_pool_begin(0x100000008ULL,false)==-EBUSY);CHECK(mk_pool_park_pin(0x100000008ULL)==-EBUSY);}free((void *)p);free_count++;}
static void mk_free_identity_pgtable(struct mk_ident_pgtable *p){CHECK(p&&owns(&mk_host_park_lock));free(p);pgt_frees++;}
static int set_memory_x(unsigned long p,int n){CHECK(p&&n==1);return fault_stage==4?-ENOMEM:0;}
static int set_memory_nx(unsigned long p,int n){CHECK(p&&n==1);return 0;}
static int mk_add_trampoline_mapping(struct mk_ident_pgtable *p,unsigned long a,unsigned long b){CHECK(p&&a==b);return fault_stage==5?-ENOMEM:0;}
static int mk_ident_map_range(struct mk_ident_pgtable *p,unsigned long a,unsigned long b){CHECK(p&&a<b);return 0;}
#define mk_default_page_offset() 0UL
static unsigned read_apic_id(void){return 8;}
#define X86_FEATURE_MWAIT 1
static bool boot_cpu_has(int f){CHECK(f==1);return false;}
typedef void (*mk_pool_park_fn)(unsigned long,unsigned long,unsigned long,unsigned long,unsigned long);
static void mock_park(unsigned long cr3,unsigned long slot,unsigned long cpu,unsigned long phys,unsigned long mwait){CHECK(cr3==0x1000&&slot&&cpu==8&&phys&&!mwait);park_calls++;}
static void mk_park_cpu(void){fallback_calls++;}
'''
for prefix in ['static int __mk_setup_host_park(', 'int mk_setup_host_park(', 'int mk_arch_pool_chunk_added(', 'int mk_prepare_host_cpu_park(', 'void mk_release_host_cpu_park(']:code+=block(arch,prefix)
code+=block((w/'before-spawn.c').read_text() if a.old_teardown else arch,'int mk_teardown_host_park(')
# Replace the jump target in the fixture: the actual selection branch remains unchanged.
code+=block(arch,'void mk_pool_park_cpu(').replace('(mk_pool_park_fn)mk_host_park.park_va','mock_park')
code+=r'''
static void cpus_read_lock(void){CHECK(owns(&mk_cpu_hotplug_mutex));CHECK(!cpu_read_depth);cpu_read_depth=1;}
static void cpus_read_unlock(void){CHECK(cpu_read_depth==1);cpu_read_depth=0;}
struct cpu {bool hotpluggable;};static struct cpu cpu_devices[16];
static void set_cpu_present(int cpu,bool b){CHECK(cpu==8);(void)b;}
static struct cpu *get_cpu_device(int cpu){CHECK(cpu==8);return &cpu_devices[8];}
static int register_cpu(struct cpu *c,int cpu){CHECK(c&&cpu==8);return 0;}
static int add_cpu(int cpu){CHECK(cpu==8&&owns(&mk_cpu_hotplug_mutex));effects++;if(!tracking_error)online=true;return backend_error;}
static int remove_cpu(int cpu){CHECK(cpu==8&&owns(&mk_cpu_hotplug_mutex)&&pool_flag);CHECK(mk_host_park_pins.nr==1||fallback);effects++;if(!backend_error)online=false;return backend_error;}
'''+block(hotplug,'struct mk_hotplug_op {')+';\n'
for prefix in ['static int mk_account_cpu_online(', 'static void mk_account_cpu_offline(', 'static int mk_do_cpu_add(', 'static int mk_do_cpu_remove(']:code+=block((w/'old-selector-hotplug.c').read_text() if a.old_selector and prefix=='static int mk_account_cpu_online(' else hotplug,prefix)
code+=r'''
static void clear_list(struct list_head *h){while(h->next!=h){struct list_head *n=h->next;list_del(n);free(n);}}
static void clear_hotplug(void){while(!list_empty(&mk_hotplug_ops)){struct mk_hotplug_op *op=list_entry(mk_hotplug_ops.next,struct mk_hotplug_op,list);list_del(&op->list);free(op);}}
static void reset(void){CHECK(!held&&!cpu_read_depth);mutex_lock(&mk_cpu_ownership_mutex);clear_list(&mk_cpu_pool_transitions);pool.nr=root.cpus->nr=0;free(mk_host_park_pins.ids);mk_host_park_pins=(struct mk_cpu_set){0};CHECK(!mk_cpu_set_add(root.cpus,0x100000008ULL));mutex_unlock(&mk_cpu_ownership_mutex);CHECK(!mk_teardown_host_park());clear_hotplug();fault_stage=alloc_count=backend_error=tracking_error=effects=alloc_fail=0;online=true;fallback=pool_flag=probe_claim=empty_pool=false;mk_cpu_uses_host_park[8]=false;}
static void inspect_pin(void){struct seq_file sf={0};CHECK(!mk_cpu_pool_transitions_show(&sf,NULL));CHECK(strstr(sf.data,"park_pin_cpu=4294967304"));}
int main(void){struct mk_cpu_set host={0};root.cpus=&host;root.name="root";
 reset();CHECK(mk_cpu_pool_begin(0x100000008ULL,false)==-EBUSY);CHECK(!mk_setup_host_park()&&mk_pool_park_available&&mk_host_park.slot);probe_claim=true;CHECK(!mk_teardown_host_park()&&!mk_pool_park_available&&!mk_host_park.slot&&free_count==2&&pgt_frees==1);probe_claim=false;CHECK(mk_cpu_pool_begin(0x100000008ULL,false)==-EBUSY);CHECK(!mk_setup_host_park()&&mk_pool_park_available);CHECK(!mk_teardown_host_park());
 puts("PARK_CHECK_AND_ADMISSION_CLOSE_BEFORE_FIRST_FREE_AND_REOPEN_AFTER_SETUP_PASS");
 for(int f=1;f<=6;f++){reset();fault_stage=f;CHECK(mk_setup_host_park()==-ENOMEM&&!mk_pool_park_available&&!mk_host_park.slot);CHECK(mk_cpu_pool_begin(0x100000008ULL,false)==-EBUSY);fault_stage=0;alloc_count=0;CHECK(!mk_setup_host_park()&&mk_pool_park_available);CHECK(!mk_teardown_host_park());}
 puts("PARK_SETUP_FAILURES_KEEP_ADMISSION_CLOSED_AND_LATER_SUCCESS_REOPENS_PASS");
 reset();CHECK(!mk_setup_host_park());alloc_fail=1;CHECK(mk_prepare_host_cpu_park(0x100000008ULL,8)==-ENOMEM&&!mk_cpu_uses_host_park[8]&&!mk_host_park_pins.nr);CHECK(!mk_do_cpu_remove(0x100000008ULL)&&!online&&mk_host_park_pins.nr==1&&mk_cpu_uses_host_park[8]);inspect_pin();int before=free_count;CHECK(mk_teardown_host_park()==-EBUSY&&free_count==before&&mk_host_park.slot);mk_pool_park_cpu();CHECK(park_calls==1);CHECK(!mk_do_cpu_add(0x100000008ULL,0,0)&&online&&!pool_flag&&!mk_cpu_uses_host_park[8]&&!mk_host_park_pins.nr);CHECK(!mk_teardown_host_park());
 puts("PARK_ALL_LOCAL_REMOVE_CALLERS_PIN_BEFORE_EFFECT_AND_KEEP_AREA_UNTIL_ONLINE_PASS");
 reset();CHECK(!mk_setup_host_park());backend_error=-EIO;CHECK(mk_do_cpu_remove(0x100000008ULL)==-EIO&&online&&mk_host_park_pins.nr==1);CHECK(mk_teardown_host_park()==-EBUSY);backend_error=0;CHECK(!mk_do_cpu_add(0x100000008ULL,0,0)&&!mk_host_park_pins.nr);CHECK(!mk_teardown_host_park());
 reset();CHECK(!mk_setup_host_park());CHECK(!mk_do_cpu_remove(0x100000008ULL));tracking_error=1;CHECK(mk_do_cpu_add(0x100000008ULL,0,0)==-EUCLEAN&&mk_host_park_pins.nr==1);CHECK(mk_teardown_host_park()==-EBUSY);tracking_error=0;mutex_lock(&mk_cpu_ownership_mutex);free(host.ids);host=(struct mk_cpu_set){0};mutex_unlock(&mk_cpu_ownership_mutex);alloc_fail=1;CHECK(mk_do_cpu_add(0x100000008ULL,0,0)==-ENOMEM&&online&&mk_host_park_pins.nr==1);CHECK(!mk_do_cpu_add(0x100000008ULL,0,0)&&!mk_host_park_pins.nr);CHECK(!mk_teardown_host_park());
 puts("PARK_REMOVE_FAILURE_FALSE_ONLINE_AND_TRACKING_FAILURE_RETAIN_PINS_PASS");
 reset();CHECK(mk_prepare_host_cpu_park(0x100000008ULL,8)==-ENODEV&&!mk_cpu_uses_host_park[8]);fallback=true;CHECK(!mk_prepare_host_cpu_park(0x100000008ULL,8)&&!mk_cpu_uses_host_park[8]&&!mk_host_park_pins.nr);int previous=park_calls;mk_pool_park_cpu();CHECK(park_calls==previous&&fallback_calls>0);CHECK(!mk_teardown_host_park());
 reset();CHECK(!mk_setup_host_park());CHECK(!mk_cpu_pool_begin(0x100000008ULL,false));CHECK(mk_teardown_host_park()==-EBUSY&&mk_pool_park_available);reset();free(host.ids);free(pool.ids);free(mk_host_park_pins.ids);
 puts("PARK_FALLBACK_SELECTION_WITHOUT_LOCAL_AREA_AND_TRANSITION_TEARDOWN_REFUSAL_PASS");return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-park-quiesce-') as td:
 source=Path(td)/'test.c';exe=Path(td)/'test';source.write_text(code)
 subprocess.run(['gcc','-std=gnu11','-pthread','-Wall','-Wextra','-Werror','-Wno-unused-function','-Wno-unused-parameter','-Wno-unused-variable','-fsanitize=undefined','-fno-sanitize-recover=all',str(source),'-o',str(exe)],check=True)
 print('PARK_ACTUAL_SETUP_TEARDOWN_SELECTION_HOTPLUG_AND_ACCOUNTING_COMPILED',flush=True);subprocess.run([str(exe)],check=True)
