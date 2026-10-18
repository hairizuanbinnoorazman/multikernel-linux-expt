#!/usr/bin/env python3
"""Check the exact instance-tree validator with real pinned libfdt and UBSan.

The validator is extracted without kernel resource mocks. No boot, mapping,
resource-semantic validation or hostile-child containment is implied.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('source', type=Path)
p.add_argument('libfdt', type=Path)
p.add_argument('--section', choices=('valid', 'malformed', 'bounds', 'all'), default='all')
p.add_argument('--expect-legacy-acceptance', action='store_true', help='record all malformed cases accepted by the old bounds-only helper')
a = p.parse_args()
if a.expect_legacy_acceptance and a.section != 'malformed':
    p.error('--expect-legacy-acceptance requires --section malformed')
s = a.source.read_text()
start = s.index('int mk_dt_check_blob_bounds(')
function = s[start:s.index('\n}', start)+2]
prelude = r'''
#define _GNU_SOURCE
#include <stdbool.h>
#include <stdint.h>
#include <limits.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <errno.h>
#include <unistd.h>
#include <sys/mman.h>
#include <libfdt.h>
#define CHECK(x) do {if(!(x)){fprintf(stderr,"instance structure line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
'''
checks = r'''
static unsigned char blob[4096] __attribute__((aligned(8)));
static void put(unsigned int at,uint32_t value){fdt32_t cell=cpu_to_fdt32(value);memcpy(blob+at,&cell,4);}
static void setup(const char *name){
 CHECK(!fdt_create(blob,sizeof(blob)));CHECK(!fdt_finish_reservemap(blob));CHECK(!fdt_begin_node(blob,name));
 fdt32_t id=cpu_to_fdt32(61);CHECK(!fdt_property(blob,"id",&id,4));
 CHECK(!fdt_begin_node(blob,"resources"));CHECK(!fdt_property_string(blob,"description","valid"));CHECK(!fdt_end_node(blob));
 CHECK(!fdt_end_node(blob));CHECK(!fdt_finish(blob));CHECK(!fdt_pack(blob));
 CHECK(!fdt_check_header(blob));
}
static int check(void){return mk_dt_check_blob_bounds(blob,fdt_totalsize(blob));}
static void refused(const char *name){CHECK(!fdt_check_header(blob));CHECK(check()==(EXPECT_LEGACY_ACCEPTANCE ? 0 : -EINVAL));printf("%s case=%s\n",EXPECT_LEGACY_ACCEPTANCE ? "LEGACY_BOUND_ONLY_MALFORMED_ACCEPTED" : "INSTANCE_MALFORMED_REFUSED",name);}
static void valid(void){
 const char *names[]={"","mk-matrix-ctr","long-instance-root-spanning-many-words"};
 for(size_t i=0;i<sizeof(names)/sizeof(*names);i++){
  setup(names[i]);CHECK(!check());CHECK(!mk_dt_check_blob_bounds(blob,sizeof(blob)));
  if(i)CHECK(fdt_check_full(blob,fdt_totalsize(blob))<0);else CHECK(!fdt_check_full(blob,fdt_totalsize(blob)));
  unsigned int end=fdt_off_dt_struct(blob)+fdt_size_dt_struct(blob)-4;
  unsigned int total=fdt_totalsize(blob);memmove(blob+end+4,blob+end,total-end);put(end,FDT_NOP);
  fdt_set_off_dt_strings(blob,fdt_off_dt_strings(blob)+4);fdt_set_size_dt_struct(blob,fdt_size_dt_struct(blob)+4);fdt_set_totalsize(blob,total+4);
  CHECK(!check());
 }
 puts("INSTANCE_NAMED_AND_STANDARD_ROOTS_PADDED_BUFFERS_AND_LEGAL_NOP_PASS");
}
static void tags(const uint32_t *values,size_t count){unsigned int off=fdt_off_dt_struct(blob);for(size_t i=0;i<count;i++)put(off+i*4,values[i]);fdt_set_size_dt_struct(blob,count*4);}
static void malformed(void){
 setup("named-root");int prop=fdt_first_property_offset(blob,0);CHECK(prop>=0);put(fdt_off_dt_struct(blob)+prop+8,fdt_size_dt_strings(blob));refused("property-name-offset");
 setup("named-root");prop=fdt_first_property_offset(blob,0);put(fdt_off_dt_struct(blob)+prop+4,0xffffffff);refused("property-size-overflow");
 setup("named-root");put(fdt_off_dt_struct(blob),0xdead);refused("unknown-tag");
 setup("named-root");fdt_set_off_mem_rsvmap(blob,fdt_totalsize(blob)-8);refused("reservation-terminator-truncated");
 setup("named-root");put(fdt_off_dt_struct(blob)+fdt_size_dt_struct(blob)-8,FDT_NOP);refused("unclosed-root");
 setup("named-root");put(fdt_off_dt_struct(blob),FDT_END_NODE);refused("unmatched-end-node");
 setup("named-root");uint32_t two[]={FDT_BEGIN_NODE,0,FDT_END_NODE,FDT_BEGIN_NODE,0,FDT_END_NODE,FDT_END};tags(two,7);refused("two-roots");
 setup("named-root");fdt_set_size_dt_struct(blob,fdt_size_dt_struct(blob)-4);refused("missing-final-end");
 setup("named-root");uint32_t empty[]={FDT_END};tags(empty,1);refused("no-root");
 setup("named-root");uint32_t before[]={FDT_PROP,0,0,FDT_END};tags(before,4);refused("property-before-root");
 setup("named-root");uint32_t after[]={FDT_BEGIN_NODE,0,FDT_END_NODE,FDT_PROP,0,0,FDT_END};tags(after,7);refused("property-after-root");
 setup("named-root");memset(blob+fdt_off_dt_strings(blob),'x',fdt_size_dt_strings(blob));refused("unterminated-property-name");
 puts(EXPECT_LEGACY_ACCEPTANCE ? "LEGACY_BOUND_ONLY_ACCEPTED_ALL_12_MALFORMED_TREES" : "INSTANCE_COMPLETE_TAG_PROPERTY_AND_RESERVATION_CHECK_PASS");
}
static void bounds(void){
 setup("named-root");long page=sysconf(_SC_PAGESIZE);CHECK(page>=4096);
 unsigned char *mapping=mmap(NULL,page*2,PROT_READ|PROT_WRITE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);CHECK(mapping!=MAP_FAILED);CHECK(!mprotect(mapping+page,page,PROT_NONE));
 for(size_t n=0;n<sizeof(struct fdt_header);n++){unsigned char *at=mapping+page-n;memcpy(at,blob,n);CHECK(mk_dt_check_blob_bounds(at,n)==-EINVAL);}
 size_t full=(fdt_totalsize(blob)+7)&~7U;fdt_set_totalsize(blob,full);unsigned char *at=mapping+page-full;memcpy(at,blob,full);CHECK(!mk_dt_check_blob_bounds(at,full));CHECK(mk_dt_check_blob_bounds(at,full-1)==-EINVAL);CHECK(mk_dt_check_blob_bounds(NULL,full)==-EINVAL);
 CHECK(!munmap(mapping,page*2));puts("INSTANCE_GUARDED_SHORT_AND_DECLARED_BUFFER_BOUNDS_PASS");
}
int main(int argc,char **argv){CHECK(argc==2);if(!strcmp(argv[1],"valid"))valid();else if(!strcmp(argv[1],"malformed"))malformed();else if(!strcmp(argv[1],"bounds"))bounds();else abort();return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-instance-structure-') as directory:
    root = Path(directory)
    source, binary = root/'fixture.c', root/'fixture'
    source.write_text(prelude+"\n#define EXPECT_LEGACY_ACCEPTANCE "+str(int(a.expect_legacy_acceptance))+"\n"+function+checks)
    library = [a.libfdt/name for name in ('fdt.c','fdt_ro.c','fdt_rw.c','fdt_wip.c','fdt_sw.c','fdt_empty_tree.c','fdt_check.c')]
    subprocess.run(['cc','-O2','-Wall','-Wextra','-Werror','-fsanitize=undefined','-fno-sanitize-recover=all','-I',str(a.libfdt),str(source),*(str(x) for x in library),'-o',str(binary)],check=True)
    for section in ('valid','malformed','bounds') if a.section=='all' else (a.section,):
        subprocess.run([str(binary),section],check=True,timeout=10)
