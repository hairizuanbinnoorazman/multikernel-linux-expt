#define _GNU_SOURCE
#include <sys/socket.h>
#include <sys/resource.h>
#include <arpa/inet.h>
#include <fcntl.h>
#include <signal.h>
#include <sys/wait.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <unistd.h>

static int port_pipe;
#ifdef MK_SERVER_QUEUE_BACKPRESSURE
static int queue_selected = -1;
static ssize_t fixture_queue_send(int fd, const void *data, size_t size, int flags);
#define send fixture_queue_send
#endif
static ssize_t fixture_pwrite(int fd,const void *data,size_t size,off_t offset);
#define pwrite fixture_pwrite
static int fixture_accept(int fd,struct sockaddr *addr,socklen_t *size);
static int fixture_socket(int family,int type,int protocol);
static int fixture_bind(int fd,const struct sockaddr *addr,socklen_t size);
static int fixture_setsockopt(int fd,int level,int option,const void *value,socklen_t size);
#define accept fixture_accept
#define socket fixture_socket
#define bind fixture_bind
#define setsockopt fixture_setsockopt
#define main native_helper_main
#include "../tools/mkvsock-nbd.c"
#undef main
#undef accept
#undef socket
#undef bind
#undef setsockopt
#undef pwrite
#ifdef MK_SERVER_QUEUE_BACKPRESSURE
#undef send
static ssize_t fixture_queue_send(int fd, const void *data, size_t size, int flags)
{
 static unsigned int attempts;
 if (fd == queue_selected) {
  attempts++;
  if (attempts <= 3 || attempts % 11 == 0) { errno = ENOSPC; return -1; }
  if (size > 37) size = 37;
 }
 return send(fd, data, size, flags);
}
#endif

static ssize_t fixture_pwrite(int fd,const void *data,size_t size,off_t offset)
{
	/* Exercise real serving and its bounded timing path, preserving a stale
	 * errno on successful writes as POSIX permits. */
	if (poll(NULL,0,300)!=0) return -1;
	errno=EUCLEAN;
	return pwrite(fd,data,size,offset);
}

