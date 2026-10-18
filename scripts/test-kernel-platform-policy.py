#!/usr/bin/env python3
"""Exercise actual platform registration/policy functions with modeled devices.

Tests ACPI identity, generic policy refusal before resource insertion, and
registration failure cleanup. No firmware enumeration or device isolation claim.
"""
from pathlib import Path
import argparse,subprocess,tempfile
p=argparse.ArgumentParser(description=__doc__);p.add_argument('platform',type=Path);p.add_argument('instance',type=Path)
p.add_argument('--section',choices=['identity','effects','all'],default='all');a=p.parse_args()
def function(text,marker):
    at=text.index(marker);return text[at:text.index('\n}',at)+2]+'\n'
s=a.platform.read_text();helpers=function(s,'static bool platform_device_mk_allowed(') if 'static bool platform_device_mk_allowed(' in s else ''
code=r'''
#include <stdbool.h>
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>
#include <stdarg.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#define CHECK(x) do{if(!(x)){fprintf(stderr,"platform policy line %d: %s\n",__LINE__,#x);exit(2);}}while(0)
#define GFP_KERNEL 0
#define PLATFORM_DEVID_NONE -1
#define PLATFORM_DEVID_AUTO -2
#define IORESOURCE_MEM 1
#define IORESOURCE_IO 2
typedef uint32_t u32;
struct list_head {struct list_head *next;};
#define entry(p,t,m) ((t*)((char*)(p)-offsetof(t,m)))
#define list_for_each_entry(p,h,m) for(p=entry((h)->next,__typeof__(*p),m);&p->m!=(h);p=entry(p->m.next,__typeof__(*p),m))
static bool list_empty(struct list_head *p){return p->next==p;}
struct mk_platform_device {struct list_head list;char name[32],hid[16];};
struct mk_instance {void *dtb_data;bool platform_devices_valid;int platform_device_count;struct list_head platform_devices;};
struct device {struct device *parent;void *bus;const char *label;struct acpi_device *companion;char generated[64];};
struct acpi_device {struct device dev;const char *name,*hid;};
struct resource {struct resource *parent;const char *name;int flags;};
struct platform_device {struct device dev;const char *name;int id;bool id_auto;unsigned num_resources;struct resource *resource;};
static struct mk_instance object,*root_instance;
static struct mk_platform_device allowed;
static struct device platform_bus;
static int platform_bus_type,platform_devid_ida;
static struct resource iomem_resource,ioport_resource;
static int manifest,inserts,releases,adds,ida_allocs,ida_frees,fail_insert,fail_add,fail_ida;
static uint64_t mk_manifest_phys(void){return manifest?4096:0;}
static void log_stub(const char *fmt,...){(void)fmt;}
#define pr_info log_stub
#define pr_debug log_stub
#define dev_err(dev,...) ((void)(dev))
#define dev_name(dev) ((dev)->label)
#ifdef CONFIG_ACPI
#define ACPI_COMPANION(dev) ((dev)->companion)
#define acpi_device_name(adev) ((adev)->name)
#define acpi_device_hid(adev) ((adev)->hid)
#endif
static int dev_set_name(struct device *d,const char *fmt,...){va_list args;va_start(args,fmt);vsnprintf(d->generated,sizeof(d->generated),fmt,args);va_end(args);d->label=d->generated;return 0;}
static int ida_alloc(int *p,int flags){(void)p;(void)flags;ida_allocs++;return fail_ida?-ENOMEM:7;}
static void ida_free(int *p,int id){(void)p;CHECK(id==7);ida_frees++;}
static int resource_type(struct resource *p){return p->flags;}
static int insert_resource(struct resource *parent,struct resource *r){inserts++;if(inserts==fail_insert)return -EBUSY;r->parent=parent;return 0;}
static int release_resource(struct resource *r){CHECK(r->parent);releases++;r->parent=NULL;return 0;}
static int device_add(struct device *d){(void)d;adds++;return fail_add?-EIO:0;}
'''+function(a.instance.read_text(),'bool mk_platform_device_allowed(')+helpers+function(s,'int platform_device_add(')+r'''
static struct platform_device pdev;
static struct acpi_device adev;
static struct resource resources[2];
static void setup(void){memset(&pdev,0,sizeof(pdev));memset(&object,0,sizeof(object));memset(&allowed,0,sizeof(allowed));memset(resources,0,sizeof(resources));root_instance=&object;object.dtb_data=&object;object.platform_devices_valid=true;object.platform_device_count=1;object.platform_devices.next=&allowed.list;allowed.list.next=&object.platform_devices;manifest=1;inserts=releases=adds=ida_allocs=ida_frees=fail_insert=fail_add=fail_ida=0;adev=(struct acpi_device){.dev={.label="ACPI0001:00"},.name="ACPI logical name",.hid="ACPI0001"};pdev.name="ACPI0001:00";pdev.id=PLATFORM_DEVID_NONE;pdev.dev.companion=&adev;pdev.num_resources=2;pdev.resource=resources;resources[0].flags=IORESOURCE_MEM;resources[1].flags=IORESOURCE_IO;}
static void granted(void){CHECK(platform_device_add(&pdev)==0&&inserts==2&&adds==1&&!releases);}
static void refused(void){CHECK(platform_device_add(&pdev)==-EPERM&&!inserts&&!adds&&!releases);}
static void identity(void){
#ifdef CONFIG_ACPI
 setup();strcpy(allowed.hid,"ACPI0001");CHECK(mk_platform_device_allowed(adev.name,adev.hid));granted();
 setup();strcpy(allowed.name,"ACPI logical name");CHECK(mk_platform_device_allowed(adev.name,adev.hid));granted();
 setup();strcpy(allowed.hid,"WRONG");strcpy(allowed.name,"ACPI logical name");granted();
 setup();strcpy(allowed.name,"WRONG");strcpy(allowed.hid,"ACPI0001");granted();
 setup();strcpy(allowed.hid,"WRONG");refused();
 setup();strcpy(allowed.hid,"ACPI0001");pdev.name="derived-child";refused();
 setup();strcpy(allowed.name,"derived-child");pdev.name="derived-child";granted();
 setup();strcpy(allowed.hid,"ACPI0001");pdev.id=1;refused();
 setup();strcpy(allowed.hid,"ACPI0001");pdev.id=PLATFORM_DEVID_AUTO;refused();CHECK(ida_allocs==1&&ida_frees==1&&pdev.id==PLATFORM_DEVID_AUTO);
 setup();strcpy(allowed.hid,"ACPI0001");adev.dev.label=NULL;refused();
#endif
 setup();pdev.dev.companion=NULL;strcpy(allowed.name,pdev.name);granted();
 setup();pdev.dev.companion=NULL;strcpy(allowed.hid,"ACPI0001");refused();
 setup();root_instance=NULL;manifest=0;granted();
 setup();root_instance=NULL;refused();
 setup();object.dtb_data=NULL;granted();
 setup();object.platform_devices_valid=false;refused();
 setup();object.platform_device_count=0;refused();
 puts("CANONICAL_ACPI_IDENTITY_AND_NO_INHERITED_HID_GRANT_PASS");}
static void effects(void){
 setup();strcpy(allowed.name,pdev.name);pdev.dev.companion=NULL;pdev.id=PLATFORM_DEVID_AUTO;fail_ida=1;CHECK(platform_device_add(&pdev)==-ENOMEM&&!inserts&&!adds&&!ida_frees);
 setup();strcpy(allowed.name,pdev.name);pdev.dev.companion=NULL;pdev.id=PLATFORM_DEVID_AUTO;fail_insert=2;CHECK(platform_device_add(&pdev)==-EBUSY&&inserts==2&&releases==1&&!adds&&ida_frees==1&&pdev.id==PLATFORM_DEVID_AUTO);
 setup();strcpy(allowed.name,pdev.name);pdev.dev.companion=NULL;pdev.id=PLATFORM_DEVID_AUTO;fail_add=1;CHECK(platform_device_add(&pdev)==-EIO&&inserts==2&&releases==2&&adds==1&&ida_frees==1&&pdev.id==PLATFORM_DEVID_AUTO);
 puts("PLATFORM_REFUSAL_BEFORE_RESOURCES_AND_FAILURE_CLEANUP_PASS");}
int main(int argc,char **argv){CHECK(argc==2);if(!strcmp(argv[1],"identity"))identity();else if(!strcmp(argv[1],"effects"))effects();else {identity();effects();}return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-platform-policy-') as tmp:
    tmp=Path(tmp);(tmp/'test.c').write_text(code)
    for acpi in [False,True]:
        command=['gcc','-std=gnu11','-O1','-Wall','-Wextra','-Werror','-DCONFIG_MULTIKERNEL=1','-fsanitize=undefined','-fno-sanitize-recover=all']
        if acpi:command+=['-DCONFIG_ACPI=1']
        subprocess.run(command+[str(tmp/'test.c'),'-o',str(tmp/'test')],check=True)
        print('ACPI model enabled='+str(acpi),flush=True)
        subprocess.run([str(tmp/'test'),a.section],check=True)
