#!/usr/bin/env python3
"""Run actual overlay memory readers with modeled property/native type widths.

This checks conversion refusal, not physical ownership, IPI delivery or hotplug.
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
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdarg.h>
#include <string.h>
#include <errno.h>
#include <arpa/inet.h>
typedef uint64_t u64;
typedef uint32_t fdt32_t;
#if PHYS_BITS == 32
typedef uint32_t phys_addr_t;
#else
typedef uint64_t phys_addr_t;
#endif
#if SIZE_BITS == 32
typedef uint32_t fixture_size_t;
#else
typedef uint64_t fixture_size_t;
#endif
#define size_t fixture_size_t
#define PHYS_ADDR_MAX ((phys_addr_t)~(phys_addr_t)0)
#define PAGE_SIZE (1ULL << PAGE_BITS)
#define PAGE_ALIGNED(n) (!((n) & (PAGE_SIZE - 1)))
#define fdt32_to_cpu(n) ntohl(n)
#define CHECK(n) do {if (!(n)) {fprintf(stderr,"overlay memory line %d: %s\n",__LINE__,#n);exit(2);}} while(0)
static void log_stub(const char *fmt,...) {(void)fmt;}
#define pr_err log_stub
static fdt32_t cells[4];
static int property_length,missing;
static const void *fdt_getprop(const void *fdt,int node,const char *name,int *len)
{(void)fdt;(void)node;(void)name;*len=property_length;return missing?NULL:cells;}
''' + function("mk_overlay_parse_mem_reg") + function("mk_overlay_parse_mem_size") + r'''
static void reg(u64 base,u64 size)
{cells[0]=htonl(base>>32);cells[1]=htonl(base);cells[2]=htonl(size>>32);cells[3]=htonl(size);property_length=16;missing=0;}
static void valid(u64 base,u64 size)
{u64 b=77,s=88;reg(base,size);CHECK(mk_overlay_parse_mem_reg(NULL,0,"memory@0",&b,&s)==0&&b==base&&s==size);}
static void invalid(u64 base,u64 size)
{u64 b=77,s=88;reg(base,size);CHECK(mk_overlay_parse_mem_reg(NULL,0,"memory@0",&b,&s)==-EINVAL&&b==77&&s==88);}
static void pool(u64 size,int expected)
{u64 result=0;reg(0,size);cells[0]=cells[2];cells[1]=cells[3];property_length=8;CHECK(mk_overlay_parse_mem_size(NULL,0,"memory@0",&result)==expected);if(!expected)CHECK(result==size);}
int main(void)
{
 u64 b=77,s=88,p=PAGE_SIZE;
 valid(0,p);valid(p,2*p);valid((u64)PHYS_ADDR_MAX-(p-1),p);
 invalid(0,0);invalid(p,0);invalid(1,p);invalid(p,p-1);invalid(p,p+1);
 invalid((u64)PHYS_ADDR_MAX-(p-1),2*p);
 invalid(UINT64_MAX-(p-1),2*p);
#if PHYS_BITS == 32
 invalid(1ULL<<32,p);invalid((1ULL<<32)-p,2*p);
#endif
#if SIZE_BITS == 32
 invalid(0,1ULL<<32);pool(1ULL<<32,-EINVAL);
#else
#if PHYS_BITS == 64
 valid(0,1ULL<<32);
#endif
 pool(1ULL<<32,0);
#endif
 reg(0,p);missing=1;CHECK(mk_overlay_parse_mem_reg(NULL,0,"x",&b,&s)==-EINVAL&&b==77&&s==88);
 for(int len=0;len<=20;len++){if(len==16)continue;reg(0,p);property_length=len;CHECK(mk_overlay_parse_mem_reg(NULL,0,"x",&b,&s)==-EINVAL&&b==77&&s==88);}
 pool(0,-EINVAL);pool(p-1,-EINVAL);pool(p+1,-EINVAL);pool(p,0);
 reg(0,p);property_length=4;CHECK(mk_overlay_parse_mem_size(NULL,0,"x",&s)==-EINVAL);
 missing=1;CHECK(mk_overlay_parse_mem_size(NULL,0,"x",&s)==-EINVAL);
 puts("OVERLAY_MEMORY_ALIGNMENT_EXTENT_AND_NATIVE_WIDTH_PASS");
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix="mk-overlay-memory-") as temporary:
    root = Path(temporary)
    source = root / "fixture.c"
    source.write_text(code)
    for physical, native, page in [(64, 64, 12), (32, 32, 12),
                                   (64, 32, 12), (32, 64, 12),
                                   (64, 64, 16)]:
        command = ["gcc", "-std=gnu11", "-O1", "-Wall", "-Wextra", "-Werror",
                   "-fsanitize=undefined", "-fno-sanitize-recover=all",
                   f"-DPHYS_BITS={physical}", f"-DSIZE_BITS={native}",
                   f"-DPAGE_BITS={page}", str(source), "-o", str(root / "fixture")]
        subprocess.run(command, check=True)
        print(f"modeled physical/native/page bits={physical}/{native}/{page}", flush=True)
        subprocess.run([str(root / "fixture")], check=True)
