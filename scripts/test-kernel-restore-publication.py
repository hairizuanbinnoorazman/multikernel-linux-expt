#!/usr/bin/env python3
"""Run extracted boot restore code with modeled registry and faulted allocators.

Parsers, topology and physical mappings are mocked. This checks private object
ownership/publication/error cleanup, not physical ownership or CPU-mask rollback.
"""
from pathlib import Path
import argparse, subprocess, tempfile
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('source',type=Path)
p.add_argument('--require-boot-policy',action='store_true',help='Apply new guard assertions to an older control source')
a=p.parse_args(); source=a.source.read_text()
def extract(marker):
    start=source.index(marker)
    return source[start:source.index('\n}',start)+2]+'\n'
code=r'''
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
#define __init
#define GFP_KERNEL 0
#define __GFP_ZERO 0
#define PAGE_SIZE 4096
#define SZ_1M (1<<20)
#define MEMREMAP_WB 0
#define MK_STATE_READY 1
#define MK_PLATFORM_DEVICE_ID_LEN 16
#define MK_PLATFORM_DEVICE_NAME_LEN 32
#define pr_info(...) ((void)0)
#define pr_warn(...) ((void)0)
#define pr_err(...) ((void)0)
#define pr_debug(...) ((void)0)
#define CHECK(x) do {if(!(x)){fprintf(stderr,"line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
struct list_head {struct list_head *next,*prev;};
#define INIT_LIST_HEAD(h) ((h)->next=(h),(h)->prev=(h))
static void list_add(struct list_head *n,struct list_head *h){n->next=h->next;n->prev=h;h->next->prev=n;h->next=n;}
static void list_add_tail(struct list_head *n,struct list_head *h){list_add(n,h->prev);}
static void list_del(struct list_head *n){n->prev->next=n->next;n->next->prev=n->prev;}
#define entry(p,t,m) ((t *)((char *)(p)-offsetof(t,m)))
#define list_for_each_entry(p,h,m) for(p=entry((h)->next,__typeof__(*p),m);&p->m!=(h);p=entry(p->m.next,__typeof__(*p),m))
#define list_for_each_entry_safe(p,n,h,m) for(p=entry((h)->next,__typeof__(*p),m),n=entry(p->m.next,__typeof__(*p),m);&p->m!=(h);p=n,n=entry(n->m.next,__typeof__(*n),m))
typedef uint64_t phys_addr_t,mk_phys_cpu_t;typedef uint32_t u32;
struct mk_shared_data {char data[8193];};
struct mk_pci_device {struct list_head list;int vendor,device,domain,bus,slot,func;};
struct mk_platform_device {struct list_head list;char hid[16],name[32];};
struct mk_instance {uint64_t generation;int id,state,refcount;char *name;int *cpus;void *dtb_data;size_t dtb_size;struct mk_shared_data *ipi_data;phys_addr_t ipi_phys;u32 ipi_pages;struct list_head list,memory_regions,pci_devices,platform_devices;bool pci_devices_valid,platform_devices_valid;int pci_device_count,platform_device_count;};
struct mk_dt_config {int *cpus;struct list_head pci_devices,platform_devices;bool pci_devices_valid,platform_devices_valid;int pci_device_count,platform_device_count;};
static struct mk_instance *root_instance,*registry[16];
static int fail_generation;
static int mk_instance_assign_generation(struct mk_instance *p){if(fail_generation)return -EOVERFLOW;p->generation=1;return 0;}
static struct list_head mk_instance_list;
static int mk_instance_mutex,mk_instance_idr,locked;
static int live,attempt,fail_at,fail_publish,fail_cpu,fail_copy,fail_map,maps,unmaps,page_allocs,page_frees,published,manifest_mode,schema_fault,early_maps,early_unmaps;
static char manifest[4096],dtb[128];
static struct mk_shared_data mapped[2];
static void *allocate(size_t n){if(++attempt==fail_at)return NULL;void *p=calloc(1,n);CHECK(p);live++;return p;}
static void *kzalloc(size_t n,int flags){(void)flags;return allocate(n);}
static void *kmalloc(size_t n,int flags){return kzalloc(n,flags);}
static char *kstrdup(const char *s,int flags){char *p=kmalloc(strlen(s)+1,flags);if(p)strcpy(p,s);return p;}
static void kfree(void *p){if(p){CHECK(live>0);live--;free(p);}}
static int get_order(size_t n){(void)n;return 2;}
static unsigned long __get_free_pages(int flags,int order){(void)flags;(void)order;void *p=allocate(16384);if(p)page_allocs++;return (unsigned long)p;}
static void free_pages(unsigned long p,int order){(void)order;if(p){page_frees++;kfree((void *)p);}}
static phys_addr_t virt_to_phys(void *p){return (uintptr_t)p;}
static int *mk_cpu_set_alloc(void){return allocate(sizeof(int));}
static void mk_cpu_set_free(int *p){kfree(p);}
static int mk_cpu_set_add(int *p,int cpu){(void)cpu;if(fail_cpu)return -ENOMEM;(*p)++;return 0;}
static int mk_cpu_set_copy(int *d,int *s){if(fail_copy)return -ENOMEM;*d=*s;return 0;}
static void mk_cpu_set_format(char *p,size_t n,int *cpus){(void)cpus;snprintf(p,n,"0,1");}
#define for_each_online_cpu(cpu) for(cpu=0;cpu<2;cpu++)
static int arch_cpu_physical_id(int cpu){return cpu;}
static void kref_init(int *p){*p=1;}
static void mutex_lock(int *p){(void)p;CHECK(!locked);locked=1;}
static void mutex_unlock(int *p){(void)p;CHECK(locked);locked=0;}
static int idr_alloc(int *idr,struct mk_instance *p,int begin,int end,int flags){(void)idr;(void)flags;CHECK(locked&&end==begin+1&&begin>=0&&end<=16);if(fail_publish)return -ENOMEM;if(registry[begin])return -ENOSPC;CHECK(p->cpus&&p->name);if(begin!=0)CHECK(p->dtb_data&&p->ipi_data&&p->pci_device_count==1&&p->platform_device_count==1);registry[begin]=p;published++;return begin;}
static void *memremap(phys_addr_t p,size_t n,int flags){(void)n;(void)flags;if(fail_map)return NULL;maps++;return &mapped[p==0x20000];}
static void memunmap(void *p){CHECK(p==&mapped[0]||p==&mapped[1]);unmaps++;}
static phys_addr_t mk_manifest_phys(void){return manifest_mode?0x1000:0;}
static void *early_memremap(phys_addr_t p,size_t n){(void)p;(void)n;early_maps++;return manifest;}
static void early_memunmap(void *p,size_t n){(void)p;(void)n;early_unmaps++;}
static int fdt_check_full(const void *p,size_t n){(void)p;(void)n;return 0;}
static int mk_manifest_validate_ipi_ranges(const void *p){(void)p;return 0;}
static int fdt_subnode_offset(const void *p,int node,const char *name){(void)p;(void)node;(void)name;return schema_fault==1?-ENOENT:1;}
static const void *fdt_getprop(const void *p,int node,const char *name,int *len){(void)p;(void)node;(void)name;*len=schema_fault==2?0:sizeof(dtb);return schema_fault==2?NULL:dtb;}
static int mk_dt_check_blob_bounds(const void *p,size_t n){(void)p;(void)n;return 0;}
static int mk_dt_extract_instance_info(const void *p,size_t n,int *id,const char **name){(void)p;(void)n;*id=schema_fault==3?0:schema_fault==4?-1:3;*name="child";return 0;}
static struct mk_pci_device config_pci;
static struct mk_platform_device config_platform;
static int config_cpu=2,fail_config_init;
static int mk_dt_config_init(struct mk_dt_config *c){memset(c,0,sizeof(*c));INIT_LIST_HEAD(&c->pci_devices);INIT_LIST_HEAD(&c->platform_devices);return fail_config_init?-ENOMEM:0;}
static int mk_dt_parse(const void *p,size_t n,struct mk_dt_config *c){(void)p;(void)n;c->cpus=&config_cpu;c->pci_devices_valid=c->platform_devices_valid=true;c->pci_device_count=c->platform_device_count=1;list_add(&config_pci.list,&c->pci_devices);list_add(&config_platform.list,&c->platform_devices);strcpy(config_platform.hid,"DEV0");strcpy(config_platform.name,"device");return 0;}
static void mk_dt_config_free(struct mk_dt_config *c){(void)c;}
static int mk_restore_instance_cpus(struct mk_dt_config *c){(void)c;return 0;}
static int mk_manifest_get_ipi_range(const void *p,const char *name,phys_addr_t *phys,u32 *pages,size_t *size){(void)p;*phys=!strcmp(name,"host-ipi-buffer")?0x20000:0x10000;*pages=3;*size=12288;return 0;}
'''
if 'static int __init mk_prepare_instance_cpus(' in source:
    code += r'''
struct cpumask {unsigned bits;};
typedef struct cpumask *cpumask_var_t;
static struct cpumask __cpu_present_mask={255};
static unsigned nr_cpu_ids=8;
static int mask_commits;
static bool alloc_cpumask_var(cpumask_var_t *p,int flags){(void)flags;*p=allocate(sizeof(**p));return *p!=NULL;}
static void free_cpumask_var(cpumask_var_t p){kfree(p);}
static void cpumask_clear(struct cpumask *p){p->bits=0;}
static bool cpumask_test_cpu(unsigned cpu,const struct cpumask *p){CHECK(cpu<8);return (p->bits&(1u<<cpu))!=0;}
static void cpumask_set_cpu(unsigned cpu,struct cpumask *p){CHECK(cpu<8);p->bits|=1u<<cpu;}
static void cpumask_copy(struct cpumask *d,const struct cpumask *s){CHECK(locked);mask_commits++;*d=*s;}
static int raw_smp_processor_id(void){return 0;}
static bool cpu_present(unsigned cpu){return cpumask_test_cpu(cpu,&__cpu_present_mask);}
static int arch_cpu_from_physical_id(uint64_t p){return p<8?(int)p:-1;}
static bool mk_cpu_set_empty(const int *p){return !p||!*p;}
#define mk_cpu_set_for_each(i,phys,set) for(i=0;i<(unsigned)*(set)&&((phys=i),1);i++)
'''
    code += extract('static int __init mk_prepare_instance_cpus(')
    code += '#define REQUIRE_CPU_COMMIT 1\n'
