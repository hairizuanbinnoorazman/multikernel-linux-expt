// SPDX-License-Identifier: GPL-2.0-or-later
#define _GNU_SOURCE
#include <sys/socket.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <limits.h>
static ssize_t fault_send(int fd,const void *data,size_t length,int flags);
static unsigned int full_events, resumed_events;
static void trace_event(const char *stage, int endpoint, int detail)
{
 if (!strcmp(stage,"mk-queue-full")) {full_events++;if(endpoint!=1||detail!=ENOSPC)exit(3);}
 if (!strcmp(stage,"mk-queue-resumed")) {resumed_events++;if(endpoint!=1||detail<=0||detail>65536)exit(3);}
 fprintf(stderr,"PROXY_TEST_EVENT stage=%s endpoint=%d detail=%d\n",stage,endpoint,detail);
}
#define NATIVE_PROXY_TRACE(stage,endpoint,detail) trace_event(stage,endpoint,detail)
#define send fault_send
#define NATIVE_PROXY_MK_ENDPOINT 1
#define NATIVE_PROXY_QUEUE_TIMEOUT_MS 20
#include "../tools/native_stream_proxy.h"
#undef send
static int endpoint;static unsigned int injected, completed_sends, last_recurrent=UINT_MAX;static const char *mode;
static ssize_t fault_send(int fd,const void *data,size_t length,int flags)
{
 if(fd==endpoint && (!strcmp(mode,"permanent") || !strcmp(mode,"left") || injected<3 ||
    (!strcmp(mode,"recurrent") && completed_sends && completed_sends%7==0 && last_recurrent!=completed_sends))) {
  if(!strcmp(mode,"recurrent"))last_recurrent=completed_sends;
  injected++;errno=ENOSPC;return -1;
 }
 ssize_t result=send(fd,data,length,flags);
 if(fd==endpoint && result>0)completed_sends++;
 return result;
}
int main(int argc,char **argv)
{
 if(argc!=4)return 2;
 mode=getenv("MK_PROXY_TEST_MODE");if(!mode)mode="transient";
 int left=atoi(argv[2]),right=atoi(argv[3]);endpoint=!strcmp(mode,"left")?left:right;
 int result=native_stream_proxy(left,right),error=errno;
 fprintf(stderr,"PROXY_BACKPRESSURE_RESULT result=%d errno=%d injected=%u\n",result,error,injected);
 if(!strcmp(mode,"transient") && (!injected || result || full_events!=1 || resumed_events!=1))return 2;
 if(!strcmp(mode,"recurrent") && (injected<=3 || result || full_events!=1 || resumed_events!=1))return 2;
 if(!strcmp(mode,"permanent") && (error!=ETIMEDOUT || injected>8 || full_events!=1 || resumed_events))return 2;
 if(!strcmp(mode,"left") && (error!=ENOSPC || injected!=1 || full_events || resumed_events))return 2;
 return result<0;
}
