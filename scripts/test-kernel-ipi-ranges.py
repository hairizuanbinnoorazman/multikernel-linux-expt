#!/usr/bin/env python3
"""Exercise actual manifest IPI readers with pinned libfdt and production layout.

Physical mapping, instance allocation and CPU registration are mocked. The
32-bit physical-address variant is a type-width model, not a 32-bit kernel run.
Legacy controls disable alignment UBSan to expose numeric refusal differences
in old x86 readers; new code runs with all undefined-behavior checks enabled.
"""
import argparse
from pathlib import Path
import re
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('source', type=Path)
p.add_argument('dts', type=Path)
p.add_argument('header', type=Path)
p.add_argument('libfdt', type=Path)
p.add_argument('--section', choices=('ranges','own','host','pair','cpus','all'), default='all')
p.add_argument('--legacy-control', action='store_true')
a = p.parse_args()
s = a.source.read_text()
def function(text, marker):
    at = text.index(marker)
    return text[at:text.index('\n}',at)+2]+'\n'
helpers = ''
for marker in ('static int __init mk_manifest_get_ipi_range(', 'static int __init mk_manifest_validate_ipi_ranges('):
    if marker in s:
        helpers += function(s,marker)
readers = ''.join(function(s,marker) for marker in ('static int __init mk_restore_instance_ipi(', 'static struct mk_instance * __init mk_restore_host_instance(', 'void __init mk_register_cpus_from_manifest('))
bounds = function(a.dts.read_text(),'int mk_dt_check_blob_bounds(')
header = a.header.read_text()
layout = ''.join(re.search(r'^#define '+name+r'\s+\d+.*$',header,re.M).group(0)+'\n' for name in ('MK_MAX_DATA_SIZE','MK_IPI_RING_SIZE'))
layout += ''.join(re.search(r'struct '+name+r' \{.*?\n\};',header,re.S).group(0)+'\n' for name in ('mk_ipi_data','mk_ipi_ring','mk_shared_data'))
prelude = r'''
#define _GNU_SOURCE
#include <stdbool.h>
#include <stdint.h>
#include <limits.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <errno.h>
#include <libfdt.h>
#define __init
#define PAGE_SHIFT 12
#define PAGE_SIZE (1UL<<PAGE_SHIFT)
#define PAGE_ALIGN(n) (((n)+PAGE_SIZE-1)&~(PAGE_SIZE-1))
#define PHYS_ADDR_MAX ((phys_addr_t)~(phys_addr_t)0)
#define MEMREMAP_WB 1
#if PHYS_BITS == 32
typedef uint32_t phys_addr_t;
#else
typedef uint64_t phys_addr_t;
#endif
typedef uint64_t u64,mk_phys_cpu_t;typedef uint32_t u32;
typedef struct { int counter; } atomic_t;
#define CHECK(x) do {if(!(x)){fprintf(stderr,"IPI range line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
'''
mocks = r'''
struct mk_instance { char *name;int *cpus;struct mk_shared_data *ipi_data;phys_addr_t ipi_phys;u32 ipi_pages; };
static unsigned char blob[4096] __attribute__((aligned(8))),embedded[1024] __attribute__((aligned(8)));
static struct mk_shared_data mapped;
static int map_calls,alloc_calls,add_calls,free_calls,map_fail,add_fail,early_maps,early_unmaps,registered;
static phys_addr_t last_phys;static size_t last_size;
static struct mk_shared_data *memremap(phys_addr_t p,size_t n,int flags){(void)flags;map_calls++;last_phys=p;last_size=n;return map_fail?NULL:&mapped;}
static struct mk_instance *alloc_mk_instance(int id,const char *name,bool ipi){(void)id;(void)name;(void)ipi;alloc_calls++;struct mk_instance *p=calloc(1,sizeof(*p));CHECK(p);p->name=strdup("");p->cpus=calloc(1,sizeof(int));CHECK(p->name&&p->cpus);return p;}
static int mk_cpu_set_add(int *p,int cpu){(void)p;(void)cpu;add_calls++;return add_fail?-ENOMEM:0;}
static void kfree(void *p){free_calls++;free(p);}
static void mk_cpu_set_free(int *p){kfree(p);}
static phys_addr_t mk_manifest_phys(void){return 0x1000;}
static const void *early_memremap(phys_addr_t p,size_t n){(void)p;(void)n;early_maps++;return blob;}
static void early_memunmap(void *p,size_t n){(void)p;(void)n;early_unmaps++;}
static void mk_arch_register_cpu(mk_phys_cpu_t p){CHECK(p==8||p==10||p==12||p==14);registered++;}
static void log_stub(const char *format,...){(void)format;}
#define pr_err log_stub
#define pr_warn log_stub
#define pr_info log_stub
#define pr_debug log_stub
'''
# This fixture covers range readers; the separate publication fixture exercises
# actual registry publication and private-object cleanup under failures.
if 'static int __init mk_publish_boot_instance(' in s:
    mocks += r'''
static int mk_publish_boot_instance(struct mk_instance *p,bool root){(void)p;(void)root;return 0;}
static void mk_free_private_instance(struct mk_instance *p,bool owned){(void)owned;kfree(p->name);mk_cpu_set_free(p->cpus);kfree(p);}
'''
if 'const struct cpumask *present)' in s:
    mocks = mocks.replace('struct mk_instance *p,bool root)', 'struct mk_instance *p,bool root,const void *present)')
