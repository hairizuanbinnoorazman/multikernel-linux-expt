#!/usr/bin/env python3
"""Check actual config initialization/free/parser entries with modeled allocators.

Checks CPU-set allocation failure and parser entry guards, not physical resource
reservation, full DT parsing or boot behavior.
"""
import argparse
from pathlib import Path
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('source', type=Path)
p.add_argument('header', type=Path)
a = p.parse_args()
s = a.source.read_text()


def function(name):
    start = next(s.index(prefix + name + '(') for prefix in ('int ', 'void ')
                 if prefix + name + '(' in s)
    return s[start:s.index('\n}', start) + 2] + '\n'


code = r'''
#include <stdbool.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#define MK_DT_CONFIG_CURRENT 1
#define NUMA_NO_NODE -1
#define CHECK(c) do{if(!(c)){fprintf(stderr,"config init line %d: %s\n",__LINE__,#c);exit(2);}}while(0)
struct list_head {struct list_head *next,*prev;};
struct mk_pci_device {struct list_head list;};
struct mk_platform_device {struct list_head list;};
struct mk_dt_config {unsigned int version;size_t memory_size;void *cpus;int numa_node;struct list_head pci_devices,platform_devices;int pci_device_count,platform_device_count;bool pci_devices_valid,platform_devices_valid;void *dtb_data;size_t dtb_size;};
static int fail_alloc,live,parser_calls,alloc_calls;
static void logger(const char *format,...){(void)format;}
#define pr_warn logger
#define pr_debug logger
#define pr_err logger
#define pr_info logger
static void INIT_LIST_HEAD(struct list_head *p){p->next=p->prev=p;}
/* Initialization creates empty lists; populated-device teardown is outside scope. */
#define list_for_each_entry_safe(p,n,h,m) for((void)(h),(p)=NULL,(n)=NULL;(p);(p)=(n))
static void list_del(struct list_head *p){(void)p;CHECK(false);}
static void kfree(void *p){CHECK(!p);}
static void *mk_cpu_set_alloc(void){alloc_calls++;if(fail_alloc)return NULL;void *p=calloc(1,1);CHECK(p);live++;return p;}
static void mk_cpu_set_free(void *p){if(p){free(p);live--;}}
static unsigned int mk_cpu_set_count(void *p){CHECK(p);return 0;}
static int mk_dt_check_blob_bounds(const void *fdt,size_t size){(void)fdt;(void)size;parser_calls++;return 0;}
static int fdt_path_offset(const void *fdt,const char *path){(void)fdt;(void)path;return 0;}
static int fdt_subnode_offset(const void *fdt,int node,const char *name){(void)fdt;(void)node;(void)name;return 1;}
#define PARSE(name) static int name(const void *fdt,int node,struct mk_dt_config *config){(void)fdt;(void)node;CHECK(config->cpus);parser_calls++;return 0;}
PARSE(mk_dt_parse_memory)
PARSE(mk_dt_parse_cpus)
PARSE(mk_dt_parse_numa)
PARSE(mk_dt_parse_devices)
''' + function('mk_dt_config_init') + function('mk_dt_config_free') + function('mk_dt_parse') + function('mk_dt_parse_resources') + r'''
static void lists(const struct mk_dt_config *c)
{CHECK(c->pci_devices.next==&c->pci_devices&&c->pci_devices.prev==&c->pci_devices&&c->platform_devices.next==&c->platform_devices&&c->platform_devices.prev==&c->platform_devices);}
int main(void)
{
 struct mk_dt_config c;int blob=1;
 CHECK(mk_dt_config_init(NULL)==-EINVAL&&!alloc_calls);
 memset(&c,0xa5,sizeof(c));fail_alloc=1;CHECK(mk_dt_config_init(&c)==-ENOMEM&&!live&&!c.cpus);lists(&c);
 CHECK(c.version==MK_DT_CONFIG_CURRENT&&c.numa_node==NUMA_NO_NODE&&!c.memory_size&&!c.dtb_data&&!c.dtb_size);
 CHECK(mk_dt_parse(&blob,sizeof(blob),&c)==-EINVAL&&!parser_calls);
 CHECK(mk_dt_parse_resources(&blob,0,"child",&c)==-EINVAL&&!parser_calls);
 mk_dt_config_free(&c);mk_dt_config_free(&c);CHECK(!live&&!c.cpus);
 fail_alloc=0;CHECK(!mk_dt_config_init(&c)&&live==1&&c.cpus);lists(&c);
 CHECK(!mk_dt_parse(&blob,sizeof(blob),&c)&&parser_calls==5);
 CHECK(!mk_dt_parse_resources(&blob,0,"child",&c)&&parser_calls==9);
 mk_dt_config_free(&c);mk_dt_config_free(&c);mk_dt_config_free(NULL);CHECK(!live&&!c.cpus);
 puts("CONFIG_INIT_REQUIRED_CPU_SET_FAILURE_CLEANUP_AND_PARSER_ENTRY_GUARDS_PASS");
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='mk-config-init-') as directory:
    root = Path(directory)
    (root/'probe.c').write_text(code)
    subprocess.run(['cc', '-std=gnu11', '-Wall', '-Wextra', '-Werror',
                    '-fsanitize=undefined', '-fno-sanitize-recover=all',
                    str(root/'probe.c'), '-o', str(root/'probe')], check=True)
    subprocess.run([str(root/'probe')], check=True)
    declaration = next(line for line in a.header.read_text().splitlines()
                       if 'mk_dt_config_init(struct' in line)
    prefix = '#define __must_check __attribute__((warn_unused_result))\nstruct mk_dt_config;\n'
    caller = '\nvoid caller(struct mk_dt_config *c){mk_dt_config_init(c);}\n'
    (root/'ignored.c').write_text(prefix + declaration + caller)
    result = subprocess.run(['cc', '-Wall', '-Wextra', '-Werror', '-c',
                             str(root/'ignored.c'), '-o', str(root/'ignored.o')],
                            text=True, capture_output=True)
    assert result.returncode and 'unused-result' in result.stderr, result.stderr
    (root/'ignored.c').write_text(prefix + declaration.replace('__must_check', '') + caller)
    subprocess.run(['cc', '-Wall', '-Wextra', '-Werror', '-c',
                    str(root/'ignored.c'), '-o', str(root/'ignored.o')], check=True)
    print('CONFIG_INIT_IGNORED_RESULT_REJECTED_WITH_UNANNOTATED_CONTROL_PASS')
