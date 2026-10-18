#!/usr/bin/env python3
"""Run actual pending-request C with modeled allocation, locks and completion.

Covers request reuse, duplicate completion, timeout cleanup, saturation and
concurrent registration; not IPI authentication, physical effects or live proof.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('source', type=Path)
p.add_argument('--header', type=Path, required=True)
p.add_argument('--section', choices=['all', 'correlation', 'once'], default='all')
p.add_argument('--old-control', action='store_true')
a = p.parse_args()
s = a.source.read_text()
h = a.header.read_text()


def block(text, prefix):
    start = text.index(prefix)
    return text[start:text.index('\n}', start) + 2] + '\n'


code = r'''
#include <assert.h>
#include <errno.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>
#include <stdatomic.h>
#include <limits.h>
typedef uint32_t u32;typedef unsigned long long u64;
#define U64_MAX ULLONG_MAX
#define GFP_KERNEL 0
#define CHECK(c) do{if(!(c)){fprintf(stderr,"request correlation line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
#define ERR_PTR(e) ((void *)(intptr_t)(e))
#define PTR_ERR(p) ((long)(intptr_t)(p))
#define IS_ERR(p) ((uintptr_t)(p)>=(uintptr_t)-4095)
struct list_head {struct list_head *next,*prev;};
#define LIST_HEAD(n) struct list_head n={&n,&n}
#define container_of(p,t,m) ((t *)((char *)(p)-offsetof(t,m)))
#define list_entry(p,t,m) container_of(p,t,m)
#define list_for_each_entry(p,h,m) for(p=list_entry((h)->next,__typeof__(*p),m);&p->m!=(h);p=list_entry(p->m.next,__typeof__(*p),m))
static void list_add(struct list_head *n,struct list_head *h){n->next=h->next;n->prev=h;h->next->prev=n;h->next=n;}
static void list_del(struct list_head *n){n->prev->next=n->next;n->next->prev=n->prev;}
#define DEFINE_SPINLOCK(n) pthread_mutex_t n=PTHREAD_MUTEX_INITIALIZER
#define spin_lock_irqsave(l,f) do{f=0;CHECK(!pthread_mutex_lock(l));}while(0)
#define spin_unlock_irqrestore(l,f) do{(void)f;CHECK(!pthread_mutex_unlock(l));}while(0)
static _Thread_local bool fail_alloc;
static atomic_int live_alloc;
static void *kzalloc(size_t n,int flags){(void)flags;if(fail_alloc)return NULL;void *p=calloc(1,n);CHECK(p);atomic_fetch_add(&live_alloc,1);return p;}
static void kfree(void *p){CHECK(p&&!IS_ERR(p));atomic_fetch_sub(&live_alloc,1);free(p);}
static u64 get_random_u64(void){return 0x1a2b3c4d55667788ULL;}
static void logger(const char *format,...){(void)format;}
#define pr_err logger
struct completion {atomic_int count;};
static void init_completion(struct completion *d){atomic_init(&d->count,0);}
static void complete(struct completion *d){atomic_fetch_add(&d->count,1);}
#define msecs_to_jiffies(x) (x)
static void (*wait_hook)(void);
static bool force_timeout;
static unsigned long wait_for_completion_timeout(struct completion *d,unsigned long timeout)
{(void)timeout;if(wait_hook){void (*hook)(void)=wait_hook;wait_hook=NULL;hook();}if(force_timeout)return 0;return atomic_load(&d->count)?1:0;}
'''
code += block(h, 'struct mk_request_id {') + ';\n'
code += block(h, 'static inline bool mk_request_id_valid(')
code += block(s, 'struct mk_pending_msg {') + ';\n'
code += 'static LIST_HEAD(mk_pending_msgs);\nstatic DEFINE_SPINLOCK(mk_pending_msgs_lock);\n'
code += 'static u64 mk_request_epoch;\nstatic u64 mk_request_sequence;\n'
for name, ret in [('mk_msg_pending_add', 'struct mk_pending_msg *'),
                  ('mk_msg_pending_complete', 'void '),
                  ('mk_msg_pending_wait', 'int ')]:
    f = block(s, ret + name + '(')
    if a.old_control and name != 'mk_msg_pending_wait':
        f = f.replace(name, 'old_' + name)
    code += f
if a.old_control:
    code += r'''
static struct mk_pending_msg *mk_msg_pending_add(u32 type,u32 op,u64 resource,struct mk_request_id *request)
{request->epoch=mk_request_epoch?mk_request_epoch:1;request->sequence=++mk_request_sequence;return old_mk_msg_pending_add(type,op,resource);}
static void mk_msg_pending_complete(u32 type,u32 op,u64 resource,const struct mk_request_id *request,int result)
{(void)request;old_mk_msg_pending_complete(type,op,resource,result);}
'''
code += r'''
static void idle(void){CHECK(!atomic_load(&live_alloc)&&mk_pending_msgs.next==&mk_pending_msgs&&mk_pending_msgs.prev==&mk_pending_msgs);}
static void correlation(void)
{
 struct mk_request_id old,fresh,other;
 struct mk_pending_msg *p=mk_msg_pending_add(2,3,0x1234567887654321ULL,&old);CHECK(p&&!IS_ERR(p));
 CHECK(mk_msg_pending_wait(p,0)==-ETIMEDOUT);idle();
 p=mk_msg_pending_add(2,3,0x1234567887654321ULL,&fresh);CHECK(p&&!IS_ERR(p));
 CHECK(old.sequence!=fresh.sequence);
 mk_msg_pending_complete(2,3,0x1234567887654321ULL,&old,-EIO);
 CHECK(mk_msg_pending_wait(p,0)==-ETIMEDOUT);idle();
 struct mk_request_id a,b;
 p=mk_msg_pending_add(2,3,8,&a);struct mk_pending_msg *q=mk_msg_pending_add(2,3,8,&b);
 mk_msg_pending_complete(2,3,8,&a,11);
 CHECK(mk_msg_pending_wait(p,0)==11);
 mk_msg_pending_complete(2,3,8,&b,12);CHECK(mk_msg_pending_wait(q,0)==12);idle();
 for(int wrong=0;wrong<7;wrong++){
  p=mk_msg_pending_add(2,3,8,&fresh);other=fresh;
  u32 type=2,op=3;u64 resource=8;
  if(wrong==0)other.epoch++;if(wrong==1)other.sequence++;
  if(wrong==2)type++;if(wrong==3)op++;if(wrong==4)resource++;
  if(wrong==5)other.epoch=0;if(wrong==6)other.sequence=0;
  mk_msg_pending_complete(type,op,resource,&other,7);
  CHECK(mk_msg_pending_wait(p,0)==-ETIMEDOUT);idle();
 }
 p=mk_msg_pending_add(4,6,9,&fresh);mk_msg_pending_complete(4,6,9,NULL,0);CHECK(mk_msg_pending_wait(p,0)==-ETIMEDOUT);idle();
 puts("REQUEST_OLD_OUT_OF_ORDER_WRONG_TUPLE_AND_ZERO_ID_REFUSED_PASS");
}
static void once(void)
{
 struct mk_request_id id;struct mk_pending_msg *p=mk_msg_pending_add(2,3,8,&id);
 mk_msg_pending_complete(2,3,8,&id,-EIO);mk_msg_pending_complete(2,3,8,&id,0);
 CHECK(mk_msg_pending_wait(p,0)==-EIO);idle();
 p=mk_msg_pending_add(2,3,8,&id);mk_msg_pending_complete(2,3,8,&id,0);mk_msg_pending_complete(2,3,8,&id,-EPERM);
 CHECK(mk_msg_pending_wait(p,0)==0);idle();
 mk_msg_pending_complete(2,3,8,&id,0);idle();
 puts("REQUEST_FIRST_COMPLETION_WINS_AND_POST_FREE_ACK_IGNORED_PASS");
}
static struct mk_request_id late_id;
static void late_ack(void){mk_msg_pending_complete(2,3,8,&late_id,0);}
static void limits(void)
{
 struct mk_request_id id={1,2};struct mk_pending_msg *p;
 CHECK(PTR_ERR(mk_msg_pending_add(2,3,8,NULL))==-EINVAL);idle();
 fail_alloc=true;p=mk_msg_pending_add(2,3,8,&id);fail_alloc=false;
 CHECK(IS_ERR(p)&&PTR_ERR(p)==-ENOMEM&&!id.epoch&&!id.sequence);idle();
 p=mk_msg_pending_add(2,3,8,&late_id);force_timeout=true;wait_hook=late_ack;
 CHECK(mk_msg_pending_wait(p,0)==-ETIMEDOUT);force_timeout=false;idle();
 p=mk_msg_pending_add(2,3,8,&id);mk_msg_pending_complete(2,3,8,&late_id,0);CHECK(mk_msg_pending_wait(p,0)==-ETIMEDOUT);idle();
 u64 saved=mk_request_sequence;mk_request_sequence=U64_MAX-1;
 p=mk_msg_pending_add(2,3,8,&id);CHECK(!IS_ERR(p)&&id.sequence==U64_MAX);CHECK(mk_msg_pending_wait(p,0)==-ETIMEDOUT);idle();
 for(int n=0;n<3;n++){id=(struct mk_request_id){1,2};p=mk_msg_pending_add(2,3,8,&id);CHECK(IS_ERR(p)&&PTR_ERR(p)==-EOVERFLOW&&!id.epoch&&!id.sequence);idle();}
 mk_request_sequence=saved;
 puts("REQUEST_TIMEOUT_RACE_ALLOCATION_AND_NONWRAPPING_LIFETIME_PASS");
}
#define THREADS 8
#define ITERATIONS 1000
static u64 sequences[THREADS*ITERATIONS];
static void *worker(void *arg)
{size_t n=(uintptr_t)arg;for(size_t i=0;i<ITERATIONS;i++){struct mk_request_id id;struct mk_pending_msg *p=mk_msg_pending_add(2,3,8,&id);CHECK(!IS_ERR(p)&&id.epoch);sequences[n*ITERATIONS+i]=id.sequence;mk_msg_pending_complete(2,3,8,&id,(int)i);CHECK(mk_msg_pending_wait(p,0)==(int)i);}return NULL;}
static int compare(const void *a,const void *b){u64 x=*(const u64 *)a,y=*(const u64 *)b;return (x>y)-(x<y);}
static void concurrent(void)
{pthread_t threads[THREADS];for(uintptr_t n=0;n<THREADS;n++)CHECK(!pthread_create(&threads[n],NULL,worker,(void *)n));for(int n=0;n<THREADS;n++)CHECK(!pthread_join(threads[n],NULL));qsort(sequences,THREADS*ITERATIONS,sizeof(u64),compare);for(int n=1;n<THREADS*ITERATIONS;n++)CHECK(sequences[n]>sequences[n-1]);idle();puts("REQUEST_8000_CONCURRENT_SAME_RESOURCE_REGISTRATIONS_PASS");}
int main(int argc,char **argv)
{CHECK(argc==2);if(!strcmp(argv[1],"correlation"))correlation();else if(!strcmp(argv[1],"once"))once();else{correlation();once();limits();concurrent();}return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-request-correlation-') as td:
    source = Path(td) / 'test.c'
    exe = Path(td) / 'test'
    source.write_text(code)
    subprocess.run(['gcc', '-std=gnu11', '-Wall', '-Wextra', '-Werror',
                    '-Wno-unused-function', '-Wno-misleading-indentation',
                    '-pthread', '-fsanitize=undefined', '-fno-sanitize-recover=all',
                    str(source), '-o', str(exe)], check=True)
    print('REQUEST_CORRELATION_ACTUAL_FUNCTIONS_COMPILED', flush=True)
    subprocess.run([str(exe), a.section], check=True)
