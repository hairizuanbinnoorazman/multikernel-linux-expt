#!/usr/bin/env python3
"""Exercise extracted generic device parsers with real pinned libfdt.

Kernel allocation, lists and base-tree ownership are modeled. This checks parser
refusal/conversion, not hardware assignment, boot or whole-G7 acceptance.
"""
import argparse
from pathlib import Path
import re
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('source', type=Path)
p.add_argument('libfdt', type=Path)
p.add_argument('header', type=Path, help='pinned include/linux/multikernel.h')
p.add_argument('--section', choices=('pci', 'platform', 'dispatch', 'names', 'all'), default='all')
a = p.parse_args()
s = a.source.read_text()
start = s.find('/* Property bytes are not C strings')
if start < 0:
    start = s.index('static int mk_dt_parse_single_pci_device(')
s = s[start:s.index('/**\n * Main device tree parsing', start)]
prelude = r'''
#define _GNU_SOURCE
#include <stdint.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <ctype.h>
#include <libfdt.h>
typedef uint16_t u16; typedef uint8_t u8;
#define GFP_KERNEL 0
#define MK_PLATFORM_DEVICE_ID_LEN 16
#define MK_PLATFORM_DEVICE_NAME_LEN 64
struct list_head { struct list_head *next,*prev; };
struct mk_pci_device { u16 vendor,device,domain; u8 bus,slot,func; struct list_head list; };
struct mk_platform_device { char hid[MK_PLATFORM_DEVICE_ID_LEN],name[MK_PLATFORM_DEVICE_NAME_LEN];struct list_head list; };
struct mk_dt_config { struct list_head pci_devices,platform_devices; int pci_device_count,platform_device_count;bool pci_devices_valid,platform_devices_valid; };
static unsigned char base[8192] __attribute__((aligned(8))), overlay[8192] __attribute__((aligned(8)));
static struct mk_dt_config config;
static void *objects[64];static int allocations,base_lookups;
#define CHECK(x) do {if(!(x)){fprintf(stderr,"generic device line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
static void init_list(struct list_head *p){p->next=p->prev=p;}
static void list_add_tail(struct list_head *p,struct list_head *h){p->prev=h->prev;p->next=h;h->prev->next=p;h->prev=p;}
static void *kzalloc(size_t n,int flags){(void)flags;CHECK(allocations<64);void *p=calloc(1,n);CHECK(p);objects[allocations++]=p;return p;}
static void __attribute__((unused)) strscpy(char *d,const char *s,size_t n){CHECK(strlen(s)<n);memcpy(d,s,strlen(s)+1);}
static void log_stub(const char *f,...){(void)f;}
#define pr_err log_stub
#define pr_info log_stub
#define pr_warn log_stub
#define pr_debug log_stub
static const void *mk_dt_get_base_fdt(void){base_lookups++;return base;}
'''
# Use production capacities, including the platform-name field's real limit.
for constant in ('MK_PLATFORM_DEVICE_ID_LEN', 'MK_PLATFORM_DEVICE_NAME_LEN'):
    value = re.search(r'^#define\s+'+constant+r'\s+(\d+)\s*$', a.header.read_text(), re.M)
    if not value:
        raise ValueError('missing production capacity: '+constant)
    prelude = re.sub(r'(#define '+constant+r') \d+', r'\g<1> '+value.group(1), prelude)

