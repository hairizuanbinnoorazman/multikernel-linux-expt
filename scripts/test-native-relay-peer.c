#define _GNU_SOURCE
#include <assert.h>
#include <errno.h>
#include <poll.h>
#include <setjmp.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <unistd.h>

static int fixture_socket(int,int,int);
static int fixture_setsockopt(int,int,int,const void*,socklen_t);
static int fixture_bind(int,const struct sockaddr*,socklen_t);
static int fixture_listen(int,int);
static int fixture_accept(int,struct sockaddr*,socklen_t*);
static int fixture_close(int);
static int fixture_poll(struct pollfd*,nfds_t,int);
static ssize_t fixture_read(int,void*,size_t);
static ssize_t fixture_write(int,const void*,size_t);
static int fixture_shutdown(int,int);
static int fixture_prctl(int,...);
static _Noreturn void fixture_exit(int);
#define socket fixture_socket
#define setsockopt fixture_setsockopt
#define bind fixture_bind
#define listen fixture_listen
#define accept fixture_accept
#define close fixture_close
#define poll fixture_poll
#define read fixture_read
#define write fixture_write
#define shutdown fixture_shutdown
#define prctl fixture_prctl
#define exit fixture_exit
#define main relay_program_main
#include "../tools/mkvsock-relay.c"
#undef main
#undef exit
#undef prctl
#undef shutdown
#undef write
#undef read
#undef poll
#undef close
#undef accept
#undef listen
#undef bind
#undef setsockopt
#undef socket

static jmp_buf ended;
static int mode,accept_calls,refused_closed,unix_opened,forwarded,listener_closed;
static int accepted_fd,approved,sent,interrupted,exit_status;
static int fixture_socket(int domain,int type,int protocol)
{
	assert(type==SOCK_STREAM && protocol==0);
	if(domain==AF_VSOCK) return 10;
	assert(domain==AF_UNIX && approved);unix_opened++;return 11;
}
static int fixture_setsockopt(int fd,int level,int option,const void *value,socklen_t size)
{
	assert(fd==10 && level==AF_VSOCK && option==SO_VM_SOCKETS_TRANSPORT);
	assert(size==sizeof(int) && *(const int*)value==VSOCK_TRANSPORT_MULTIKERNEL);return 0;
}
static int fixture_bind(int fd,const struct sockaddr *address,socklen_t size)
{
	if(fd==10) {const struct sockaddr_vm *v=(const void*)address;assert(size==sizeof(*v) && v->family==AF_VSOCK && v->port==7200 && v->cid==VMADDR_CID_ANY);}
	else {sa_family_t family;memcpy(&family,address,sizeof(family));assert(fd==11 && family==AF_UNIX);}
	return 0;
}
static int fixture_listen(int fd,int count){assert((fd==10||fd==11)&&count==1);return 0;}
static int fixture_accept(int fd,struct sockaddr *address,socklen_t *size)
{
	if(fd==11){assert(approved && !address && !size);return 200;}
	assert(fd==10 && address && size && *size==sizeof(struct sockaddr_vm));
	if(mode==2 && !interrupted){interrupted=1;errno=EINTR;return -1;}
	struct sockaddr_vm peer={.family=AF_VSOCK,.cid=40};socklen_t actual=sizeof(peer);
	int index=accept_calls++;
	if(mode==1) peer.cid=41;
	else if(mode==0) {
		if(index==0)peer.cid=41;
		if(index==1)peer.family=AF_UNIX;
		if(index==2)actual--;
		if(index==3)actual++;
		if(index==4)peer.cid=VMADDR_CID_ANY;
	}
	memcpy(address,&peer,sizeof(peer));*size=actual;
	if(peer.cid==40 && peer.family==AF_VSOCK && actual==sizeof(peer)){approved=1;accepted_fd=100+index;}
	return 100+index;
}
static int fixture_close(int fd)
{
	if(fd==10)listener_closed++;
	else if(fd>=100 && fd<164 && fd!=accepted_fd)refused_closed++;
	else assert(fd==11 || fd==200 || fd==accepted_fd);
	return 0;
}
static int fixture_poll(struct pollfd *p,nfds_t count,int timeout)
{
	assert(approved && count==2 && timeout==-1);
	for(nfds_t i=0;i<count;i++)p[i].revents=p[i].fd<0?0:POLLIN;
	return 2;
}
static ssize_t fixture_read(int fd,void *buffer,size_t size)
{
	assert(approved && size>0 && (fd==accepted_fd||fd==200));
	if(fd==accepted_fd && !sent){sent=1;*(char*)buffer='x';return 1;}
	return 0;
}
static ssize_t fixture_write(int fd,const void *buffer,size_t size)
{
	assert(approved && fd==200 && size==1 && *(const char*)buffer=='x');forwarded++;return 1;
}
static int fixture_shutdown(int fd,int how){assert(how==SHUT_WR);if(fd<0){errno=EBADF;return -1;}assert(fd==accepted_fd||fd==200);return 0;}
static int fixture_prctl(int option,...){assert(option==PR_SET_DUMPABLE||option==PR_GET_DUMPABLE);return 0;}
static _Noreturn void fixture_exit(int status){exit_status=status;longjmp(ended,1);}
static void reset(int value)
{
	mode=value;accept_calls=refused_closed=unix_opened=forwarded=listener_closed=0;
	accepted_fd=-1;approved=sent=interrupted=exit_status=0;
}
int main(void)
{
	char *args[]={"relay","server-peer","40","7200","/unused-peer-fixture.sock",NULL};
	for(volatile int scenario=0;scenario<3;scenario++) {
		reset(scenario);
		if(!setjmp(ended)) {
			int result=relay_program_main(5,args);assert(result==0 && scenario!=1);
		} else assert(scenario==1 && exit_status==1);
		if(scenario==1) {
			assert(accept_calls==64 && refused_closed==64 && listener_closed==1);
			assert(!unix_opened && !forwarded && !approved);
		} else {
			assert(approved && unix_opened==1 && forwarded==1 && listener_closed==1);
			assert(accept_calls==(scenario==0?6:1));assert(refused_closed==(scenario==0?5:0));
			if(scenario==2)assert(interrupted);
		}
	}
	puts("RELAY_PINNED_CID_FAMILY_SIZE_REJECTIONS_PRECEDE_UNIX_BRIDGE_AND_FORWARDING_PASS");
	puts("RELAY_64_PEER_REFUSAL_CAP_CLOSES_ALL_FDS_WITHOUT_UNIX_PUBLICATION_PASS");
	puts("RELAY_EINTR_RETRY_AND_MATCHED_PEER_FORWARDING_PASS");return 0;
}
