#!/usr/bin/env python3
"""Run the actual generation allocator with C11 atomic models and pthreads."""
import argparse
from pathlib import Path
import subprocess
import tempfile
p=argparse.ArgumentParser(description=__doc__);p.add_argument('source',type=Path);a=p.parse_args();s=a.source.read_text()
at=s.index('int mk_instance_assign_generation(');function=s[at:s.index('\n}',at)+2]
code=r'''
#include <stdint.h>
#include <stdatomic.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#define CHECK(c) do{if(!(c)){fprintf(stderr,"instance generation line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
typedef int64_t s64;typedef uint64_t u64;typedef _Atomic int64_t atomic64_t;
#define ATOMIC64_INIT(n) (n)
#define S64_MAX INT64_MAX
#define atomic64_read(p) atomic_load(p)
#define atomic64_try_cmpxchg(p,old,next) atomic_compare_exchange_strong(p,old,next)
struct mk_instance {u64 generation;};
static atomic64_t mk_instance_next_generation=ATOMIC64_INIT(0);
'''+function+r'''
#define THREADS 8
#define ITEMS 512
static struct mk_instance instances[THREADS][ITEMS];
static void *worker(void *arg){unsigned thread=(unsigned)(uintptr_t)arg;for(unsigned i=0;i<ITEMS;i++)CHECK(!mk_instance_assign_generation(&instances[thread][i]));return NULL;}
static int compare(const void *a,const void *b){u64 x=*(const u64 *)a,y=*(const u64 *)b;return (x>y)-(x<y);}
int main(void){pthread_t threads[THREADS];u64 values[THREADS*ITEMS];
 for(unsigned i=0;i<THREADS;i++)CHECK(!pthread_create(&threads[i],NULL,worker,(void *)(uintptr_t)i));
 for(unsigned i=0;i<THREADS;i++)CHECK(!pthread_join(threads[i],NULL));
 for(unsigned i=0;i<THREADS;i++)for(unsigned j=0;j<ITEMS;j++)values[i*ITEMS+j]=instances[i][j].generation;
 qsort(values,THREADS*ITEMS,sizeof(*values),compare);for(unsigned i=0;i<THREADS*ITEMS;i++)CHECK(values[i]==i+1);
 CHECK(atomic64_read(&mk_instance_next_generation)==THREADS*ITEMS);
 struct mk_instance old={.generation=17},last={0},refused={0};
 CHECK(mk_instance_assign_generation(&old)==-EALREADY&&old.generation==17&&atomic64_read(&mk_instance_next_generation)==THREADS*ITEMS);
 atomic_store(&mk_instance_next_generation,S64_MAX-1);CHECK(!mk_instance_assign_generation(&last)&&last.generation==S64_MAX);
 for(unsigned i=0;i<3;i++)CHECK(mk_instance_assign_generation(&refused)==-EOVERFLOW&&!refused.generation&&atomic64_read(&mk_instance_next_generation)==S64_MAX);
 puts("INSTANCE_GENERATION_CONCURRENT_UNIQUE_IMMUTABLE_AND_SATURATION_PASS threads=8 assignments=4096");return 0;}
'''
with tempfile.TemporaryDirectory(prefix='mk-instance-generation-') as tmp:
    root=Path(tmp);(root/'fixture.c').write_text(code)
    subprocess.run(['gcc','-std=gnu11','-O1','-Wall','-Wextra','-Werror','-pthread','-fsanitize=undefined','-fno-sanitize-recover=all',str(root/'fixture.c'),'-o',str(root/'fixture')],check=True)
    print('PRODUCTION_GENERATION_ALLOCATOR_COMPILE_PASS',flush=True)
    subprocess.run([str(root/'fixture')],check=True)
