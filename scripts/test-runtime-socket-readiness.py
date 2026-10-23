#!/usr/bin/env python3
import os
import pathlib
import socket
import subprocess
import sys
import tempfile
import unittest
from runtime_socket_readiness import wait_for_socket_owner


class SocketReadinessTests(unittest.TestCase):
    def test_stale_socket_and_live_wrong_owner_do_not_prove_readiness(self):
        class AbsentOwner:
            pid = 0

            def poll(self):
                return None

        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'control.sock'
            with socket.socket(socket.AF_UNIX) as listener:
                listener.bind(str(path))
                listener.listen(8)
                self.assertTrue(path.is_socket())
                with self.assertRaises(TimeoutError):
                    wait_for_socket_owner(path, AbsentOwner(), timeout=.1)
            self.assertTrue(path.is_socket())
            with self.assertRaises(TimeoutError):
                wait_for_socket_owner(path, AbsentOwner(), timeout=.1)

    def test_replacement_listener_is_bound_to_its_real_process(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'control.sock'
            old = socket.socket(socket.AF_UNIX)
            old.bind(str(path)); old.listen(8)
            code = 'import socket,sys,os,time; time.sleep(.15); os.unlink(sys.argv[1]); s=socket.socket(socket.AF_UNIX); s.bind(sys.argv[1]); s.listen(8); sys.stdin.read()'
            child = subprocess.Popen([sys.executable, '-c', code, str(path)], stdin=subprocess.PIPE)
            try:
                observed = wait_for_socket_owner(path, child, timeout=3)
                self.assertEqual(observed, {'pid': child.pid, 'uid': os.geteuid(), 'gid': os.getegid()})
            finally:
                old.close()
                child.communicate(timeout=3)

    def test_terminal_process_is_not_a_readiness_wait(self):
        child = subprocess.Popen(['/bin/false'])
        child.wait(timeout=3)
        with self.assertRaises(RuntimeError):
            wait_for_socket_owner('/not-a-listener', child, timeout=3)


if __name__ == '__main__':
    unittest.main()
