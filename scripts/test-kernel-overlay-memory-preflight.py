#!/usr/bin/env python3
"""Check actual memory operation loops with modeled readers and memory backends.

This covers operation-local validation ordering, not live physical ownership,
whole-overlay atomicity, hotplug or inter-kernel delivery.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("source", type=Path)
args = parser.parse_args()
text = args.source.read_text()


def function(name):
    start = text.index("static int " + name + "(")
    return text[start:text.index("\n}", start) + 2] + "\n"


code = r'''
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
typedef unsigned long long u64; typedef unsigned int u32;
#define MK_OVERLAY_TARGET_POOL 0
#define PAGE_SHIFT 12
#define CHECK(c) do { if (!(c)) { fprintf(stderr,"memory preflight line %d: %s\n",__LINE__,#c); exit(2); } } while(0)
struct mk_overlay_tx {int id;bool validate_only;};
struct mk_instance {int id; const char *name;};
struct mk_overlay_target {int kind; const char *path; struct mk_instance *instance;};
static int bad_item,bad_field,forbidden_reg,skip_item,check_error,moves,fail_move;
static bool validate_only;
static int ids[3],routes[3],reads[3][4];
#define fdt_for_each_subnode(n,f,o) for((void)(f),(void)(o),(n)=0;(n)<3;(n)++)
static void logger(const char *format,...) {(void)format;}
#define pr_info logger
#define pr_warn logger
#define pr_err logger
static int read_field(int item,int field)
{reads[item][field]++;return item==bad_item && field==bad_field ? -EINVAL : 0;}
static const char *fdt_get_name(const void *fdt,int item,int *len)
{(void)fdt;(void)len;return item==skip_item ? "unrelated" : "memory@0";}
static const void *fdt_getprop(const void *fdt,int item,const char *name,int *len)
{(void)fdt;CHECK(!strcmp(name,"reg"));*len=item==forbidden_reg ? 16 : -1;return item==forbidden_reg ? name : NULL;}
static int mk_overlay_check_resource_target(struct mk_overlay_tx *tx,const struct mk_overlay_target *target,const char *name)
{(void)tx;(void)target;(void)name;return check_error;}
static int mk_overlay_parse_numa(const void *fdt,int item,const char *name,int *node)
{(void)fdt;(void)name;*node=item;return read_field(item,0);}
static int mk_overlay_parse_u32(const void *fdt,int item,const char *name,u32 *type)
{(void)fdt;CHECK(!strcmp(name,"mem-type"));*type=item+16;return read_field(item,1);}
static int mk_overlay_parse_mem_size(const void *fdt,int item,const char *name,u64 *size)
{(void)fdt;(void)name;*size=(u64)(item+1)<<PAGE_SHIFT;return read_field(item,3);}
static int mk_overlay_parse_mem_reg(const void *fdt,int item,const char *name,u64 *base,u64 *size)
{(void)fdt;(void)name;*base=(u64)(item+8)<<PAGE_SHIFT;*size=(u64)(item+1)<<PAGE_SHIFT;return read_field(item,3);}
static int move(int route,int item)
{CHECK(moves<3);routes[moves]=route;ids[moves++]=item;return moves==fail_move ? -EIO : 0;}
static int mk_pool_mem_grow(u64 size,int node,void *out)
{CHECK(!out && size==(u64)(node+1)<<PAGE_SHIFT);return move(1,node);}
static int mk_overlay_pool_mem_remove(struct mk_overlay_tx *tx,u64 base,u64 size)
{(void)tx;int item=(base>>PAGE_SHIFT)-8;CHECK(size==(u64)(item+1)<<PAGE_SHIFT);return move(2,item);}
static int mk_send_mem_add(int instance,u64 base,u64 pages,u32 node,u32 type)
{CHECK(instance==7 && base==node+8 && pages==node+1 && type==node+16);return move(3,node);}
static int mk_send_mem_remove(int instance,u64 base,u64 pages)
{CHECK(instance==7 && pages==base-7);return move(4,base-8);}
''' + function("mk_overlay_op_memory_add") + function("mk_overlay_op_memory_remove") + r'''
static void reset(void)
{bad_item=bad_field=forbidden_reg=skip_item=-1;check_error=moves=fail_move=0;validate_only=false;memset(reads,0,sizeof(reads));}
static int operation(bool add,int kind,bool undo)
{struct mk_overlay_tx tx={.id=1,.validate_only=validate_only};struct mk_instance instance={7,"instance"};struct mk_overlay_target target={kind,"pool",validate_only?NULL:&instance};
 return add ? mk_overlay_op_memory_add(&tx,NULL,0,&target,undo) : mk_overlay_op_memory_remove(&tx,NULL,0,&target,undo);}
int main(void)
{
 int refused=0;int fields[]={0,1,3};
 for(int add=0;add<2;add++) for(int kind=0;kind<2;kind++) for(int undo=0;undo<2;undo++) {
  if(add && !kind && undo) {reset();CHECK(operation(add,kind,undo)==-EOPNOTSUPP && !moves);continue;}
  for(int item=0;item<3;item++) for(int f=0;f<3;f++) {
   reset();bad_item=item;bad_field=fields[f];
   CHECK(operation(add,kind,undo)==-EINVAL && moves==0);refused++;
  }
  reset();CHECK(operation(add,kind,undo)==0 && moves==3);
  for(int i=0;i<3;i++) {CHECK(ids[i]==i && routes[i]==(!kind ? (add || undo ? 1:2) : (add!=undo ? 3:4)));for(int f=0;f<3;f++)CHECK(reads[i][fields[f]]==2);}
  reset();skip_item=1;CHECK(operation(add,kind,undo)==0 && moves==2 && ids[0]==0 && ids[1]==2);
  reset();check_error=-EPERM;CHECK(operation(add,kind,undo)==-EPERM && !moves && !reads[0][0]);
  reset();fail_move=2;CHECK(operation(add,kind,undo)==-EIO && moves==2);
  reset();validate_only=true;CHECK(operation(add,kind,undo)==0 && moves==0);
  for(int i=0;i<3;i++)for(int f=0;f<3;f++)CHECK(reads[i][fields[f]]==1);
 }
 for(int item=0;item<3;item++) {reset();forbidden_reg=item;CHECK(operation(true,0,false)==-EINVAL && !moves);refused++;}
 printf("OVERLAY_MEMORY_LIST_PREFLIGHT_PASS refused=%d forward_undo_pool_instance=true order_metadata=true unsupported_inverse_preserved=true backend_failure_preserved=true validation_only_no_backend=true\n",refused);
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix="mk-overlay-memory-items-") as directory:
    root = Path(directory)
    (root / "probe.c").write_text(code)
    subprocess.run(["cc", "-std=gnu11", "-Wall", "-Wextra", "-Werror",
                    str(root / "probe.c"), "-o", str(root / "probe")], check=True)
    subprocess.run([str(root / "probe")], check=True)
