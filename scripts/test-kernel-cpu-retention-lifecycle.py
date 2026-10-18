#!/usr/bin/env python3
"""Exercise actual CPU confirmation, memory/instance/image retention and delete.

Uses modeled park/resource/refcount/allocator backends; not live teardown proof.
"""
import argparse
from pathlib import Path
import subprocess,tempfile
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('directory',type=Path);p.add_argument('--old-confirm',action='store_true');p.add_argument('--old-release',action='store_true');a=p.parse_args();w=a.directory
core=(w/'kernel/multikernel/core.c').read_text();kexec=(w/'kernel/kexec_core.c').read_text();kernfs=(w/'kernel/multikernel/kernfs.c').read_text();header=(w/'include/linux/multikernel.h').read_text()
def block(text,prefix):
 start=text.index(prefix);return text[start:text.index('\n}',start)+2]+'\n'
code=r'''
#include <stdbool.h>
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
typedef unsigned long long u64;typedef u64 mk_phys_cpu_t;typedef unsigned long kimage_entry_t;
#define CHECK(c) do{if(!(c)){fprintf(stderr,"CPU retention line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
#define READ_ONCE(x) __atomic_load_n(&(x),__ATOMIC_SEQ_CST)
#define WRITE_ONCE(x,v) __atomic_store_n(&(x),(v),__ATOMIC_SEQ_CST)
#define cmpxchg(p,o,n) ({int e=(o);__atomic_compare_exchange_n((p),&e,(n),0,__ATOMIC_SEQ_CST,__ATOMIC_SEQ_CST);e;})
#define MK_STATE_ACTIVE 3
#define MK_STATE_LOADED 2
#define MK_STATE_READY 1
#define CONFIG_MULTIKERNEL 1
#define KEXEC_TYPE_MULTIKERNEL 2
#define KEXEC_TYPE_DEFAULT 0
#define KEXEC_TYPE_CRASH 1
#define IND_INDIRECTION 1
#define IND_SOURCE 2
#define MK_CTRL_BLOCK_SIZE 4096
struct list_head {struct list_head *next,*prev;};
#define INIT_LIST_HEAD(h) ((h)->next=(h)->prev=(h))
#define container_of(p,t,m) ((t *)((char *)(p)-offsetof(t,m)))
#define list_entry(p,t,m) container_of(p,t,m)
#define list_for_each_entry_safe(p,n,h,m) for(p=list_entry((h)->next,__typeof__(*p),m),n=list_entry(p->m.next,__typeof__(*p),m);&p->m!=(h);p=n,n=list_entry(n->m.next,__typeof__(*n),m))
static void list_del(struct list_head *n){n->prev->next=n->next;n->next->prev=n->prev;}
static void list_add(struct list_head *n,struct list_head *h){n->next=h->next;n->prev=h;h->next->prev=n;h->next=n;}
static bool list_empty(struct list_head *h){return h->next==h;}
struct kref {int value;};
struct mk_cpu_set {unsigned nr;u64 ids[8];};
static bool mk_cpu_set_empty(struct mk_cpu_set *s){return !s||!s->nr;}
#define mk_cpu_set_for_each(i,id,s) for((i)=0;(s)&&(i)<(s)->nr&&(((id)=(s)->ids[i]),1);(i)++)
struct resource {u64 start,end;char *name;void *parent;};
struct mk_memory_region {struct list_head list;struct resource res;};
struct mk_instance {int id,state,cpu_transfer_state;u64 generation;char *name;struct mk_cpu_set *cpus,*cpus_on_slot;void *spawn_ctx;struct list_head memory_regions,list;int region_count;void *instance_pool;size_t pool_size;void *ctrl_va;u64 ctrl_phys;size_t ctrl_used;void *dtb_data;struct kref refcount;void *kn,*ipi_data,*kimage;u64 ipi_phys;};
'''+block(header,'enum mk_cpu_transfer_state {')+';\n'+r'''
static int effects,frees,returned,ref_puts,retains,arch_calls,fail_arch,fail_return,bad_cpu,confirm_calls,removed,control_frees,pool_frees;
static int registry_removes,cleanup_calls;static struct mk_instance *retained,*observed;
static bool arch_safe;
static struct mk_instance root,*root_instance=&root;
static void logger(const char *fmt,...){(void)fmt;}
#define pr_err logger
#define pr_info logger
#define pr_debug logger
static int mk_arch_confirm_parked(struct mk_instance *i,u64 cpu){CHECK(i);confirm_calls++;return (int)cpu==bad_cpu?-EBUSY:0;}
static int mk_arch_release_instance(struct mk_instance *i){CHECK(i);arch_calls++;if(fail_arch||i->cpu_transfer_state==MK_CPU_TRANSFER_MOVING)return -EBUSY;arch_safe=true;if(i->cpus_on_slot){free(i->cpus_on_slot);i->cpus_on_slot=NULL;}i->spawn_ctx=NULL;return 0;}
static int mk_instance_return_cpus(struct mk_instance *i,const struct mk_cpu_set *cpus){CHECK(arch_safe);if(fail_return)return -ENOMEM;returned+=(int)cpus->nr;i->cpus->nr=0;return 0;}
static void mk_instance_return_pci_devices(struct mk_instance *i){CHECK(arch_safe&&i);effects++;}
static void mk_instance_return_platform_devices(struct mk_instance *i){CHECK(arch_safe&&i);effects++;}
static void mk_instance_retain(struct mk_instance *i,int error){CHECK(i&&error&&!frees&&!returned);retains++;retained=i;}
static void kfree(void *p){if(p){frees++;free(p);}}
static void mk_cpu_set_free(struct mk_cpu_set *s){kfree(s);}
static u64 resource_size(struct resource *r){return r->end-r->start+1;}
static void remove_resource(struct resource *r){CHECK(arch_safe&&r);removed++;}
static void mk_instance_free(struct mk_instance *i,void *p,size_t n){CHECK(arch_safe&&i&&p&&n==MK_CTRL_BLOCK_SIZE);free(p);control_frees++;}
static void multikernel_destroy_instance_pool(void *p){CHECK(arch_safe&&p);free(p);pool_frees++;}
static void mk_instance_put(struct mk_instance *i){CHECK(i);ref_puts++;}
static void mk_instance_set_state(struct mk_instance *i,int state){i->state=state;}
static int mk_instance_mutex,mk_instance_idr;
#define lockdep_assert_held(p) ((void)(p))
static void idr_remove(int *idr,int id){CHECK(idr==&mk_instance_idr&&id==7);registry_removes++;}
static void kernfs_remove(void *kn){CHECK(kn);effects++;}
'''
code+=block(core,'int mk_instance_begin_release(')
confirm=(w/'before/kernel/multikernel/core.c').read_text() if a.old_confirm else core
release=(w/'before/kernel/multikernel/core.c').read_text() if a.old_release else core
code+=block(confirm,'int mk_instance_confirm_parked(')
code+=block(release,'void mk_instance_free_memory(')
code+=block(release,('static void ' if a.old_release else 'static int ')+'mk_instance_return_all_cpus(')
code+=block(release,'static void mk_instance_release(')
code+=block(kernfs,'int mk_instance_destroy(')
code+=r'''
struct kimage {int type,file_mode;unsigned long nr_segments;struct {unsigned long mem,memsz;void *kbuf;} segment[1];struct mk_instance *mk_instance;unsigned long mk_manifest,mk_ipi;struct list_head control_pages;void *cmdline_buf;};
static struct kimage *kexec_image,*kexec_crash_image;
static void image_effect(void){CHECK(observed&&observed->cpu_transfer_state==MK_CPU_TRANSFER_RELEASING&&!ref_puts&&bad_cpu<0);effects++;}
static void kimage_remove_from_list(struct kimage *i){CHECK(i);image_effect();registry_removes++;}
static void kimage_update_compat_pointers(void *p,int type){(void)p;(void)type;image_effect();}
static void *phys_to_virt(unsigned long p){return (void *)p;}
static void *phys_to_page(unsigned long p){return (void *)p;}
static void mk_kimage_free(struct kimage *i,void *p,size_t n){(void)i;(void)p;(void)n;image_effect();}
static void kimage_file_post_load_cleanup(struct kimage *i){CHECK(i);image_effect();cleanup_calls++;}
static void put_page(void *p){CHECK(p);image_effect();}
static unsigned get_order(size_t n){CHECK(n);return 0;}
struct mk_shared_data {int unused;};
static void __free_pages(void *p,unsigned order){(void)order;CHECK(p);image_effect();}
static void kimage_free_extra_pages(struct kimage *i){CHECK(i);image_effect();}
#define for_each_kimage_entry(image,ptr,entry) for((ptr)=NULL,(entry)=0;((void)(ptr),(entry)!=0);(entry)=0)
static void kimage_free_entry(kimage_entry_t e){(void)e;image_effect();}
static void kimage_free_cma(struct kimage *i){CHECK(i);image_effect();}
static void machine_kexec_cleanup(struct kimage *i){CHECK(i);image_effect();}
static void kimage_free_page_list(struct list_head *h){CHECK(h);image_effect();}
'''+(block(kexec,'int kimage_free_checked(struct kimage *image)') if 'int kimage_free_checked(' in kexec else '')+block(kexec,'void kimage_free(struct kimage *image)')+r'''
static void reset(void){effects=frees=returned=ref_puts=retains=arch_calls=fail_arch=fail_return=confirm_calls=removed=control_frees=pool_frees=registry_removes=cleanup_calls=0;arch_safe=false;bad_cpu=-1;retained=observed=NULL;}
static struct mk_instance *make_instance(void)
{
 struct mk_instance *i=calloc(1,sizeof(*i));CHECK(i);i->id=7;i->state=MK_STATE_READY;i->generation=99;i->name=strdup("test");i->dtb_data=malloc(1);i->cpus=calloc(1,sizeof(*i->cpus));i->cpus_on_slot=calloc(1,sizeof(*i->cpus_on_slot));i->cpus->nr=2;i->cpus->ids[0]=8;i->cpus->ids[1]=10;i->cpus_on_slot->nr=1;i->cpus_on_slot->ids[0]=8;i->spawn_ctx=(void *)1;i->instance_pool=malloc(1);i->pool_size=4096;i->ctrl_va=malloc(1);INIT_LIST_HEAD(&i->memory_regions);INIT_LIST_HEAD(&i->list);i->kn=(void *)1;
 struct mk_memory_region *r=calloc(1,sizeof(*r));CHECK(r);r->res=(struct resource){.start=4096,.end=8191,.name=strdup("region"),.parent=(void *)1};list_add(&r->list,&i->memory_regions);i->region_count=1;return i;
}
static void discard(struct mk_instance *i){fail_arch=0;fail_return=0;i->cpu_transfer_state=0;mk_instance_free_memory(i);free(i->cpus);free(i->name);free(i->dtb_data);free(i);}
int main(void)
{
 struct mk_instance *i;struct kimage *image;
 reset();i=make_instance();i->cpu_transfer_state=MK_CPU_TRANSFER_MOVING;CHECK(mk_instance_confirm_parked(i)==-EBUSY&&!confirm_calls);discard(i);
 reset();i=make_instance();bad_cpu=10;CHECK(mk_instance_confirm_parked(i)==-EBUSY&&confirm_calls==2);bad_cpu=-1;discard(i);
 puts("CPU_CONFIRM_ALL_OWNED_AND_INFLIGHT_REFUSAL_PASS");
 reset();i=make_instance();fail_arch=1;mk_instance_free_memory(i);CHECK(!frees&&!removed&&!control_frees&&!pool_frees&&i->region_count==1&&!list_empty(&i->memory_regions)&&i->instance_pool&&i->cpus_on_slot);discard(i);
 reset();i=make_instance();fail_arch=1;mk_instance_release(&i->refcount);CHECK(retains==1&&retained==i&&!returned&&!frees&&i->instance_pool&&i->region_count==1);discard(i);
 reset();i=make_instance();fail_return=1;mk_instance_release(&i->refcount);CHECK(retains==1&&retained==i&&!returned&&!frees&&i->instance_pool);discard(i);
 reset();i=make_instance();mk_instance_release(&i->refcount);CHECK(returned==2&&pool_frees==1&&control_frees==1&&removed==1&&!retains&&frees>=5);
 puts("CPU_RELEASE_PROOF_BEFORE_POOL_OR_METADATA_FREE_AND_LASTREF_RETENTION_PASS");
 reset();i=make_instance();i->cpu_transfer_state=MK_CPU_TRANSFER_MOVING;CHECK(mk_instance_destroy(i)==-EBUSY&&!registry_removes&&!ref_puts);i->cpu_transfer_state=MK_CPU_TRANSFER_UNCERTAIN;fail_arch=1;CHECK(mk_instance_destroy(i)==-EBUSY&&!registry_removes&&!ref_puts&&i->cpu_transfer_state==MK_CPU_TRANSFER_UNCERTAIN);fail_arch=0;CHECK(!mk_instance_destroy(i)&&i->cpu_transfer_state==MK_CPU_TRANSFER_RELEASING&&registry_removes==1&&ref_puts==2);discard(i);
 puts("CPU_DELETE_REFUSES_UNSAFE_RELEASE_AND_PRESERVES_REGISTRY_PASS");
 reset();i=make_instance();observed=i;i->state=MK_STATE_LOADED;image=calloc(1,sizeof(*image));image->type=KEXEC_TYPE_MULTIKERNEL;image->mk_instance=i;image->mk_ipi=1;image->file_mode=1;image->cmdline_buf=malloc(1);
 i->cpu_transfer_state=MK_CPU_TRANSFER_MOVING;kimage_free(image);CHECK(!effects&&!frees&&!ref_puts&&image->mk_instance==i);
 i->cpu_transfer_state=MK_CPU_TRANSFER_UNCERTAIN;bad_cpu=10;kimage_free(image);CHECK(!effects&&!frees&&!ref_puts&&image->mk_instance==i&&i->cpu_transfer_state==MK_CPU_TRANSFER_UNCERTAIN);
 bad_cpu=-1;kimage_free(image);CHECK(registry_removes==1&&ref_puts==1&&cleanup_calls==2&&i->state==MK_STATE_READY&&i->cpu_transfer_state==MK_CPU_TRANSFER_UNCERTAIN);discard(i);
 puts("CPU_IMAGE_RETENTION_AND_REFERENCE_GATE_THROUGH_LAST_CLEANUP_PASS");return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-cpu-retention-') as td:
 if 'int kimage_free_checked(' in kexec:
  code=code.replace('kimage_free(image);CHECK(!effects', 'CHECK(kimage_free_checked(image)==-EBUSY);CHECK(!effects').replace('bad_cpu=-1;kimage_free(image);CHECK(registry_removes', 'bad_cpu=-1;CHECK(!kimage_free_checked(image));CHECK(registry_removes')
 f=Path(td)/'test.c';exe=Path(td)/'test';f.write_text(code)
 subprocess.run(['gcc','-std=gnu11','-Wall','-Wextra','-Werror','-Wno-unused-function','-fsanitize=undefined','-fno-sanitize-recover=all',str(f),'-o',str(exe)],check=True)
 print('CPU_RETENTION_ACTUAL_LIFECYCLE_FUNCTIONS_COMPILED',flush=True);subprocess.run([str(exe)],check=True)