checks = r'''
static void setup(void){CHECK(!fdt_create_empty_tree(blob,sizeof(blob)));map_calls=alloc_calls=add_calls=free_calls=map_fail=add_fail=early_maps=early_unmaps=registered=0;last_phys=0;last_size=0;}
static unsigned pages(void){return PAGE_ALIGN(sizeof(struct mk_shared_data))>>PAGE_SHIFT;}
static int node(const char *name){return fdt_subnode_offset(blob,0,name);}
static void range(const char *name,u64 address,u32 count){int n=node(name);if(n<0)n=fdt_add_subnode(blob,0,name);CHECK(n>=0);CHECK(!fdt_setprop_u64(blob,n,"phys-addr",address));CHECK(!fdt_setprop_u32(blob,n,"pages",count));}
static struct mk_instance sentinel(void){return (struct mk_instance){.ipi_data=(void*)0x20,.ipi_phys=0x3000,.ipi_pages=7};}
static bool unchanged(struct mk_instance *p){return p->ipi_data==(void*)0x20&&p->ipi_phys==0x3000&&p->ipi_pages==7;}
static void own(void){
 setup();range("ipi-buffer",0x200000,pages());struct mk_instance i=sentinel();CHECK(!mk_restore_instance_ipi(blob,&i));CHECK(map_calls==1&&last_phys==0x200000&&last_size==PAGE_ALIGN(sizeof(struct mk_shared_data)));CHECK(i.ipi_data==&mapped&&i.ipi_pages==pages());
 const u32 counts[]={1,0,64,66,UINT32_MAX};
 for(size_t n=0;n<sizeof(counts)/sizeof(*counts);n++){setup();range("ipi-buffer",0x200000,counts[n]);i=sentinel();int ret=mk_restore_instance_ipi(blob,&i);printf("OWN_OBSERVATION pages=%u ret=%d maps=%d size=%zu\n",counts[n],ret,map_calls,last_size);CHECK(ret==-EINVAL&&map_calls==0&&unchanged(&i));}
 setup();i=sentinel();CHECK(!mk_restore_instance_ipi(blob,&i)&&!map_calls&&!i.ipi_data);
 setup();range("ipi-buffer",0x200000,pages());i=sentinel();map_fail=1;CHECK(mk_restore_instance_ipi(blob,&i)==-ENOMEM&&map_calls==1&&unchanged(&i));
 puts("IPI_OWN_EXACT_SIZE_NO_MAPPING_ON_REFUSAL_AND_OUTPUT_PRESERVATION_PASS");
}
static void host(void){
 setup();range("host-ipi-buffer",0x400000,pages());struct mk_instance *h=mk_restore_host_instance(blob);CHECK(h&&alloc_calls==1&&map_calls==1&&h->ipi_data==&mapped&&h->ipi_pages==pages());kfree(h->name);mk_cpu_set_free(h->cpus);kfree(h);
 setup();range("host-ipi-buffer",0x400000,1);h=mk_restore_host_instance(blob);printf("HOST_OBSERVATION allocations=%d maps=%d\n",alloc_calls,map_calls);CHECK(!h&&!alloc_calls&&!map_calls);
 setup();CHECK(!mk_restore_host_instance(blob)&&!alloc_calls&&!map_calls);
 setup();range("host-ipi-buffer",0x400000,pages());map_fail=1;CHECK(!mk_restore_host_instance(blob)&&alloc_calls==1&&map_calls==1&&free_calls==3);
 setup();range("host-ipi-buffer",0x400000,pages());add_fail=1;CHECK(!mk_restore_host_instance(blob)&&alloc_calls==1&&!map_calls&&free_calls==3);
 puts("IPI_HOST_PREFLIGHT_BEFORE_ALLOCATION_AND_MAPPING_PASS");
}
#ifdef HAVE_RANGE_HELPER
static void check_bad(const char *name){phys_addr_t physical=123;u32 count=456;size_t size=789;CHECK(mk_manifest_get_ipi_range(blob,name,&physical,&count,&size)==-EINVAL);CHECK(physical==123&&count==456&&size==789);}
static void ranges(void){
 const char *names[]={"ipi-buffer","host-ipi-buffer"};
 for(int kind=0;kind<2;kind++){
  const char *name=names[kind];phys_addr_t physical=123;u32 count=456;size_t size=789;
  setup();CHECK(mk_manifest_get_ipi_range(blob,name,&physical,&count,&size)==-ENOENT&&physical==123&&count==456&&size==789);
  const u64 bad[]={0,1,0x200001,((u64)PHYS_ADDR_MAX)&~(PAGE_SIZE-1)};
  for(size_t n=0;n<sizeof(bad)/sizeof(*bad);n++){setup();range(name,bad[n],pages());check_bad(name);}
  setup();u64 high=(u64)PHYS_ADDR_MAX-PAGE_ALIGN(sizeof(struct mk_shared_data))+1;range(name,high,pages());CHECK(!mk_manifest_get_ipi_range(blob,name,&physical,&count,&size)&&physical==high&&count==pages()&&size==PAGE_ALIGN(sizeof(struct mk_shared_data)));
#if PHYS_BITS == 32
  setup();range(name,0x100200000ULL,pages());check_bad(name);
#endif
  for(int width=0;width<=12;width++){
   if(width!=8){setup();range(name,0x200000,pages());unsigned char data[12]={0};CHECK(!fdt_setprop(blob,node(name),"phys-addr",data,width));check_bad(name);}
   if(width!=4){setup();range(name,0x200000,pages());unsigned char data[12]={0};CHECK(!fdt_setprop(blob,node(name),"pages",data,width));check_bad(name);}
  }
  for(int field=0;field<2;field++){setup();range(name,0x200000,pages());CHECK(!fdt_delprop(blob,node(name),field?"pages":"phys-addr"));check_bad(name);}
 }
 puts("IPI_WIDTH_ALIGNMENT_PHYSICAL_TYPE_EXTENT_AND_EXACT_ABI_BOUNDS_PASS");
}
static void pair(void){
 setup();CHECK(!mk_manifest_validate_ipi_ranges(blob));
 setup();range("ipi-buffer",0x200000,pages());CHECK(!mk_manifest_validate_ipi_ranges(blob));
 setup();range("host-ipi-buffer",0x400000,pages());CHECK(!mk_manifest_validate_ipi_ranges(blob));
 setup();range("ipi-buffer",0x200000,pages());range("host-ipi-buffer",0x400000,pages());CHECK(!mk_manifest_validate_ipi_ranges(blob));
 const u64 overlap[]={0x200000,0x201000,0x1ff000};
 for(size_t n=0;n<sizeof(overlap)/sizeof(*overlap);n++){setup();range("ipi-buffer",0x200000,pages());range("host-ipi-buffer",overlap[n],pages());CHECK(mk_manifest_validate_ipi_ranges(blob)==-EINVAL&&!map_calls&&!alloc_calls);}
 setup();range("ipi-buffer",0x200000,pages());range("host-ipi-buffer",0x200000+PAGE_ALIGN(sizeof(struct mk_shared_data)),pages());CHECK(!mk_manifest_validate_ipi_ranges(blob));
 puts("IPI_PAIR_DISJOINT_OR_OPTIONAL_RANGES_PREFLIGHT_PASS");
}
#endif
static void cpu_setup(void){
 setup();CHECK(!fdt_create(embedded,sizeof(embedded)));CHECK(!fdt_finish_reservemap(embedded));CHECK(!fdt_begin_node(embedded,"spawn"));CHECK(!fdt_begin_node(embedded,"resources"));fdt64_t cpus[]={cpu_to_fdt64(8),cpu_to_fdt64(10)};CHECK(!fdt_property(embedded,"cpus",cpus,sizeof(cpus)));CHECK(!fdt_end_node(embedded));CHECK(!fdt_end_node(embedded));CHECK(!fdt_finish(embedded));CHECK(!fdt_pack(embedded));
 int n=fdt_add_subnode(blob,0,"multikernel");CHECK(n>=0);CHECK(!fdt_setprop(blob,n,"dtb-data",embedded,fdt_totalsize(embedded)));fdt64_t pool[]={cpu_to_fdt64(12),cpu_to_fdt64(14)};CHECK(!fdt_setprop(blob,n,"pool-cpus",pool,sizeof(pool)));
 int len;const void *copy=fdt_getprop(blob,n,"dtb-data",&len);if((uintptr_t)copy&7){CHECK(!fdt_setprop(blob,n,"pad",NULL,0));copy=fdt_getprop(blob,n,"dtb-data",&len);}CHECK(!((uintptr_t)copy&7));CHECK(!mk_dt_check_blob_bounds(copy,len));
 range("ipi-buffer",0x200000,pages());range("host-ipi-buffer",0x400000,pages());
 /* Adding sibling nodes can shift the embedded property by four bytes. */
 n=node("multikernel");copy=fdt_getprop(blob,n,"dtb-data",&len);if((uintptr_t)copy&7)CHECK(!fdt_setprop(blob,n,"pad2",NULL,0));
}
static void cpus(void){
 cpu_setup();mk_register_cpus_from_manifest();CHECK(registered==4&&early_maps==1&&early_unmaps==1);
 for(int host=0;host<2;host++){cpu_setup();range(host?"host-ipi-buffer":"ipi-buffer",0x200000,1);mk_register_cpus_from_manifest();printf("CPU_PREFLIGHT_OBSERVATION registrations=%d\n",registered);CHECK(!registered&&early_maps==1&&early_unmaps==1);}
 cpu_setup();range("host-ipi-buffer",0x200000,pages());mk_register_cpus_from_manifest();CHECK(!registered&&early_maps==1&&early_unmaps==1);
 puts("IPI_INVALID_RANGES_BLOCK_CPU_REGISTRATION_AND_UNMAP_MANIFEST_PASS");
}
int main(int argc,char **argv){CHECK(argc==2);printf("IPI_ABI size=%zu pages=%u physical_bits=%d\n",sizeof(struct mk_shared_data),pages(),PHYS_BITS);
 if(!strcmp(argv[1],"own"))own();else if(!strcmp(argv[1],"host"))host();else if(!strcmp(argv[1],"cpus"))cpus();
#ifdef HAVE_RANGE_HELPER
 else if(!strcmp(argv[1],"ranges"))ranges();else if(!strcmp(argv[1],"pair"))pair();
#endif
 else abort();return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-ipi-range-') as directory:
    root = Path(directory)
    source,binary=root/'fixture.c',root/'fixture'
    source.write_text(prelude+layout+mocks+bounds+helpers+readers+checks)
    library=[a.libfdt/n for n in ('fdt.c','fdt_ro.c','fdt_rw.c','fdt_wip.c','fdt_sw.c','fdt_empty_tree.c','fdt_check.c')]
    sections=('ranges','own','host','pair','cpus') if a.section=='all' else (a.section,)
    for bits in (64,32):
        command=['cc','-O2','-Wall','-Wextra','-Werror','-Wno-unused-parameter','-Wno-unused-function','-Wno-misleading-indentation','-fsanitize=undefined','-fno-sanitize-recover=all',f'-DPHYS_BITS={bits}']
        if a.legacy_control:command+=['-fno-sanitize=alignment','-Wno-sign-compare']
        if helpers:command+=['-DHAVE_RANGE_HELPER=1']
        command+=['-I',str(a.libfdt),str(source),*(str(x) for x in library),'-o',str(binary)]
        subprocess.run(command,check=True)
        for section in sections:subprocess.run([str(binary),section],check=True,timeout=10)