for marker in ['static struct mk_instance * __init alloc_mk_instance(', 'static void __init mk_free_private_instance(', 'static int __init mk_publish_boot_instance(', 'static int __init mk_copy_pci_devices(', 'static int __init mk_copy_platform_devices(', 'static int __init mk_restore_instance_ipi(', 'static struct mk_instance * __init mk_restore_host_instance(', 'int __init mk_instance_restore_from_manifest(']:
    code+=extract(marker)
if 'static int __init mk_instance_boot_init(' in source or a.require_boot_policy:
    code += '#define REQUIRE_HANDOFF_POLICY 1\n'
    code += r'''
typedef uint8_t u8;typedef uint16_t u16;
struct pci_bus {int number;};
#define PCI_SLOT(n) (((n)>>3)&31)
#define PCI_FUNC(n) ((n)&7)
#define PCI_HEADER_TYPE 0
#define PCI_HEADER_TYPE_MASK 0x7f
#define PCI_HEADER_TYPE_BRIDGE 1
#define PCI_SECONDARY_BUS 1
#define PCI_SUBORDINATE_BUS 2
static int pci_domain_nr(struct pci_bus *p){(void)p;return 0;}
static int pci_bus_read_config_byte(struct pci_bus *p,int devfn,int field,u8 *out){(void)p;(void)devfn;(void)field;*out=0;return 0;}
static int list_empty(struct list_head *p){return p->next==p;}
'''
    code += extract('bool mk_pci_should_probe(')
    code += extract('bool mk_platform_device_allowed(')

