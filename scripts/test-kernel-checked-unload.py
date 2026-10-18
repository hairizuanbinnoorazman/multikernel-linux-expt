#!/usr/bin/env python3
"""Exercise actual unload branches/helper and bounded retention with modeled backends."""
import argparse
from pathlib import Path
import subprocess,tempfile
p=argparse.ArgumentParser(description=__doc__);p.add_argument('directory',type=Path);p.add_argument('--old-callers',action='store_true');p.add_argument('--old-target',action='store_true');p.add_argument('--old-report',action='store_true');a=p.parse_args();w=a.directory
def block(text,prefix):
 start=text.index(prefix);return text[start:text.index('\n}',start)+2]+'\n'
legacy=(w/('before-kexec.c' if a.old_callers else 'kexec.c')).read_text()
file=(w/('before-kexec_file.c' if a.old_target else 'kexec_file.c')).read_text()
core=(w/('before-core.c' if a.old_report else 'core.c')).read_text()
start=legacy.index('\t\t} else if (flags & KEXEC_MULTIKERNEL) {');end=legacy.index('\n\t\t} else {',start)
bulk=legacy[start+len('\t\t} else if (flags & KEXEC_MULTIKERNEL) {'):end]
start=file.index('\t\tif (flags & KEXEC_FILE_UNLOAD) {');end=file.index('\n\t\t} else {',start)
target=file[start+len('\t\tif (flags & KEXEC_FILE_UNLOAD) {'):end]
code=r'''
#include <stdio.h>
#include <stdbool.h>
#include <stdlib.h>
#include <stdarg.h>
#include <stddef.h>
#include <errno.h>
#define CHECK(c) do{if(!(c)){fprintf(stderr,"checked unload line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
#define KEXEC_TYPE_MULTIKERNEL 2
static int locked,remaining,attempts,releases,refuse_at,reason,unlinks;
struct kimage {int id;};static struct kimage images[300];
static struct kimage *kimage_find_by_type(int type){CHECK(type==2&&locked);return remaining?&images[releases]:NULL;}
static struct kimage *kimage_find_by_id(int id){CHECK(locked);return remaining&&id==7?&images[releases]:NULL;}
static int kimage_free_checked(struct kimage *i){CHECK(locked&&i==&images[releases]);attempts++;if(releases==refuse_at)return reason;remaining--;releases++;unlinks++;return 0;}
static void kimage_free(struct kimage *i){if(!i)return;int held=locked;locked=1;(void)kimage_free_checked(i);locked=held;}
static void kimage_remove_from_list(struct kimage *i){CHECK(i);unlinks++;}
static int kimage_get_all_by_type(int type,struct kimage **out,int max){CHECK(type==2);int n=remaining<max?remaining:max;for(int j=0;j<n;j++)out[j]=&images[releases+j];return n;}
#define pr_err(...) ((void)0)
#define pr_info(...) ((void)0)
'''+block((w/'kexec_core.c').read_text(),'int kimage_unload_multikernels(void)')+'''
static int unload_bulk(void){int ret=0;locked=1;
'''+bulk+'''
ret=0;goto out_unlock;
out_unlock:locked=0;return ret;}
static int unload_target(int mk_id){int ret=0;struct kimage *image=NULL;locked=1;
'''+target+'''
out:locked=0;kimage_free(image);return ret;}
'''+r'''
struct list_head {struct list_head *next,*prev;};
#define list_entry(p,t,m) ((t *)((char *)(p)-offsetof(t,m)))
#define list_for_each_entry(p,h,m) for(p=list_entry((h)->next,__typeof__(*p),m);&p->m!=(h);p=list_entry(p->m.next,__typeof__(*p),m))
struct mk_instance {int id,release_error,region_count;unsigned long long generation;unsigned *cpus;struct list_head retained_list;};
static struct list_head mk_retained_instances;static int mk_retained_instances_lock,report_locked;
#define spin_lock_irqsave(p,f) do{CHECK((p)==&mk_retained_instances_lock&&!report_locked);(f)=1;report_locked=1;}while(0)
#define spin_unlock_irqrestore(p,f) do{CHECK((p)==&mk_retained_instances_lock&&(f)&&report_locked);report_locked=0;}while(0)
struct seq_file {int rows,truncated;};
static unsigned mk_cpu_set_count(unsigned *cpus){CHECK(report_locked);return *cpus;}
static void seq_printf(struct seq_file *sf,const char *fmt,...){CHECK(report_locked&&fmt);sf->rows++;}
static void seq_puts(struct seq_file *sf,const char *s){CHECK(report_locked&&s);sf->truncated++;}
'''+block(core,'int mk_retained_instances_show(')+r'''
static void reset(int n,int fail){remaining=n;attempts=releases=unlinks=0;refuse_at=fail;reason=-EBUSY;locked=0;}
int main(void){
 reset(0,-1);CHECK(!unload_bulk()&&!attempts&&!locked);
 reset(11,-1);CHECK(!unload_bulk()&&releases==11&&!remaining&&!locked);
 reset(128,-1);CHECK(!unload_bulk()&&releases==128&&!remaining);
 reset(129,-1);CHECK(unload_bulk()==-E2BIG&&releases==128&&remaining==1);CHECK(!unload_bulk()&&!remaining&&releases==129);
 reset(20,3);CHECK(unload_bulk()==-EBUSY&&releases==3&&remaining==17&&unlinks==3&&!locked);refuse_at=-1;CHECK(!unload_bulk()&&!remaining);
 puts("CHECKED_BULK_UNLOAD_EMPTY_ELEVEN_BUDGET_RETRY_AND_PARTIAL_REFUSAL_PASS");
 reset(1,0);CHECK(unload_target(8)==-ENOENT&&remaining==1&&!attempts&&!unlinks&&!locked);
 for(int r=0;r<3;r++){reset(1,0);reason=r==0?-EBUSY:r==1?-EUCLEAN:-ENOMEM;CHECK(unload_target(7)==reason&&remaining==1&&attempts==1&&!unlinks&&!locked);refuse_at=-1;CHECK(!unload_target(7)&&!remaining&&unlinks==1);}
 puts("CHECKED_TARGET_UNLOAD_EXACT_ERRORS_REGISTRY_RETENTION_AND_SINGLE_SUCCESS_PASS");
 struct mk_instance records[1000];unsigned cpus=2;
 for(int count=0;count<=1000;count++){
  mk_retained_instances.next=mk_retained_instances.prev=&mk_retained_instances;
  for(int j=0;j<count;j++){records[j].cpus=&cpus;struct list_head *n=&records[j].retained_list;n->next=&mk_retained_instances;n->prev=mk_retained_instances.prev;n->prev->next=n;mk_retained_instances.prev=n;}
  struct seq_file sf={0};CHECK(!mk_retained_instances_show(&sf,NULL));CHECK(sf.rows==(count<64?count:64)&&sf.truncated==(count>64)&&!report_locked);
 }
 puts("RETAINED_REPORT_EMPTY_EXACT_BOUND_AND_EXPLICIT_TRUNCATION_PASS");return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-checked-unload-') as td:
 source=Path(td)/'test.c';exe=Path(td)/'test';source.write_text(code)
 subprocess.run(['gcc','-std=gnu11','-Wall','-Wextra','-Werror','-Wno-unused-function','-Wno-unused-parameter','-fsanitize=undefined','-fno-sanitize-recover=all',str(source),'-o',str(exe)],check=True)
 print('CHECKED_UNLOAD_ACTUAL_BRANCHES_AND_HELPERS_COMPILED',flush=True);subprocess.run([str(exe)],check=True)
