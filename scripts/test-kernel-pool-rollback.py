#!/usr/bin/env python3
"""Exercise the exact baseline grow function with owned-chunk failure injection.

Pass an original or patched pinned baseline.c. This is a userspace transaction
fixture; it does not qualify kernel locking, page allocation or boot behavior.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("source", type=Path)
parser.add_argument("--host-park", action="store_true", help="Model architecture-owned park pages created by the first grow")
args = parser.parse_args()
text = args.source.read_text()
start = text.index("static int mk_baseline_grow_pool(")
end = text.index("\nstatic int mk_baseline_initialize_cpus", start)
function = text[start:end]
prelude = r'''
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <errno.h>
#include <string.h>
#include <stdbool.h>
typedef uint64_t phys_addr_t;
#define GFP_KERNEL 0
#define MK_BASELINE_MAX_MEM_REQS 8
#define pr_err(...) ((void)0)
struct mk_baseline_mem_req { unsigned long long size; int node; };
static int calls, fail_at, rollback_fail, shrink_calls, alloc_fail;
static int park_enabled, park_alive, initially_empty, teardown_calls, teardown_fail;
static int live[8], shrink_order[8];
static const phys_addr_t existing = 0x1000;
static void *test_calloc(size_t n,size_t size,int flags) {
 (void)flags; return alloc_fail ? NULL : calloc(n,size);
}
#define kcalloc test_calloc
#define kfree free
static int mk_pool_mem_grow(size_t size,int node,phys_addr_t *base) {
 (void)size;(void)node;
 int index=calls++;if(index==fail_at)return -ENOMEM;
 if(park_enabled && initially_empty && index==0)park_alive=1;
 live[index]=1;if(base)*base=existing+0x1000*(index+1);return 0;
}
static int mk_pool_mem_shrink(phys_addr_t base,size_t size) {
 (void)size;
 if(base==existing)abort();
 int index=(int)((base-existing)/0x1000)-1;
 if(index<0 || index>=8 || !live[index])abort();
 shrink_order[shrink_calls++]=index;
 if(index==rollback_fail || (park_alive && initially_empty && index==0))return -EBUSY;
 live[index]=0;return 0;
}
static bool mk_pool_empty(void) { return initially_empty; }
static int mk_teardown_host_park(void) { teardown_calls++;if(teardown_fail)return -EBUSY;park_alive=0;return 0; }
#define CHECK(value) do {if(!(value)){fprintf(stderr,"rollback fixture line%d\n",__LINE__);exit(2);}}while(0)
'''
checks = r'''
static void reset(void) {
 calls=shrink_calls=alloc_fail=0;fail_at=rollback_fail=-1;
 park_enabled=park_alive=teardown_calls=teardown_fail=0;initially_empty=1;
 memset(live,0,sizeof(live));memset(shrink_order,0,sizeof(shrink_order));
}
int main(void) {
 struct mk_baseline_mem_req reqs[4]={{1ULL<<30,0},{1ULL<<30,1},{1ULL<<30,2},{1ULL<<30,3}};
 if(TEST_HOST_PARK) {
  reset();park_enabled=1;fail_at=2;
  CHECK(mk_baseline_grow_pool(reqs,4)==-ENOMEM);
  CHECK(teardown_calls==1 && !park_alive && shrink_calls==2 && !live[0] && !live[1]);
  puts("NEW_POOL_HOST_PARK_RELEASED_BEFORE_CHUNK_ROLLBACK_PASS");
  reset();park_enabled=park_alive=1;initially_empty=0;fail_at=2;
  CHECK(mk_baseline_grow_pool(reqs,4)==-ENOMEM && !teardown_calls && park_alive && !live[0] && !live[1]);
  puts("PREEXISTING_POOL_HOST_PARK_PRESERVED_PASS");
  reset();park_enabled=teardown_fail=1;fail_at=2;
  CHECK(mk_baseline_grow_pool(reqs,4)==-EUCLEAN && teardown_calls==1 && park_alive && live[0] && !live[1]);
  puts("PARK_QUIESCENCE_FAILURE_RETAINS_BUSY_CHUNK_AND_REPORTS_INCOMPLETE_CLEANUP_PASS");
  reset();park_enabled=1;fail_at=0;
  CHECK(mk_baseline_grow_pool(reqs,4)==-ENOMEM && !teardown_calls);
  puts("FIRST_GROW_FAILURE_DOES_NOT_TEARDOWN_PARK_PASS");return 0;
 }
 reset();fail_at=2;CHECK(mk_baseline_grow_pool(reqs,4)==-ENOMEM);
 CHECK(calls==3 && shrink_calls==2 && shrink_order[0]==1 && shrink_order[1]==0);
 for(int i=0;i<4;i++)CHECK(!live[i]);
 puts("PARTIAL_GROW_ROLLBACK_PRESERVES_EXISTING_POOL_PASS");
 reset();CHECK(mk_baseline_grow_pool(reqs,4)==0);
 CHECK(calls==4 && !shrink_calls);for(int i=0;i<4;i++)CHECK(live[i]);
 puts("SUCCESSFUL_GROW_RETAINS_EXACT_GRANT_PASS");
 reset();fail_at=0;CHECK(mk_baseline_grow_pool(reqs,4)==-ENOMEM && !shrink_calls);
 reset();alloc_fail=1;CHECK(mk_baseline_grow_pool(reqs,4)==-ENOMEM && !calls);
 puts("FIRST_FAILURE_AND_TRACKING_ALLOCATION_FAILURE_PASS");
 reset();fail_at=3;rollback_fail=1;CHECK(mk_baseline_grow_pool(reqs,4)==-EUCLEAN);
 CHECK(shrink_calls==3 && !live[0] && live[1] && !live[2]);
 puts("BUSY_CHUNK_RETAINED_REPORTED_AND_OTHER_ROLLBACK_CONTINUES_PASS");
 reset();CHECK(mk_baseline_grow_pool(reqs,0)==0 && !calls);
 CHECK(mk_baseline_grow_pool(reqs,-1)==-EINVAL && !calls);
 CHECK(mk_baseline_grow_pool(reqs,MK_BASELINE_MAX_MEM_REQS+1)==-EINVAL && !calls);
 puts("BOUNDED_REQUEST_COUNT_AND_EMPTY_REQUEST_PASS");
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix="mk-pool-rollback-") as directory:
    root = Path(directory)
    source = root / "fixture.c"
    binary = root / "fixture"
    source.write_text("#define TEST_HOST_PARK " + str(int(args.host_park)) + "\n" + prelude + function + checks)
    subprocess.run(["cc", "-O2", "-Wall", "-Wextra", "-Werror", "-Wno-unused-function", str(source), "-o", str(binary)], check=True)
    subprocess.run([str(binary)], check=True, timeout=10)
