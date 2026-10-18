#!/usr/bin/env python3
"""Exercise production overlay scalar/create and full allocator with modeled effects.

No live resource ownership, device effects, concurrency or full transaction claim.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('overlay',type=Path);p.add_argument('kernfs',type=Path)
p.add_argument('--section',choices=['scalars','numa','ids','allocator','generations','overflow','cpu','preflight','config-failure','all'],default='all')
a=p.parse_args();s=a.overlay.read_text();k=a.kernfs.read_text()
def function(text,name):
    for prefix in ['static int ','static u32 ','int ']:
        marker=prefix+name+'('
        if marker in text:
            at=text.index(marker);return text[at:text.index('\n}',at)+2]+'\n'
    raise ValueError(name)
code=r'''
#include <stdint.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>
#include <errno.h>
#include <limits.h>
#include <arpa/inet.h>
#include <endian.h>
typedef uint64_t u64;typedef uint32_t u32;typedef uint32_t fdt32_t;typedef uint64_t fdt64_t;typedef uint64_t mk_phys_cpu_t;
#define fdt32_to_cpu(v) ntohl(v)
#define fdt64_to_cpu(v) be64toh(v)
static uint32_t fdt32_ld(const void *p){uint32_t v;memcpy(&v,p,4);return ntohl(v);}
static uint64_t fdt64_ld(const void *p){uint64_t v;memcpy(&v,p,8);return be64toh(v);}
#define CHECK(c) do{if(!(c)){fprintf(stderr,"overlay scalars line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
#define FDT_ERR_NOTFOUND 1
#define MAX_NUMNODES 4
#define NUMA_NO_NODE -1
#define GFP_KERNEL 0
#define MK_STATE_READY 1
#define IS_ERR(p) ((intptr_t)(p)<0)
#define PTR_ERR(p) ((int)(intptr_t)(p))
struct list_head {int unused;};struct kernfs_node {int unused;};struct mk_cpu_set {int unused;};struct mk_dt_config {int unused;void *cpus;};
struct mk_instance {u64 generation;int id,state,refcount;char *name;struct mk_cpu_set *cpus;struct kernfs_node *kn;struct list_head list,memory_regions,pci_devices,platform_devices;void *dtb_data;size_t dtb_size;};
struct mk_overlay_tx {bool validate_only;u64 operation_generation;int id;size_t dtbo_size;};struct mk_overlay_target {int kind;};
static struct list_head mk_instance_list;static struct kernfs_node directory,*mk_instances_kn=&directory;
static int mk_instance_mutex,mk_instance_idr;
static _Alignas(8) unsigned char storage[32];static const char *property;
static int present,length,read_error,allocations,live,lookups,resource_lookups,directories,attributes,reservations,parses,locks,range_start,range_end;
static struct mk_instance *owned;
static u64 next_generation;
static int generation_error,parse_error,config_error;
static int mk_instance_assign_generation(struct mk_instance *instance){if(generation_error)return generation_error;instance->generation=++next_generation;return 0;}
static void logger(const char *fmt,...){(void)fmt;}
#define pr_info logger
#define pr_err logger
static const void *fdt_getprop(const void *fdt,int node,const char *name,int *len){(void)fdt;(void)node;CHECK(!strcmp(name,property));*len=present?length:(read_error?read_error:-FDT_ERR_NOTFOUND);return present?storage+4:NULL;}
static int fdt_subnode_offset(const void *fdt,int node,const char *name){(void)fdt;(void)node;CHECK(!strcmp(name,"resources"));resource_lookups++;return 1;}
static void *allocate(size_t n){void *p=calloc(1,n);CHECK(p);allocations++;live++;return p;}
#define kzalloc(n,f) allocate(n)
static void kfree(void *p){if(p){live--;free(p);}}
static char *kstrdup(const char *s,int f){(void)f;size_t n=strlen(s)+1;char *p=allocate(n);memcpy(p,s,n);return p;}
static struct mk_cpu_set *mk_cpu_set_alloc(void){return allocate(sizeof(struct mk_cpu_set));}
static void mk_cpu_set_free(struct mk_cpu_set *p){kfree(p);}
static int mk_dt_config_init(struct mk_dt_config *p){p->unused=0;p->cpus=config_error?NULL:p;return config_error?-ENOMEM:0;}
static void mk_dt_config_free(struct mk_dt_config *p){(void)p;}
static int mk_dt_parse_resources(const void *fdt,int node,const char *name,struct mk_dt_config *p){(void)fdt;(void)node;(void)name;(void)p;parses++;return parse_error;}
static void INIT_LIST_HEAD(struct list_head *p){p->unused=0;}
static void list_add_tail(struct list_head *p,struct list_head *head){(void)p;(void)head;}
static void kref_init(int *p){*p=1;}
static void mk_instance_get(struct mk_instance *p){p->refcount++;}
static void mk_instance_put(struct mk_instance *p){p->refcount--;}
static struct kernfs_node *kernfs_create_dir(struct kernfs_node *root,const char *name,int mode,struct mk_instance *instance){(void)root;(void)name;(void)mode;directories++;owned=instance;return &directory;}
static void kernfs_remove(struct kernfs_node *p){(void)p;}
static void kernfs_activate(struct kernfs_node *p){(void)p;}
static int mk_create_instance_files(struct mk_instance *p){(void)p;attributes++;return 0;}
static int mk_instance_reserve_resources(struct mk_instance *p,struct mk_dt_config *c){(void)p;(void)c;reservations++;return 0;}
static int mk_dt_generate_instance_dtb(struct mk_instance *p,void **data,size_t *size){(void)p;*size=4;*data=allocate(4);return 0;}
static void mk_instance_free_memory(struct mk_instance *p){(void)p;}
static void mk_instance_set_state(struct mk_instance *p,int state){p->state=state;}
static int idr_alloc(int *idr,struct mk_instance *p,int first,int end,int flags){(void)idr;(void)p;(void)flags;CHECK(first>0&&end>first);range_start=first;range_end=end;return first==1&&end==INT_MAX?7:first;}
static void idr_remove(int *idr,int id){(void)idr;(void)id;}
static void *idr_find(int *idr,int id){(void)idr;(void)id;lookups++;return NULL;}
static void mutex_lock(int *p){(void)p;locks++;}
static void mutex_unlock(int *p){(void)p;locks--;}
static struct mk_instance *mk_instance_find_by_name(const char *name){(void)name;lookups++;return owned;}
static int mk_overlay_check_namespace_target(struct mk_overlay_tx *tx,const struct mk_overlay_target *t,const char *name){(void)tx;(void)t;(void)name;return 0;}
static const char *mk_overlay_instance_name(struct mk_overlay_tx *tx,const void *fdt,int node,const char *verb,const char *op){(void)tx;(void)fdt;(void)node;(void)verb;(void)op;return "alpha";}
'''
# Keep both production versions compilable so old controls fail at runtime.
if 'static int mk_overlay_parse_instance_id(' in s:code+=function(s,'mk_overlay_parse_instance_id')
for name in ['mk_overlay_parse_u32','mk_overlay_parse_numa','mk_overlay_parse_cpu_reg']:code+=function(s,name)
code+=function(k,'mk_create_instance_from_dtb')+function(s,'mk_overlay_op_instance_create')
code+='static int read_scalar(u32 *value){'+('return mk_overlay_parse_u32(NULL,1,property,value);' if 'static int mk_overlay_parse_u32(' in s else '*value=mk_overlay_parse_u32(NULL,1,property);return 0;')+'}\n'
code+=r'''
static void reset(const char *name){CHECK(!live);property=name;present=1;length=4;read_error=allocations=lookups=resource_lookups=directories=attributes=reservations=parses=locks=range_start=range_end=0;owned=NULL;generation_error=parse_error=config_error=0;memset(storage,0,sizeof(storage));}
static void cell(u64 value,int n){length=n;for(int i=0;i<n;i++)storage[4+i]=(unsigned char)(value>>(8*(n-i-1)));}
static void cleanup(void){CHECK(owned);kfree(owned->dtb_data);mk_cpu_set_free(owned->cpus);kfree(owned->name);kfree(owned);owned=NULL;CHECK(!live);}
static void no_effects(void){CHECK(!allocations&&!live&&!lookups&&!resource_lookups&&!directories&&!attributes&&!reservations&&!parses&&!locks);}
static void scalars(void){
 for(int field=0;field<2;field++){
  reset(field?"mem-type":"flags");u32 out=99;present=0;CHECK(read_scalar(&out)==0&&out==0);
  reset(field?"mem-type":"flags");cell(UINT32_MAX,4);CHECK(read_scalar(&out)==0&&out==UINT32_MAX);
  for(int n=0;n<=8;n++){if(n==4)continue;reset(field?"mem-type":"flags");length=n;out=99;CHECK(read_scalar(&out)==-EINVAL&&out==99);no_effects();}
  reset(field?"mem-type":"flags");present=0;read_error=-9;out=99;CHECK(read_scalar(&out)==-EINVAL&&out==99);no_effects();
 }
 puts("OVERLAY_OPTIONAL_U32_ABSENCE_ONLY_DEFAULT_AND_EXACT_WIDTH_PASS");}
static void numa(void){
 int out=99;reset("numa-node-id");present=0;CHECK(mk_overlay_parse_numa(NULL,1,"cpu@0",&out)==0&&out==NUMA_NO_NODE);
 for(u32 n=0;n<MAX_NUMNODES;n++){reset("numa-node-id");cell(n,4);CHECK(mk_overlay_parse_numa(NULL,1,"cpu@0",&out)==0&&out==(int)n);}
 reset("numa-node-id");present=0;read_error=-9;out=99;CHECK(mk_overlay_parse_numa(NULL,1,"cpu@0",&out)==-EINVAL&&out==99);
 reset("numa-node-id");cell(MAX_NUMNODES,4);out=99;CHECK(mk_overlay_parse_numa(NULL,1,"cpu@0",&out)==-EINVAL&&out==99);
 puts("OVERLAY_NUMA_READ_ERROR_AND_RANGE_REFUSAL_PASS");}
static int create(void){struct mk_overlay_tx tx={.id=1,.dtbo_size=128};struct mk_overlay_target target={0};int ret=mk_overlay_op_instance_create(&tx,NULL,1,&target);if(!ret&&owned)CHECK(tx.operation_generation==owned->generation);return ret;}
static void valid_id(u32 value,int absent,int expected){reset("id");cell(value,4);present=!absent;CHECK(create()==0&&owned&&owned->id==expected&&reservations==1&&!locks);cleanup();}
static void ids(void){
 valid_id(0,1,7);valid_id(UINT32_MAX,0,7);valid_id(61,0,61);valid_id(INT_MAX-1,0,INT_MAX-1);
 u32 bad[]={0x80000000U,0xfffffffeU,INT_MAX,0};
 for(unsigned i=0;i<sizeof(bad)/sizeof(bad[0]);i++){reset("id");cell(bad[i],4);CHECK(create()==-EINVAL);no_effects();}
 for(int n=0;n<=8;n++){if(n==4)continue;reset("id");length=n;CHECK(create()==-EINVAL);no_effects();}
 reset("id");present=0;read_error=-9;CHECK(create()==-EINVAL);no_effects();
 puts("OVERLAY_ID_CHECKED_BEFORE_LOOKUP_OR_ALLOCATION_PASS");}
static void allocator(void){
 int good[]={-1,1,61,INT_MAX-1};for(unsigned i=0;i<sizeof(good)/sizeof(good[0]);i++){reset("id");CHECK(mk_create_instance_from_dtb("alpha",good[i],NULL,1,128)==0);CHECK(range_start==(good[i]==-1?1:good[i])&&range_end==(good[i]==-1?INT_MAX:good[i]+1));cleanup();}
 int bad[]={-2,INT_MIN,0,INT_MAX};for(unsigned i=0;i<sizeof(bad)/sizeof(bad[0]);i++){reset("id");CHECK(mk_create_instance_from_dtb("alpha",bad[i],NULL,1,128)==-EINVAL);no_effects();}
 puts("ALLOCATOR_ID_INVARIANT_BEFORE_PRIVATE_OR_KERNFS_EFFECTS_PASS");}
static void generations(void){
 reset("id");CHECK(!mk_create_instance_from_dtb("alpha",61,NULL,1,128));u64 first=owned->generation;CHECK(first);cleanup();
 reset("id");CHECK(!mk_create_instance_from_dtb("alpha",61,NULL,1,128)&&owned->generation>first);cleanup();
 reset("id");generation_error=-EOVERFLOW;CHECK(mk_create_instance_from_dtb("alpha",61,NULL,1,128)==-EOVERFLOW);
 CHECK(allocations==1&&!live&&!directories&&!attributes&&!reservations&&!parses&&!lookups&&!range_start);
 puts("INSTANCE_RUNTIME_GENERATION_REUSE_AND_EXHAUSTION_CLEANUP_PASS");}
static void overflow(void){reset("id");CHECK(mk_create_instance_from_dtb("alpha",INT_MAX,NULL,1,128)==-EINVAL);no_effects();puts("ALLOCATOR_EXCLUSIVE_ID_BOUND_NO_SIGNED_OVERFLOW_PASS");}
static void cpu(void){
 mk_phys_cpu_t out=0;reset("reg");cell(0x1234567887654321ULL,8);CHECK(((uintptr_t)(storage+4)&7)==4);CHECK(mk_overlay_parse_cpu_reg(NULL,1,"cpu@0",&out)==0&&out==0x1234567887654321ULL);
 reset("reg");cell(0xfedcba98,4);CHECK(mk_overlay_parse_cpu_reg(NULL,1,"cpu@0",&out)==0&&out==0xfedcba98);
 puts("OVERLAY_CPU_UNALIGNED_SAFE_32_AND_64_CELL_LOAD_PASS");}
static void preflight(void){
 for(int error=0;error<3;error++){
  reset("id");cell(61,4);parse_error=error==1?-EINVAL:0;config_error=error==2;
  struct mk_overlay_tx tx={.id=1,.dtbo_size=128,.validate_only=true};struct mk_overlay_target target={0};
  CHECK(mk_overlay_op_instance_create(&tx,NULL,1,&target)==(config_error?-ENOMEM:parse_error));
  CHECK(parses==!config_error&&resource_lookups==1&&!lookups&&!locks&&!allocations&&!live&&!directories&&!attributes&&!reservations&&!owned&&!tx.operation_generation);
 }
 puts("INSTANCE_CREATE_PREFLIGHT_CONFIG_READ_NO_ID_OR_RESOURCE_EFFECTS_PASS");}
static void config_failure(void){
 for(int error=0;error<2;error++){
  reset("id");config_error=1;
  CHECK(mk_create_instance_from_dtb("alpha",error?-1:61,NULL,1,128)==-ENOMEM);
  CHECK(!live&&!owned&&!lookups&&!parses&&!directories&&!attributes&&!reservations&&!range_start);
 }
 puts("INSTANCE_RUNTIME_CONFIG_INIT_FAILURE_FREES_PRIVATE_STATE_BEFORE_EFFECTS_PASS");}
int main(int argc,char **argv){CHECK(argc==2);if(!strcmp(argv[1],"scalars"))scalars();else if(!strcmp(argv[1],"numa"))numa();else if(!strcmp(argv[1],"ids"))ids();else if(!strcmp(argv[1],"allocator"))allocator();else if(!strcmp(argv[1],"generations"))generations();else if(!strcmp(argv[1],"overflow"))overflow();else if(!strcmp(argv[1],"cpu"))cpu();else if(!strcmp(argv[1],"preflight"))preflight();else if(!strcmp(argv[1],"config-failure"))config_failure();else{scalars();numa();ids();allocator();overflow();cpu();}return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-overlay-scalars-') as tmp:
    root=Path(tmp);(root/'fixture.c').write_text(code)
    subprocess.run(['gcc','-std=gnu11','-O1','-Wall','-Wextra','-Werror','-Wno-unused-function','-fsanitize=undefined','-fno-sanitize-recover=all',str(root/'fixture.c'),'-o',str(root/'fixture')],check=True)
    print('PRODUCTION_FUNCTIONS_COMPILE_PASS',flush=True)
    subprocess.run([str(root/'fixture'),a.section],check=True)