checks = r'''
static int node(const char *name){char path[300];snprintf(path,sizeof(path),"/resources/devices/%s",name);int n=fdt_path_offset(base,path);CHECK(n>=0);return n;}
static void prop(const char *device,const char *name,const void *data,int n){CHECK(!fdt_setprop(base,node(device),name,data,n));}
static void string(const char *device,const char *name,const char *value){prop(device,name,value,strlen(value)+1);}
static void cell(const char *device,const char *name,uint32_t value){fdt32_t v=cpu_to_fdt32(value);prop(device,name,&v,4);}
static void setup(void){
 for(int i=0;i<allocations;i++)free(objects[i]);allocations=base_lookups=0;
 memset(&config,0,sizeof(config));init_list(&config.pci_devices);init_list(&config.platform_devices);config.pci_devices_valid=config.platform_devices_valid=true;
 CHECK(!fdt_create_empty_tree(base,sizeof(base)));int r=fdt_add_subnode(base,0,"resources");CHECK(r>=0);int d=fdt_add_subnode(base,r,"devices");CHECK(d>=0);
 CHECK(fdt_add_subnode(base,d,"pci0")>=0);d=fdt_path_offset(base,"/resources/devices");CHECK(fdt_add_subnode(base,d,"serial0")>=0);
 string("pci0","device-type","pci");string("pci0","pci-id","ffff:ff:1f.7");cell("pci0","vendor-id",65535);cell("pci0","device-id",65535);
 string("serial0","device-type","platform");string("serial0","device-name","serial8250");string("serial0","acpi-hid","PNP0501");
 CHECK(!fdt_create_empty_tree(overlay,sizeof(overlay)));CHECK(fdt_add_subnode(overlay,0,"resources")>=0);
}
static int pci_parse(void){return mk_dt_parse_single_pci_device(base,node("pci0"),&config,"pci0");}
static int platform_parse(void){return mk_dt_parse_single_platform_device(base,node("serial0"),&config,"serial0");}
static int list_parse(const void *data,int n){CHECK(!fdt_setprop(overlay,fdt_path_offset(overlay,"/resources"),"device-names",data,n));return mk_dt_parse_devices(overlay,fdt_path_offset(overlay,"/resources"),&config);}
static void refused(int ret){CHECK(ret==-EINVAL);CHECK(!allocations&&!config.pci_device_count&&!config.platform_device_count);CHECK(config.pci_devices.next==&config.pci_devices&&config.platform_devices.next==&config.platform_devices);}
static void pci(void){
 const char *bad[]={"ffff:ff:20.0","ffff:ff:1f.8","10000:00:00.0","0000:100:00.0","0000:00:00.0junk","-1:00:00.0","+1:00:00.0"," 1:00:00.0","0x1:0:0.0","0000:00:00.0 ","0000:00:00.00","0000:00:00.0:0"};
 setup();CHECK(!pci_parse());CHECK(allocations==1&&config.pci_device_count==1);struct mk_pci_device *p=objects[0];CHECK(p->domain==65535&&p->bus==255&&p->slot==31&&p->func==7&&p->vendor==65535&&p->device==65535);
 setup();string("pci0","pci-id","0:0:0.0");CHECK(!pci_parse());
 for(size_t i=0;i<sizeof(bad)/sizeof(*bad);i++){setup();string("pci0","pci-id",bad[i]);refused(pci_parse());}
 const char *fields[]={"vendor-id","device-id"};
 for(int i=0;i<2;i++){setup();cell("pci0",fields[i],65536);refused(pci_parse());setup();prop("pci0",fields[i],"abc",3);refused(pci_parse());}
 const char multi[]="0:0:0.0\0hidden";setup();prop("pci0","pci-id",multi,sizeof(multi));refused(pci_parse());
 setup();prop("pci0","pci-id","0:0:0.0",7);refused(pci_parse());
 setup();prop("pci0","pci-id","",1);refused(pci_parse());
 puts("GENERIC_PCI_STRING_FULL_CONSUMPTION_AND_RANGE_PASS");
}
static void platform(void){
 setup();CHECK(!platform_parse());struct mk_platform_device *p=objects[0];CHECK(!strcmp(p->hid,"PNP0501")&&!strcmp(p->name,"serial8250"));
 const char *fields[]={"acpi-hid","device-name"};int limits[]={MK_PLATFORM_DEVICE_ID_LEN,MK_PLATFORM_DEVICE_NAME_LEN};
 for(int i=0;i<2;i++){
  char value[80];memset(value,'a',sizeof(value));value[limits[i]]=0;
  setup();string("serial0",fields[i],value);refused(platform_parse());
  value[limits[i]-1]=0;setup();string("serial0",fields[i],value);CHECK(!platform_parse());
  setup();prop("serial0",fields[i],"bad",3);refused(platform_parse());
  setup();prop("serial0",fields[i],"",1);refused(platform_parse());
  setup();prop("serial0",fields[i],"a\0b",4);refused(platform_parse());
  setup();CHECK(!fdt_delprop(base,node("serial0"),fields[i]));CHECK(!platform_parse());
 }
 setup();CHECK(!fdt_delprop(base,node("serial0"),"acpi-hid"));CHECK(!fdt_delprop(base,node("serial0"),"device-name"));refused(platform_parse());
 puts("GENERIC_PLATFORM_BOUNDED_OPTIONAL_STRINGS_AND_NO_TRUNCATION_PASS");
}
static void dispatch(void){
 const char multi[]="pci\0hidden";
 setup();CHECK(!mk_dt_parse_embedded_devices(base,fdt_path_offset(base,"/resources"),&config));CHECK(allocations==2);
 setup();prop("serial0","device-type",multi,sizeof(multi));refused(mk_dt_parse_embedded_devices(base,fdt_path_offset(base,"/resources"),&config));
 setup();prop("serial0","device-type","platform",8);refused(mk_dt_parse_embedded_devices(base,fdt_path_offset(base,"/resources"),&config));
 setup();string("serial0","device-type","unknown");refused(mk_dt_parse_embedded_devices(base,fdt_path_offset(base,"/resources"),&config));
 setup();prop("pci0","device-type",multi,sizeof(multi));refused(list_parse("pci0",5));
 setup();CHECK(!fdt_delprop(base,node("serial0"),"device-type"));CHECK(!mk_dt_parse_embedded_devices(base,fdt_path_offset(base,"/resources"),&config));CHECK(allocations==1);
 puts("GENERIC_DEVICE_DISPATCH_PROPERTY_LOCAL_VALIDATION_PASS");
}
static void names(void){
 setup();CHECK(!list_parse("pci0\0serial0",13));CHECK(allocations==2&&base_lookups==1);
 const char *bad[]={"",".","..","../pci0","pci0/child"};
 for(size_t i=0;i<sizeof(bad)/sizeof(*bad);i++){setup();refused(list_parse(bad[i],strlen(bad[i])+1));CHECK(!base_lookups);}
 setup();refused(list_parse("pci0\0\0serial0",14));CHECK(!base_lookups);
 setup();refused(list_parse("pci0\0serial0",12));CHECK(!base_lookups);
 setup();CHECK(!list_parse("",0));CHECK(!allocations&&!base_lookups);
 char large[300];memset(large,'a',sizeof(large));large[sizeof(large)-1]=0;setup();refused(list_parse(large,sizeof(large)));CHECK(!base_lookups);
 /* Largest complete path must fit without truncating to a different node. */
 int max=256-sizeof("/resources/devices/");large[max]=0;
 setup();int d=fdt_path_offset(base,"/resources/devices");CHECK(fdt_add_subnode(base,d,large)>=0);string(large,"device-type","platform");string(large,"device-name","long-name");CHECK(!list_parse(large,max+1));CHECK(allocations==1);
 puts("GENERIC_DEVICE_NAME_LIST_PREFLIGHT_AND_PATH_BOUND_PASS");
}
int main(int argc,char **argv){CHECK(argc==2);if(!strcmp(argv[1],"pci"))pci();else if(!strcmp(argv[1],"platform"))platform();else if(!strcmp(argv[1],"dispatch"))dispatch();else if(!strcmp(argv[1],"names"))names();else abort();return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-device-properties-') as directory:
    root = Path(directory)
    source, binary = root/'fixture.c', root/'fixture'
    source.write_text(prelude+s+checks)
    library = [a.libfdt/name for name in ('fdt.c','fdt_ro.c','fdt_rw.c','fdt_wip.c','fdt_sw.c','fdt_empty_tree.c')]
    command = ['cc','-O2','-Wall','-Wextra','-Werror','-Wno-unused-parameter','-Wno-misleading-indentation','-fsanitize=undefined','-fno-sanitize-recover=all','-I',str(a.libfdt),str(source),*(str(x) for x in library),'-o',str(binary)]
    subprocess.run(command,check=True)
    for section in ('pci','platform','dispatch','names') if a.section=='all' else (a.section,):
        subprocess.run([str(binary),section],check=True,timeout=10)
