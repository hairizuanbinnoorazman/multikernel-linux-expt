#!/usr/bin/env python3
"""Exercise actual transaction preflight/dispatcher/apply driver with modeled ops.

Operation readers and resource backends are modeled here; dedicated item fixtures
exercise production readers/loops. This is not live resource atomicity proof.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("source", type=Path)
a = p.parse_args()
s = a.source.read_text()


def function(name):
    start = s.index("static int " + name + "(")
    return s[start:s.index("\n}", start) + 2] + "\n"


code = r'''
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
typedef unsigned char u8;
#define CHECK(c) do{if(!(c)){fprintf(stderr,"transaction preflight line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
#define GFP_KERNEL 0
#define MK_OVERLAY_TARGET_POOL 0
#define MK_OVERLAY_TARGET_INSTANCE 1
#define MK_OVERLAY_TARGET_INSTANCES 2
struct mk_overlay_step {int fragment,op;};
struct mk_overlay_tx {int id;size_t dtbo_size;bool validate_only,rollback_uncertain;unsigned int steps_done;struct mk_overlay_step *steps;};
struct mk_overlay_fragment {int node;};
struct mk_overlay_target {int kind;};
static int bad_fragment,bad_op,collect_error,resolve_error,overlay_error;
static int kinds[3],present[3];
static int reads,moves,live,ledger_allocations,fail_ledger,apply_error,absent;
static void logger(const char *format,...){(void)format;}
#define pr_err logger
static void *allocate(size_t count,size_t size){void *p=calloc(count,size);CHECK(p);live++;return p;}
static void *kcalloc(size_t count,size_t size,int flags){(void)flags;ledger_allocations++;return fail_ledger?NULL:allocate(count,size);}
static void kfree(void *p){if(p){free(p);live--;}}
static int mk_overlay_collect_fragments(const void *fdt,struct mk_overlay_fragment **out)
{(void)fdt;if(collect_error)return collect_error;*out=allocate(3,sizeof(**out));for(int i=0;i<3;i++)(*out)[i].node=i;return 3;}
static int mk_overlay_resolve_target(struct mk_overlay_tx *tx,const void *fdt,int fragment,struct mk_overlay_target *target)
{(void)fdt;CHECK(tx->validate_only);if(fragment==resolve_error)return -EINVAL;target->kind=kinds[fragment];return 0;}
static int mk_overlay_fragment_overlay_node(struct mk_overlay_tx *tx,const void *fdt,int fragment,const struct mk_overlay_target *target)
{(void)fdt;(void)target;CHECK(tx->validate_only);return fragment==overlay_error?-EINVAL:fragment;}
'''
start = s.index('enum mk_overlay_op {')
code += s[start:s.index('static int mk_overlay_run_op(', start)]
code += r'''
static int fdt_subnode_offset(const void *fdt,int fragment,const char *name)
{(void)fdt;for(int op=0;op<MK_OVERLAY_OP_COUNT;op++)if(!strcmp(name,mk_overlay_op_names[op]))return op==absent || !(present[fragment]&(1<<op))?-1:fragment*8+op;CHECK(false);return -1;}
static int validate(struct mk_overlay_tx *tx,int node)
{CHECK(tx->validate_only);CHECK(tx->id==7&&tx->dtbo_size==128&&!tx->steps&&!tx->steps_done&&!tx->rollback_uncertain);reads++;return node==bad_fragment*8+bad_op?-EINVAL:0;}
static int mk_overlay_op_instance_create(struct mk_overlay_tx *t,const void *f,int n,const struct mk_overlay_target *target)
{(void)f;return target->kind==MK_OVERLAY_TARGET_INSTANCES?validate(t,n):-EINVAL;}
static int mk_overlay_undo_instance_create(struct mk_overlay_tx *t,const void *f,int n,const struct mk_overlay_target *target)
{(void)t;(void)f;(void)n;(void)target;CHECK(false);return -EINVAL;}
#define SIMPLE_OP(name) static int name(struct mk_overlay_tx *t,const void *f,int n,const struct mk_overlay_target *target,bool undo){(void)f;(void)target;CHECK(!undo);bool namespace_op=n%8==MK_OVERLAY_OP_INSTANCE_REMOVE;return (target->kind==MK_OVERLAY_TARGET_INSTANCES)==namespace_op?validate(t,n):-EINVAL;}
SIMPLE_OP(mk_overlay_op_memory_remove)
SIMPLE_OP(mk_overlay_op_memory_add)
SIMPLE_OP(mk_overlay_op_instance_remove)
#define MOVE_OP(name) static int name(struct mk_overlay_tx *t,const void *f,int n,const struct mk_overlay_target *target,bool give,bool undo){(void)f;(void)target;CHECK(!undo);CHECK(give==(n%8==MK_OVERLAY_OP_CPU_ADD||n%8==MK_OVERLAY_OP_DEVICE_ADD));return target->kind==MK_OVERLAY_TARGET_INSTANCES?-EINVAL:validate(t,n);}
MOVE_OP(mk_overlay_op_cpu)
MOVE_OP(mk_overlay_op_device)
'''
code += function('mk_overlay_run_op')
if 'static int mk_overlay_preflight_fragments(' in s:
    code += function('mk_overlay_preflight_fragments')
code += r'''
static int mk_overlay_apply_fragment(struct mk_overlay_tx *tx,const void *fdt,int fragment)
{(void)fdt;CHECK(!tx->validate_only);CHECK(fragment==moves);moves++;return apply_error;}
'''
code += function('mk_overlay_parse_and_apply')
code += r'''
static void reset(void)
{bad_fragment=bad_op=resolve_error=overlay_error=absent=-1;collect_error=reads=moves=ledger_allocations=fail_ledger=apply_error=0;for(int i=0;i<3;i++){kinds[i]=i%2;present[i]=0x7e;}CHECK(!live);}
int main(void)
{
 struct mk_overlay_tx tx={.id=7,.dtbo_size=128};int refused=0;
 for(int fragment=0;fragment<3;fragment++)for(int op=0;op<8;op++) {
  reset();bad_fragment=fragment;bad_op=op;if(op==MK_OVERLAY_OP_INSTANCE_CREATE||op==MK_OVERLAY_OP_INSTANCE_REMOVE){kinds[fragment]=MK_OVERLAY_TARGET_INSTANCES;present[fragment]=0x81;}
  CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-EINVAL&&!moves&&!ledger_allocations&&!live&&!tx.steps&&!tx.validate_only);refused++;
 }
 for(int fragment=0;fragment<3;fragment++) {
  reset();resolve_error=fragment;CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-EINVAL&&!moves&&!live);
  reset();overlay_error=fragment;CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-EINVAL&&!moves&&!live);
 }
 reset();CHECK(!mk_overlay_parse_and_apply(&tx,NULL)&&reads==18&&moves==3&&ledger_allocations==1&&!tx.validate_only);kfree(tx.steps);tx.steps=NULL;
 reset();absent=MK_OVERLAY_OP_DEVICE_ADD;CHECK(!mk_overlay_parse_and_apply(&tx,NULL)&&reads==15&&moves==3);kfree(tx.steps);tx.steps=NULL;
 reset();collect_error=-ENOMEM;CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-ENOMEM&&!reads&&!moves&&!live);
 reset();fail_ledger=1;CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-ENOMEM&&reads==18&&!moves&&!live);
 reset();apply_error=-EIO;CHECK(mk_overlay_parse_and_apply(&tx,NULL)==-EIO&&reads==18&&moves==1&&!tx.validate_only);kfree(tx.steps);
 CHECK(!live);printf("OVERLAY_TRANSACTION_PREFLIGHT_PASS malformed_operations=%d all_fragments_before_effects=true private_validation_descriptor=true apply_failures_preserved=true\n",refused);
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-overlay-transaction-') as directory:
    root = Path(directory)
    (root/'probe.c').write_text(code)
    subprocess.run(['cc', '-std=gnu11', '-Wall', '-Wextra', '-Werror',
                    '-Wno-unused-const-variable', '-Wno-unused-function',
                    str(root/'probe.c'), '-o', str(root/'probe')], check=True)
    subprocess.run([str(root/'probe')], check=True)
