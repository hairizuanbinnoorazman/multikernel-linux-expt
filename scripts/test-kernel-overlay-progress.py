#!/usr/bin/env python3
"""Exercise actual overlay completion/rollback functions with modeled effects.

This verifies ledger decisions, not kernel resource atomicity or instance identity.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('source',type=Path)
p.add_argument('--section',choices=['progress','identity','irreversible','all'],default='all')
a=p.parse_args();source=a.source.read_text()
def function(name):
    at=next(source.index(prefix+name+'(') for prefix in ('static int ','int ') if prefix+name+'(' in source)
    return source[at:source.index('\n}',at)+2]+'\n'
code=r'''
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#define CHECK(c) do{if(!(c)){fprintf(stderr,"overlay progress line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
#define GFP_KERNEL 0
#define PAGE_SHIFT 12
#define MK_OVERLAY_TARGET_POOL 0
#define MK_OVERLAY_TARGET_INSTANCE 1
#define MK_OVERLAY_TARGET_INSTANCES 2
static void logger(const char *format,...){(void)format;}
#define pr_err logger
#define pr_info logger
#define pr_warn logger
typedef uint8_t u8;
typedef uint64_t u64;
typedef uint32_t u32;
struct mk_overlay_step {int fragment;unsigned int op;u64 generation;};
struct list_head {bool linked;};
struct mk_overlay_tx {int id,status;struct list_head list;void *dtbo_data;struct mk_overlay_step *steps;unsigned int steps_done;bool rollback_uncertain;bool validate_only;size_t dtbo_size;u64 operation_generation;};
struct mk_instance {int id;const char *name;u64 generation;};
struct mk_overlay_target {int kind;const char *path;struct mk_instance *instance;u64 generation;};
struct mk_overlay_fragment {u32 unit;int node;};
static struct mk_instance instance={.id=7,.name="existing",.generation=17};
static int mk_instance_mutex;
static void mutex_lock(int *p){(void)p;}
static void mutex_unlock(int *p){(void)p;}
static int live,fail_ledger,collect_error,resolve_calls,resolve_fail_call;
static int apply_partial,apply_fail,inverse_fail,forward[64],inverse[64],nf,ni,mutations,preexisting;
static void *allocate(size_t n){void *p=calloc(1,n);CHECK(p);live++;return p;}
static void *kcalloc(size_t n,size_t size,int flags){(void)flags;return fail_ledger?NULL:allocate(n*size);}
static void kfree(void *p){if(p){live--;free(p);}}
static int mk_overlay_collect_fragments(const void *fdt,struct mk_overlay_fragment **out){(void)fdt;if(collect_error)return collect_error;*out=allocate(2*sizeof(**out));(*out)[0].node=10;(*out)[1].node=20;return 2;}
static int mk_overlay_resolve_target(struct mk_overlay_tx *tx,const void *fdt,int fragment,struct mk_overlay_target *target){(void)tx;(void)fdt;
 if(++resolve_calls==resolve_fail_call)return -ENOENT;
 target->kind=fragment==10?MK_OVERLAY_TARGET_POOL:MK_OVERLAY_TARGET_INSTANCE;target->path="target";target->instance=&instance;target->generation=instance.generation;return 0;}
static int mk_overlay_fragment_overlay_node(struct mk_overlay_tx *tx,const void *fdt,int fragment,const struct mk_overlay_target *target){(void)tx;(void)fdt;(void)target;return fragment;}
'''
at=source.index('enum mk_overlay_op {');end=source.index('static int mk_overlay_run_op(',at)
code+=source[at:end]
code+=r'''
static int mk_overlay_run_op(struct mk_overlay_tx *tx,const void *fdt,int node,const struct mk_overlay_target *target,enum mk_overlay_op op,bool undo){
 (void)tx;(void)fdt;(void)target;int key=node*8+op;
 if(undo){inverse[ni++]=key;if(op==MK_OVERLAY_OP_INSTANCE_CREATE)preexisting=0;mutations++;if(key==inverse_fail)return -EIO;}
 else{tx->operation_generation=instance.generation;forward[nf++]=key;if(key==apply_fail){if(apply_partial)mutations++;return -EEXIST;}mutations++;}
 return 0;}
'''
if 'static int mk_overlay_walk_fragment(' in source:code+=function('mk_overlay_walk_fragment')
code+=function('mk_overlay_apply_fragment')
if 'static int mk_overlay_rollback_fragment(' in source:code+=function('mk_overlay_rollback_fragment')
# The dedicated transaction-preflight fixture covers validation; this fixture
# keeps its existing modeled apply/rollback failure schedules.
if 'static int mk_overlay_preflight_fragments(' in source:
 code+='static int mk_overlay_preflight_fragments(struct mk_overlay_tx *tx,const void *fdt,const struct mk_overlay_fragment *frags,int nr){(void)tx;(void)fdt;(void)frags;(void)nr;return 0;}\n'
for n in ('mk_overlay_parse_and_apply','mk_overlay_parse_and_rollback'):code+=function(n)
code+=r'''
static int mk_overlay_check_resource_target(struct mk_overlay_tx *tx,const struct mk_overlay_target *t,const char *name){(void)tx;(void)name;return t->kind==MK_OVERLAY_TARGET_INSTANCES?-EINVAL:0;}
static int mk_overlay_check_namespace_target(struct mk_overlay_tx *tx,const struct mk_overlay_target *t,const char *name){(void)tx;(void)name;return t->kind==MK_OVERLAY_TARGET_INSTANCES?0:-EINVAL;}
static const char *mk_overlay_instance_name(struct mk_overlay_tx *tx,const void *fdt,int op,const char *verb,const char *name){(void)tx;(void)fdt;(void)op;(void)verb;(void)name;return "existing";}
static struct mk_instance *mk_instance_find_by_name(const char *name){(void)name;return &instance;}
static int mk_instance_destroy(struct mk_instance *i){(void)i;mutations++;return 0;}
#define fdt_for_each_subnode(n,f,p) for((void)(f),(void)(p),(n)=1;(n)==1;(n)++)
static const char *fdt_get_name(const void *fdt,int node,int *len){(void)fdt;(void)node;(void)len;return "memory@0";}
static const void *fdt_getprop(const void *fdt,int node,const char *name,int *len){(void)fdt;(void)node;(void)name;*len=-1;return NULL;}
static int mk_overlay_parse_numa(const void *fdt,int node,const char *name,int *value){(void)fdt;(void)node;(void)name;*value=0;return 0;}
static int mk_overlay_parse_u32(const void *fdt,int node,const char *name,u32 *value){(void)fdt;(void)node;(void)name;*value=0;return 0;}
static int mk_overlay_parse_mem_size(const void *fdt,int node,const char *name,u64 *value){(void)fdt;(void)node;(void)name;*value=4096;return 0;}
static int mk_overlay_parse_mem_reg(const void *fdt,int node,const char *name,u64 *base,u64 *size){(void)fdt;(void)node;(void)name;*base=*size=4096;return 0;}
static int mk_pool_mem_grow(u64 size,int node,u64 *base){(void)size;(void)node;(void)base;mutations++;return 0;}
static int mk_send_mem_remove(int id,u64 base,u64 size){(void)id;(void)base;(void)size;mutations++;return 0;}
static int mk_send_mem_add(int id,u64 base,u64 size,u32 node,u32 type){(void)id;(void)base;(void)size;(void)node;(void)type;mutations++;return 0;}
'''
code+=r'''
#define MK_OVERLAY_TX_APPLIED 1
#define MK_OVERLAY_TX_FAILED 2
#define MK_OVERLAY_TX_REMOVED 3
#define scoped_guard(t,p) for(int once=0;once<1;once++)
struct kernfs_node {void *priv;struct kernfs_node *parent;};
static struct kernfs_node root_node,*mk_overlay_root_kn=&root_node;
static int removed_self;
static struct kernfs_node *kernfs_get_parent(struct kernfs_node *kn){return kn->parent;}
static void kernfs_put(struct kernfs_node *kn){(void)kn;}
static bool list_empty(struct list_head *list){return !list->linked;}
static void list_del_init(struct list_head *list){list->linked=false;}
static void kernfs_remove_self(struct kernfs_node *kn){(void)kn;removed_self++;}
'''
code+=function('mk_overlay_rmdir')
code+=function('mk_overlay_op_memory_add')+function('mk_overlay_op_instance_remove')+function('mk_overlay_undo_instance_create')
code+=r'''
static void reset(void){CHECK(!live);apply_partial=removed_self=fail_ledger=collect_error=resolve_calls=resolve_fail_call=nf=ni=mutations=0;apply_fail=inverse_fail=-1;preexisting=1;instance.generation=17;}
static void cleanup(struct mk_overlay_tx *tx){kfree(tx->steps);memset(tx,0,sizeof(*tx));CHECK(!live);}
static void progress(void){struct mk_overlay_tx tx={0};
 reset();apply_fail=10*8+MK_OVERLAY_OP_INSTANCE_CREATE;
 CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-EEXIST&&nf==1);
 CHECK(mk_overlay_parse_and_rollback(&tx,NULL)==-EUCLEAN);CHECK(!ni&&preexisting);cleanup(&tx);
 reset();apply_fail=10*8+MK_OVERLAY_OP_CPU_REMOVE;apply_partial=1;
 CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-EEXIST&&tx.steps_done==1&&mutations==2);
 CHECK(mk_overlay_parse_and_rollback(&tx,NULL)==-EUCLEAN&&!ni);cleanup(&tx);
 reset();fail_ledger=1;CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-ENOMEM&&!nf&&!live);cleanup(&tx);
 reset();collect_error=-EINVAL;CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-EINVAL&&!nf&&!live);CHECK(!mk_overlay_parse_and_rollback(&tx,NULL));cleanup(&tx);
 reset();resolve_fail_call=2;CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-ENOENT&&nf==8);resolve_fail_call=0;
 CHECK(!mk_overlay_parse_and_rollback(&tx,NULL)&&ni==8&&!tx.steps_done);
 for(int i=0;i<ni;i++)CHECK(inverse[i]==forward[nf-1-i]);cleanup(&tx);
 reset();CHECK(!mk_overlay_parse_and_apply(&tx,NULL)&&nf==16);CHECK(!mk_overlay_parse_and_rollback(&tx,NULL)&&ni==16&&!tx.steps_done);
 for(int i=0;i<ni;i++)CHECK(inverse[i]==forward[nf-1-i]);CHECK(!mk_overlay_parse_and_rollback(&tx,NULL)&&ni==16);cleanup(&tx);
 reset();CHECK(!mk_overlay_parse_and_apply(&tx,NULL));inverse_fail=forward[nf-3];
 CHECK(mk_overlay_parse_and_rollback(&tx,NULL)==-EIO&&ni==3&&tx.steps_done==14&&tx.rollback_uncertain);
 CHECK(mk_overlay_parse_and_rollback(&tx,NULL)==-EUCLEAN&&ni==3&&tx.steps_done==14);cleanup(&tx);
 reset();CHECK(!mk_overlay_parse_and_apply(&tx,NULL));resolve_calls=0;resolve_fail_call=3;
 CHECK(mk_overlay_parse_and_rollback(&tx,NULL)==-ENOENT&&ni==2&&tx.steps_done==14&&!tx.rollback_uncertain);
 resolve_fail_call=0;CHECK(!mk_overlay_parse_and_rollback(&tx,NULL)&&ni==16&&!tx.steps_done);
 for(int i=0;i<ni;i++)CHECK(inverse[i]==forward[nf-1-i]);cleanup(&tx);
 puts("OVERLAY_PREALLOCATED_COMPLETION_LEDGER_AND_REVERSE_CONSUMPTION_PASS");
 puts("OVERLAY_FAILED_APPLY_NO_INVERSE_OF_PREEXISTING_INSTANCE_PASS");
 puts("OVERLAY_PARTIAL_INVERSE_NO_REPLAY_AND_LOOKUP_RETRY_CURSOR_PASS");}
static void rmdir_cases(void){struct kernfs_node kn={.parent=&root_node};struct mk_overlay_tx *tx;
 reset();tx=allocate(sizeof(*tx));tx->dtbo_data=allocate(1);tx->list.linked=true;kn.priv=tx;
 apply_fail=10*8+MK_OVERLAY_OP_INSTANCE_CREATE;CHECK(mk_overlay_parse_and_apply(tx,NULL)==-EEXIST);
 CHECK(mk_overlay_rmdir(&kn)==-EUCLEAN&&tx->status==MK_OVERLAY_TX_FAILED&&tx->list.linked&&live==3&&!removed_self&&!ni&&preexisting);
 kfree(tx->steps);kfree(tx->dtbo_data);kfree(tx);CHECK(!live);
 reset();tx=allocate(sizeof(*tx));tx->dtbo_data=allocate(1);tx->list.linked=true;kn.priv=tx;
 CHECK(!mk_overlay_parse_and_apply(tx,NULL));inverse_fail=forward[nf-3];
 CHECK(mk_overlay_rmdir(&kn)==-EIO&&tx->status==MK_OVERLAY_TX_FAILED&&live==3&&!removed_self&&ni==3);
 CHECK(mk_overlay_rmdir(&kn)==-EUCLEAN&&live==3&&ni==3&&tx->list.linked);
 kfree(tx->steps);kfree(tx->dtbo_data);kfree(tx);CHECK(!live);
 reset();tx=allocate(sizeof(*tx));tx->dtbo_data=allocate(1);tx->list.linked=true;kn.priv=tx;
 CHECK(!mk_overlay_parse_and_apply(tx,NULL));resolve_calls=0;resolve_fail_call=3;
 CHECK(mk_overlay_rmdir(&kn)==-ENOENT&&tx->status==MK_OVERLAY_TX_FAILED&&live==3&&ni==2);
 resolve_fail_call=0;CHECK(!mk_overlay_rmdir(&kn)&&!live&&ni==16&&removed_self==1);
 for(int i=0;i<ni;i++)CHECK(inverse[i]==forward[nf-1-i]);
 puts("OVERLAY_RMDIR_RETAINS_UNCERTAIN_RECORD_AND_FREES_COMPLETED_LEDGER_PASS");}
static void identity(void){struct mk_overlay_tx tx={0};struct mk_overlay_target target={.kind=MK_OVERLAY_TARGET_INSTANCES,.instance=&instance};
 reset();instance.generation=(1ULL<<40)|17;CHECK(!mk_overlay_parse_and_apply(&tx,NULL)&&tx.steps_done==16);
 instance.generation=17;CHECK(mk_overlay_parse_and_rollback(&tx,NULL)==-ESTALE&&!ni&&tx.steps_done==16&&!tx.rollback_uncertain);cleanup(&tx);
 reset();tx.operation_generation=(1ULL<<40)|17;instance.generation=17;
 CHECK(mk_overlay_undo_instance_create(&tx,NULL,1,&target)==-ESTALE&&!mutations);
 instance.generation=(1ULL<<40)|17;CHECK(!mk_overlay_undo_instance_create(&tx,NULL,1,&target)&&mutations==1);
 reset();tx.operation_generation=0;CHECK(mk_overlay_undo_instance_create(&tx,NULL,1,&target)==-ESTALE&&!mutations);
 puts("OVERLAY_REUSED_NAME_ID_RESOURCE_AND_CREATED_INSTANCE_REFUSAL_PASS");}
static void irreversible(void){reset();struct mk_overlay_tx tx={0};struct mk_overlay_target target={.kind=MK_OVERLAY_TARGET_POOL,.instance=&instance,.path="pool"};
 CHECK(mk_overlay_op_memory_add(&tx,NULL,1,&target,true)==-EOPNOTSUPP&&!mutations);
 target.kind=MK_OVERLAY_TARGET_INSTANCES;CHECK(mk_overlay_op_instance_remove(&tx,NULL,1,&target,true)==-EOPNOTSUPP&&!mutations);
 reset();tx.validate_only=true;CHECK(!mk_overlay_op_instance_remove(&tx,NULL,1,&target,false)&&!mutations);
 puts("OVERLAY_UNSUPPORTED_INVERSES_REFUSE_WITHOUT_FALSE_SUCCESS_PASS");}
int main(int argc,char **argv){CHECK(argc==2);if(!strcmp(argv[1],"progress")){progress();rmdir_cases();}else if(!strcmp(argv[1],"identity"))identity();else if(!strcmp(argv[1],"irreversible"))irreversible();else{progress();rmdir_cases();identity();irreversible();}return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-overlay-progress-') as tmp:
    root=Path(tmp);(root/'fixture.c').write_text(code)
    subprocess.run(['gcc','-std=gnu11','-O1','-Wall','-Wextra','-Werror','-Wno-unused-const-variable','-Wno-unused-function','-Wno-misleading-indentation','-fsanitize=undefined','-fno-sanitize-recover=all',str(root/'fixture.c'),'-o',str(root/'fixture')],check=True)
    print('PRODUCTION_FUNCTIONS_COMPILE_PASS',flush=True)
    subprocess.run([str(root/'fixture'),a.section],check=True)
