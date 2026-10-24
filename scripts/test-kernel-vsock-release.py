#!/usr/bin/env python3
"""Compile actual release callbacks against the observed core table-ref contract.

This userspace control does not qualify kernel socket lifetime/concurrency.
"""
import pathlib,subprocess,tempfile,hashlib
root=pathlib.Path(__file__).resolve().parent.parent
fixture=root/'scripts/fixtures/mk-transport-rx/mk_transport.c'
assert hashlib.sha256(fixture.read_bytes()).hexdigest()=='a877cbe5f1306c3068b80074750045ca4c362ea18adb1ac0493e78ece317707e'
def release(text):
    start=text.index('static void mk_transport_release(');end=text.index('\nstatic int mk_transport_connect(',start)
    return text[start:end]
prelude=r'''
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <assert.h>
#include <errno.h>
typedef uint16_t u16;typedef uint32_t u32;typedef uint64_t u64;
#define GFP_KERNEL 0
#define TCP_ESTABLISHED 1
#define TCP_SYN_SENT 2
#define TCP_CLOSE 7
#define TCP_LISTEN 10
#define VIRTIO_VSOCK_OP_SHUTDOWN 4
#define VIRTIO_VSOCK_TYPE_STREAM 1
#define VIRTIO_VSOCK_SHUTDOWN_RCV 1
#define VIRTIO_VSOCK_SHUTDOWN_SEND 2
#define cpu_to_le16(x) (x)
#define cpu_to_le32(x) (x)
#define cpu_to_le64(x) (x)
struct virtio_vsock_hdr {u64 src_cid,dst_cid;u32 src_port,dst_port;u16 type,op;u32 flags,len,buf_alloc,fwd_cnt;};
struct sk_buff {unsigned char *data;};
struct vsock_sock {struct {int sk_state;} sk;struct {u32 svm_cid,svm_port;} remote_addr,local_addr;int dead,bound,connected,refs;};
static int allocation_fails,send_error,live_buffers,send_calls,remove_calls;
static u32 mk_transport_get_local_cid(void){return 0;}
static struct sk_buff *alloc_skb(size_t length,int flags){(void)flags;if(allocation_fails)return NULL;struct sk_buff *s=calloc(1,sizeof(*s));assert(s);s->data=calloc(1,length);assert(s->data);live_buffers++;return s;}
static void *skb_put(struct sk_buff *s,size_t length){(void)length;return s->data;}
static int mk_send_pkt(struct sk_buff *s){struct virtio_vsock_hdr *h=(void *)s->data;assert(h->src_cid==0 && h->dst_cid==40 && h->src_port==4061 && h->dst_port==9000);assert(h->type==VIRTIO_VSOCK_TYPE_STREAM && h->op==VIRTIO_VSOCK_OP_SHUTDOWN && h->flags==3 && !h->len);send_calls++;return send_error;}
static void kfree_skb(struct sk_buff *s){free(s->data);free(s);live_buffers--;}
/* Match the inspected core's rule: SOCK_DEAD permits bound-table removal;
 * transport reassignment keeps the binding. Connected-table refs are removed. */
static void vsock_remove_sock(struct vsock_sock *v){remove_calls++;if(v->dead && v->bound){v->bound=0;v->refs--;}if(v->connected){v->connected=0;v->refs--;}}
static struct vsock_sock socket_fixture(int state,int bound,int connected,int dead){struct vsock_sock v={0};v.sk.sk_state=state;v.remote_addr.svm_cid=40;v.remote_addr.svm_port=9000;v.local_addr.svm_port=4061;v.bound=bound;v.connected=connected;v.dead=dead;v.refs=1+bound+connected;return v;}
'''
checks=r'''
int main(void){
 if(CONTROL){struct vsock_sock v=socket_fixture(TCP_ESTABLISHED,0,1,1);mk_transport_release(&v);v.refs--;assert(v.connected && v.refs==1 && !remove_calls && !live_buffers);puts("VSOCK_ORIGINAL_RELEASE_RETAINS_CONNECTED_TABLE_REFERENCE_CONFIRMED");return 0;}
 int states[]={TCP_ESTABLISHED,TCP_SYN_SENT,TCP_CLOSE,TCP_LISTEN};
 for(unsigned state=0;state<4;state++)for(int bound=0;bound<2;bound++)for(int connected=0;connected<2;connected++)for(int fail=0;fail<2;fail++)for(int error=0;error<2;error++){
  allocation_fails=fail;send_error=error?-EIO:0;send_calls=remove_calls=0;
  struct vsock_sock v=socket_fixture(states[state],bound,connected,1);mk_transport_release(&v);
  assert(!v.bound && !v.connected && v.refs==1 && remove_calls==1 && !live_buffers);
  assert(send_calls==(states[state]==TCP_ESTABLISHED && !allocation_fails));v.refs--;assert(!v.refs);
 }
 puts("VSOCK_RELEASE_UNLINKS_ALL_CLOSE_STATES_DESPITE_ALLOC_OR_SEND_FAILURE_PASS");
 allocation_fails=send_error=send_calls=remove_calls=0;
 struct vsock_sock v=socket_fixture(TCP_CLOSE,1,1,0);mk_transport_release(&v);
 assert(v.bound && !v.connected && v.refs==2 && remove_calls==1);
 v.dead=1;mk_transport_release(&v);assert(!v.bound && v.refs==1 && remove_calls==2);v.refs--;assert(!v.refs);
 puts("VSOCK_RELEASE_PRESERVES_REASSIGNMENT_BINDING_AND_DROPS_FINAL_REFS_PASS");return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='vsock-release-') as directory:
    work=pathlib.Path(directory);path=work/'net/vmw_vsock/mk_transport.c';path.parent.mkdir(parents=True);path.write_bytes(fixture.read_bytes())
    for patch in ['linux-v7.0-mk2-vsock-rx-accounting.patch','linux-v7.0-mk2-vsock-release-sockets.patch']:
        subprocess.run(['git','apply','--check',str(root/'patches/linux'/patch)],cwd=work,check=True)
        subprocess.run(['git','apply',str(root/'patches/linux'/patch)],cwd=work,check=True)
    for name,text,control in [('original',fixture.read_text(),1),('candidate',path.read_text(),0)]:
        source=work/(name+'.c');source.write_text(prelude+'\n#define CONTROL '+str(control)+'\n'+release(text)+checks)
        binary=work/name;subprocess.run(['cc','-O2','-Wall','-Wextra','-Werror','-Wno-unused-function',str(source),'-o',str(binary)],check=True)
        subprocess.run([str(binary)],check=True,timeout=5)
