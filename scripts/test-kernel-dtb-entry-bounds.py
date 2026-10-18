#!/usr/bin/env python3
"""Exercise extracted DTB entry readers with real pinned libfdt.

Mapping, allocation and overlay application are mocked. No boot or real kernel
resource effects are claimed. --control selects old readers with the new shared
bounds helper still available, for focused negative controls.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('sources', type=Path)
p.add_argument('libfdt', type=Path)
p.add_argument('--control', type=Path)
p.add_argument('--section', choices=('short', 'overlay', 'generic', 'identity', 'id-limit', 'manifest', 'integration', 'all'), default='all')
a = p.parse_args()
selected = a.control or a.sources

def extract(directory, name, marker):
    text = (directory / name).read_text()
    start = text.rindex(marker)
    return text[start:text.index('\n}', start) + 2] + '\n'

helper = extract(a.sources, 'dts.c', 'int mk_dt_check_blob_bounds(')
readers = ''.join(extract(selected, name, marker) for name, marker in (
    ('overlay.c', 'static ssize_t mk_overlay_new_write('),
    ('dts.c', 'int mk_dt_get_property_size('),
    ('instance_dt.c', 'static int mk_dt_extract_instance_info('),
    ('manifest.c', 'void __init mk_manifest_populate(')))
prelude = r'''
#define _GNU_SOURCE
#include <stdint.h>
#include <stdbool.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <sys/mman.h>
#include <sys/resource.h>
#include <sys/wait.h>
#include <unistd.h>
#include <libfdt.h>
#define __init
#define GFP_KERNEL 0
#define PAGE_SIZE 4096UL
#define MK_FDT_COMPATIBLE "multikernel-v1"
typedef uint64_t phys_addr_t;
typedef uint64_t u64;
struct kernfs_open_file { int unused; };
static int alloc_calls,free_calls,apply_calls,map_calls,unmap_calls,map_fail;
static unsigned char blob[4096] __attribute__((aligned(8)));
static phys_addr_t mk_manifest_fdt_phys;
static void log_stub(const char *format, ...) { (void)format; }
#define pr_info log_stub
#define pr_warn log_stub
#define pr_err log_stub
static void *kmalloc(size_t n,int flags) { (void)flags;alloc_calls++;return malloc(n); }
static void kfree(void *p) { free_calls++;free(p); }
static int mk_overlay_apply(const void *p,size_t n) { (void)p;(void)n;apply_calls++;return 0; }
static void *early_memremap(phys_addr_t addr,size_t n) { (void)addr;(void)n;map_calls++;return map_fail?NULL:blob; }
static void early_memunmap(void *p,size_t n) { (void)p;(void)n;unmap_calls++; }
#define CHECK(x) do {if(!(x)){fprintf(stderr,"DTB entry line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
'''
# Trap the terminal boot halt only to retain existing parser/effect assertions.
# The dedicated boot-policy fixture asserts which paths must halt.
if 'refusing invalid boot manifest' in readers:
    prelude += r'''
#include <setjmp.h>
static jmp_buf manifest_halt;
static void __attribute__((noreturn)) panic(const char *format,...){(void)format;longjmp(manifest_halt,1);}
'''
    readers += r'''
static void observe_manifest(phys_addr_t p,u64 n){if(!setjmp(manifest_halt))mk_manifest_populate(p,n);}
#define mk_manifest_populate observe_manifest
'''
checks = r'''
static void setup(bool named) {
 CHECK(fdt_create_empty_tree(blob,sizeof(blob))==0);
 if(named)CHECK(fdt_set_name(blob,0,"mk-test")==0);
 alloc_calls=free_calls=apply_calls=map_calls=unmap_calls=map_fail=0;
 mk_manifest_fdt_phys=0;
}
static void short_readers(void) {
 struct rlimit r={0,0};CHECK(setrlimit(RLIMIT_CORE,&r)==0);
 long page=sysconf(_SC_PAGESIZE);CHECK(page>0);
 void *guard=mmap(NULL,page,PROT_NONE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);CHECK(guard!=MAP_FAILED);
 pid_t child=fork();CHECK(child>=0);
 if(!child) {
  for(size_t n=0;n<sizeof(struct fdt_header);n++) {
   CHECK(mk_overlay_new_write(NULL,guard,n,0)==-EINVAL);
   CHECK(mk_dt_get_property_size(guard,n,"probe")==-EINVAL);
  }
  CHECK(!alloc_calls && !apply_calls);_exit(0);
 }
 int status;CHECK(waitpid(child,&status,0)==child);
 if(WIFSIGNALED(status))fprintf(stderr,"short reader died signal=%d\n",WTERMSIG(status));
 CHECK(WIFEXITED(status) && WEXITSTATUS(status)==0);
 CHECK(munmap(guard,page)==0);
 puts("ALL_40_SHORT_INPUTS_REFUSED_BEFORE_READ_OR_OVERLAY_ALLOCATION_PASS");
}
static void overlay(void) {
 setup(false);CHECK(mk_overlay_new_write(NULL,(char*)blob,sizeof(blob),1)==-EINVAL && !alloc_calls);
 CHECK(mk_overlay_new_write(NULL,(char*)blob,1024*1024+1,0)==-EFBIG && !alloc_calls);
 CHECK(mk_overlay_new_write(NULL,(char*)blob,sizeof(blob),0)==sizeof(blob));
 CHECK(alloc_calls==1 && free_calls==1 && apply_calls==1);
 setup(false);fdt32_t *tags=(fdt32_t*)(blob+fdt_off_dt_struct(blob));tags[0]=cpu_to_fdt32(0xbad);
 CHECK(fdt_check_header(blob)==0);
 CHECK(mk_overlay_new_write(NULL,(char*)blob,sizeof(blob),0)==-EINVAL);
 CHECK(alloc_calls==1 && free_calls==1 && !apply_calls);
 setup(false);CHECK(mk_overlay_new_write(NULL,(char*)blob,sizeof(struct fdt_header),0)==-EINVAL);
 CHECK(alloc_calls==1 && free_calls==1 && !apply_calls);
 puts("VALID_OVERLAY_REACHES_APPLICATION_MALFORMED_AND_TRUNCATED_REFUSED_WITH_COPY_FREED_PASS");
}
static void generic(void) {
 setup(true);int chosen=fdt_add_subnode(blob,0,"chosen");CHECK(chosen>=0);
 CHECK(fdt_setprop_string(blob,chosen,"probe","value")==0);
 CHECK(fdt_check_full(blob,sizeof(blob))<0); /* Existing named-root format. */
 CHECK(mk_dt_check_blob_bounds(blob,sizeof(blob))==0);
 CHECK(mk_dt_get_property_size(blob,sizeof(blob),"probe")==6);
 CHECK(mk_dt_get_property_size(blob,sizeof(blob)-1,"probe")==-EINVAL);
 CHECK(mk_dt_check_blob_bounds(NULL,sizeof(blob))==-EINVAL);
 puts("NAMED_INSTANCE_ROOT_PRESERVED_AND_IGNORED_PROPERTY_BUFFER_LENGTH_NOW_ENFORCED_PASS");
}
static void id_limit(void) {
 const uint32_t bad[]={INT_MAX,0x80000000,UINT32_MAX};
 for(unsigned int i=0;i<sizeof(bad)/sizeof(bad[0]);i++) {
  setup(true);fdt32_t overflow=cpu_to_fdt32(bad[i]);
  CHECK(fdt_setprop(blob,0,"id",&overflow,4)==0);
  int out=-1;const char *name=NULL;
  CHECK(mk_dt_extract_instance_info(blob,sizeof(blob),&out,&name)==-EINVAL && out==-1 && !name);
 }
 puts("INSTANCE_ID_EXCLUSIVE_END_AND_SIGN_OVERFLOW_REFUSED_PASS");
}
static void identity(void) {
 for(unsigned int i=0;i<3;i++) {
  setup(true);uint32_t id=i==0?0:i==1?40:INT_MAX-1;fdt32_t encoded=cpu_to_fdt32(id);
  CHECK(fdt_setprop(blob,0,"id",&encoded,4)==0);
  int out=-1;const char *name=NULL;
  CHECK(mk_dt_extract_instance_info(blob,sizeof(blob),&out,&name)==0);
  CHECK(out==(int)id && strcmp(name,"mk-test")==0);
 }
 const int bad[]={0,1,3,8};
 for(unsigned int i=0;i<sizeof(bad)/sizeof(bad[0]);i++) {
  setup(true);fdt32_t ids[2]={cpu_to_fdt32(40),cpu_to_fdt32(40)};
  CHECK(fdt_setprop(blob,0,"id",ids,bad[i])==0);
  int out=-1;const char *name=NULL;
  CHECK(mk_dt_extract_instance_info(blob,sizeof(blob),&out,&name)==-EINVAL);
  CHECK(out==-1 && name==NULL);
 }
 id_limit();
 puts("EXACT_FOUR_BYTE_ID_WITH_SAFE_EXCLUSIVE_END_AND_UNCHANGED_OUTPUT_ON_REFUSAL_PASS");
}
static void integration(void) {
 setup(true);int chosen=fdt_add_subnode(blob,0,"chosen");CHECK(chosen>=0);
 CHECK(fdt_setprop_string(blob,chosen,"probe","value")==0);
 int child=fdt_add_subnode(blob,0,"ignored");CHECK(child>=0);
 CHECK(fdt_setprop_string(blob,child,"bad","value")==0);
 struct fdt_property *prop=(struct fdt_property*)(blob+fdt_off_dt_struct(blob)+fdt_first_property_offset(blob,child));
 prop->nameoff=cpu_to_fdt32(fdt_size_dt_strings(blob)+1);
 CHECK(fdt_check_header(blob)==0);
 CHECK(mk_dt_get_property_size(blob,sizeof(blob),"probe")==-EINVAL);
 int id=-1;const char *name=NULL;
 CHECK(mk_dt_extract_instance_info(blob,sizeof(blob),&id,&name)==-EINVAL);
 CHECK(id==-1 && !name);
 puts("MALFORMED_UNREAD_SUBTREE_REFUSED_BEFORE_PROPERTY_OR_ID_OUTPUT_PASS");
}
static void manifest(void) {
 const phys_addr_t invalid_pages[]={0,0x1001,UINT64_MAX};
 for(unsigned int i=0;i<sizeof(invalid_pages)/sizeof(invalid_pages[0]);i++) {
  setup(false);CHECK(fdt_setprop_string(blob,0,"compatible",MK_FDT_COMPATIBLE)==0);
  mk_manifest_populate(invalid_pages[i],PAGE_SIZE);
  CHECK(!map_calls && !mk_manifest_fdt_phys);
 }
 puts("MANIFEST_NULL_OR_UNALIGNED_PRODUCER_PAGE_REFUSED_BEFORE_MAPPING_PASS");
 for(size_t n=0;n<sizeof(struct fdt_header);n++) {
  setup(false);CHECK(fdt_setprop_string(blob,0,"compatible",MK_FDT_COMPATIBLE)==0);
  mk_manifest_populate(0x1000,n);CHECK(!map_calls && !mk_manifest_fdt_phys);
 }
 setup(false);mk_manifest_populate(0x1000,PAGE_SIZE+1);CHECK(!map_calls && !mk_manifest_fdt_phys);
 setup(false);CHECK(fdt_setprop_string(blob,0,"compatible",MK_FDT_COMPATIBLE)==0);
 mk_manifest_populate(0x1000,PAGE_SIZE);CHECK(mk_manifest_fdt_phys==0x1000 && map_calls==1 && unmap_calls==1);
 setup(false);CHECK(fdt_setprop_string(blob,0,"compatible",MK_FDT_COMPATIBLE)==0);
 int child=fdt_add_subnode(blob,0,"ignored");CHECK(child>=0);
 CHECK(fdt_setprop_string(blob,child,"bad","value")==0);
 struct fdt_property *prop=(struct fdt_property*)(blob+fdt_off_dt_struct(blob)+fdt_first_property_offset(blob,child));
 prop->nameoff=cpu_to_fdt32(fdt_size_dt_strings(blob)+1);
 CHECK(fdt_check_header(blob)==0);
 mk_manifest_populate(0x1000,PAGE_SIZE);CHECK(!mk_manifest_fdt_phys && map_calls==1 && unmap_calls==1);
 setup(false);map_fail=1;mk_manifest_populate(0x1000,PAGE_SIZE);CHECK(!mk_manifest_fdt_phys && map_calls==1 && !unmap_calls);
 puts("MANIFEST_LENGTH_REFUSED_BEFORE_MAPPING_AND_FULL_TREE_VALIDATED_BEFORE_ACCEPTANCE_PASS");
}
int main(int argc,char **argv) {
 CHECK(argc==2);
 if(!strcmp(argv[1],"short"))short_readers();
 else if(!strcmp(argv[1],"overlay"))overlay();
 else if(!strcmp(argv[1],"generic"))generic();
 else if(!strcmp(argv[1],"identity"))identity();
 else if(!strcmp(argv[1],"id-limit"))id_limit();
 else if(!strcmp(argv[1],"manifest"))manifest();
 else if(!strcmp(argv[1],"integration"))integration();else abort();
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-dtb-entry-') as tmp:
    root = Path(tmp)
    source, binary = root / 'fixture.c', root / 'fixture'
    source.write_text(prelude + helper + readers + checks)
    library = [a.libfdt / n for n in ('fdt.c', 'fdt_ro.c', 'fdt_rw.c', 'fdt_wip.c', 'fdt_sw.c', 'fdt_empty_tree.c', 'fdt_check.c')]
    subprocess.run(['cc', '-O2', '-Wall', '-Wextra', '-Werror', '-Wno-unused-parameter', '-I', str(a.libfdt), str(source), *(str(x) for x in library), '-o', str(binary)], check=True)
    sections = ('short', 'overlay', 'generic', 'identity', 'manifest', 'integration') if a.section == 'all' else (a.section,)
    for section in sections:
        subprocess.run([str(binary), section], check=True, timeout=10)
