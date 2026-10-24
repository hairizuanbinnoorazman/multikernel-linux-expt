// SPDX-License-Identifier: GPL-2.0-or-later
#define _GNU_SOURCE
#include <sys/socket.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
static ssize_t fault_send(int fd,const void *data,size_t length,int flags);
#define send fault_send
#define NATIVE_PROXY_MK_ENDPOINT 1
#define NATIVE_PROXY_QUEUE_TIMEOUT_MS 20
#include "../tools/native_stream_proxy.h"
#undef send
static int endpoint;static unsigned int injected;static const char *mode;
static ssize_t fault_send(int fd,const void *data,size_t length,int flags)
{
 if(fd==endpoint && (!strcmp(mode,"permanent") || !strcmp(mode,"left") || injected<3)) {
  injected++;errno=ENOSPC;return -1;
 }
 return send(fd,data,length,flags);
}
int main(int argc,char **argv)
{
 if(argc!=4)return 2;
 mode=getenv("MK_PROXY_TEST_MODE");if(!mode)mode="transient";
 int left=atoi(argv[2]),right=atoi(argv[3]);endpoint=!strcmp(mode,"left")?left:right;
 int result=native_stream_proxy(left,right),error=errno;
 fprintf(stderr,"PROXY_BACKPRESSURE_RESULT result=%d errno=%d injected=%u\n",result,error,injected);
 if(!strcmp(mode,"transient") && (!injected || result))return 2;
 if(!strcmp(mode,"permanent") && (error!=ETIMEDOUT || injected>8))return 2;
 if(!strcmp(mode,"left") && (error!=ENOSPC || injected!=1))return 2;
 return result<0;
}
