#!/usr/bin/env python3
"""Docker bootstrap authority cannot be selected as a workload identity."""
import copy
from pathlib import Path
import runpy
import unittest
m=runpy.run_path(str(Path(__file__).with_name('validate-runtime-docker-root.py')))
class PolicyTests(unittest.TestCase):
 def test_fixed_identity_and_refusals(self):
  valid={'process':{'user':{'uid':1000,'gid':1000}}}
  m['validate_workload'](valid)
  plus=copy.deepcopy(valid);plus['process'].update(noNewPrivileges=True);plus['process']['user']['additionalGids']=[1000];m['validate_workload'](plus)
  changes=[{'user':{'uid':0,'gid':0}},{'user':{'uid':1000,'gid':0}},{'user':{'uid':1001,'gid':1000}},{'user':{'uid':1000,'gid':1000,'additionalGids':[0]}},{'noNewPrivileges':False},{'noNewPrivileges':None},{'capabilities':{'effective':['CAP_NET_RAW']}},{'capabilities':{}},{'capabilities':{'bounding':[]}},{'user':{'uid':True,'gid':1000}}]
  for change in changes:
   case=copy.deepcopy(valid);case['process'].update(change)
   with self.subTest(change=change):
    with self.assertRaises(m['ApprovalError']):m['validate_workload'](case)
if __name__=='__main__':unittest.main()
