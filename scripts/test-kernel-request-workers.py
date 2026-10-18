#!/usr/bin/env python3
"""Exercise actual request decoders/deferred workers with modeled backends.

Checks copied correlation IDs, strict layouts, version refusal and shutdown ACK
propagation. This does not model IPI source authentication or hardware outcomes.
"""
import argparse
from pathlib import Path
import re
import subprocess
import tempfile
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('source',type=Path);p.add_argument('--header',type=Path,required=True);p.add_argument('--core',type=Path,required=True)
a=p.parse_args();s=a.source.read_text();h=a.header.read_text();core=a.core.read_text()
def block(text,prefix):
 start=text.index(prefix);return text[start:text.index('\n}',start)+2]+'\n'
code=r'''
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <limits.h>
#include <setjmp.h>
typedef unsigned long long mk_phys_cpu_t;typedef unsigned long long u64;typedef uint32_t u32;typedef uint16_t u16;typedef uint8_t u8;
#define CHECK(c) do{if(!(c)){fprintf(stderr,"request workers line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
#define container_of(p,t,m) ((t *)((char *)(p)-offsetof(t,m)))
#define GFP_ATOMIC 0
#define PCI_SLOT(x) ((x)>>3)
#define PCI_FUNC(x) ((x)&7)
#define __noreturn __attribute__((noreturn))
static void logger(const char *fmt,...){(void)fmt;}
#define pr_err logger
#define pr_warn logger
#define pr_info logger
struct work_struct {void (*function)(struct work_struct *);};
#define INIT_WORK(w,f) ((w)->function=(f))
static struct work_struct *queued;
static int allocations,effects,sends,completes,backend_result,fail_alloc;
static void *kmalloc(size_t size,int flags){(void)flags;if(fail_alloc)return NULL;void *p=malloc(size);CHECK(p);memset(p,0xa5,size);allocations++;return p;}
static void kfree(void *p){CHECK(p&&allocations);allocations--;free(p);}
static void schedule_work(struct work_struct *w){CHECK(!queued);queued=w;}
static void run_work(void){struct work_struct *w=queued;CHECK(w);queued=NULL;w->function(w);}
static void strscpy(char *out,const char *in,size_t n){size_t len=strnlen(in,n);if(len==n)len=n-1;memcpy(out,in,len);out[len]=0;}
static int mk_do_cpu_add(u64 cpu,u32 node,u32 flags){CHECK(cpu==8&&node==2&&flags==16);effects++;return backend_result;}
static int mk_do_cpu_remove(u64 cpu){CHECK(cpu==8);effects++;return backend_result;}
static int mk_do_mem_add(u64 pfn,u64 pages,u32 node,u32 type){CHECK(pfn==0x1234567887654321ULL&&pages==32&&node==2&&type==1);effects++;return backend_result;}
static int mk_do_mem_remove(u64 pfn,u64 pages){CHECK(pfn==0x1234567887654321ULL&&pages==32);effects++;return backend_result;}
static int mk_do_device_add(u16 domain,u8 bus,u8 devfn,const char *driver,u32 flags){CHECK(domain==2&&bus==3&&devfn==4&&!strcmp(driver,"vfio-pci")&&flags==16);effects++;return backend_result;}
static int mk_do_device_remove(u16 domain,u8 bus,u8 devfn){CHECK(domain==2&&bus==3&&devfn==4);effects++;return backend_result;}
'''
for line in h.splitlines():
 if re.match(r'#define MK_(MSG_|RES_|SYS_|SHUTDOWN_)',line):code+=line+'\n'
for name in ['mk_request_id','mk_cpu_resource_payload','mk_mem_resource_payload','mk_device_resource_payload','mk_resource_ack','mk_shutdown_payload']:
 code+=block(h,'struct '+name+' {')+';\n'
code+=block(h,'static inline bool mk_request_id_valid(')
code+=r'''
static struct mk_resource_ack sent;
static int sent_to;static u32 sent_type,sent_subtype,completed_type,completed_op;static u64 completed_resource;static struct mk_request_id completed_request;
static int mk_send_message(int id,u32 type,u32 subtype,void *data,u32 len)
{CHECK(data&&len==sizeof(sent));memcpy(&sent,data,len);sent_to=id;sent_type=type;sent_subtype=subtype;sends++;return 0;}
static void mk_msg_pending_complete(u32 type,u32 op,u64 resource,const struct mk_request_id *request,int result)
{CHECK(request);completed_type=type;completed_op=op;completed_resource=resource;completed_request=*request;(void)result;completes++;}
static jmp_buf park_return;
struct mk_instance {int id;};static struct mk_instance root={.id=7},*root_instance=&root;
static void __noreturn mk_enter_pool_state(void *ignored){(void)ignored;longjmp(park_return,1);}
static void smp_call_function(void (*f)(void *),void *data,int wait){(void)f;(void)data;CHECK(!wait);}
'''
for name in ['mk_cpu_hotplug_work','mk_mem_hotplug_work','mk_device_hotplug_work']:code+=block(s,'struct '+name+' {')+';\n'
for kind in ['cpu','mem','device']:
 for operation in ['add','remove']:
  code+=block(s,'static void mk_'+kind+'_'+operation+'_work_fn(')
  prefix='static int ' if kind!='cpu' else 'int '
  code+=block(s,prefix+'mk_handle_'+kind+'_'+operation+'(')
