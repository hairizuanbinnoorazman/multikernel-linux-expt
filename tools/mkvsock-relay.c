// SPDX-License-Identifier: GPL-2.0-or-later
#include <errno.h>
#include <poll.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/prctl.h>
#include <sys/un.h>
#include <unistd.h>
#include "native_args.h"
#ifndef AF_VSOCK
#define AF_VSOCK 40
#endif
#define VMADDR_CID_ANY 0xffffffffU
#define SO_VM_SOCKETS_TRANSPORT 9
#define VSOCK_TRANSPORT_MULTIKERNEL 1
struct sockaddr_vm { unsigned short family,reserved; unsigned int port,cid; unsigned char flags,zero[3]; };
static void die(const char*s){perror(s);exit(1);}static int vsock(void){int f=socket(AF_VSOCK,SOCK_STREAM,0),t=VSOCK_TRANSPORT_MULTIKERNEL;if(f<0)die("socket");if(setsockopt(f,AF_VSOCK,SO_VM_SOCKETS_TRANSPORT,&t,sizeof(t)))die("setsockopt");return f;}
static int vserver(unsigned p){int f=vsock();struct sockaddr_vm a={.family=AF_VSOCK,.port=p,.cid=VMADDR_CID_ANY};if(bind(f,(void*)&a,sizeof(a))||listen(f,1))die("vsock server");int c=accept(f,0,0);if(c<0)die("vsock accept");close(f);return c;}
/* The host can pin a child before exposing its Unix bridge. Authentication
 * still binds the agent request; CID matching is not process attestation. */
static int relay_peer_matches(const struct sockaddr_vm *peer, socklen_t size,
                              unsigned int expected_cid)
{
	return size == sizeof(*peer) && peer->family == AF_VSOCK &&
		expected_cid && expected_cid != VMADDR_CID_ANY && peer->cid == expected_cid;
}
static int vserver_peer(unsigned int port, unsigned int expected_cid)
{
	int f=vsock();
	struct sockaddr_vm address={.family=AF_VSOCK,.port=port,.cid=VMADDR_CID_ANY};
	if(bind(f,(void*)&address,sizeof(address))||listen(f,1)) die("vsock server");
	for(unsigned int refused=0;refused<64;) {
		struct sockaddr_vm peer={0};socklen_t size=sizeof(peer);
		int client=accept(f,(void*)&peer,&size);
		if(client<0) {if(errno==EINTR) continue;close(f);die("vsock accept");}
		if(relay_peer_matches(&peer,size,expected_cid)) {close(f);return client;}
		close(client);refused++;
		/* One bounded diagnostic, without guest-controlled metadata. */
		if(refused==1) fprintf(stderr,"relay peer rejected before Unix bridge publication\n");
	}
	close(f);errno=EACCES;die("relay peer refusal limit");return -1;
}
static int vclient(unsigned c,unsigned p){int f=vsock();struct sockaddr_vm a={.family=AF_VSOCK,.port=p,.cid=c};if(connect(f,(void*)&a,sizeof(a)))die("vsock connect");return f;}
static void upath(struct sockaddr_un*a,const char*p){memset(a,0,sizeof(*a));a->sun_family=AF_UNIX;if(strlen(p)>=sizeof(a->sun_path)){errno=ENAMETOOLONG;die("unix path");}strcpy(a->sun_path,p);}
/* The supervisor owns pathname validation and cleanup.  Removing here would
 * let this less-informed helper unlink a path substituted around bind/exit. */
static int userver(const char*p){int f=socket(AF_UNIX,SOCK_STREAM,0);struct sockaddr_un a;if(f<0)die("unix socket");upath(&a,p);if(bind(f,(void*)&a,sizeof(a))||listen(f,1))die("unix server");int c=accept(f,0,0);if(c<0)die("unix accept");close(f);return c;}
static int uclient(const char*p){for(int i=0;i<100;i++){int f=socket(AF_UNIX,SOCK_STREAM,0);struct sockaddr_un a;if(f<0)die("unix socket");upath(&a,p);if(!connect(f,(void*)&a,sizeof(a)))return f;close(f);usleep(100000);}die("unix connect");return-1;}
static void wr(int f,const void*v,size_t n){const char*p=v;while(n){ssize_t x=write(f,p,n);if(x<0&&errno==EINTR)continue;if(x<=0)die("relay write");p+=x;n-=x;}}
static void pump(int a,int b){struct pollfd p[2]={{a,POLLIN,0},{b,POLLIN,0}};char q[65536];int n=2;while(n){if(poll(p,2,-1)<0){if(errno==EINTR)continue;die("poll");}for(int i=0;i<2;i++)if(p[i].fd>=0&&(p[i].revents&(POLLIN|POLLHUP))){ssize_t x=read(p[i].fd,q,sizeof(q));if(x>0)wr(p[1-i].fd,q,x);else{shutdown(p[1-i].fd,SHUT_WR);p[i].fd=-1;n--;}}}}
int main(int n,char**v)
{
	/* Child workloads can also have UID 0. Protect outer-root handles and
	 * transport buffers before opening either endpoint, not just after setup. */
	if(prctl(PR_SET_DUMPABLE,0,0,0,0)<0) die("protect relay process");
	if(prctl(PR_GET_DUMPABLE,0,0,0,0)!=0) {
		errno=EPERM;die("verify relay process protection");
	}
	int a,b;
	uint64_t cid=0,port=0;
	int pinned_server=0;
	const char *path;
	if(n==4&&!strcmp(v[1],"server")) {
		path=v[3];
		if(!native_decimal(v[2],1,UINT32_MAX-1,&port)) goto invalid;
	} else if(n==5&&!strcmp(v[1],"server-peer")) {
		path=v[4];pinned_server=1;
		if(!native_decimal(v[2],1,UINT32_MAX-1,&cid)||
		   !native_decimal(v[3],1,UINT32_MAX-1,&port)) goto invalid;
	} else if(n==5&&!strcmp(v[1],"client")) {
		path=v[4];
		if(!native_decimal(v[2],0,UINT32_MAX-1,&cid)||
		   !native_decimal(v[3],1,UINT32_MAX-1,&port)) goto invalid;
	} else {
		fprintf(stderr,"usage: %s server PORT UNIX | server-peer CID PORT UNIX | client CID PORT UNIX\n",v[0]);
		return 2;
	}
	if(!*path||strlen(path)>=sizeof(((struct sockaddr_un*)0)->sun_path)) goto invalid;
	if(pinned_server) {a=vserver_peer((unsigned)port,(unsigned)cid);b=userver(path);}
	else if(n==4) {a=vserver((unsigned)port);b=userver(path);}
	else {a=vclient((unsigned)cid,(unsigned)port);b=uclient(path);}
	pump(a,b);close(a);close(b);return 0;
invalid:
	fprintf(stderr,"invalid relay arguments\n");
	return 2;
}
