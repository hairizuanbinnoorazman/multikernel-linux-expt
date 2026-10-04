#!/usr/bin/env python3
import errno
import os
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest import mock

import runtime_safe_publish as publisher


class SafePublishTests(unittest.TestCase):
    def setUp(self):
        self.temp = Path(tempfile.mkdtemp(prefix="mk-safe-publish-test-"))
        self.addCleanup(shutil.rmtree, self.temp)

    def assert_no_attempt_artifacts(self, destination: Path) -> None:
        self.assertFalse(destination.exists())
        self.assertEqual(list(self.temp.glob(f".{destination.name}.*")), [])

    def test_atomic_write_enospc_boundaries_remove_exact_temporary(self):
        destination = self.temp / "result"

        with self.subTest(boundary="write"):
            with self.assertRaises(OSError) as raised:
                publisher.atomic_write(
                    destination,
                    lambda stream: (_ for _ in ()).throw(OSError(errno.ENOSPC, "injected write ENOSPC")),
                )
            self.assertEqual(raised.exception.errno, errno.ENOSPC)
            self.assert_no_attempt_artifacts(destination)

        original_fsync = publisher.os.fsync
        calls = 0

        def fail_first_fsync(descriptor):
            nonlocal calls
            calls += 1
            if calls == 1:
                raise OSError(errno.ENOSPC, "injected fsync ENOSPC")
            return original_fsync(descriptor)

        with self.subTest(boundary="fsync"), mock.patch.object(
            publisher.os, "fsync", side_effect=fail_first_fsync
        ):
            with self.assertRaises(OSError) as raised:
                publisher.atomic_write(destination, lambda stream: stream.write(b"content"))
            self.assertEqual(raised.exception.errno, errno.ENOSPC)
            self.assert_no_attempt_artifacts(destination)

        original_link = publisher.os.link

        def fail_link(source, target, **kwargs):
            if Path(target) == destination:
                raise OSError(errno.ENOSPC, "injected publication ENOSPC")
            return original_link(source, target, **kwargs)

        with self.subTest(boundary="publish"), mock.patch.object(
            publisher.os, "link", side_effect=fail_link
        ):
            with self.assertRaises(OSError) as raised:
                publisher.atomic_write(destination, lambda stream: stream.write(b"content"))
            self.assertEqual(raised.exception.errno, errno.ENOSPC)
            self.assert_no_attempt_artifacts(destination)


if __name__ == "__main__":
    unittest.main()