code+=block(s,'static void mk_resource_msg_handler(')
for name in ['mk_shutdown_work','mk_halted_work']:code+=block(core,'struct '+name+' {')+';\n'
code+=block(core,'static void __noreturn mk_notify_down_and_park(')
code+=block(core,'void __noreturn mk_halt_to_pool(')
code+=block(core,'static void mk_shutdown_work_fn(')
code+='static void mk_halted_work_fn(struct work_struct *work){kfree(container_of(work,struct mk_halted_work,work));}\n'
code+=block(core,'static void mk_system_msg_handler(')
code+=r'''
static void clean(void){CHECK(!queued&&!allocations);effects=sends=completes=0;}
static void resource(u32 op,void *payload,size_t len,struct mk_request_id *id,u64 resource)
{
 struct mk_request_id expected=*id;clean();backend_result=-EIO;
 mk_resource_msg_handler(MK_MSG_RESOURCE,op,payload,len,NULL);CHECK(queued&&allocations==1&&!effects&&!sends);
 *id=(struct mk_request_id){0,0};run_work();CHECK(!queued&&!allocations&&effects==1&&sends==1);
 CHECK(sent.request_id.epoch==expected.epoch&&sent.request_id.sequence==expected.sequence);
 CHECK(sent_to==11&&sent_type==MK_MSG_RESOURCE&&sent_subtype==MK_RES_ACK&&sent.operation==op&&sent.resource_id==resource&&(int)sent.result==-EIO);
 *id=expected;
 for(size_t n=0;n<=len+1;n++)if(n!=len){clean();mk_resource_msg_handler(MK_MSG_RESOURCE,op,payload,n,NULL);CHECK(!effects&&!queued&&!allocations);}
 clean();mk_resource_msg_handler(MK_MSG_RESOURCE,op,NULL,len,NULL);CHECK(!queued&&!effects);
 id->epoch=0;mk_resource_msg_handler(MK_MSG_RESOURCE,op,payload,len,NULL);CHECK(!queued&&!effects);*id=expected;
 id->sequence=0;mk_resource_msg_handler(MK_MSG_RESOURCE,op,payload,len,NULL);CHECK(!queued&&!effects);*id=expected;
 fail_alloc=1;mk_resource_msg_handler(MK_MSG_RESOURCE,op,payload,len,NULL);fail_alloc=0;CHECK(!queued&&!effects&&!allocations);
 clean();
}
int main(void)
{
 struct mk_cpu_resource_payload cpu={.request_id={9,17},.cpu_id=8,.numa_node=2,.flags=16,.sender_instance_id=11};
 struct mk_mem_resource_payload mem={.request_id={9,18},.start_pfn=0x1234567887654321ULL,.nr_pages=32,.numa_node=2,.mem_type=1,.sender_instance_id=11};
 struct mk_device_resource_payload dev={.request_id={9,19},.domain=2,.bus=3,.devfn=4,.flags=16,.driver_override="vfio-pci",.sender_instance_id=11};
 resource(MK_RES_CPU_ADD,&cpu,sizeof(cpu),&cpu.request_id,8);resource(MK_RES_CPU_REMOVE,&cpu,sizeof(cpu),&cpu.request_id,8);
 resource(MK_RES_MEM_ADD,&mem,sizeof(mem),&mem.request_id,mem.start_pfn);resource(MK_RES_MEM_REMOVE,&mem,sizeof(mem),&mem.request_id,mem.start_pfn);
 resource(MK_RES_DEVICE_ADD,&dev,sizeof(dev),&dev.request_id,0x20304);resource(MK_RES_DEVICE_REMOVE,&dev,sizeof(dev),&dev.request_id,0x20304);
 puts("REQUEST_SIX_ACTUAL_DEFERRED_WORKERS_COPY_AND_ECHO_ID_PASS");
 struct mk_resource_ack ack={.request_id={9,17},.operation=MK_RES_CPU_ADD,.resource_id=8};
 mk_resource_msg_handler(MK_MSG_RESOURCE,MK_RES_ACK,&ack,sizeof(ack),NULL);
 CHECK(completes==1&&completed_type==MK_MSG_RESOURCE&&completed_op==MK_RES_CPU_ADD&&completed_resource==8&&completed_request.epoch==9&&completed_request.sequence==17);
 for(size_t n=0;n<=sizeof(ack)+1;n++)if(n!=sizeof(ack)){clean();mk_resource_msg_handler(MK_MSG_RESOURCE,MK_RES_ACK,&ack,n,NULL);CHECK(!completes);}
 for(u32 op=1;op<=7;op++){clean();mk_resource_msg_handler(MK_MSG_RESOURCE,MK_MSG_RESOURCE+op,&cpu,sizeof(cpu),NULL);CHECK(!effects&&!queued&&!completes);}
 clean();mk_resource_msg_handler(MK_MSG_RESOURCE,MK_MSG_RESOURCE+0x100,&ack,sizeof(ack),NULL);CHECK(!completes);
 for(u32 op=2;op<=4;op++){clean();mk_system_msg_handler(MK_MSG_SYSTEM,MK_MSG_SYSTEM+op,&cpu,sizeof(cpu),NULL);CHECK(!queued&&!completes);}
 puts("REQUEST_LEGACY_WIRE_SUBTYPES_REFUSED_BEFORE_EFFECTS_PASS");
 struct mk_shutdown_payload shutdown={.request_id={13,29},.flags=MK_SHUTDOWN_GRACEFUL,.sender_instance_id=11};
 clean();mk_system_msg_handler(MK_MSG_SYSTEM,MK_SYS_SHUTDOWN,&shutdown,sizeof(shutdown),NULL);CHECK(queued&&allocations==1);
 shutdown.request_id=(struct mk_request_id){0,0};if(!setjmp(park_return))run_work();CHECK(!queued&&!allocations&&sends==1);
 CHECK(sent_to==11&&sent_type==MK_MSG_SYSTEM&&sent_subtype==MK_SYS_SHUTDOWN_ACK&&sent.request_id.epoch==13&&sent.request_id.sequence==29&&sent.operation==MK_SYS_SHUTDOWN&&sent.resource_id==7);
 clean();if(!setjmp(park_return))mk_halt_to_pool();CHECK(sends==1&&sent_subtype==MK_SYS_HALTED&&!sent.request_id.epoch&&!sent.request_id.sequence);
 shutdown.request_id=(struct mk_request_id){13,29};
 for(size_t n=0;n<=sizeof(shutdown)+1;n++)if(n!=sizeof(shutdown)){clean();mk_system_msg_handler(MK_MSG_SYSTEM,MK_SYS_SHUTDOWN,&shutdown,n,NULL);CHECK(!queued&&!allocations);}
 clean();shutdown.request_id.sequence=0;mk_system_msg_handler(MK_MSG_SYSTEM,MK_SYS_SHUTDOWN,&shutdown,sizeof(shutdown),NULL);CHECK(!queued);
 ack=(struct mk_resource_ack){.request_id={13,29},.operation=MK_SYS_SHUTDOWN,.resource_id=7};
 mk_system_msg_handler(MK_MSG_SYSTEM,MK_SYS_SHUTDOWN_ACK,&ack,sizeof(ack),NULL);CHECK(completes==1&&completed_type==MK_MSG_SYSTEM&&completed_op==MK_SYS_SHUTDOWN&&completed_resource==7&&completed_request.epoch==13&&completed_request.sequence==29);
 clean();ack.operation++;mk_system_msg_handler(MK_MSG_SYSTEM,MK_SYS_SHUTDOWN_ACK,&ack,sizeof(ack),NULL);CHECK(!completes);
 ack.operation=MK_SYS_SHUTDOWN;ack.resource_id=0x100000007ULL;mk_system_msg_handler(MK_MSG_SYSTEM,MK_SYS_SHUTDOWN_ACK,&ack,sizeof(ack),NULL);CHECK(!completes);
 mk_system_msg_handler(MK_MSG_SYSTEM,MK_SYS_HALTED,&ack,sizeof(ack),NULL);CHECK(!queued&&!allocations);
 clean();puts("REQUEST_SHUTDOWN_DEFERRED_COPY_ACK_AND_STRICT_LAYOUT_PASS");return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-request-workers-') as td:
 source=Path(td)/'test.c';exe=Path(td)/'test';source.write_text(code)
 subprocess.run(['gcc','-std=gnu11','-Wall','-Wextra','-Werror','-Wno-unused-parameter','-fsanitize=undefined','-fno-sanitize-recover=all',str(source),'-o',str(exe)],check=True)
 print('REQUEST_WORKERS_ACTUAL_FUNCTIONS_COMPILED',flush=True)
 subprocess.run([str(exe)],check=True)
