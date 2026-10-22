#!/usr/bin/env python3
import unittest
from runtime_service_policy import parse_policy, command_with_policy, check_observation


class ServicePolicyTests(unittest.TestCase):
    names = ['chown', 'dac_override', 'net_admin']
    text = '[Unit]\nNoNewPrivileges=no\n[Service]\nNoNewPrivileges=yes\nAmbientCapabilities=\nCapabilityBoundingSet=CAP_CHOWN CAP_NET_ADMIN\n'

    def test_declared_bounds_and_observed_enforcement(self):
        policy = parse_policy(self.text, self.names)
        self.assertEqual(policy['expected_capability_mask'], '0000000000000005')
        command = command_with_policy(['/trusted/daemon', '--config=/trusted/config'], policy)
        self.assertIn('--bounding-set=-all,+chown,+net_admin', command)
        observed = {key: '0000000000000005' for key in ['CapBnd', 'CapEff', 'CapPrm']}
        observed.update(CapInh='0000000000000000', CapAmb='0000000000000000', NoNewPrivs='1', Uid='0\t0\t0\t0')
        check_observation(observed, policy)
        for key, value in [('CapBnd', '0000000000000007'), ('CapEff', '0000000000000004'),
                           ('CapAmb', '0000000000000001'), ('NoNewPrivs', '0'), ('Uid', '1000 1000 1000 1000')]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                check_observation({**observed, key: value}, policy)

    def test_weaker_ambiguous_or_unknown_policy_refused(self):
        for old, new in [('NoNewPrivileges=yes', 'NoNewPrivileges=no'),
                         ('AmbientCapabilities=', 'AmbientCapabilities=CAP_CHOWN'),
                         ('CAP_NET_ADMIN', 'CAP_UNKNOWN'), ('CAP_NET_ADMIN', '~CAP_NET_ADMIN'),
                         ('CAP_NET_ADMIN', 'cap_net_admin'),
                         ('CapabilityBoundingSet=CAP_CHOWN CAP_NET_ADMIN', 'CapabilityBoundingSet=CAP_CHOWN\nCapabilityBoundingSet=CAP_NET_ADMIN')]:
            with self.subTest(new=new), self.assertRaises(ValueError):
                parse_policy(self.text.replace(old, new), self.names)


if __name__ == '__main__':
    unittest.main()
