#!/usr/bin/env python3
"""Exercise real manifest validation and boot guard with a modeled panic sink.

Pinned libfdt parses real blobs. Physical mapping is modeled. The terminal panic
is trapped with setjmp; no kernel panic or actual boot is performed.
"""
from pathlib import Path
import argparse,subprocess,tempfile
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('sources',type=Path);p.add_argument('libfdt',type=Path)
p.add_argument('--control-manifest',type=Path,help='Run the old manifest reader against the new refusal assertions')
a=p.parse_args()
def function(name,marker):
    selected=a.control_manifest if name=='manifest.c' and a.control_manifest else a.sources/name
    s=selected.read_text();at=s.index(marker)
    return s[at:s.index('\n}',at)+2]+'\n'
source=(a.sources/'instance_dt.c').read_text()
assert 'early_initcall(mk_instance_boot_init);' in source
assert 'early_initcall(mk_instance_restore_from_manifest);' not in source
code=r'''
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <errno.h>
#include <setjmp.h>
#include <string.h>
#include <libfdt.h>
#define __init
#define PAGE_SIZE 4096UL
#define MK_FDT_COMPATIBLE "multikernel-v1"
typedef uint64_t phys_addr_t,u64;
#define CHECK(x) do {if(!(x)){fprintf(stderr,"boot policy line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
static unsigned char blob[4096] __attribute__((aligned(8)));
static phys_addr_t mk_manifest_fdt_phys;
static int maps,unmaps,map_fail,panics,restore_ret,restores;
static int root_object,*root_instance;
static jmp_buf halted;
static void log_stub(const char *fmt,...){(void)fmt;}
#define pr_info log_stub
#define pr_warn log_stub
#define pr_err log_stub
static void __attribute__((noreturn)) panic(const char *fmt,...){(void)fmt;panics++;longjmp(halted,1);}
static void *early_memremap(phys_addr_t p,size_t n){(void)p;(void)n;maps++;return map_fail?NULL:blob;}
static void early_memunmap(void *p,size_t n){(void)p;(void)n;unmaps++;}
static int mk_instance_restore_from_manifest(void){restores++;return restore_ret;}
'''
code+=function('manifest.c','void __init mk_manifest_populate(')
code+=function('instance_dt.c','static int __init mk_instance_boot_init(')
code+=r'''
static void setup(void){CHECK(!fdt_create_empty_tree(blob,sizeof(blob)));CHECK(!fdt_setprop_string(blob,0,"compatible",MK_FDT_COMPATIBLE));maps=unmaps=map_fail=panics=restores=restore_ret=0;mk_manifest_fdt_phys=0;root_instance=NULL;}
static void accept(phys_addr_t p,size_t n,int expect_halt){int stop=setjmp(halted);if(!stop)mk_manifest_populate(p,n);CHECK((stop!=0)==expect_halt);}
int main(void){
 const phys_addr_t bad[]={0,0x1001,UINT64_MAX};
 for(unsigned i=0;i<sizeof(bad)/sizeof(bad[0]);i++){setup();accept(bad[i],PAGE_SIZE,1);CHECK(!maps&&!unmaps&&!mk_manifest_fdt_phys&&panics==1);}
 for(size_t n=0;n<sizeof(struct fdt_header);n++){setup();accept(0x1000,n,1);CHECK(!maps&&!mk_manifest_fdt_phys);}
 setup();accept(0x1000,PAGE_SIZE+1,1);CHECK(!maps&&!mk_manifest_fdt_phys);
 setup();map_fail=1;accept(0x1000,PAGE_SIZE,1);CHECK(maps==1&&!unmaps&&!mk_manifest_fdt_phys);
 setup();((struct fdt_header*)blob)->magic=0;accept(0x1000,PAGE_SIZE,1);CHECK(maps==1&&unmaps==1&&!mk_manifest_fdt_phys);
 setup();CHECK(!fdt_setprop_string(blob,0,"compatible","wrong"));accept(0x1000,PAGE_SIZE,1);CHECK(maps==1&&unmaps==1&&!mk_manifest_fdt_phys);
 setup();int n=fdt_add_subnode(blob,0,"ignored");CHECK(n>=0);CHECK(!fdt_setprop_string(blob,n,"bad","value"));struct fdt_property *prop=(struct fdt_property*)(blob+fdt_off_dt_struct(blob)+fdt_first_property_offset(blob,n));prop->nameoff=cpu_to_fdt32(fdt_size_dt_strings(blob)+1);CHECK(!fdt_check_header(blob));accept(0x1000,PAGE_SIZE,1);CHECK(!mk_manifest_fdt_phys&&unmaps==1);
 setup();accept(0x1000,PAGE_SIZE,0);CHECK(mk_manifest_fdt_phys==0x1000&&maps==1&&unmaps==1&&!panics);accept(0x2000,PAGE_SIZE,1);CHECK(mk_manifest_fdt_phys==0x1000&&maps==1&&unmaps==1&&panics==1);
 puts("INVALID_OR_DUPLICATE_HANDOFF_HALTS_BEFORE_HOST_FALLBACK_PASS");
 const int errors[]={-ENOMEM,-EFAULT,-EINVAL,-ENOENT,-EEXIST};
 for(unsigned i=0;i<sizeof(errors)/sizeof(errors[0]);i++)for(int present=0;present<2;present++){setup();restore_ret=errors[i];root_instance=present?&root_object:NULL;if(!setjmp(halted)){mk_instance_boot_init();CHECK(0);}CHECK(panics==1&&restores==1);}
 setup();if(!setjmp(halted)){mk_instance_boot_init();CHECK(0);}CHECK(panics==1&&restores==1);
 setup();root_instance=&root_object;CHECK(!mk_instance_boot_init());CHECK(restores==1&&!panics);
 puts("BOOT_GUARD_HALTS_ON_RESTORE_ERROR_OR_ABSENT_ROOT_PASS");return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-boot-policy-') as tmp:
    tmp=Path(tmp);(tmp/'test.c').write_text(code)
    library=[a.libfdt/n for n in ['fdt.c','fdt_ro.c','fdt_rw.c','fdt_wip.c','fdt_sw.c','fdt_empty_tree.c','fdt_check.c']]
    subprocess.run(['gcc','-std=gnu11','-O1','-Wall','-Wextra','-Werror','-fsanitize=undefined','-fno-sanitize-recover=all','-I',str(a.libfdt),str(tmp/'test.c'),*(str(x) for x in library),'-o',str(tmp/'test')],check=True)
    subprocess.run([str(tmp/'test')],check=True)
