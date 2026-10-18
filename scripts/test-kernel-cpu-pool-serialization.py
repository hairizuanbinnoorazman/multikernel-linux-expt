#!/usr/bin/env python3
"""Run actual pool claims/transfers/cpuset functions with pthreads and modeled hotplug."""
import argparse
from pathlib import Path
import subprocess,tempfile
p=argparse.ArgumentParser(description=__doc__);p.add_argument('directory',type=Path);p.add_argument('--old-transfer',action='store_true');p.add_argument('--old-reserve',action='store_true');a=p.parse_args();w=a.directory
def block(text,prefix):
 start=text.index(prefix);return text[start:text.index('\n}',start)+2]+'\n'
core=(w/'core.c').read_text();cpuset=(w/'cpuset.c').read_text();hotplug=(w/'hotplug.c').read_text()
code=r'''
#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>
#include <stdint.h>
#include <stddef.h>
#include <string.h>
#include <errno.h>
#include <limits.h>
#include <pthread.h>
#include <stdarg.h>
#define CHECK(c) do{if(!(c)){fprintf(stderr,"CPU pool line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
typedef unsigned long long mk_phys_cpu_t;typedef uint32_t u32;
#define GFP_KERNEL 0
#define check_add_overflow(a,b,c) __builtin_add_overflow(a,b,c)
#define max_t(t,a,b) ((t)(a)>(t)(b)?(t)(a):(t)(b))
struct mutex {pthread_mutex_t value;};
#define DEFINE_MUTEX(n) struct mutex n={PTHREAD_MUTEX_INITIALIZER}
static __thread int held;static int alloc_fail;
static void mutex_lock(struct mutex *m){CHECK(!held);CHECK(!pthread_mutex_lock(&m->value));held=1;}
static void mutex_unlock(struct mutex *m){CHECK(held);held=0;CHECK(!pthread_mutex_unlock(&m->value));}
#define lockdep_assert_held(p) CHECK(held && (p)==&mk_cpu_ownership_mutex)
static struct mutex *lock_acquire(struct mutex *m){mutex_lock(m);return m;}
static void lock_release(struct mutex **m){mutex_unlock(*m);}
#define guard(type) struct mutex *guard_lock __attribute__((cleanup(lock_release)))=lock_acquire
static void *kzalloc(size_t n,int f){(void)f;if(alloc_fail){alloc_fail=0;return NULL;}return calloc(1,n);}
static void kfree(void *p){free(p);}
static void *krealloc_array(void *p,size_t n,size_t size,int f){(void)f;CHECK(held);if(alloc_fail){alloc_fail=0;return NULL;}CHECK(n<=4096);return realloc(p,n*size);}
struct list_head {struct list_head *next,*prev;};
#define LIST_HEAD(n) struct list_head n={&n,&n}
#define list_entry(p,t,m) ((t *)((char *)(p)-offsetof(t,m)))
#define list_for_each_entry(p,h,m) for(p=list_entry((h)->next,__typeof__(*p),m);&p->m!=(h);p=list_entry(p->m.next,__typeof__(*p),m))
static void list_add_tail(struct list_head *n,struct list_head *h){n->next=h;n->prev=h->prev;h->prev->next=n;h->prev=n;}
static void list_del(struct list_head *n){n->next->prev=n->prev;n->prev->next=n->next;}
struct mk_cpu_set {unsigned nr,cap;mk_phys_cpu_t *ids;};
static unsigned mk_cpu_set_count(const struct mk_cpu_set *s){return s?s->nr:0;}
#define mk_cpu_set_for_each(i,id,s) for((i)=0;(s)&&(i)<(s)->nr&&(((id)=(s)->ids[i]),1);(i)++)
struct mk_instance {int id;char *name;struct mk_cpu_set *cpus,*cpus_on_slot;};
static struct mk_instance root,*root_instance=&root;static struct mk_cpu_set pool;static struct mk_cpu_set *mk_cpu_pool=&pool;
static int mk_cpu_set_format(char *b,size_t n,const struct mk_cpu_set *s){CHECK(held);(void)s;*b=0;return (int)n;}
static int arch_cpu_from_physical_id(mk_phys_cpu_t c){return c==0x100000008ULL?8:-EINVAL;}
#define pr_err(...) ((void)0)
#define pr_info(...) ((void)0)
#define pr_warn(...) ((void)0)
struct seq_file {char data[16384];size_t used;unsigned rows,truncated;};
static void seq_printf(struct seq_file *sf,const char *fmt,...){CHECK(held);va_list ap;va_start(ap,fmt);sf->used+=(size_t)vsnprintf(sf->data+sf->used,sizeof(sf->data)-sf->used,fmt,ap);va_end(ap);sf->rows++;}
static void seq_puts(struct seq_file *sf,const char *s){CHECK(held);sf->used+=(size_t)snprintf(sf->data+sf->used,sizeof(sf->data)-sf->used,"%s",s);sf->truncated++;}
'''
for prefix in ['static int mk_cpu_set_index(','bool mk_cpu_set_contains(','int mk_cpu_set_reserve(','int mk_cpu_set_add(','bool mk_cpu_set_del(','int mk_cpu_set_copy(']:
 source=(w/'before-cpuset.c').read_text() if a.old_reserve and prefix.startswith('int mk_cpu_set_reserve') else cpuset
 function=block(source,prefix)
 if prefix.startswith('static int mk_cpu_set_index'):function=function.replace('\n\tunsigned int i;', '\n\tCHECK(held);\n\tunsigned int i;',1)
 code+=function
