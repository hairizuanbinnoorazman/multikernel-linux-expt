"""Strict correlation of pool-capacity candidates at private Docker hold points."""
import json
from runtime_doctor_checkpoint import qualify as qualify_storage, unique_object

UNOBSERVED = ['kernel-incarnation', 'cpu-transition-health', 'allocator-internal-metadata',
              'native-storage-health', 'native-network-health', 'service-readiness',
              'docker-engine', 'containment']


def qualify(data, boot, owner, storage_report, released=False):
    qualify_storage(json.dumps(storage_report).encode(), boot, owner, released=released)
    if not isinstance(data, bytes) or len(data) > 1 << 20:
        raise ValueError('pool doctor checkpoint exceeds its bound')
    r = json.loads(data, object_pairs_hook=unique_object)
    fields = {'version','scope','status','revision','boot_id','boot_stable','ownership','kernel','findings',
              'cpu_candidates','remaining_configured_memory_bytes','memory_candidate_upper_bound_bytes',
              'admission_guaranteed','unobserved'}
    if not isinstance(r, dict) or set(r) != fields:
        raise ValueError('invalid pool checkpoint fields')
    if (type(r['version']) is not int or r['version'] != 1 or r['scope'] != 'configured-durable-and-kernel-pool-capacity' or
            r['status'] != 'consistent' or r['boot_id'] != boot or r['boot_stable'] is not True or
            r['revision'] != storage_report['revision'] or r['findings'] != [] or
            r['admission_guaranteed'] is not False or r['unobserved'] != UNOBSERVED):
        raise ValueError('pool observation is not stable scoped capacity')

    def number(value):
        if type(value) is not int or not 0 <= value < 1 << 64:
            raise ValueError('invalid pool numeric metadata')
        return value

    def cpus(values):
        if (not isinstance(values, list) or len(values) > 4096 or
                any(type(v) is not int or not 0 < v < 1 << 64 for v in values) or
                sorted(set(values)) != values):
            raise ValueError('invalid pool CPU metadata')
        return values

    o = r['ownership']
    if (not isinstance(o, dict) or set(o) != {'version','source','lifecycle','configured_cpus','configured_memory_bytes','memory_reserve_bytes','initialized'} or
            type(o['version']) is not int or o['version'] != 1 or o['source'] != 'configured-pool-and-durable-lifecycle' or
            o['initialized'] is not True or json.dumps(o['lifecycle'],sort_keys=True) != json.dumps(storage_report['ownership'],sort_keys=True)):
        raise ValueError('pool configuration and held lifecycle differ')
    configured = cpus(o['configured_cpus'])
    total, reserve = number(o['configured_memory_bytes']), number(o['memory_reserve_bytes'])
    if total == 0 or reserve >= total:
        raise ValueError('invalid configured pool memory')
    claimed = [v for v in o['lifecycle']['sandboxes'] if v['state'] != 'ABSENT']
    claimed_cpus = [cpu for v in claimed for cpu in v['cpus']]
    claimed_memory = sum(number(v['memory_bytes']) for v in claimed)
    if len(set(claimed_cpus)) != len(claimed_cpus) or not set(claimed_cpus) <= set(configured) or claimed_memory > total-reserve:
        raise ValueError('pool durable claims exceed policy')
    k = r['kernel']
    if not isinstance(k, dict) or set(k) != {'cpus','memory'} or not isinstance(k['cpus'],dict) or set(k['cpus']) != {'members','available'}:
        raise ValueError('invalid kernel pool fields')
    members, available = cpus(k['cpus']['members']), cpus(k['cpus']['available'])
    if members != configured or available != sorted(set(members)-set(claimed_cpus)):
        raise ValueError('kernel pool CPU assignment differs from held lifecycle')
    m = k['memory']
    if not isinstance(m, dict) or set(m) != {'chunks','total_bytes','allocated_bytes','available_bytes','largest_free_extent_bytes'}:
        raise ValueError('invalid kernel pool memory fields')
    for value in m.values():
        number(value)
    if (not 0 < m['chunks'] <= 8192 or m['total_bytes'] < total or m['allocated_bytes'] != claimed_memory or
            m['available_bytes'] != m['total_bytes']-claimed_memory or
            not 0 < m['largest_free_extent_bytes'] <= m['available_bytes']):
        raise ValueError('kernel memory differs from held lifecycle')
    remaining = total-reserve-claimed_memory
    upper = min(remaining, max(0,m['available_bytes']-reserve),m['largest_free_extent_bytes'])
    if (cpus(r['cpu_candidates']) != available or number(r['remaining_configured_memory_bytes']) != remaining or
            number(r['memory_candidate_upper_bound_bytes']) != upper):
        raise ValueError('pool candidate exceeds the independently computed bound')
    return r
