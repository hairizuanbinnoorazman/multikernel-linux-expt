#!/usr/bin/env python3
"""Run the exact pool grow function with injected ownership/publication failures.

Mocks model resource ownership and allocator visibility, not real pages/locks.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("source", type=Path)
args = parser.parse_args()
text = args.source.read_text()
start = text.index("int mk_pool_mem_grow(")
end = text.index("\n/**", start)
function = text[start:end]
prelude = r'''
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <string.h>
typedef uint64_t phys_addr_t;
#define GFP_KERNEL 0
#define PAGE_SHIFT 12
#define PAGE_ALIGNED(n) (!((n)&4095))
#define NUMA_NO_NODE -1
#define MAX_NUMNODES 8
#define IORESOURCE_BUSY 1
#define IORESOURCE_MEM 2
#define IORES_DESC_RESERVED 3
static void test_log(const char *format,...){(void)format;}
#define pr_warn test_log
#define pr_info test_log
#define pr_err test_log
#define CHECK(v) do{if(!(v)){fprintf(stderr,"publication fixture line%d\n",__LINE__);exit(2);}}while(0)
struct page {int unused;}; struct gen_pool {int unused;};
struct resource {const char *name;phys_addr_t start,end;int flags,desc;struct resource *parent;};
struct mk_pool_chunk {unsigned long nr_pages;struct page *pages;int node;struct resource res;int list;};
static struct resource iomem_resource;
static struct page pages;static struct gen_pool pool;
static int multikernel_mem_mutex,mk_pool_chunks;
static struct mk_pool_chunk *published_chunk;
static int fail_resource,fail_add,fail_arch,fail_shrink,fail_pages,fail_pool;
static int registered,allocator,published,freed,add_calls,remove_calls,lock_held;
#define kzalloc_obj(obj,flags) calloc(1,sizeof(obj))
#define kfree free
static int node_online(int node){return node>=0 && node<8;}
static void mutex_lock(int *lock){(void)lock;CHECK(!lock_held);lock_held=1;}
static void mutex_unlock(int *lock){(void)lock;CHECK(lock_held);lock_held=0;}
static struct page *mk_alloc_contig_pages(unsigned long count,int node){(void)count;(void)node;return fail_pages ? NULL : &pages;}
static void mk_free_contig_pages(struct page *value,unsigned long count){(void)count;CHECK(value==&pages);freed++;}
static int page_to_nid(struct page *value){(void)value;return 0;}
static phys_addr_t page_to_phys(struct page *value){(void)value;return 0x100000000ULL;}
static struct gen_pool *mk_node_pool_get(int node){(void)node;return fail_pool ? NULL : &pool;}
static int insert_resource(struct resource *parent,struct resource *res){CHECK(lock_held);if(fail_resource)return -EBUSY;registered=1;res->parent=parent;return 0;}
static int __attribute__((unused)) remove_resource(struct resource *res){CHECK(registered);registered=0;res->parent=NULL;remove_calls++;return 0;}
static int gen_pool_add(struct gen_pool *value,phys_addr_t addr,size_t size,int node){(void)value;(void)addr;(void)size;(void)node;CHECK(lock_held);add_calls++;CHECK(registered);if(fail_add)return -ENOMEM;allocator=1;return 0;}
static void list_add_tail(int *list,int *head){(void)head;CHECK(lock_held && registered && allocator);published=1;published_chunk=(struct mk_pool_chunk *)((char *)list-offsetof(struct mk_pool_chunk,list));}
static int mk_arch_pool_chunk_added(phys_addr_t base,size_t size){(void)base;(void)size;CHECK(!lock_held && published);return fail_arch ? -EIO : 0;}
static int mk_pool_mem_shrink(phys_addr_t base,size_t size){(void)base;(void)size;CHECK(registered && allocator && published);if(fail_shrink)return -EBUSY;registered=allocator=published=0;freed++;free(published_chunk);published_chunk=NULL;return 0;}
'''
checks = r'''
static void reset(void){
 if(published_chunk){free(published_chunk);published_chunk=NULL;}
 fail_resource=fail_add=fail_arch=fail_shrink=fail_pages=fail_pool=0;
 registered=allocator=published=freed=add_calls=remove_calls=lock_held=0;
}
int main(void){
 phys_addr_t out=777;
 reset();fail_resource=1;CHECK(mk_pool_mem_grow(1ULL<<30,0,&out)==-EBUSY);
 CHECK(!registered && !allocator && !published && !add_calls && freed==1 && out==777);
 puts("RESOURCE_FAILURE_NEVER_PUBLISHES_PAGES_PASS");
 reset();fail_add=1;CHECK(mk_pool_mem_grow(1ULL<<30,0,&out)==-ENOMEM);
 CHECK(!registered && !allocator && !published && remove_calls==1 && freed==1);
 puts("ALLOCATOR_FAILURE_REMOVES_RESOURCE_AND_FREES_PAGES_PASS");
 reset();CHECK(mk_pool_mem_grow(1ULL<<30,0,&out)==0);
 CHECK(registered && allocator && published && !freed && out==0x100000000ULL);
 puts("SUCCESS_REGISTERS_BEFORE_ALLOCATOR_AND_LIST_PUBLICATION_PASS");
 reset();fail_arch=1;CHECK(mk_pool_mem_grow(1ULL<<30,0,NULL)==-EIO);
 CHECK(!registered && !allocator && !published && freed==1);
 reset();fail_arch=fail_shrink=1;CHECK(mk_pool_mem_grow(1ULL<<30,0,NULL)==-EUCLEAN);
 CHECK(registered && allocator && published && !freed);
 puts("ARCH_FAILURE_ROLLBACK_OR_EXPLICIT_BUSY_OWNERSHIP_PASS");
 reset();fail_pages=1;CHECK(mk_pool_mem_grow(1ULL<<30,0,NULL)==-ENOMEM && !freed && !add_calls);
 reset();fail_pool=1;CHECK(mk_pool_mem_grow(1ULL<<30,0,NULL)==-ENOMEM && freed==1 && !add_calls);
 puts("EARLY_FAILURES_NEVER_PUBLISH_PASS");
 reset();return 0;
}
'''
with tempfile.TemporaryDirectory(prefix="mk-pool-publication-") as directory:
    root=Path(directory);source=root/"fixture.c";binary=root/"fixture"
    source.write_text(prelude+function+checks)
    subprocess.run(["cc","-O2","-Wall","-Wextra","-Werror",str(source),"-o",str(binary)],check=True)
    subprocess.run([str(binary)],check=True,timeout=10)