if 'mk_instance_assign_generation(instance)' in source:code+='#define REQUIRE_INSTANCE_GENERATION\n'
code+=r'''
static void reset(void){CHECK(live==0&&maps==unmaps&&page_allocs==page_frees&&!locked);for(int i=0;i<16;i++)CHECK(!registry[i]);CHECK(!root_instance);INIT_LIST_HEAD(&mk_instance_list);fail_generation=attempt=fail_at=fail_publish=fail_cpu=fail_copy=fail_map=published=0;manifest_mode=schema_fault=fail_config_init=0;early_maps=early_unmaps=0;
#ifdef REQUIRE_CPU_COMMIT
 __cpu_present_mask.bits=255;mask_commits=0;
#endif
}
static void release_published(void){for(int i=0;i<16;i++)if(registry[i]){struct mk_instance *p=registry[i];list_del(&p->list);registry[i]=NULL;mk_free_private_instance(p,!manifest_mode);}root_instance=NULL;CHECK(mk_instance_list.next==&mk_instance_list);}
int main(void){
#ifdef REQUIRE_INSTANCE_GENERATION
 reset();fail_generation=1;CHECK(!alloc_mk_instance(1,"refused",true)&&!live&&!page_allocs&&!published);reset();
 puts("BOOT_GENERATION_EXHAUSTION_BEFORE_RESOURCE_PREPARATION_PASS");
#endif

 reset();struct mk_instance *p=alloc_mk_instance(0,"",true);CHECK(p&&!published&&!root_instance&&mk_instance_list.next==&mk_instance_list);mk_free_private_instance(p,true);
 for(int mode=0;mode<2;mode++){
  reset();manifest_mode=mode;CHECK(!mk_instance_restore_from_manifest());CHECK(root_instance&&published==(mode?2:1));int allocations=attempt;release_published();
  for(int fail=1;fail<=allocations;fail++){
   reset();manifest_mode=mode;fail_at=fail;int ret=mk_instance_restore_from_manifest();
   if(ret){
#ifdef REQUIRE_CPU_COMMIT
    CHECK(__cpu_present_mask.bits==255&&!mask_commits);
#endif
    CHECK(!published&&!root_instance&&mk_instance_list.next==&mk_instance_list);}else{CHECK(mode&&root_instance&&published==1);release_published();}
   CHECK(live==0&&maps==unmaps&&page_allocs==page_frees&&early_maps==early_unmaps);
  }
  reset();manifest_mode=mode;fail_publish=1;CHECK(mk_instance_restore_from_manifest()<0);CHECK(!published&&!root_instance);
  reset();manifest_mode=mode;fail_cpu=1;int ret=mk_instance_restore_from_manifest();if(mode){CHECK(!ret&&published==1);release_published();}else CHECK(ret<0&&!published);
 }
 reset();manifest_mode=1;fail_config_init=1;CHECK(mk_instance_restore_from_manifest()==-ENOMEM&&!published&&!root_instance&&!mask_commits);CHECK(!live&&early_maps==early_unmaps);puts("BOOT_CONFIG_INIT_FAILURE_STOPS_BEFORE_PUBLICATION_AND_CPU_COMMIT_PASS");
 reset();manifest_mode=1;fail_copy=1;CHECK(mk_instance_restore_from_manifest()<0&&!published);
 reset();manifest_mode=1;fail_map=1;CHECK(mk_instance_restore_from_manifest()<0&&!published);
 reset();manifest_mode=1;fail_map=1;CHECK(!mk_restore_host_instance(manifest)&&!published);
 reset();manifest_mode=1;fail_publish=1;CHECK(!mk_restore_host_instance(manifest)&&!published);CHECK(maps==unmaps);
 reset();manifest_mode=1;fail_cpu=1;CHECK(!mk_restore_host_instance(manifest)&&!published);
 reset();manifest_mode=1;p=mk_restore_host_instance(manifest);CHECK(p&&published==1);int before=live;CHECK(!mk_restore_host_instance(manifest)&&live==before&&registry[0]==p);release_published();

#ifdef REQUIRE_HANDOFF_POLICY
 reset();struct pci_bus bus={0};CHECK(mk_pci_should_probe(&bus,0)&&mk_platform_device_allowed("test","DEV"));manifest_mode=1;CHECK(!mk_pci_should_probe(&bus,0)&&!mk_platform_device_allowed("test","DEV"));manifest_mode=0;CHECK(!mk_instance_restore_from_manifest());CHECK(mk_pci_should_probe(&bus,0)&&mk_platform_device_allowed("test","DEV"));release_published();
 puts("PRE_RESTORE_CHILD_DENIES_PROBING_NORMAL_HOST_POLICY_PRESERVED_PASS");
 for(int fault=1;fault<=4;fault++){reset();manifest_mode=1;schema_fault=fault;CHECK(mk_instance_restore_from_manifest()==-EINVAL);CHECK(!published&&!root_instance&&live==0&&early_maps==early_unmaps);}
 puts("MISSING_REQUIRED_NODE_DTB_AND_NONPOSITIVE_CHILD_ID_REFUSED_PASS");
#endif

#ifdef REQUIRE_CPU_COMMIT
 for(int fault=1;fault<=3;fault++){reset();manifest_mode=1;if(fault==1)fail_publish=1;if(fault==2)fail_copy=1;if(fault==3)fail_map=1;CHECK(mk_instance_restore_from_manifest()<0);CHECK(__cpu_present_mask.bits==255&&!mask_commits);}
 reset();manifest_mode=1;CHECK(!mk_instance_restore_from_manifest());CHECK(__cpu_present_mask.bits==3&&mask_commits==1);release_published();
 puts("CPU_POLICY_COMMITTED_ONLY_AFTER_FALLIBLE_PREPARATION_AND_IDR_SUCCESS_PASS");
#endif
 reset();puts("BOOT_RESTORE_PRIVATE_PUBLICATION_AND_FAILURE_CLEANUP_PASS");return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-restore-publication-') as tmp:
    tmp=Path(tmp);(tmp/'test.c').write_text(code)
    subprocess.run(['gcc','-std=gnu11','-Wall','-Wextra','-Werror','-Wno-unused-function','-fsanitize=undefined','-fno-sanitize-recover=all','-g',str(tmp/'test.c'),'-o',str(tmp/'test')],check=True)
    subprocess.run([str(tmp/'test')],check=True)
