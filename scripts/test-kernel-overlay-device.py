#!/usr/bin/env python3
"""Exercise extracted overlay PCI readers and device operation with modeled effects.

No PCI hardware, IPI authorization or multi-item transaction atomicity claim.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('source', type=Path)
p.add_argument('--section', choices=['pci', 'driver', 'scalars', 'all'], default='all')
a = p.parse_args()
s = a.source.read_text()

def function(name):
    at = next(s.index(prefix+name+'(') for prefix in ['static int ', 'static u32 '] if prefix+name+'(' in s)
    return s[at:s.index('\n}', at)+2]+'\n'

code = r'''
#include <stdint.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>
#include <ctype.h>
#include <errno.h>
#include <arpa/inet.h>
typedef uint16_t u16;
typedef uint8_t u8;
typedef uint32_t u32;
typedef u32 fdt32_t;
#define fdt32_to_cpu(v) ntohl(v)
static inline u32 fdt32_ld(const void *p){u32 v;memcpy(&v,p,4);return ntohl(v);}
#define PCI_DEVFN(s,f) (((s)<<3)|(f))
#define PCI_SLOT(d) ((d)>>3)
#define PCI_FUNC(d) ((d)&7)
#define FDT_ERR_NOTFOUND 1
#define sizeof_field(t,m) sizeof(((t*)0)->m)
#define CHECK(c) do{if(!(c)){fprintf(stderr,"overlay device line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
struct mk_device_resource_payload {u16 domain;u8 bus,devfn;u32 flags;char driver_override[64];int sender_instance_id;};
struct mk_overlay_tx {int id;bool validate_only;};
struct mk_instance {int id;};
#define MK_OVERLAY_TARGET_POOL 0
#define MK_OVERLAY_TARGET_INSTANCE 1
struct mk_overlay_target {int kind;struct mk_instance *instance;};
static char pci[128],driver_bytes[128];
static int pci_len,driver_len,driver_present,driver_error,mutations,route,failure;
static u16 actual_domain;static u8 actual_bus,actual_devfn;
static const char *actual_driver;
static int flags_present,flags_len,flags_error;
static fdt32_t flags_cell;
static u32 actual_flags;
static const void *fdt_getprop(const void *fdt,int node,const char *name,int *len)
{(void)fdt;(void)node;if(!strcmp(name,"pci-id")){*len=pci_len;return pci_len==-1?NULL:pci;}if(!strcmp(name,"driver")){*len=driver_present?driver_len:(driver_error?-9:-FDT_ERR_NOTFOUND);return driver_present?driver_bytes:NULL;}if(!strcmp(name,"flags")){*len=flags_present?flags_len:(flags_error?-9:-FDT_ERR_NOTFOUND);return flags_present?&flags_cell:NULL;}*len=-FDT_ERR_NOTFOUND;return NULL;}
static const char *fdt_get_name(const void *fdt,int node,int *len){(void)fdt;(void)node;(void)len;return "pci@0";}
#define fdt_for_each_subnode(n,f,o) for((void)(f),(void)(o),(n)=0;(n)<1;(n)++)
static void logger(const char *fmt,...){(void)fmt;}
#define pr_err logger
#define pr_info logger
static int mk_overlay_check_resource_target(struct mk_overlay_tx *tx,const struct mk_overlay_target *target,const char *name){(void)tx;(void)target;(void)name;return 0;}
static const char *mk_overlay_target_name(const struct mk_overlay_target *t){(void)t;return "target";}
SCALAR_READER_PLACEHOLDER
static int move(int r,u16 d,u8 b,u8 v,const char *driver){mutations++;route=r;actual_domain=d;actual_bus=b;actual_devfn=v;actual_driver=driver;return failure;}
static int mk_pool_device_add(u16 d,u8 b,u8 v){return move(1,d,b,v,NULL);}
static int mk_pool_device_remove(u16 d,u8 b,u8 v,const char *s,u32 flags){(void)flags;return move(2,d,b,v,s);}
static int mk_send_device_add(int id,u16 d,u8 b,u8 v,const char *s,u32 flags){CHECK(id==7);actual_flags=flags;return move(3,d,b,v,s);}
static int mk_send_device_remove(int id,u16 d,u8 b,u8 v){CHECK(id==7);return move(4,d,b,v,NULL);}
'''+function('mk_overlay_parse_pci_id')+(function('mk_overlay_parse_driver') if 'static int mk_overlay_parse_driver(' in s else '')+function('mk_overlay_op_device')+r'''
static void reset(void){strcpy(pci,"0000:01:1f.7");pci_len=strlen(pci)+1;driver_present=driver_error=mutations=route=failure=0;actual_driver=NULL;actual_flags=0;flags_present=flags_len=flags_error=0;flags_cell=0;memset(driver_bytes,0,sizeof(driver_bytes));}
static int operation(int kind,bool give,bool undo){struct mk_overlay_tx tx={.id=1};struct mk_instance i={7};struct mk_overlay_target t={kind,&i};return mk_overlay_op_device(&tx,NULL,0,&t,give,undo);}
static void valid(const char *s,u16 d,u8 b,u8 v){reset();strcpy(pci,s);pci_len=strlen(s)+1;CHECK(operation(1,true,false)==0&&mutations==1&&route==3&&actual_domain==d&&actual_bus==b&&actual_devfn==v);}
static void refused(void){u16 d=99;u8 b=98,v=97;CHECK(mk_overlay_parse_pci_id(NULL,0,"pci@0",&d,&b,&v)==-EINVAL&&d==99&&b==98&&v==97);for(int kind=0;kind<2;kind++)for(int give=0;give<2;give++)for(int undo=0;undo<2;undo++){mutations=0;CHECK(operation(kind,give,undo)==-EINVAL&&!mutations);}}
static void pci_cases(void){
 valid("0:0:0.0",0,0,0);valid("FFFF:FF:1F.7",65535,255,255);valid("000a:02:03.4",10,2,28);
 const char *bad[]={"", "0:0:20.0", "0:0:0.8", "10000:0:0.0", "0:100:0.0", "0:0:100.0", "0:0:0.10", "-1:0:0.0", "+0:0:0.0", " 0:0:0.0", "0x0:0:0.0", "0:0:0.0x", "0:0:0.0 ", "0:0:0.0\n", "0:0:0", "0::0.0", "g:0:0.0", "fffffffff:0:0.0"};
 for(unsigned j=0;j<sizeof(bad)/sizeof(bad[0]);j++){reset();strcpy(pci,bad[j]);pci_len=strlen(pci)+1;refused();}
 reset();pci_len=-1;refused();reset();pci_len=0;refused();reset();pci_len=12;pci[11]='7';refused();
 reset();memcpy(pci,"0:0:0.0\0tail\0",13);pci_len=13;refused();
 reset();pci[0]=(char)0xff;refused();
 puts("OVERLAY_PCI_EXACT_STRING_RANGE_AND_PRE_EFFECT_REFUSAL_PASS");}
static void driver_cases(void){
 reset();CHECK(operation(1,true,false)==0&&actual_driver==NULL&&mutations==1);
 reset();driver_present=1;driver_len=1;CHECK(operation(1,true,false)==0&&actual_driver==driver_bytes&&mutations==1);
 reset();driver_present=1;driver_len=64;memset(driver_bytes,'x',63);CHECK(operation(1,true,false)==0&&actual_driver==driver_bytes&&mutations==1);
 for(int bad=0;bad<5;bad++)for(int kind=0;kind<2;kind++)for(int give=0;give<2;give++)for(int undo=0;undo<2;undo++){
  reset();driver_present=1;driver_len=4;memcpy(driver_bytes,"abc\0",4);
  if(bad==0)driver_len=0;
  if(bad==1){driver_len=65;memset(driver_bytes,'x',64);driver_bytes[64]=0;}
  if(bad==2)driver_bytes[3]='x';
  if(bad==3){driver_bytes[1]=0;}
  if(bad==4){driver_present=0;driver_error=1;}
  CHECK(operation(kind,give,undo)==-EINVAL&&!mutations);
 }
 for(int kind=0;kind<2;kind++)for(int give=0;give<2;give++)for(int undo=0;undo<2;undo++){reset();CHECK(operation(kind,give,undo)==0&&mutations==1&&route==(kind?(give?3:4):(give?1:2)));}
 reset();failure=-EBUSY;CHECK(operation(1,true,false)==-EBUSY&&mutations==1);
 puts("OVERLAY_DRIVER_BOUND_AND_ALL_DEVICE_ROUTES_PASS");}
static void scalar_cases(void){
 reset();flags_present=1;flags_len=4;flags_cell=htonl(UINT32_MAX);CHECK(operation(1,true,false)==0&&actual_flags==UINT32_MAX&&mutations==1);
 for(int bad=0;bad<6;bad++)for(int kind=0;kind<2;kind++)for(int give=0;give<2;give++)for(int undo=0;undo<2;undo++){
  reset();flags_present=1;flags_len=bad;
  if(bad==4){flags_present=0;flags_error=1;}
  CHECK(operation(kind,give,undo)==-EINVAL&&!mutations);
 }
 puts("OVERLAY_DEVICE_MALFORMED_FLAGS_REFUSED_BEFORE_ALL_ROUTES_PASS");}
int main(int argc,char **argv){CHECK(argc==2);if(!strcmp(argv[1],"pci"))pci_cases();else if(!strcmp(argv[1],"driver"))driver_cases();else if(!strcmp(argv[1],"scalars"))scalar_cases();else{pci_cases();driver_cases();scalar_cases();}return 0;}
'''
code=code.replace('SCALAR_READER_PLACEHOLDER',function('mk_overlay_parse_u32'))
with tempfile.TemporaryDirectory(prefix='mk-overlay-device-') as tmp:
    root = Path(tmp)
    (root/'fixture.c').write_text(code)
    subprocess.run(['gcc','-std=gnu11','-O1','-Wall','-Wextra','-Werror',
                    '-fsanitize=undefined','-fno-sanitize-recover=all',
                    str(root/'fixture.c'),'-o',str(root/'fixture')],check=True)
    print('PRODUCTION_FUNCTIONS_COMPILE_PASS',flush=True)
    subprocess.run([str(root/'fixture'),a.section],check=True)
