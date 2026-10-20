#!/usr/bin/env python3
"""Export only aggregate review facts from a private host dependency inventory."""
import argparse
import collections
import hashlib
import json
from pathlib import Path

FAMILIES=frozenset(['runtime-go','kernel-profile','native-helper','kerf','python','native-tool','cni','running-service','native-library','kerf-source'])
REASONS=frozenset(['artifact_hash_mismatch','ownership_or_write_policy','artifact_unavailable_unstable_or_not_regular','tool_not_installed','loader_cache_entry_absent','hash_or_ownership_mismatch','checkout_revision_mismatch'])

def summary(data, digest):
    assert data['schema_version']==1 and data['review_status']=='REQUIRES_REVIEW'
    artifacts=data['artifacts'];assert {r['family']for r in artifacts}<=FAMILIES
    reasons=collections.Counter()
    for r in data['collection_gaps']:
        assert r['family']in FAMILIES|{'deployment'} and r['reason']in REASONS
        reasons[(r['family'],r['reason'])]+=1
    licenses=data['selected_package_licenses'];python=data['python_distributions']
    return {'schema_version':1,'private_report_sha256':digest,'status':'INVENTORY_COLLECTED_REVIEW_REQUIRED',
      'scope':'Only fixed schema, aggregate counts, booleans and private-report digest. No artifact paths, software identities, versions, license text or configuration.',
      'artifact_count':len(artifacts),'family_counts':dict(sorted(collections.Counter(r['family']for r in artifacts).items())),
      'unprotected_artifact_count':sum(not r['protected_root_owned']for r in artifacts),
      'hash_bound_artifact_count':sum('matches_expected'in r for r in artifacts),
      'hash_mismatch_count':sum(r.get('matches_expected')is False for r in artifacts),
      'support_file_count':len(data['support']['files']),
      'support_mismatch_count':sum(not r['matches_expected']or not r['protected_root_owned']for r in data['support']['files']),
      'installed_package_count':len(data['installed_packages']),
      'selected_package_count':len(licenses),
      'copyright_present_count':sum('copyright'in r for r in licenses.values()),
      'copyright_label_review_count':sum(r['status']!='declared_labels'for r in licenses.values()),
      'python_distribution_count':len(python),
      'python_license_review_count':sum(r['license_status']!='declared'for r in python),
      'python_direct_reference_count':sum(r['direct_reference_count']for r in python),
      'go_metadata_available_count':sum(r['go_status']=='available'for r in data['binary_metadata']),
      'go_dependency_identity_count':sum(len(r.get('dependencies',[]))for r in data['binary_metadata']),
      'kerf_revision_matches_pin':data['kerf_checkout']['matches_profile'],
      'kerf_tracked_tree_modified':data['kerf_checkout']['tracked_tree_modified'],
      'running_service_count':len(data['services']),
      'collection_gap_count':len(data['collection_gaps']),
      'collection_gap_categories':[{'family':f,'reason':r,'count':n}for (f,r),n in sorted(reasons.items())],
      'coverage_limit_count':len(data['coverage_limits'])}

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('private_report',type=Path);a=p.parse_args()
    data=a.private_report.read_bytes();value=summary(json.loads(data),hashlib.sha256(data).hexdigest())
    print(json.dumps(value,indent=2))
if __name__=='__main__':main()
