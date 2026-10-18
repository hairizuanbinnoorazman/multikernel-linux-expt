#!/usr/bin/env python3
"""Exercise extracted PCI probe policy with modeled BDF and config-read effects.

Models failed configuration reads that still write output bytes. No hardware
access, PCI passthrough or physical device containment is claimed.
"""
from pathlib import Path
import argparse,subprocess,tempfile
p=argparse.ArgumentParser(description=__doc__);p.add_argument('source',type=Path)
p.add_argument('--section',choices=['domain','bridge','all'],default='all');a=p.parse_args()
s=a.source.read_text();at=s.index('bool mk_pci_should_probe(');actual=s[at:s.index('\n}',at)+2]
code=r'''
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <stddef.h>
#define CHECK(x) do{if(!(x)){fprintf(stderr,"PCI guard line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
typedef uint8_t u8;typedef uint16_t u16;
struct list_head {struct list_head *next;};
struct mk_pci_device {struct list_head list;u16 domain;u8 bus,slot,func;};
struct mk_instance {void *dtb_data;bool pci_devices_valid;unsigned pci_device_count;struct list_head pci_devices;};
struct pci_bus {unsigned char number;int domain;};
#define entry(p,t,m) ((t*)((char*)(p)-offsetof(t,m)))
#define list_for_each_entry(p,h,m) for(p=entry((h)->next,__typeof__(*p),m);&p->m!=(h);p=entry(p->m.next,__typeof__(*p),m))
#define PCI_SLOT(n) (((n)>>3)&31)
#define PCI_FUNC(n) ((n)&7)
#define PCI_HEADER_TYPE 0
#define PCI_HEADER_TYPE_MASK 0x7f
#define PCI_HEADER_TYPE_BRIDGE 1
#define PCI_SECONDARY_BUS 1
#define PCI_SUBORDINATE_BUS 2
static struct mk_instance object,*root_instance;
static struct mk_pci_device allowed;
static struct pci_bus bus;
static int manifest,reads,failed_field;
static unsigned char values[3];
static int pci_domain_nr(struct pci_bus *p){return p->domain;}
static uint64_t mk_manifest_phys(void){return manifest?4096:0;}
static int pci_bus_read_config_byte(struct pci_bus *p,int devfn,int field,u8 *out){(void)p;(void)devfn;CHECK(field>=0&&field<3);reads++;*out=values[field];return field==failed_field?0x86:0;}
'''+actual+r'''
static void setup(void){root_instance=&object;object.dtb_data=&object;object.pci_devices_valid=true;object.pci_device_count=1;object.pci_devices.next=&allowed.list;allowed.list.next=&object.pci_devices;allowed.domain=0;allowed.bus=3;allowed.slot=2;allowed.func=0;bus.number=0;bus.domain=0;manifest=1;reads=0;failed_field=-1;values[0]=PCI_HEADER_TYPE_BRIDGE;values[1]=2;values[2]=4;}
static void domains(void){
 const int invalid[]={-1,65536,65537,131072,2147483647};
 for(unsigned i=0;i<sizeof(invalid)/sizeof(invalid[0]);i++){setup();bus.domain=invalid[i];allowed.domain=(u16)invalid[i];CHECK(!mk_pci_should_probe(&bus,0));CHECK(!reads);}
 setup();bus.domain=65535;allowed.domain=65535;CHECK(mk_pci_should_probe(&bus,0)&&reads==3);
 setup();bus.number=3;CHECK(mk_pci_should_probe(&bus,2<<3)&&!reads);
 setup();bus.domain=1;CHECK(!mk_pci_should_probe(&bus,0)&&!reads);
 puts("PCI_FULL_DOMAIN_NO_NARROWING_ALIAS_PASS");}
static void bridges(void){
 for(int field=0;field<3;field++){setup();failed_field=field;CHECK(!mk_pci_should_probe(&bus,0));CHECK(reads==field+1);}
 setup();failed_field=PCI_SUBORDINATE_BUS;values[2]=255;CHECK(!mk_pci_should_probe(&bus,0));CHECK(reads==3);
 setup();allowed.bus=255;failed_field=PCI_SECONDARY_BUS;values[1]=values[2]=255;CHECK(!mk_pci_should_probe(&bus,0));CHECK(reads==2);
 setup();CHECK(mk_pci_should_probe(&bus,0)&&reads==3);
 setup();values[0]|=0x80;CHECK(mk_pci_should_probe(&bus,0));
 setup();values[0]=0;CHECK(!mk_pci_should_probe(&bus,0)&&reads==1);
 setup();values[1]=0;CHECK(!mk_pci_should_probe(&bus,0));
 setup();values[1]=5;values[2]=4;CHECK(!mk_pci_should_probe(&bus,0));
 setup();values[1]=values[2]=4;CHECK(!mk_pci_should_probe(&bus,0));
 setup();root_instance=NULL;CHECK(!mk_pci_should_probe(&bus,0)&&!reads);manifest=0;CHECK(mk_pci_should_probe(&bus,0)&&!reads);
 setup();object.dtb_data=NULL;CHECK(mk_pci_should_probe(&bus,0)&&!reads);
 setup();object.pci_devices_valid=false;CHECK(!mk_pci_should_probe(&bus,0)&&!reads);
 setup();object.pci_device_count=0;CHECK(!mk_pci_should_probe(&bus,0)&&!reads);
 puts("PCI_BRIDGE_READ_ERRORS_REFUSED_VALID_ANCESTORS_AND_HOST_COMPATIBILITY_PASS");}
int main(int argc,char **argv){CHECK(argc==2);if(!__builtin_strcmp(argv[1],"domain"))domains();else if(!__builtin_strcmp(argv[1],"bridge"))bridges();else {domains();bridges();}return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-pci-probe-') as tmp:
    tmp=Path(tmp);(tmp/'test.c').write_text(code)
    subprocess.run(['gcc','-std=gnu11','-O1','-Wall','-Wextra','-Werror','-fsanitize=undefined','-fno-sanitize-recover=all',str(tmp/'test.c'),'-o',str(tmp/'test')],check=True)
    subprocess.run([str(tmp/'test'),a.section],check=True)