#define CHECK(value) do { if (!(value)) { fprintf(stderr,"pressure fixture line%d errno%d\n",__LINE__,errno);exit(2); } } while(0)
static int fixture_accept(int fd,struct sockaddr *addr,socklen_t *size)
{
 struct sockaddr_in tcp;socklen_t tcp_size=sizeof(tcp);
 int client=accept(fd,(struct sockaddr *)&tcp,&tcp_size);
#ifdef MK_SERVER_QUEUE_BACKPRESSURE
 static unsigned int accepted;
 if (client >= 0 && accepted++ == 256) queue_selected = client;
#endif
 if(client>=0 && addr && size) {
  struct sockaddr_vm peer={.svm_family=AF_VSOCK,.svm_cid=1,.svm_port=50000};
  socklen_t copy=*size<sizeof(peer) ? *size : sizeof(peer);
  memcpy(addr,&peer,copy);*size=sizeof(peer);
 }
 return client;
}
static int fixture_socket(int family,int type,int protocol)
{
	return socket(family==AF_VSOCK ? AF_INET : family,type,protocol);
}
static int fixture_bind(int fd,const struct sockaddr *addr,socklen_t size)
{
	if (addr->sa_family!=AF_VSOCK) return bind(fd,addr,size);
	struct sockaddr_in tcp={.sin_family=AF_INET,.sin_addr.s_addr=htonl(INADDR_LOOPBACK)};
	int rc=bind(fd,(struct sockaddr *)&tcp,sizeof(tcp));
	if(rc<0)return rc;
	socklen_t length=sizeof(tcp);
	CHECK(getsockname(fd,(struct sockaddr *)&tcp,&length)==0);
	CHECK(write(port_pipe,&tcp.sin_port,sizeof(tcp.sin_port))==sizeof(tcp.sin_port));
	return 0;
}
static int fixture_setsockopt(int fd,int level,int option,const void *value,socklen_t size)
{
	return level==AF_VSOCK ? 0 : setsockopt(fd,level,option,value,size);
}
static int connect_fixture(uint16_t port)
{
	struct sockaddr_in addr={.sin_family=AF_INET,.sin_port=port,.sin_addr.s_addr=htonl(INADDR_LOOPBACK)};
	int fd=socket(AF_INET,SOCK_STREAM,0);CHECK(fd>=0);
	for(int attempt=0;attempt<100;attempt++) {
		if(connect(fd,(struct sockaddr *)&addr,sizeof(addr))==0)return fd;
		CHECK(errno==ECONNREFUSED);usleep(1000);
	}
	CHECK(0);return -1;
}
static void exchange_request(int fd,uint32_t command,uint32_t length)
{
	struct nbd_request_wire request={.magic=htonl(NBD_REQUEST_MAGIC),.type=htonl(command),.len=htonl(length)};
	request.handle[0]=(uint8_t)command;
	xwrite(fd,&request,sizeof(request));
	uint32_t kind=command & NBD_CMD_MASK_COMMAND;
	char data[512];memset(data,0x5a,sizeof(data));
	if(kind==NBD_CMD_WRITE)xwrite(fd,data,length);
	if(kind==NBD_CMD_DISC)return;
	struct nbd_reply_wire reply;CHECK(read_exact_or_eof(fd,&reply,sizeof(reply))==1);
	CHECK(ntohl(reply.magic)==NBD_REPLY_MAGIC && !reply.error && reply.handle[0]==request.handle[0]);
	if(kind==NBD_CMD_READ) {
		char result[512];CHECK(read_exact_or_eof(fd,result,length)==1);
		CHECK(!memcmp(result,data,length));
	}
}
int main(int argc,char **argv)
{
	CHECK(argc==3);
	int image=open(argv[1],O_RDWR|O_CREAT|O_EXCL,0600);CHECK(image>=0 && ftruncate(image,4096)==0);close(image);
	int log=open(argv[2],O_RDWR|O_CREAT|O_EXCL,0600);CHECK(log>=0);
	int ports[2];CHECK(pipe(ports)==0);
	pid_t child=fork();CHECK(child>=0);
	if(!child) {
		close(ports[0]);port_pipe=ports[1];
		CHECK(dup2(log,1)==1 && dup2(log,2)==2);
		run_server(argv[1],1,"pressure-image","pressure-generation",1);exit(0);
	}
	close(ports[1]);uint16_t port;CHECK(read(ports[0],&port,sizeof(port))==sizeof(port));close(ports[0]);
	for(int i=0;i<256;i++) {
		int fd=connect_fixture(port);struct hello_wire hello,response;
		fill_hello(&hello,4096,"wrong-image","pressure-generation");
		xwrite(fd,&hello,sizeof(hello));CHECK(read_exact_or_eof(fd,&response,sizeof(response))==1);
		CHECK(ntohl(response.status)==EACCES && !response.size && !response.image_id[0] && !response.generation[0]);close(fd);
	}
	int fd=connect_fixture(port);struct hello_wire hello,response;
	fill_hello(&hello,4096,"pressure-image","pressure-generation");xwrite(fd,&hello,sizeof(hello));
	CHECK(read_exact_or_eof(fd,&response,sizeof(response))==1 && hello_matches(&response,4096,"pressure-image","pressure-generation"));
	for(int i=0;i<17;i++)exchange_request(fd,NBD_CMD_WRITE|NBD_CMD_FLAG_FUA,512);
	exchange_request(fd,NBD_CMD_READ,512);exchange_request(fd,NBD_CMD_FLUSH,0);exchange_request(fd,NBD_CMD_DISC,0);close(fd);
	int status;CHECK(waitpid(child,&status,0)==child && WIFEXITED(status) && WEXITSTATUS(status)==0);
	char output[8192];ssize_t length=pread(log,output,sizeof(output)-1,0);CHECK(length>0 && length<4096);output[length]=0;
#ifdef MK_SERVER_QUEUE_BACKPRESSURE
 CHECK(strstr(output,"stage=server-mk-queue-full endpoint=1 detail=28\n"));
 CHECK(strstr(output,"stage=server-mk-queue-resumed endpoint=1 detail=37\n"));
 unsigned int queue_events=0;
 for(char *p=output;(p=strstr(p,"stage=server-mk-queue-"));p++)queue_events++;
 CHECK(queue_events==2);
 puts("NATIVE_SERVER_REAL_NBD_REPLIES_AND_READ_PAYLOAD_QUEUE_RETRY_PASS");
#endif
	int details=0;for(char *p=output;(p=strstr(p,"MKNBD_SERVER_REFUSED reason="));p++)details++;
	CHECK(details==16 && strstr(output,"MKNBD_SERVER_REFUSALS_SUPPRESSED") && strstr(output,"MKNBD_SERVER_REFUSAL_SUMMARY total=256 details=16 suppressed=240\n"));
	CHECK(strstr(output,"MKNBD_SERVER_READY") && strstr(output,"MKNBD_SERVER_CLIENT_ACCEPTED\n"));
	const char *terminal="MKNBD_SERVER_CLOSED synced=1 reads=1 read_bytes=512 writes=17 write_bytes=8704 flushes=1\n";
	CHECK((size_t)length>=strlen(terminal) && !strcmp(output+length-strlen(terminal),terminal));
	int timed=0;for(char *p=output;(p=strstr(p,"MKNBD_SERVER_IO phase=image-write duration_us="));p++) {
		long long duration;long result;int error;
		CHECK(sscanf(p,"MKNBD_SERVER_IO phase=image-write duration_us=%lld result=%ld errno=%d",&duration,&result,&error)==3);
		CHECK(duration>=250000 && result==512 && error==0);timed++;
	}
	CHECK(timed==16 && strstr(output,"MKNBD_SERVER_IO_SUPPRESSED count=1\n"));
	const char *summary=strstr(output,"MKNBD_SERVER_IO_SUMMARY phase=image-write ");
	unsigned long long calls,errors,slow,maximum;
	CHECK(summary && sscanf(summary,"MKNBD_SERVER_IO_SUMMARY phase=image-write calls=%llu errors=%llu slow=%llu max_us=%llu",&calls,&errors,&slow,&maximum)==4);
	CHECK(calls==17 && errors==0 && slow==17 && maximum>=250000);
	CHECK(strstr(output,"MKNBD_SERVER_IO_SUMMARY phase=image-sync calls=19 errors=0 "));
	puts("SERVER_PHASE_SUMMARY_COVERS_SUPPRESSED_IO_PASS");
	puts("SERVER_SLOW_IO_TIMING_BOUNDED_AND_ERRNO_PRESERVED_PASS");
	fputs(output,stdout);printf("SERVER_REFUSAL_PRESSURE_EXPORT_SURVIVAL_PASS refusals=256 log_bytes=%zd\n",length);
	close(log);CHECK(unlink(argv[1])==0 && unlink(argv[2])==0);
	/* Limits fixture runs separately so the supervisor retains its limits. */
	fflush(stdout);child=fork();CHECK(child>=0);
	if(!child) {
		CHECK(native_server_set_limits()==0);struct rlimit limit;
		CHECK(getrlimit(RLIMIT_CORE,&limit)==0 && !limit.rlim_cur && !limit.rlim_max);
		CHECK(getrlimit(RLIMIT_AS,&limit)==0 && limit.rlim_cur<=NATIVE_SERVER_ADDRESS_LIMIT && limit.rlim_max<=NATIVE_SERVER_ADDRESS_LIMIT);
		CHECK(getrlimit(RLIMIT_NOFILE,&limit)==0 && limit.rlim_cur<=NATIVE_SERVER_DESCRIPTOR_LIMIT && limit.rlim_max<=NATIVE_SERVER_DESCRIPTOR_LIMIT);
		void *mapping=mmap(NULL,NATIVE_SERVER_ADDRESS_LIMIT+4096,PROT_READ|PROT_WRITE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);CHECK(mapping==MAP_FAILED && errno==ENOMEM);
		int fds[32],count=0;while(count<32 && (fds[count]=open("/dev/null",O_RDONLY))>=0)count++;
		CHECK(count<16 && errno==EMFILE);while(count)close(fds[--count]);
		puts("SERVER_ADDRESS_DESCRIPTOR_CORE_LIMITS_PASS");exit(0);
	}
	CHECK(waitpid(child,&status,0)==child && WIFEXITED(status) && WEXITSTATUS(status)==0);return 0;
}
