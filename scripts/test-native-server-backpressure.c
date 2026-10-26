#define _GNU_SOURCE
#include <sys/socket.h>
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <string.h>
#include <unistd.h>
#include <sys/wait.h>
static ssize_t fault_send(int fd,const void *data,size_t size,int flags);
#define send fault_send
#define main native_helper_main
#ifndef SERVER_BACKPRESSURE_SOURCE
#define SERVER_BACKPRESSURE_SOURCE "../tools/mkvsock-nbd.c"
#endif
#include SERVER_BACKPRESSURE_SOURCE
#undef main
#undef send

#define CHECK(x) do{if(!(x)){fprintf(stderr,"server queue control line=%d errno=%d\n",__LINE__,errno);exit(2);}}while(0)
static int selected=-1, permanent, calls, injected, accepted, partial=3;
static ssize_t fault_send(int fd,const void *data,size_t size,int flags)
{
 if(fd==selected){
  calls++;
  if(permanent || injected<3 || (accepted>0 && accepted%7==0)){
   /* Only one recurrent refusal at each accepted-byte boundary. */
   if(permanent || injected<3 || injected<4){injected++;errno=ENOSPC;return -1;}
  }
  if(size>(size_t)partial)size=(size_t)partial;
 }
 ssize_t result=send(fd,data,size,flags);
 if(fd==selected && result>0)accepted+=(int)result;
 return result;
}
static void setup(int pair[2])
{
 CHECK(socketpair(AF_UNIX,SOCK_STREAM,0,pair)==0);selected=pair[0];calls=injected=accepted=permanent=0;
}
int main(void)
{
 int pair[2];setup(pair);
 struct nbd_request_wire request={.magic=htonl(NBD_REQUEST_MAGIC)};
 memset(request.handle,0x5a,sizeof(request.handle));
 struct timespec deadline=frame_deadline(1000);
 CHECK(send_reply(pair[0],&request,0,&deadline)==1);
 struct nbd_reply_wire reply;CHECK(read_exact_or_eof(pair[1],&reply,sizeof(reply))==1);
 CHECK(reply.magic==htonl(NBD_REPLY_MAGIC) && reply.error==0 && !memcmp(reply.handle,request.handle,sizeof(reply.handle)));
 CHECK(injected>=3 && calls<32);close(pair[0]);close(pair[1]);
 puts("NATIVE_SERVER_MK_REPLY_RETRY_PARTIAL_BYTES_PASS");
 setup(pair);permanent=1;deadline=frame_deadline(40);
 CHECK(send_reply(pair[0],&request,0,&deadline)==-1 && errno==ETIMEDOUT);
 CHECK(calls>1 && calls<16 && accepted==0);close(pair[0]);close(pair[1]);
 puts("NATIVE_SERVER_MK_QUEUE_PERMANENT_ABSOLUTE_DEADLINE_PASS");
 setup(pair);permanent=1;deadline=frame_deadline(1000);char byte='s';
 CHECK(transfer_until(pair[0],&byte,1,1,&deadline)==-1 && errno==ENOSPC);
 CHECK(calls==1);close(pair[0]);close(pair[1]);
 puts("NATIVE_SERVER_UNMARKED_ENOSPC_FATAL_PASS");
 /* Positive progress cannot renew the original absolute frame deadline. */
 setup(pair);partial=1;char payload[4096];memset(payload,0x6a,sizeof(payload));
 deadline=frame_deadline(40);
 CHECK(transfer_until_kind(pair[0],payload,sizeof(payload),1,&deadline,1)==-1 && errno==ETIMEDOUT);
 CHECK(accepted>0 && accepted<(int)sizeof(payload));close(pair[0]);close(pair[1]);
 puts("NATIVE_SERVER_PROGRESS_CANNOT_RENEW_FRAME_DEADLINE_PASS");

 return 0;
}
