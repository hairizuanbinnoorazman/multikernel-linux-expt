#!/usr/bin/env python3
"""Inject retained-DTB allocation failures into the exact baseline initializer.

This userspace ownership fixture mocks kernel resources. It does not qualify
kernel locking, CPU/device rollback, or a booted candidate.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("source", type=Path)
parser.add_argument("--staging", action="store_true", help="Check staged platform ownership across initializer errors")
args = parser.parse_args()
text = args.source.read_text()
function = text[text.index("int mk_baseline_validate_and_initialize("):]
prelude = r'''
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <stdbool.h>
#define GFP_KERNEL 0
#define MK_BASELINE_MAX_MEM_REQS 8
#define LIST_HEAD(name) int name = 0
static void log_stub(const char *format, ...) { (void)format; }
#define pr_err log_stub
#define pr_info log_stub
struct mk_baseline_mem_req { int unused; };
struct fdt_header { unsigned char bytes[40]; };
struct mk_cpu_set { int unused; };
struct instance { struct mk_cpu_set *cpus; int pci_device_count; void *dtb_data; size_t dtb_size; int platform_devices,platform_device_count; bool platform_devices_valid; };
static struct instance instance;
static struct instance *root_instance = &instance;
static struct mk_cpu_set pool, cpus;
static struct mk_cpu_set *mk_cpu_pool = &pool;
static int dtb_fail, stage_fail, mutations, dtb_allocs, dtb_frees, old_live;
static int staged_live, cleared;
static void *old_tree, *new_tree;
static const size_t tree_size = 64;
static void *test_dtb_alloc(size_t size) {
 dtb_allocs++;
 if (dtb_fail) return NULL;
 new_tree = malloc(size); if (!new_tree) abort(); return new_tree;
}
static void *test_kmemdup(const void *source, size_t size, int flags) {
 (void)flags; void *p = test_dtb_alloc(size); if(p)memcpy(p,source,size); return p;
}
static void *test_kmalloc(size_t size, int flags) { (void)flags; return test_dtb_alloc(size); }
static void *test_kcalloc(size_t count, size_t size, int flags) { (void)flags; return calloc(count,size); }
static void test_free(void *p) {
 if (!p)return;
 if (p==old_tree)old_live=0;
 if (p==new_tree)dtb_frees++;
 free(p);
}
#define kmemdup test_kmemdup
#define kmalloc test_kmalloc
#define kcalloc test_kcalloc
#define kfree test_free
static int mk_pool_empty(void) { return 1; }
static int mk_cpu_set_empty(struct mk_cpu_set *p) { (void)p;return 1; }
static int fdt_check_header(const void *p) { (void)p;return 0; }
static int __attribute__((unused)) fdt_check_full(const void *p,size_t n) { (void)n;return fdt_check_header(p); }
static unsigned int fdt_totalsize(const void *p) { (void)p;return tree_size; }
static int fdt_path_offset(const void *p,const char *name) { (void)p;(void)name;return 1; }
static struct mk_cpu_set *mk_cpu_set_alloc(void) { return calloc(1,sizeof(struct mk_cpu_set)); }
static void mk_cpu_set_free(struct mk_cpu_set *p) { free(p); }
static void mk_baseline_clear_resources(struct instance *p) { p->platform_devices=p->platform_device_count=0;p->platform_devices_valid=false;mutations++;cleared++; }
static int mk_baseline_parse_cpus(const void *p,int n,struct mk_cpu_set *r) { (void)p;(void)n;(void)r;return stage_fail==1?-EINVAL:0; }
static int mk_baseline_parse_memory(const void *p,int n,struct mk_baseline_mem_req *r,int max) { (void)p;(void)n;(void)r;(void)max;return stage_fail==2?-EINVAL:1; }
PARSE_DEVICE_STUB
static int mk_baseline_validate_cpus(struct mk_cpu_set *p) { (void)p;return stage_fail==4?-EINVAL:0; }
static int mk_baseline_grow_pool(struct mk_baseline_mem_req *r,int n) { (void)r;(void)n;mutations++;return stage_fail==8?-ENOMEM:0; }
static int mk_setup_host_park(void) { mutations++;return stage_fail==5?-EBUSY:0; }
static int mk_baseline_initialize_cpus(struct mk_cpu_set *p) { (void)p;mutations++;return stage_fail==6?-EBUSY:0; }
static int mk_baseline_initialize_devices(int *list) { (void)list;mutations++;return stage_fail==7?-EBUSY:0; }
static void mk_baseline_free_pci_list(int *list) { (void)list; }
static void mk_baseline_free_platform_list(int *list) { if(*list){staged_live--;*list=0;} }
static void list_splice_tail_init(int *from,int *to) { *to=*from;*from=0; }
#define CHECK(x) do { if(!(x)){fprintf(stderr,"DTB commit assertion line %d: %s\n",__LINE__,#x);exit(2);} }while(0)
'''
checks = r'''
static void setup(void) {
 dtb_fail=stage_fail=mutations=dtb_allocs=dtb_frees=staged_live=cleared=0;new_tree=NULL;
 old_tree=malloc(32);CHECK(old_tree);memset(old_tree,0xA5,32);old_live=1;
 instance=(struct instance){.cpus=&cpus,.dtb_data=old_tree,.dtb_size=32,.platform_devices=7,.platform_device_count=7,.platform_devices_valid=true};
}
int main(void) {
 unsigned char tree[64];memset(tree,0x5A,sizeof(tree));
 if(TEST_STAGING) {
  for(int stage=1;stage<=8;stage++) {
   setup();stage_fail=stage;CHECK(mk_baseline_validate_and_initialize(tree,sizeof(tree))<0);
   CHECK(old_live && instance.dtb_data==old_tree && instance.dtb_size==32 && dtb_frees==1);
   if(stage<=4)CHECK(!cleared && instance.platform_devices==7 && instance.platform_device_count==7 && instance.platform_devices_valid);
   else CHECK(cleared==1 && !instance.platform_devices && !instance.platform_device_count && !instance.platform_devices_valid);
   CHECK(staged_live==0);free(old_tree);
  }
  puts("PARSE_VALIDATION_FAILURES_PRESERVE_OLD_METADATA_AND_ALL_ERRORS_FREE_STAGING_PASS");
  setup();CHECK(mk_baseline_validate_and_initialize(tree,sizeof(tree))==0);
  CHECK(cleared==1 && staged_live==1 && instance.platform_devices==1 && instance.platform_device_count==1 && instance.platform_devices_valid);
  CHECK(!old_live && instance.dtb_data==new_tree && !dtb_frees);free(new_tree);
  puts("SUCCESS_TRANSFERS_PRIVATE_PLATFORM_METADATA_ONCE_PASS");return 0;
 }
 setup();dtb_fail=1;
 CHECK(mk_baseline_validate_and_initialize(tree,sizeof(tree))==-ENOMEM);
 CHECK(mutations==0 && old_live && instance.dtb_data==old_tree && instance.dtb_size==32);
 CHECK(dtb_allocs==1 && !new_tree);free(old_tree);
 puts("DTB_ALLOCATION_FAILURE_PRECEDES_MUTATION_AND_PRESERVES_OLD_TREE_PASS");
 for(int stage=1;stage<=7;stage++) {
  setup();stage_fail=stage;
  CHECK(mk_baseline_validate_and_initialize(tree,sizeof(tree))<0);
  CHECK(old_live && instance.dtb_data==old_tree && instance.dtb_size==32);
  CHECK(dtb_allocs==1 && dtb_frees==1);free(old_tree);
 }
 puts("PARSE_VALIDATION_AND_RESOURCE_ERRORS_RELEASE_PRIVATE_TREE_PASS");
 setup();CHECK(mk_baseline_validate_and_initialize(tree,sizeof(tree))==0);
 CHECK(!old_live && instance.dtb_data==new_tree && instance.dtb_size==sizeof(tree));
 CHECK(dtb_allocs==1 && !dtb_frees && mutations==5);
 CHECK(memcmp(instance.dtb_data,tree,sizeof(tree))==0);free(new_tree);
 puts("SUCCESS_COMMITS_EXACT_PREALLOCATED_TREE_AND_REPLACES_OLD_TREE_PASS");
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix="mk-baseline-dtb-") as directory:
    root = Path(directory)
    source, binary = root / "fixture.c", root / "fixture"
    stub = r'''
static int mk_baseline_parse_devices(const void *p,int n,int *platform,int *pci,int *count) {
 (void)p;(void)n;(void)pci;*platform=1;*count=1;staged_live++;return stage_fail==3?-EINVAL:0;
}
''' if "&platform_list" in function else r'''
static int mk_baseline_parse_devices(const void *p,int n,struct instance *r,int *pci) {
 (void)p;(void)n;(void)pci;r->platform_devices=r->platform_device_count=1;r->platform_devices_valid=true;return stage_fail==3?-EINVAL:0;
}
'''
    prelude = "#define TEST_STAGING " + str(int(args.staging)) + "\n" + prelude.replace("PARSE_DEVICE_STUB", stub)
    source.write_text(prelude + function + checks)
    subprocess.run(["cc", "-O2", "-Wall", "-Wextra", "-Werror",
                    "-Wno-unused-function", str(source), "-o", str(binary)], check=True)
    subprocess.run([str(binary)], check=True, timeout=10)
