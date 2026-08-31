// SPDX-License-Identifier: GPL-2.0-or-later
#include <errno.h>
#include <poll.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#ifndef AF_VSOCK
#define AF_VSOCK 40
#endif
#define VMADDR_CID_ANY 0xffffffffU
#define SO_VM_SOCKETS_TRANSPORT 9
#define VSOCK_TRANSPORT_MULTIKERNEL 1
struct sockaddr_vm { unsigned short family,reserved; unsigned int port,cid; unsigned char flags,zero[3]; };
static void die(const char*s){perror(s);exit(1);}static int vsock(void){int f=socket(AF_VSOCK,SOCK_STREAM,0),t=VSOCK_TRANSPORT_MULTIKERNEL;if(f<0)die("socket");if(setsockopt(f,AF_VSOCK,SO_VM_SOCKETS_TRANSPORT,&t,sizeof(t)))die("setsockopt");return f;}
static int vserver(unsigned p){int f=vsock();struct sockaddr_vm a={.family=AF_VSOCK,.port=p,.cid=VMADDR_CID_ANY};if(bind(f,(void*)&a,sizeof(a))||listen(f,1))die("vsock server");int c=accept(f,0,0);if(c<0)die("vsock accept");close(f);return c;}
static int vclient(unsigned c,unsigned p){int f=vsock();struct sockaddr_vm a={.family=AF_VSOCK,.port=p,.cid=c};if(connect(f,(void*)&a,sizeof(a)))die("vsock connect");return f;}
static void upath(struct sockaddr_un*a,const char*p){a->sun_family=AF_UNIX;if(strlen(p)>=sizeof(a->sun_path)){errno=ENAMETOOLONG;die("unix path");}strcpy(a->sun_path,p);}
static int userver(const char*p){int f=socket(AF_UNIX,SOCK_STREAM,0);struct sockaddr_un a;if(f<0)die("unix socket");upath(&a,p);unlink(p);if(bind(f,(void*)&a,sizeof(a))||listen(f,1))die("unix server");int c=accept(f,0,0);if(c<0)die("unix accept");close(f);unlink(p);return c;}
static int uclient(const char*p){for(int i=0;i<100;i++){int f=socket(AF_UNIX,SOCK_STREAM,0);struct sockaddr_un a;if(f<0)die("unix socket");upath(&a,p);if(!connect(f,(void*)&a,sizeof(a)))return f;close(f);usleep(100000);}die("unix connect");return-1;}
static void wr(int f,const void*v,size_t n){const char*p=v;while(n){ssize_t x=write(f,p,n);if(x<0&&errno==EINTR)continue;if(x<=0)die("relay write");p+=x;n-=x;}}
static void pump(int a,int b){struct pollfd p[2]={{a,POLLIN,0},{b,POLLIN,0}};char q[65536];int n=2;while(n){if(poll(p,2,-1)<0){if(errno==EINTR)continue;die("poll");}for(int i=0;i<2;i++)if(p[i].fd>=0&&(p[i].revents&(POLLIN|POLLHUP))){ssize_t x=read(p[i].fd,q,sizeof(q));if(x>0)wr(p[1-i].fd,q,x);else{shutdown(p[1-i].fd,SHUT_WR);p[i].fd=-1;n--;}}}}
int main(int n,char**v){int a,b;if(n==4&&!strcmp(v[1],"server")){a=vserver(strtoul(v[2],0,10));b=userver(v[3]);}else if(n==5&&!strcmp(v[1],"client")){a=vclient(strtoul(v[2],0,10),strtoul(v[3],0,10));b=uclient(v[4]);}else{fprintf(stderr,"usage: %s server PORT UNIX | client CID PORT UNIX\n",v[0]);return 2;}pump(a,b);close(a);close(b);return 0;}
