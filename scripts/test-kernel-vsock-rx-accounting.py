#!/usr/bin/env python3
"""Compile real queue functions; compare accounting with an independent queue walk.

This userspace fixture does not qualify kernel locking, IPI delivery or live I/O.
"""
import pathlib, subprocess, tempfile, hashlib
root=pathlib.Path(__file__).resolve().parent.parent
original=root/'scripts/fixtures/mk-transport-rx/mk_transport.c'
patch=root/'patches/linux/linux-v7.0-mk2-vsock-rx-accounting.patch'
assert hashlib.sha256(original.read_bytes()).hexdigest()=='a877cbe5f1306c3068b80074750045ca4c362ea18adb1ac0493e78ece317707e'
def function(text,name):
    start=text.rfind('\nstatic ',0,text.index(name+'('))+1
    opening=text.index('{',text.index(name+'(',start));depth=1;end=opening+1
    while depth:
        depth+=(text[end]=='{')-(text[end]=='}');end+=1
    return text[start:end]+'\n'
prelude=r'''
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <assert.h>
typedef uint32_t u32;typedef uint64_t u64;typedef int64_t s64;
struct virtio_vsock_hdr {u32 len;};
struct sk_buff {struct sk_buff *next;unsigned char *data;struct {u32 offset;} cb;};
struct sk_buff_head {struct sk_buff *head,*tail;};
typedef int spinlock_t;
ACTUAL_TRANSPORT_STRUCT
struct vsock_sock {struct mk_vsock_sock *trans;};
struct msghdr {unsigned char *data;size_t position;};
static int copy_error;static unsigned long visits;static unsigned char payload_tag=0x31;
static void (*during_copy)(void);
#define le32_to_cpu(x) (x)
#define VIRTIO_VSOCK_SKB_CB(skb) (&(skb)->cb)
#define min(a,b) ((a)<(b)?(a):(b))
#define spin_lock_bh(lock) do {assert(!*(lock));*(lock)=1;}while(0)
#define spin_unlock_bh(lock) do {assert(*(lock));*(lock)=0;}while(0)
#define skb_queue_walk(q,s) for((s)=(q)->head;(s) && (++visits);(s)=(s)->next)
static struct mk_vsock_sock *mk_vsock_sk(struct vsock_sock *vsk){return vsk->trans;}
static void __skb_queue_tail(struct sk_buff_head *q,struct sk_buff *s){s->next=NULL;if(q->tail)q->tail->next=s;else q->head=s;q->tail=s;}
static void __skb_queue_head(struct sk_buff_head *q,struct sk_buff *s){s->next=q->head;q->head=s;if(!q->tail)q->tail=s;}
static struct sk_buff *__skb_dequeue(struct sk_buff_head *q){struct sk_buff *s=q->head;if(s){q->head=s->next;if(!q->head)q->tail=NULL;}return s;}
static void kfree_skb(struct sk_buff *s){free(s->data);free(s);}
static void skb_queue_purge(struct sk_buff_head *q){struct sk_buff *s;while((s=__skb_dequeue(q)))kfree_skb(s);}
static int skb_copy_datagram_msg(struct sk_buff *s,size_t offset,struct msghdr *m,size_t count){if(during_copy){void (*hook)(void)=during_copy;during_copy=NULL;hook();}if(copy_error)return -EFAULT;memcpy(m->data+m->position,s->data+offset,count);m->position+=count;return 0;}
static struct sk_buff *packet(u32 length,int full){struct sk_buff *s=calloc(1,sizeof(*s));assert(s);s->data=calloc(1,sizeof(struct virtio_vsock_hdr)+(full?length:0));assert(s->data);((struct virtio_vsock_hdr *)s->data)->len=length;if(full)memset(s->data+sizeof(struct virtio_vsock_hdr),payload_tag++,length);return s;}
static u64 reference(struct mk_vsock_sock *v){u64 sum=0;for(struct sk_buff *s=v->rx_queue.head;s;s=s->next)sum+=((struct virtio_vsock_hdr *)s->data)->len-s->cb.offset;return sum;}
'''
checks=r'''
static struct mk_vsock_sock queue;static struct vsock_sock socket_fixture={&queue};
static void check(void){u64 expected=reference(&queue);assert(queue.rx_bytes==expected);assert((u64)mk_transport_stream_has_data(&socket_fixture)==expected);assert(!queue.rx_lock);}
static void concurrent_arrival(void){assert(!queue.rx_lock);mk_transport_queue_rx(&queue,packet(13,1));check();}
int main(void){
 unsigned char data[4096];struct msghdr msg={data,0};
 assert(mk_transport_stream_dequeue(&socket_fixture,&msg,10,0)==-EAGAIN);check();
 mk_transport_queue_rx(&queue,packet(100,1));mk_transport_queue_rx(&queue,packet(200,1));check();
 assert(mk_transport_stream_dequeue(&socket_fixture,&msg,37,0)==37);check();assert(msg.position==37 && queue.fwd_cnt==37);
 for(size_t i=0;i<msg.position;i++)assert(data[i]==0x31);
 u32 forwarded=queue.fwd_cnt;copy_error=1;msg.position=0;during_copy=concurrent_arrival;
 assert(mk_transport_stream_dequeue(&socket_fixture,&msg,50,0)==-EFAULT);assert(!msg.position && queue.fwd_cnt==forwarded);check();copy_error=0;
 size_t delivered=37;
 while(reference(&queue)){msg.position=0;assert(mk_transport_stream_dequeue(&socket_fixture,&msg,29,0)>0);for(size_t i=0;i<msg.position;i++,delivered++)assert(data[i]==(delivered<100?0x31:delivered<300?0x32:0x33));check();}
 assert(delivered==313 && queue.fwd_cnt==313);
 mk_transport_queue_rx(&queue,packet(17,1));mk_transport_queue_rx(&queue,packet(19,1));check();assert(!mk_transport_cancel_pkt(&socket_fixture));check();
 puts("VSOCK_RX_PARTIAL_COPY_FAULT_ARRIVAL_AND_CANCEL_PASS");
 for(unsigned i=0;i<16384;i++)mk_transport_queue_rx(&queue,packet(64,0));
 assert(reference(&queue)==1048576);visits=0;
 for(unsigned i=0;i<64;i++)assert(mk_transport_stream_has_data(&socket_fixture)==1048576);
 assert(visits==0);assert(!mk_transport_cancel_pkt(&socket_fixture));check();
 puts("VSOCK_RX_CONSTANT_TIME_READINESS_PASS queue_packets=16384 polls=64 walked_nodes=0");
 /* Payloads are represented by their real header lengths, not allocated in
  * this accounting fixture. This exercises a >4 GiB queue with bounded RAM. */
 for(unsigned i=0;i<1048577;i++)mk_transport_queue_rx(&queue,packet(4096,0));
 assert(reference(&queue)==4294971392ULL);assert(mk_transport_stream_has_data(&socket_fixture)==4294971392LL);assert(!mk_transport_cancel_pkt(&socket_fixture));check();
 puts("VSOCK_RX_64_BIT_PENDING_BYTES_PASS");
 return 0;
}
'''
control=r'''
int main(void){struct mk_vsock_sock q={0};struct vsock_sock v={&q};for(unsigned i=0;i<16384;i++)__skb_queue_tail(&q.rx_queue,packet(64,0));visits=0;for(unsigned i=0;i<64;i++)assert(mk_transport_stream_has_data(&v)==1048576);assert(visits==1048576);printf("VSOCK_RX_LINEAR_READINESS_CONTROL_CONFIRMED queue_packets=16384 polls=64 walked_nodes=%lu\n",visits);skb_queue_purge(&q.rx_queue);return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-vsock-rx-') as directory:
    temp=pathlib.Path(directory);target=temp/'net/vmw_vsock/mk_transport.c';target.parent.mkdir(parents=True);target.write_bytes(original.read_bytes())
    subprocess.run(['git','apply','--check',str(patch)],cwd=temp,check=True)
    subprocess.run(['git','apply',str(patch)],cwd=temp,check=True)
    candidate=target.read_text();base=original.read_text()
    for name,text,body in [('control',base,control),('candidate',candidate,checks)]:
        functions=['mk_transport_stream_has_data'] if name=='control' else ['mk_transport_queue_rx','mk_transport_cancel_pkt','mk_transport_stream_dequeue','mk_transport_stream_has_data']
        start=text.index('struct mk_vsock_sock {');end=text.index('\n};',start)+3
        source=temp/(name+'.c');source.write_text(prelude.replace('ACTUAL_TRANSPORT_STRUCT',text[start:end])+'\n'.join(function(text,fn) for fn in functions)+body)
        binary=temp/name
        subprocess.run(['cc','-O2','-Wall','-Wextra','-Werror','-Wno-unused-function','-Wno-unused-parameter',str(source),'-o',str(binary)],check=True)
        subprocess.run([str(binary)],check=True,timeout=15)