start=core.index('DEFINE_MUTEX(mk_cpu_ownership_mutex);');end=core.index('/* Shared by boot restore',start);code+=core[start:end]
for prefix in ['int mk_instance_transfer_cpus(','int mk_instance_return_cpus(']:code+=block((w/'before-core.c').read_text() if a.old_transfer else core,prefix)
code+=r'''
static int effects,backend_error,tracking_error;
static int mk_cpu_to_logical(mk_phys_cpu_t c){return arch_cpu_from_physical_id(c);}
static bool cpu_online(int cpu){return cpu==8;}
static void mk_set_pool_cpu(int cpu,bool b){CHECK(cpu==8);(void)b;}
static int mk_do_cpu_remove(mk_phys_cpu_t c){CHECK(!held);effects++;mutex_lock(&mk_cpu_ownership_mutex);CHECK(mk_cpu_pool_cpu_busy(c));mk_cpu_set_del(root.cpus,c);mutex_unlock(&mk_cpu_ownership_mutex);return backend_error;}
static int mk_do_cpu_add(mk_phys_cpu_t c,u32 n,u32 f){(void)n;(void)f;CHECK(!held);effects++;mutex_lock(&mk_cpu_ownership_mutex);CHECK(mk_cpu_pool_cpu_busy(c)&&!mk_cpu_set_contains(mk_cpu_pool,c));if(!tracking_error)CHECK(!mk_cpu_set_add(root.cpus,c));mutex_unlock(&mk_cpu_ownership_mutex);return backend_error;}
'''+block(hotplug,'int mk_pool_cpu_add(')+block(hotplug,'int mk_pool_cpu_remove(')+r'''
static void clear_ops(void){struct list_head *h=&mk_cpu_pool_transitions;while(h->next!=h){struct mk_cpu_pool_transition *op=list_entry(h->next,struct mk_cpu_pool_transition,list);list_del(&op->list);free(op);}}
static void setup(bool free_cpu){CHECK(!held);effects=backend_error=tracking_error=alloc_fail=0;mutex_lock(&mk_cpu_ownership_mutex);clear_ops();pool.nr=root.cpus->nr=0;CHECK(!mk_cpu_set_add(free_cpu?&pool:root.cpus,0x100000008ULL));mutex_unlock(&mk_cpu_ownership_mutex);}
struct contender {struct mk_instance i;struct mk_cpu_set cpus;int result;};static pthread_barrier_t barrier;
static void *claim(void *arg){struct contender *c=arg;mk_phys_cpu_t id=0x100000008ULL;struct mk_cpu_set req={1,1,&id};pthread_barrier_wait(&barrier);c->result=mk_instance_transfer_cpus(&c->i,&req);return NULL;}
int main(void){
 struct mk_cpu_set host={0};root.cpus=&host;root.name="root";
 for(int j=0;j<128;j++){setup(true);struct contender c[2]={0};pthread_t t[2];CHECK(!pthread_barrier_init(&barrier,NULL,2));for(int k=0;k<2;k++){c[k].i=(struct mk_instance){.id=k+1,.name="contender",.cpus=&c[k].cpus};CHECK(!pthread_create(&t[k],NULL,claim,&c[k]));}for(int k=0;k<2;k++)CHECK(!pthread_join(t[k],NULL));CHECK(!pthread_barrier_destroy(&barrier));CHECK((c[0].result==0)+(c[1].result==0)==1);CHECK(c[0].cpus.nr+c[1].cpus.nr==1&&!pool.nr);int owner=c[0].result==0?0:1;CHECK(!mk_instance_return_cpus(&c[owner].i,c[owner].i.cpus));CHECK(pool.nr==1&&!c[owner].cpus.nr&&!held);free(c[0].cpus.ids);free(c[1].cpus.ids);}
 puts("CPU_POOL_TWO_INSTANCE_CONTENDERS_SINGLE_OWNER_AND_ALIAS_RETURN_PASS");
 setup(true);struct mk_cpu_set mine={0};struct mk_instance target={.id=7,.name="alias",.cpus=&mine};CHECK(mk_instance_transfer_cpus(&target,&pool)==-EINVAL&&pool.nr==1&&!mine.nr);CHECK(mk_instance_transfer_cpus(&target,&mine)==-EINVAL&&pool.nr==1);CHECK(mk_instance_transfer_cpus(NULL,&pool)==-EINVAL&&pool.nr==1);CHECK(mk_instance_return_cpus(NULL,&pool)==-EINVAL&&pool.nr==1);
 puts("CPU_POOL_TRANSFER_ALIASES_AND_NULL_OWNER_REFUSED_PASS");
 setup(true);CHECK(!mk_cpu_pool_begin(0x100000008ULL,true)&&!pool.nr);CHECK(mk_cpu_pool_begin(0x100000008ULL,true)==-EBUSY);CHECK(mk_cpu_pool_finish(0x100000008ULL,true,-ETIMEDOUT)==-EUCLEAN);CHECK(mk_cpu_pool_begin(0x100000008ULL,false)==-EBUSY);struct seq_file sf={0};CHECK(!mk_cpu_pool_transitions_show(&sf,NULL));CHECK(strstr(sf.data,"reconciliation-required")&&strstr(sf.data,"error=-110"));
 mutex_lock(&mk_cpu_ownership_mutex);struct mk_cpu_set snap={0};CHECK(!mk_cpu_pool_copy_busy(&snap)&&snap.nr==1);mutex_unlock(&mk_cpu_ownership_mutex);free(snap.ids);
 puts("CPU_POOL_CLAIM_BEFORE_EFFECT_TIMEOUT_RETENTION_AND_BUSY_SNAPSHOT_PASS");
 setup(false);CHECK(!mk_pool_cpu_add(0x100000008ULL)&&effects==1&&pool.nr==1&&!host.nr);CHECK(!mk_pool_cpu_remove(0x100000008ULL,0,0)&&effects==2&&!pool.nr&&host.nr==1);
 setup(true);backend_error=-ETIMEDOUT;CHECK(mk_pool_cpu_remove(0x100000008ULL,0,0)==-EUCLEAN&&!pool.nr);CHECK(mk_pool_cpu_remove(0x100000008ULL,0,0)==-EBUSY&&effects==1);
 setup(false);backend_error=-EIO;CHECK(mk_pool_cpu_add(0x100000008ULL)==-EUCLEAN&&!pool.nr);CHECK(mk_pool_cpu_add(0x100000008ULL)==-EBUSY&&effects==1);
 setup(true);tracking_error=1;CHECK(mk_pool_cpu_remove(0x100000008ULL,0,0)==-EUCLEAN&&!host.nr&&!pool.nr);
 setup(true);alloc_fail=1;CHECK(mk_cpu_pool_begin(0x100000008ULL,true)==-ENOMEM&&pool.nr==1&&!effects);
 setup(false);CHECK(!mk_cpu_pool_begin(0x100000008ULL,false));mutex_lock(&mk_cpu_ownership_mutex);host.nr=0;free(pool.ids);pool=(struct mk_cpu_set){0};alloc_fail=1;mutex_unlock(&mk_cpu_ownership_mutex);CHECK(mk_cpu_pool_finish(0x100000008ULL,false,0)==-EUCLEAN&&!pool.nr);
 puts("CPU_POOL_ACTUAL_LOCAL_WRAPPERS_NO_LOCK_OVER_BACKEND_AND_ERROR_QUARANTINE_PASS");
 setup(false);mutex_lock(&mk_cpu_ownership_mutex);for(int n=100;n<200;n++)CHECK(!mk_cpu_set_add(root.cpus,(mk_phys_cpu_t)n));mutex_unlock(&mk_cpu_ownership_mutex);
 for(int n=100;n<200;n++){CHECK(!mk_cpu_pool_begin((mk_phys_cpu_t)n,false));}
 sf=(struct seq_file){0};CHECK(!mk_cpu_pool_transitions_show(&sf,NULL)&&sf.rows==64&&sf.truncated==1);
 puts("CPU_POOL_TRANSITION_REPORT_BOUNDED_WITH_EXPLICIT_TRUNCATION_PASS");
 setup(true);mutex_lock(&mk_cpu_ownership_mutex);struct mk_cpu_set overflow={.nr=UINT_MAX,.cap=UINT_MAX};CHECK(mk_cpu_set_reserve(&overflow,1)==-EOVERFLOW);clear_ops();mutex_unlock(&mk_cpu_ownership_mutex);free(pool.ids);free(host.ids);
 puts("CPU_SET_RESERVE_UNSIGNED_OVERFLOW_REFUSAL_PASS");return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-pool-owner-') as td:
 source=Path(td)/'test.c';exe=Path(td)/'test';source.write_text(code)
 subprocess.run(['gcc','-std=gnu11','-pthread','-Wall','-Wextra','-Werror','-Wno-unused-function','-Wno-unused-parameter','-fsanitize=undefined','-fno-sanitize-recover=all',str(source),'-o',str(exe)],check=True)
 print('CPU_POOL_ACTUAL_CLAIMS_TRANSFERS_WRAPPERS_COMPILED',flush=True);subprocess.run([str(exe)],check=True)
