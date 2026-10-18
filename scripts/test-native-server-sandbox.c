#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include "../tools/native_privileges.h"
#include "../tools/native_server_sandbox.h"

#define CHECK(value) do { if (!(value)) { dprintf(2, "sandbox fixture failed line%d errno%d\n", __LINE__, errno); _exit(2); } } while (0)
#define DENIED(expression) do { errno=0; CHECK((expression)==-1 && errno==EPERM); } while (0)

int main(int argc, char **argv)
{
	CHECK(argc==2);
	int image=open(argv[1],O_RDWR|O_CREAT|O_EXCL,0600);
	CHECK(image>=0 && ftruncate(image,4096)==0);
	int canary=open("/etc/passwd",O_RDONLY);
	CHECK(canary>=0);
	int listener=socket(AF_INET,SOCK_STREAM,0);
	CHECK(listener>=4);
	struct sockaddr_in addr={.sin_family=AF_INET,.sin_addr.s_addr=htonl(INADDR_LOOPBACK)};
	CHECK(bind(listener,(struct sockaddr *)&addr,sizeof(addr))==0 && listen(listener,1)==0);
	socklen_t size=sizeof(addr);CHECK(getsockname(listener,(struct sockaddr *)&addr,&size)==0);
	pid_t parent=getpid(),child=fork();CHECK(child>=0);
	if (!child) {
		CHECK(native_server_close_descriptors(image,listener)==0);
		CHECK(native_drop_request_privileges()==0);
		CHECK(native_server_install_filter(listener)==0);
		char buffer[16];errno=0;CHECK(read(canary,buffer,1)==-1 && errno==EBADF);
		DENIED(open("/etc/passwd",O_RDONLY));
#if defined(__x86_64__)
		DENIED(syscall(0x40000000U | SYS_read,3,buffer,1));
#endif
		DENIED(open(argv[1],O_RDWR));
		DENIED(unlink(argv[1]));
		DENIED(socket(AF_UNIX,SOCK_STREAM,0));
		DENIED(syscall(SYS_connect,listener,&addr,sizeof(addr)));
		DENIED(kill(parent,0));
		DENIED(syscall(SYS_ptrace,0,0,0,0));
		DENIED(syscall(SYS_mount,"none","/","tmpfs",0,0));
		DENIED(syscall(SYS_clone,0,0,0,0,0));
		DENIED(syscall(SYS_execve,"/bin/true",(char *[]){"true",NULL},(char *[]){NULL}));
		DENIED(syscall(SYS_pwrite64,1,"x",1,0));
		DENIED(syscall(SYS_pwrite64,(1ULL<<32)|3,"x",1,0));
		DENIED(syscall(SYS_accept4,3,NULL,NULL,0));
		DENIED(syscall(SYS_setsockopt,listener,SOL_SOCKET,SO_REUSEADDR,&size,sizeof(size)));
		DENIED(syscall(SYS_setsockopt,listener,(1ULL<<32)|SOL_SOCKET,SO_RCVTIMEO,&size,sizeof(size)));
		DENIED(syscall(SYS_mmap,NULL,4096,PROT_READ|PROT_EXEC,MAP_PRIVATE|MAP_ANONYMOUS,-1,0));
		DENIED(syscall(SYS_mmap,NULL,4096,(1ULL<<32)|PROT_READ,MAP_PRIVATE|MAP_ANONYMOUS,-1,0));
		DENIED(syscall(SYS_mmap,NULL,4096,PROT_READ,MAP_PRIVATE,3,0));
		void *memory=mmap(NULL,4096,PROT_READ|PROT_WRITE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);
		CHECK(memory!=MAP_FAILED);DENIED(mprotect(memory,4096,PROT_READ|PROT_EXEC));CHECK(munmap(memory,4096)==0);
		CHECK(pwrite(3,"sandbox-image",13,0)==13 && fdatasync(3)==0);
		CHECK(pread(3,buffer,13,0)==13 && !memcmp(buffer,"sandbox-image",13));
		int sock=accept4(listener,NULL,NULL,SOCK_CLOEXEC);CHECK(sock>=0);
		struct timeval timeout={.tv_sec=2};
		CHECK(setsockopt(sock,SOL_SOCKET,SO_RCVTIMEO,&timeout,sizeof(timeout))==0);
		CHECK(setsockopt(sock,SOL_SOCKET,SO_SNDTIMEO,&timeout,sizeof(timeout))==0);
		CHECK(recv(sock,buffer,4,0)==4 && !memcmp(buffer,"ping",4));
		CHECK(send(sock,"pong",4,MSG_NOSIGNAL)==4);
		CHECK(shutdown(sock,SHUT_RDWR)==0 && close(sock)==0);
		puts("SERVER_SANDBOX_DESCRIPTOR_IO_AND_HOST_AUTHORITY_REFUSAL_PASS");
		fflush(stdout);_exit(0);
	}
	int client=socket(AF_INET,SOCK_STREAM,0);CHECK(client>=0);
	CHECK(connect(client,(struct sockaddr *)&addr,sizeof(addr))==0);
	CHECK(send(client,"ping",4,MSG_NOSIGNAL)==4);
	char reply[4];CHECK(recv(client,reply,4,MSG_WAITALL)==4 && !memcmp(reply,"pong",4));
	int status;CHECK(waitpid(child,&status,0)==child && WIFEXITED(status) && WEXITSTATUS(status)==0);
	CHECK(close(client)==0 && close(listener)==0 && close(canary)==0 && close(image)==0);
	CHECK(unlink(argv[1])==0);return 0;
}
