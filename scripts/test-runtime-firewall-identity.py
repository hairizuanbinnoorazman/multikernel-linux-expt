#!/usr/bin/env python3
import unittest
from runtime_firewall_identity import identity


class FirewallIdentityTests(unittest.TestCase):
    raw = b'# Generated at time A\n*filter\n:INPUT ACCEPT [1:42]\n:MK-proof - [0:0]\n-A INPUT -s 10.1.2.3 -p tcp --dport 22 -j ACCEPT\nCOMMIT\n'

    def test_only_timestamps_and_chain_counters_are_volatile(self):
        changed = self.raw.replace(b'time A', b'time B').replace(b'[1:42]', b'[500:99999]')
        self.assertEqual(identity(self.raw), identity(changed))

    def test_policies_rules_addresses_ports_and_chains_remain_bound(self):
        for old, new in [(b'INPUT ACCEPT', b'INPUT DROP'), (b'10.1.2.3', b'10.1.2.4'),
                         (b'dport 22', b'dport 23'), (b'MK-proof', b'MK-foreign'),
                         (b'-j ACCEPT', b'-j DROP'), (b'COMMIT', b'-A INPUT -j DROP\nCOMMIT')]:
            with self.subTest(old=old):
                self.assertNotEqual(identity(self.raw), identity(self.raw.replace(old, new)))

    def test_unrecognized_chain_syntax_is_refused(self):
        with self.assertRaises(ValueError):
            identity(self.raw.replace(b'[1:42]', b'[unknown]'))


if __name__ == '__main__':
    unittest.main()
