#!/usr/bin/env python3
"""Exercise extracted baseline input code with the pinned, real libfdt parser.

Kernel allocation/list/device operations are mocked; DTB parsing is real.
Structural checks exercise the root writer; baseline resource effects stay mocked.
This does not qualify a booted kernel or resource rollback.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("baseline", type=Path)
parser.add_argument("kernfs", type=Path)
parser.add_argument("libfdt", type=Path)
parser.add_argument("--section", choices=("headers", "devices", "transactions", "structure", "all"), default="all")
args = parser.parse_args()
baseline, kernfs = args.baseline.read_text(), args.kernfs.read_text()
start = baseline.find("/* Device properties are single")
if start < 0:
    start = baseline.index("static int mk_baseline_parse_devices(")
devices = baseline[start:baseline.index("\nstatic int mk_baseline_grow_pool(", start)]
staged = "struct list_head *platform_list" in devices
start = kernfs.index("static ssize_t root_device_tree_write(")
writer = kernfs[start:kernfs.index("\n/**", start)]
prelude = r'''
#define _GNU_SOURCE
#include <stdint.h>
#include <stdbool.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <ctype.h>
#include <unistd.h>
#include <sys/mman.h>
#include <sys/wait.h>
#include <sys/resource.h>
#include <libfdt.h>
typedef uint16_t u16;
typedef uint8_t u8;
#define GFP_KERNEL 0
struct list_head { struct list_head *next,*prev; };
static void list_init(struct list_head *p) { p->next=p->prev=p; }
static void list_add_tail(struct list_head *p,struct list_head *head) {
 p->prev=head->prev;p->next=head;head->prev->next=p;head->prev=p;
}
static void __attribute__((unused)) list_splice_tail_init(struct list_head *from,struct list_head *to) {
 if(from->next==from)return;
 from->next->prev=to->prev;to->prev->next=from->next;from->prev->next=to;to->prev=from->prev;list_init(from);
}
struct mk_pci_device { char name[64]; u16 vendor,device,domain; u8 bus,slot,func; struct list_head list; };
struct mk_platform_device { char name[64]; struct list_head list; };
struct mk_instance { struct list_head platform_devices; int platform_device_count; bool platform_devices_valid; };
static void *objects[32];static int object_count,baseline_calls,mutex_calls;
static void *allocate(size_t n) { void *p=calloc(1,n);if(!p || object_count==32)abort();objects[object_count++]=p;return p; }
#define kzalloc_obj(obj,flags) allocate(sizeof(obj))
static void strscpy(char *d,const char *s,size_t n) { snprintf(d,n,"%s",s); }
static void log_stub(const char *format, ...) { (void)format; }
#define pr_err log_stub
#define pr_info log_stub
#define pr_warn log_stub
#define pr_debug log_stub
struct kernfs_open_file { int unused; };
static int mk_overlay_mutex;
static void mutex_lock(int *p) { (void)p;mutex_calls++; }
static void mutex_unlock(int *p) { (void)p; }
static int mk_baseline_validate_and_initialize(const void *p,size_t n) { (void)p;(void)n;baseline_calls++;return 0; }
#define CHECK(x) do {if(!(x)){fprintf(stderr,"baseline input line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
'''
checks = r'''
static unsigned char blob[4096] __attribute__((aligned(8)));
static struct mk_instance instance;
static struct list_head pci_list;
static struct list_head staged_platforms;
static int staged_count;
static int dev;
static void property(const char *name,const void *data,int len) { CHECK(fdt_setprop(blob,dev,name,data,len)==0); }
static void string_property(const char *name,const char *value) { property(name,value,strlen(value)+1); }
static void setup(void) {
 for(int i=0;i<object_count;i++){free(objects[i]);}
 object_count=0;
 memset(&instance,0,sizeof(instance));list_init(&instance.platform_devices);list_init(&pci_list);list_init(&staged_platforms);staged_count=0;
 CHECK(fdt_create_empty_tree(blob,sizeof(blob))==0);
 int devices=fdt_add_subnode(blob,0,"devices");CHECK(devices>=0);
 dev=fdt_add_subnode(blob,devices,"device@0");CHECK(dev>=0);
 string_property("device-type","pci");string_property("pci-id","0000:ff:1f.7");
 fdt32_t value=cpu_to_fdt32(0xffff);property("vendor-id",&value,4);property("device-id",&value,4);
}
PARSE_ADAPTER
static void devices(void) {
 setup();CHECK(parse()==0 && object_count==1);
 struct mk_pci_device *p=objects[0];CHECK(p->domain==0 && p->bus==255 && p->slot==31 && p->func==7 && p->vendor==65535);
 setup();property("device-type","pci\0junk",9);CHECK(parse()==-EINVAL && !object_count);
 setup();property("device-type","pci",3);CHECK(parse()==-EINVAL && !object_count);
 setup();property("device-type","",1);CHECK(parse()==-EINVAL && !object_count);
 puts("DEVICE_TYPE_STRING_BOUNDS_AND_EXACT_SINGLE_VALUE_PASS");
 const char *bad[]={"00000:00:00.0","0000:100:00.0","0000:00:20.0","0000:00:00.8","0000:00:00.00","0000:00:00.0junk","-1:00:00.0"," 0:0:0.0","0x0:0:0.0","0:0:0","","0:0:0.0:0"};
 for(unsigned int i=0;i<sizeof(bad)/sizeof(bad[0]);i++) {setup();string_property("pci-id",bad[i]);CHECK(parse()==-EINVAL && !object_count);}
 setup();property("pci-id","0:0:0.0",7);CHECK(parse()==-EINVAL && !object_count);
 setup();string_property("pci-id","0:0:0.0");CHECK(parse()==0 && object_count==1);
 puts("PCI_ADDRESS_FULL_CONSUMPTION_WIDTH_AND_RANGE_PASS");
 for(int field=0;field<2;field++) { setup();fdt32_t v=cpu_to_fdt32(0x10000);property(field?"device-id":"vendor-id",&v,4);CHECK(parse()==-EINVAL && !object_count); }
 puts("PCI_VENDOR_AND_DEVICE_NARROWING_REFUSAL_PASS");
 setup();string_property("device-type","platform");string_property("device-name","safe-name");CHECK(parse()==0 && instance.platform_device_count==1);
 CHECK(strcmp(((struct mk_platform_device*)objects[0])->name,"safe-name")==0);
 setup();string_property("device-type","platform");CHECK(parse()==0 && instance.platform_device_count==1);
 setup();string_property("device-type","platform");property("device-name","safe-name",9);CHECK(parse()==-EINVAL && !object_count);
 setup();string_property("device-type","platform");property("device-name","safe\0alias",11);CHECK(parse()==-EINVAL && !object_count);
 puts("PLATFORM_OPTIONAL_NAME_AND_BOUNDED_SINGLE_VALUE_PASS");
 for(int i=0;i<object_count;i++){free(objects[i]);}
 object_count=0;
}
static void headers(void) {
 struct rlimit limit={0,0};CHECK(setrlimit(RLIMIT_CORE,&limit)==0);
 long page=sysconf(_SC_PAGESIZE);CHECK(page>0);
 void *guard=mmap(NULL,page,PROT_NONE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);CHECK(guard!=MAP_FAILED);
 pid_t child=fork();CHECK(child>=0);
 if(!child) {
  for(size_t n=0;n<sizeof(struct fdt_header);n++)CHECK(root_device_tree_write(NULL,guard,n,0)==-EINVAL);
  CHECK(!baseline_calls && !mutex_calls);_exit(0);
 }
 int status;CHECK(waitpid(child,&status,0)==child);
 if(WIFSIGNALED(status))fprintf(stderr,"short-header reader terminated by signal %d\n",WTERMSIG(status));
 CHECK(WIFEXITED(status) && WEXITSTATUS(status)==0);CHECK(munmap(guard,page)==0);
 setup();baseline_calls=mutex_calls=0;
 CHECK(root_device_tree_write(NULL,(char*)blob,sizeof(blob),1)==-EINVAL && !baseline_calls && !mutex_calls);
 CHECK(root_device_tree_write(NULL,(char*)blob,sizeof(blob),0)==sizeof(blob) && baseline_calls==1 && mutex_calls==1);
 puts("ALL_SHORT_HEADERS_AND_NONZERO_OFFSETS_REFUSED_BEFORE_PARSER_PASS");
 puts("COMPLETE_DTB_REACHES_SERIALIZED_BASELINE_PASS");
}
static void transactions(void) {
 setup();
 string_property("device-type","pci");string_property("pci-id","invalid");
 int parent=fdt_path_offset(blob,"/devices");CHECK(parent>=0);
 dev=fdt_add_subnode(blob,parent,"device@1");CHECK(dev>=0);
 string_property("device-type","platform");string_property("device-name","new-private-device");
 struct mk_platform_device existing={.name="existing-device"};
 list_add_tail(&existing.list,&instance.platform_devices);
 instance.platform_device_count=1;instance.platform_devices_valid=true;
 CHECK(parse()==-EINVAL);
 CHECK(instance.platform_device_count==1 && instance.platform_devices_valid);
 CHECK(instance.platform_devices.next==&existing.list && instance.platform_devices.prev==&existing.list);
 CHECK(existing.list.next==&instance.platform_devices && existing.list.prev==&instance.platform_devices);
 CHECK(strcmp(existing.name,"existing-device")==0);
 CHECK(staged_count==1 && staged_platforms.next!=&staged_platforms);
 puts("PARTIAL_DEVICE_PARSE_KEEPS_NEW_METADATA_PRIVATE_AND_OLD_METADATA_UNCHANGED_PASS");
 for(int i=0;i<object_count;i++){free(objects[i]);}
 object_count=0;
}

static void structure_setup(void) {
 CHECK(fdt_create_empty_tree(blob,sizeof(blob))==0);
 CHECK(fdt_setprop_string(blob,0,"sample","value")==0);
 baseline_calls=mutex_calls=0;
}
static void structure_refused(const char *name) {
 CHECK(fdt_check_header(blob)==0);
 int ret=root_device_tree_write(NULL,(char*)blob,fdt_totalsize(blob),0);
 if(ret!=-EINVAL || baseline_calls || mutex_calls)
  fprintf(stderr,"structural refusal %s: ret=%d baseline=%d mutex=%d\n",name,ret,baseline_calls,mutex_calls);
 CHECK(ret==-EINVAL && !baseline_calls && !mutex_calls);
}
static void structure(void) {
 structure_setup();
 CHECK(root_device_tree_write(NULL,(char*)blob,fdt_totalsize(blob),0)==fdt_totalsize(blob));
 CHECK(baseline_calls==1 && mutex_calls==1);
 structure_setup();CHECK(fdt_pack(blob)==0);
 CHECK(root_device_tree_write(NULL,(char*)blob,fdt_totalsize(blob),0)==fdt_totalsize(blob));
 puts("PADDED_AND_PACKED_VALID_TREES_REACH_BASELINE_PASS");
 CHECK(fdt_create_empty_tree(blob,sizeof(blob))==0);
 fdt32_t *tags=(fdt32_t*)(blob+fdt_off_dt_struct(blob));
 tags[3]=cpu_to_fdt32(FDT_NOP);tags[4]=cpu_to_fdt32(FDT_END);
 fdt_set_size_dt_struct(blob,20);
 fdt_set_off_dt_strings(blob,fdt_off_dt_struct(blob)+20);
 baseline_calls=mutex_calls=0;
 CHECK(root_device_tree_write(NULL,(char*)blob,fdt_totalsize(blob),0)==fdt_totalsize(blob));
 CHECK(baseline_calls==1 && mutex_calls==1);
 puts("LEGAL_NOP_AFTER_ROOT_ACCEPTED_PASS");
 structure_setup();
 struct fdt_property *prop=(struct fdt_property*)(blob+fdt_off_dt_struct(blob)+fdt_first_property_offset(blob,0));
 prop->nameoff=cpu_to_fdt32(fdt_size_dt_strings(blob)+1);
 structure_refused("property name outside strings block");
 structure_setup();prop=(struct fdt_property*)(blob+fdt_off_dt_struct(blob)+fdt_first_property_offset(blob,0));
 prop->len=cpu_to_fdt32(UINT32_MAX);
 structure_refused("property length exceeds structure");
 structure_setup();tags=(fdt32_t*)(blob+fdt_off_dt_struct(blob));tags[0]=cpu_to_fdt32(0xbad);
 structure_refused("unknown structure tag");
 structure_setup();fdt_set_off_mem_rsvmap(blob,fdt_totalsize(blob)-8);
 structure_refused("truncated reservation terminator");
 structure_setup();tags=(fdt32_t*)(blob+fdt_off_dt_struct(blob));
 tags[fdt_size_dt_struct(blob)/4-2]=cpu_to_fdt32(FDT_NOP);
 structure_refused("unclosed root");
 structure_setup();tags=(fdt32_t*)(blob+fdt_off_dt_struct(blob));tags[0]=cpu_to_fdt32(FDT_END_NODE);
 structure_refused("unmatched end node");
 CHECK(fdt_create_empty_tree(blob,sizeof(blob))==0);
 tags=(fdt32_t*)(blob+fdt_off_dt_struct(blob));
 const uint32_t two_roots[]={FDT_BEGIN_NODE,0,FDT_END_NODE,FDT_BEGIN_NODE,0,FDT_END_NODE,FDT_END};
 for(unsigned int i=0;i<sizeof(two_roots)/sizeof(two_roots[0]);i++)tags[i]=cpu_to_fdt32(two_roots[i]);
 fdt_set_size_dt_struct(blob,sizeof(two_roots));fdt_set_off_dt_strings(blob,fdt_off_dt_struct(blob)+sizeof(two_roots));
 baseline_calls=mutex_calls=0;structure_refused("second root");
 structure_setup();blob[fdt_off_dt_struct(blob)+4]='x';structure_refused("nonempty root name");
 structure_setup();fdt_set_size_dt_struct(blob,fdt_size_dt_struct(blob)-4);
 structure_refused("missing final end tag");
 puts("NINE_HEADER_VALID_MALFORMED_TREES_REFUSED_BEFORE_BASELINE_AND_LOCK_PASS");
 structure_setup();unsigned int full=fdt_totalsize(blob);
 CHECK(root_device_tree_write(NULL,(char*)blob,full-1,0)==-EINVAL);
 CHECK(!baseline_calls && !mutex_calls);
 puts("DECLARED_TREE_LARGER_THAN_SUPPLIED_BUFFER_REFUSED_PASS");
}

int main(int argc,char **argv) {
 CHECK(argc==2);
 if(strcmp(argv[1],"devices")==0)devices();
 else if(strcmp(argv[1],"headers")==0)headers();
 else if(strcmp(argv[1],"transactions")==0)transactions();
 else if(strcmp(argv[1],"structure")==0)structure();else abort();
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix="mk-baseline-inputs-") as directory:
    root = Path(directory)
    source, binary = root / "fixture.c", root / "fixture"
    adapter = r'''
static int parse(void) {
 int ret=mk_baseline_parse_devices(blob,0,&staged_platforms,&pci_list,&staged_count);
 if(!ret) {list_splice_tail_init(&staged_platforms,&instance.platform_devices);instance.platform_device_count=staged_count;instance.platform_devices_valid=staged_count>0;}
 return ret;
}
''' if staged else r'''
static int parse(void) { return mk_baseline_parse_devices(blob,0,&instance,&pci_list); }
'''
    checks = checks.replace("PARSE_ADAPTER", adapter)
    source.write_text(prelude + devices + writer + checks)
    library = [args.libfdt / name for name in (
        "fdt.c", "fdt_ro.c", "fdt_rw.c", "fdt_wip.c", "fdt_sw.c", "fdt_empty_tree.c")]
    if (args.libfdt / "fdt_check.c").exists():
        library.append(args.libfdt / "fdt_check.c")
    subprocess.run(["cc", "-O2", "-Wall", "-Wextra", "-Werror",
                    "-Wno-unused-parameter",
                    "-I", str(args.libfdt), str(source), *(str(p) for p in library),
                    "-o", str(binary)], check=True)
    sections = ("headers", "devices", "transactions") if args.section == "all" else (args.section,)
    if args.section == "all" and "fdt_check_full(" in writer:
        sections += ("structure",)
    for section in sections:
        subprocess.run([str(binary), section], check=True, timeout=10)
