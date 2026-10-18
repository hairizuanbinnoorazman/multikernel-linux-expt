#!/usr/bin/env python3
"""Run actual overlay metadata/selector/apply functions with modeled FDT/effects.

Checks property-local strings and pre-publication refusal, not full transactions.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('source',type=Path)
p.add_argument('--section',choices=['metadata','selectors','publication','ids','all'],default='all')
a=p.parse_args();s=a.source.read_text()
def function(name):
    at=next(s.index(prefix+name+'(') for prefix in ['static int ','static void ','static const char *'] if prefix+name+'(' in s)
    return s[at:s.index('\n}',at)+2]+'\n'
code=r'''
#include <stdint.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>
#include <errno.h>
#include <limits.h>
#define CHECK(c) do{if(!(c)){fprintf(stderr,"overlay strings line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
#define FDT_ERR_NOTFOUND 1
#define GFP_KERNEL 0
#define MK_OVERLAY_TARGET_POOL 0
#define MK_OVERLAY_TARGET_INSTANCES 1
#define MK_OVERLAY_TARGET_INSTANCE 2
#define MK_OVERLAY_POOL_PATH "/resources"
#define MK_OVERLAY_INSTANCES_PATH "/instances"
#define MK_OVERLAY_INSTANCE_PREFIX "/instances/"
#define MK_OVERLAY_TX_PENDING 0
#define MK_OVERLAY_TX_APPLIED 1
#define MK_OVERLAY_TX_FAILED 2
#define IS_ERR(p) ((intptr_t)(p)<0)
#define PTR_ERR(p) ((int)(intptr_t)(p))
#define guard(t) (void)
struct list_head {int dummy;};
struct kernfs_node {int dummy;};
struct mk_instance {int id;uint64_t generation;};
struct mk_overlay_step;
struct mk_overlay_tx {uint64_t operation_generation;struct mk_overlay_step *steps;unsigned int steps_done;bool rollback_uncertain;bool validate_only;int id,status;char target_path[128],resources[256];void *dtbo_data;size_t dtbo_size;struct kernfs_node *dir_kn;struct list_head list;};
struct mk_overlay_fragment {int node;};
struct mk_overlay_target {int kind;const char *path;struct mk_instance *instance;uint64_t generation;};
static char resources[512],target[256],instance_name[128];
static int resource_len,target_len,name_len,resource_error,missing_target,missing_name;
static int allocated,effects,published,lookups,locks,fragment_error,header_error;
static int allocation_failure,created_files,activated,listed,apply_error,removed;
static int mk_overlay_next_id,mk_overlay_mutex,mk_instance_mutex;
static int pool;static void *mk_cpu_pool=&pool;
static struct list_head mk_overlay_tx_list;
static struct kernfs_node directory,*mk_overlay_root_kn=&directory;
static struct mk_instance existing={.id=7,.generation=17};
static struct mk_overlay_tx *last_tx;
static void logger(const char *fmt,...){(void)fmt;}
#define pr_info logger
#define pr_err logger
#define pr_warn logger
static void *zalloc(size_t size){void *p=calloc(1,size);CHECK(p);allocated++;return p;}
#define kzalloc(n,f) zalloc(n)
static void *kmemdup(const void *source,size_t n,int flags){(void)flags;void *p=zalloc(n);memcpy(p,source,n);return p;}
static void kfree(void *p){if(p){allocated--;free(p);}}
static void strscpy(char *to,const char *from,size_t n){size_t len=strnlen(from,n-1);memcpy(to,from,len);to[len]=0;}
static int fdt_check_header(const void *fdt){(void)fdt;return header_error;}
static int fdt_path_offset(const void *fdt,const char *path){(void)fdt;(void)path;return 0;}
static const void *fdt_getprop(const void *fdt,int node,const char *name,int *len){(void)fdt;(void)node;
 if(!strcmp(name,"mk,resources")){*len=resource_error?resource_error:resource_len;return resource_error?NULL:resources;}
 if(!strcmp(name,"target-path")){*len=missing_target?-FDT_ERR_NOTFOUND:target_len;return missing_target?NULL:target;}
 if(!strcmp(name,"instance-name")){*len=missing_name?-FDT_ERR_NOTFOUND:name_len;return missing_name?NULL:instance_name;}
 abort();}
static int mk_overlay_collect_fragments(const void *fdt,struct mk_overlay_fragment **out){(void)fdt;if(fragment_error)return fragment_error;*out=zalloc(sizeof(**out));(*out)->node=1;return 1;}
static void mutex_lock(int *p){(void)p;locks++;}
static void mutex_unlock(int *p){(void)p;locks--;}
static struct mk_instance *mk_instance_find_by_name(const char *name){lookups++;return !strcmp(name,"alpha")?&existing:NULL;}
#define atomic_inc_return(value) ((*(value))=(int)((unsigned int)(*(value))+1U))
static void INIT_LIST_HEAD(struct list_head *p){p->dummy=0;}
static void list_add_tail(struct list_head *p,struct list_head *head){(void)p;(void)head;listed++;}
static int mk_overlay_parse_and_apply(struct mk_overlay_tx *tx,const void *fdt){(void)tx;(void)fdt;effects++;return apply_error;}
static struct kernfs_node *kernfs_create_dir(struct kernfs_node *root,const char *name,int mode,struct mk_overlay_tx *tx){(void)root;(void)name;(void)mode;if(allocation_failure==1)return (void *)(intptr_t)-ENOMEM;published++;last_tx=tx;return &directory;}
struct kernfs_ops {int dummy;};
static struct kernfs_ops tx_id_ops,tx_status_ops,tx_instance_ops,tx_resources_ops,tx_dtbo_ops;
#define GLOBAL_ROOT_UID 0
#define GLOBAL_ROOT_GID 0
static struct kernfs_node *__kernfs_create_file(struct kernfs_node *parent,const char *name,int mode,int uid,int gid,int size,struct kernfs_ops *ops,struct mk_overlay_tx *tx,void *a,void *b){
 (void)parent;(void)name;(void)mode;(void)uid;(void)gid;(void)size;(void)ops;(void)tx;(void)a;(void)b;
 created_files++;if(allocation_failure==created_files+1)return (void *)(intptr_t)-ENOMEM;return &directory;}

static void kernfs_activate(struct kernfs_node *kn){(void)kn;activated++;}
static void kernfs_remove(struct kernfs_node *kn){(void)kn;removed++;last_tx=NULL;}
'''
if 'static int mk_overlay_get_string(' in s:code+=function('mk_overlay_get_string')
for name in ['mk_overlay_parse_metadata','mk_overlay_resolve_target','mk_overlay_instance_name','mk_overlay_create_tx_files','mk_overlay_apply']:code+=function(name)
code+=r'''
static void reset(void){CHECK(!allocated);memset(resources,0,sizeof(resources));strcpy(resources,"cpu,memory");resource_len=strlen(resources)+1;resource_error=0;strcpy(target,"/resources");target_len=strlen(target)+1;strcpy(instance_name,"alpha");name_len=6;missing_target=missing_name=header_error=fragment_error=0;effects=published=lookups=locks=mk_overlay_next_id=0;last_tx=NULL;allocation_failure=created_files=activated=listed=apply_error=removed=0;mk_cpu_pool=&pool;}
static int apply(void){char data=0;return mk_overlay_apply(&data,1);}
static void cleanup(void){CHECK(last_tx);kfree(last_tx->dtbo_data);kfree(last_tx);last_tx=NULL;CHECK(!allocated);}
static void refused(int error){CHECK(apply()==error);CHECK(!allocated&&!effects&&!published&&!mk_overlay_next_id);}
static void metadata(void){
 reset();CHECK(apply()==1&&effects==1&&published==1&&mk_overlay_next_id==1);CHECK(!strcmp(last_tx->resources,"cpu,memory")&&!strcmp(last_tx->target_path,"/resources"));cleanup();
 reset();resource_error=-FDT_ERR_NOTFOUND;CHECK(apply()==1&&!strcmp(last_tx->resources,"unknown"));cleanup();
 reset();resource_len=1;resources[0]=0;CHECK(apply()==1&&!last_tx->resources[0]);cleanup();
 reset();memset(resources,'a',511);resources[511]=0;resource_len=512;CHECK(apply()==1&&strlen(last_tx->resources)==255);cleanup();
 for(int c=0;c<4;c++){reset();if(c==0)resource_len=0;if(c==1){memcpy(resources,"abcdoutside",12);resource_len=4;}if(c==2){memcpy(resources,"a\0b\0",4);resource_len=4;}if(c==3)resource_error=-9;refused(-EINVAL);}
 for(int c=0;c<4;c++){reset();if(c==0){target_len=1;target[0]=0;}if(c==1){target_len=10;target[9]='x';}if(c==2){memcpy(target,"/resources\0x\0",13);target_len=13;}if(c==3)missing_target=1;refused(c==3?-ENOENT:-EINVAL);}
 reset();fragment_error=-ENOMEM;refused(-ENOMEM);
 puts("OVERLAY_METADATA_LOCAL_STRING_AND_PRE_PUBLICATION_REFUSAL_PASS");}
static void selectors(void){
 struct mk_overlay_tx tx={0};struct mk_overlay_target t;
 reset();CHECK(mk_overlay_resolve_target(&tx,NULL,1,&t)==0&&t.kind==MK_OVERLAY_TARGET_POOL&&!lookups);
 reset();strcpy(target,"/instances");target_len=strlen(target)+1;CHECK(mk_overlay_resolve_target(&tx,NULL,1,&t)==0&&t.kind==MK_OVERLAY_TARGET_INSTANCES);
 reset();strcpy(target,"/instances/alpha");target_len=strlen(target)+1;CHECK(mk_overlay_resolve_target(&tx,NULL,1,&t)==0&&t.instance==&existing&&lookups==1&&!locks);
 reset();mk_cpu_pool=NULL;CHECK(mk_overlay_resolve_target(&tx,NULL,1,&t)==-EPERM&&!lookups);
 reset();memcpy(target,"/instances/alpha\0x\0",19);target_len=19;CHECK(mk_overlay_resolve_target(&tx,NULL,1,&t)==-EINVAL&&!lookups&&!locks);
 reset();memcpy(target,"/resources\0x\0",13);target_len=13;CHECK(mk_overlay_resolve_target(&tx,NULL,1,&t)==-EINVAL&&!lookups);
 reset();CHECK(mk_overlay_instance_name(&tx,NULL,1,"Overlay","instance-create")==instance_name);
 reset();memcpy(instance_name,"alpha\0tail\0",11);name_len=11;CHECK(mk_overlay_instance_name(&tx,NULL,1,"Overlay","instance-create")==NULL);
 reset();missing_name=1;CHECK(!mk_overlay_instance_name(&tx,NULL,1,"Rollback","instance-create"));
 reset();strcpy(instance_name,"a/b");name_len=4;CHECK(!mk_overlay_instance_name(&tx,NULL,1,"Overlay","instance-remove"));
 reset();instance_name[0]=0;name_len=1;CHECK(!mk_overlay_instance_name(&tx,NULL,1,"Overlay","instance-create"));
 reset();strcpy(target,"/instances/future");target_len=strlen(target)+1;tx.validate_only=true;
 CHECK(!mk_overlay_resolve_target(&tx,NULL,1,&t)&&t.kind==MK_OVERLAY_TARGET_INSTANCE&&!t.instance&&!t.generation&&!lookups&&!locks);
 puts("OVERLAY_TARGET_AND_INSTANCE_NAME_NO_PREFIX_ALIAS_PASS");}
static void publication(void){
 for(int stage=1;stage<=6;stage++){reset();allocation_failure=stage;
  CHECK(apply()==-ENOMEM);CHECK(!effects&&!allocated&&!activated&&!listed);
  CHECK(created_files==(stage==1?0:stage-1));CHECK(removed==(stage==1?0:1));
 }
 reset();CHECK(apply()==1);CHECK(effects==1&&created_files==5&&activated==1&&listed==1&&last_tx->status==MK_OVERLAY_TX_APPLIED);cleanup();
 reset();apply_error=-EIO;CHECK(apply()==1);CHECK(effects==1&&created_files==5&&activated==1&&listed==1&&last_tx->status==MK_OVERLAY_TX_FAILED);cleanup();
 puts("OVERLAY_ALL_TRACKING_ALLOCATIONS_BEFORE_EFFECTS_AND_FAILURE_RECORD_PASS");}
static void ids(void){
 reset();mk_overlay_next_id=INT_MAX-1;
 CHECK(apply()==INT_MAX);CHECK(last_tx->id==INT_MAX&&effects==1&&listed==1);cleanup();
 for(int attempt=0;attempt<3;attempt++){
  effects=published=created_files=activated=listed=0;
  CHECK(apply()==-EOVERFLOW);CHECK(mk_overlay_next_id==INT_MAX);
  CHECK(!allocated&&!effects&&!published&&!created_files&&!activated&&!listed);
 }
 reset();allocation_failure=1;CHECK(apply()==-ENOMEM&&mk_overlay_next_id==1);
 allocation_failure=0;CHECK(apply()==2&&last_tx->id==2);cleanup();
 puts("OVERLAY_TRANSACTION_ID_SATURATION_NO_WRAP_OR_EFFECTS_PASS");}
int main(int argc,char **argv){CHECK(argc==2);if(!strcmp(argv[1],"metadata"))metadata();else if(!strcmp(argv[1],"selectors"))selectors();else if(!strcmp(argv[1],"publication"))publication();else if(!strcmp(argv[1],"ids"))ids();else{metadata();selectors();publication();ids();}return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-overlay-strings-') as tmp:
    root=Path(tmp);(root/'fixture.c').write_text(code)
    subprocess.run(['gcc','-std=gnu11','-O1','-Wall','-Wextra','-Werror','-fsanitize=undefined','-fno-sanitize-recover=all',str(root/'fixture.c'),'-o',str(root/'fixture')],check=True)
    print('PRODUCTION_FUNCTIONS_COMPILE_PASS',flush=True)
    subprocess.run([str(root/'fixture'),a.section],check=True)
