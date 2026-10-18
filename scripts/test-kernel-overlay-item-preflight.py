#!/usr/bin/env python3
"""Run production CPU/device loops with modeled readers and resource backends.

Checks validation ordering for multi-item input; does not prove backend atomicity,
physical ownership, inter-kernel delivery or whole-overlay preflight.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("source", type=Path)
args = parser.parse_args()
source = args.source.read_text()


def function(name):
    start = source.index("static int " + name + "(")
    return source[start:source.index("\n}", start) + 2] + "\n"


code = r'''
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
typedef uint16_t u16; typedef uint8_t u8; typedef uint32_t u32;
typedef unsigned long long mk_phys_cpu_t;
#define MK_OVERLAY_TARGET_POOL 0
#define MK_OVERLAY_TARGET_INSTANCE 1
#define PCI_SLOT(d) ((d)>>3)
#define PCI_FUNC(d) ((d)&7)
#define CHECK(c) do { if (!(c)) { fprintf(stderr,"preflight line %d: %s\n",__LINE__,#c); exit(2); } } while(0)
struct mk_overlay_tx {int id;bool validate_only;};
struct mk_instance {int id;};
struct mk_overlay_target {int kind; struct mk_instance *instance;};
static int cpu, bad_item, bad_field, check_error, moves, fail_move, skip_item;
static bool validate_only;
static int reads[3][3], moved_ids[3], routes[3];
static void logger(const char *format, ...) {(void)format;}
#define pr_info logger
#define pr_err logger
#define fdt_for_each_subnode(n,f,o) for((void)(f),(void)(o),(n)=0;(n)<3;(n)++)
static const char *fdt_get_name(const void *fdt,int node,int *len)
{(void)fdt;(void)len;return node==skip_item ? "unrelated" : cpu ? "cpu@0" : "pci@0";}
static int read_field(int item,int field)
{reads[item][field]++;return item==bad_item && field==bad_field ? -EINVAL : 0;}
static int mk_overlay_parse_cpu_reg(const void *fdt,int item,const char *name,mk_phys_cpu_t *id)
{(void)fdt;(void)name;*id=item+8;return read_field(item,0);}
static int mk_overlay_parse_numa(const void *fdt,int item,const char *name,int *node)
{(void)fdt;(void)name;*node=item;return read_field(item,1);}
static int mk_overlay_parse_u32(const void *fdt,int item,const char *name,u32 *flags)
{(void)fdt;CHECK(!strcmp(name,"flags"));*flags=item+16;return read_field(item,2);}
static int mk_overlay_parse_pci_id(const void *fdt,int item,const char *name,u16 *d,u8 *b,u8 *v)
{(void)fdt;(void)name;*d=0;*b=1;*v=item+8;return read_field(item,0);}
static int mk_overlay_parse_driver(const void *fdt,int item,const char **driver)
{(void)fdt;*driver="driver";return read_field(item,1);}
static int mk_overlay_check_resource_target(struct mk_overlay_tx *tx,const struct mk_overlay_target *t,const char *name)
{(void)tx;(void)t;(void)name;return check_error;}
static const char *mk_overlay_target_name(const struct mk_overlay_target *t)
{(void)t;return "target";}
static int move(int route,int id)
{CHECK(moves<3);moved_ids[moves]=id;routes[moves++]=route;return moves==fail_move ? -EIO : 0;}
static int mk_pool_cpu_add(mk_phys_cpu_t id) {return move(1,id);}
static int mk_pool_cpu_remove(mk_phys_cpu_t id,u32 node,u32 flags)
{CHECK(node==id-8 && flags==id+8);return move(2,id);}
static int mk_send_cpu_add(int instance,mk_phys_cpu_t id,u32 node,u32 flags)
{CHECK(instance==7 && node==id-8 && flags==id+8);return move(3,id);}
static int mk_send_cpu_remove(int instance,mk_phys_cpu_t id)
{CHECK(instance==7);return move(4,id);}
static int mk_pool_device_add(u16 d,u8 b,u8 v)
{CHECK(d==0 && b==1);return move(1,v);}
static int mk_pool_device_remove(u16 d,u8 b,u8 v,const char *driver,u32 flags)
{CHECK(d==0 && b==1 && !driver && !flags);return move(2,v);}
static int mk_send_device_add(int instance,u16 d,u8 b,u8 v,const char *driver,u32 flags)
{CHECK(instance==7 && d==0 && b==1 && !strcmp(driver,"driver") && flags==(u32)v+8);return move(3,v);}
static int mk_send_device_remove(int instance,u16 d,u8 b,u8 v)
{CHECK(instance==7 && d==0 && b==1);return move(4,v);}
''' + function("mk_overlay_op_cpu") + function("mk_overlay_op_device") + r'''
static void reset(void)
{bad_item=bad_field=skip_item=-1;check_error=moves=fail_move=0;validate_only=false;memset(reads,0,sizeof(reads));}
static int operation(int kind,bool give,bool undo)
{struct mk_overlay_tx tx={.id=1,.validate_only=validate_only};struct mk_instance instance={7};struct mk_overlay_target t={kind,validate_only?NULL:&instance};
 return cpu ? mk_overlay_op_cpu(&tx,NULL,0,&t,give,undo) : mk_overlay_op_device(&tx,NULL,0,&t,give,undo);}
int main(void)
{
 int refusals=0;
 for(cpu=0;cpu<2;cpu++) for(int kind=0;kind<2;kind++) for(int give=0;give<2;give++) for(int undo=0;undo<2;undo++) {
  for(int item=0;item<3;item++) for(int field=0;field<3;field++) {
   reset();bad_item=item;bad_field=field;
   CHECK(operation(kind,give,undo)==-EINVAL && moves==0);refusals++;
  }
  reset();CHECK(operation(kind,give,undo)==0 && moves==3);
  for(int i=0;i<3;i++) {CHECK(moved_ids[i]==i+8 && routes[i]==(kind ? (give?3:4) : (give?1:2)));for(int f=0;f<3;f++)CHECK(reads[i][f]==2);}
  reset();skip_item=1;CHECK(operation(kind,give,undo)==0 && moves==2 && moved_ids[0]==8 && moved_ids[1]==10);
  reset();check_error=-EPERM;CHECK(operation(kind,give,undo)==-EPERM && moves==0 && reads[0][0]==0);
  reset();fail_move=2;CHECK(operation(kind,give,undo)==-EIO && moves==2);
  reset();validate_only=true;CHECK(operation(kind,give,undo)==0 && moves==0);
  for(int i=0;i<3;i++)for(int f=0;f<3;f++)CHECK(reads[i][f]==1);
 }
 printf("OVERLAY_ITEM_PREFLIGHT_PASS refused=%d forward_undo_pool_instance=true valid_order_metadata=true backend_failure_preserved=true validation_only_no_backend=true\n",refusals);
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix="mk-overlay-items-") as directory:
    root = Path(directory)
    (root / "probe.c").write_text(code)
    subprocess.run(["cc", "-std=gnu11", "-Wall", "-Wextra", "-Werror",
                    str(root / "probe.c"), "-o", str(root / "probe")], check=True)
    subprocess.run([str(root / "probe")], check=True)
