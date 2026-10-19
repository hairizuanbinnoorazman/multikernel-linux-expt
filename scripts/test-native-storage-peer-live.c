#define main native_helper_main
#include "mkvsock-nbd.c"
#undef main
#define REQUIRE(value) do { if (!(value)) { fprintf(stderr,"PEER_PROBE_ASSERT line=%d errno=%d\n",__LINE__,errno);exit(2); } } while(0)
static void authority(void) {
 FILE *status=fopen("/proc/self/status","r");REQUIRE(status);
 char line[512];int caps=0,nnp=0;
 while(fgets(line,sizeof(line),status)) {
  if(!strncmp(line,"CapInh:",7)||!strncmp(line,"CapPrm:",7)||!strncmp(line,"CapEff:",7)||!strncmp(line,"CapAmb:",7)) {
   unsigned long long bits; REQUIRE(sscanf(line+7,"%llx",&bits)==1 && bits==0);caps++;fputs(line,stdout);
  }
  if(!strncmp(line,"NoNewPrivs:",11)) {int enabled;REQUIRE(sscanf(line+11,"%d",&enabled)==1 && enabled==1);nnp++;fputs(line,stdout);}
 }
 fclose(status);REQUIRE(caps==4 && nnp==1);printf("PROBE_UID=%lu\n",(unsigned long)geteuid());
}
static void request(int fd,uint32_t type,uint32_t length) {
 struct nbd_request_wire q={.magic=htonl(NBD_REQUEST_MAGIC),.type=htonl(type),.len=htonl(length)};q.handle[0]=(uint8_t)type;
 xwrite(fd,&q,sizeof(q));char canary[512];memset(canary,0x5a,sizeof(canary));
 if((type&NBD_CMD_MASK_COMMAND)==NBD_CMD_WRITE)xwrite(fd,canary,length);
 if((type&NBD_CMD_MASK_COMMAND)==NBD_CMD_DISC)return;
 struct nbd_reply_wire r;REQUIRE(read_exact_or_eof(fd,&r,sizeof(r))==1 && ntohl(r.magic)==NBD_REPLY_MAGIC && !r.error && r.handle[0]==q.handle[0]);
 if((type&NBD_CMD_MASK_COMMAND)==NBD_CMD_READ) {char result[512];REQUIRE(read_exact_or_eof(fd,result,length)==1 && !memcmp(result,canary,length));}
}
int main(int argc,char **argv) {
 REQUIRE(argc==3);authority();uint64_t port;REQUIRE(native_decimal(argv[2],1024,65535,&port));
 int fd=vsock_socket();struct sockaddr_vm address={.svm_family=AF_VSOCK,.svm_cid=0,.svm_port=port};REQUIRE(connect(fd,(struct sockaddr *)&address,sizeof(address))==0);
 struct hello_wire hello,response;fill_hello(&hello,4096,"peer-canary-image","peer-canary-generation");
 if(!strcmp(argv[1],"refuse")) {
  ssize_t sent=send(fd,&hello,sizeof(hello),MSG_NOSIGNAL);REQUIRE(sent==sizeof(hello)||(sent<0&&(errno==EPIPE||errno==ECONNRESET)));
  char byte;ssize_t received=recv(fd,&byte,1,0);REQUIRE(received==0||(received<0&&errno==ECONNRESET));
  close(fd);puts("LIVE_WRONG_CID_ZERO_RESPONSE_PASS");return 0;
 }
 REQUIRE(!strcmp(argv[1],"accept")||!strcmp(argv[1],"old-accept"));xwrite(fd,&hello,sizeof(hello));
 REQUIRE(read_exact_or_eof(fd,&response,sizeof(response))==1 && hello_matches(&response,4096,"peer-canary-image","peer-canary-generation"));
 request(fd,NBD_CMD_WRITE|NBD_CMD_FLAG_FUA,512);request(fd,NBD_CMD_READ,512);request(fd,NBD_CMD_FLUSH,0);request(fd,NBD_CMD_DISC,0);close(fd);
 puts(!strcmp(argv[1],"old-accept")?"LIVE_OLD_WRONG_CID_ACCEPTED_CONTROL":"LIVE_RIGHT_CID_READ_WRITE_FLUSH_PASS");return 0;
}
