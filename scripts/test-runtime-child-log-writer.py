#!/usr/bin/env python3
"""Real finite output, producer failure and closed-consumer controls."""
from pathlib import Path
import shlex,subprocess,tempfile,unittest
from runtime_child_log_pressure import WRITER_SCRIPT,WRITER_BYTES


class WriterTests(unittest.TestCase):
    def command(self,directory,writer=WRITER_SCRIPT):
        return ['/bin/sh','-c','umask 077; p="$1"; '+writer+'\n: >"$p/done"','controlled-writer',str(directory)]

    def test_actual_finite_busybox_writer_completes_exact_payload(self):
        with tempfile.TemporaryDirectory() as raw:
            p=Path(raw)
            payload=p/'controlled-payload'
            with payload.open('xb') as output:
                result=subprocess.run(['/usr/bin/prlimit','--fsize='+str(WRITER_BYTES+1)+':'+str(WRITER_BYTES+1),'--core=0:0','--',*self.command(p)],stdout=output,stderr=subprocess.PIPE,timeout=3)
            self.assertEqual(result.returncode,0);self.assertEqual(result.stderr,b'')
            total=payload.stat().st_size;self.assertEqual(total,WRITER_BYTES)
            with payload.open('rb') as stream:
                while data:=stream.read(65536):self.assertEqual(data,bytes(len(data)))
            self.assertEqual((p/'writer-status').read_bytes(),b'0\n')
            self.assertEqual((p/'writer-bytes').read_bytes(),str(total).encode()+b'\n');self.assertTrue((p/'done').is_file())
            self.assertEqual((p/'writer-status').stat().st_mode&0o777,0o600)

    def test_broken_consumer_refuses_without_byte_or_completion_claim(self):
        with tempfile.TemporaryDirectory() as raw:
            p=Path(raw);process=subprocess.Popen(self.command(p),stdout=subprocess.PIPE,stderr=subprocess.PIPE)
            process.stdout.close();self.assertNotEqual(process.wait(timeout=3),0)
            self.assertEqual(process.stderr.read(),b'');process.stderr.close()
            self.assertNotEqual((p/'writer-status').read_bytes(),b'0\n')
            self.assertFalse((p/'writer-bytes').exists());self.assertFalse((p/'done').exists())

    def test_short_failed_producer_cannot_publish_success(self):
        with tempfile.TemporaryDirectory() as raw:
            p=Path(raw);tool=p/'busybox-fault'
            tool.write_text('#!/bin/sh\nprintf short\nexit 23\n');tool.chmod(0o700)
            writer=WRITER_SCRIPT.replace('/bin/busybox',shlex.quote(str(tool)))
            result=subprocess.run(self.command(p,writer),capture_output=True,timeout=3)
            self.assertEqual(result.returncode,23);self.assertEqual(result.stdout,b'short')
            self.assertEqual((p/'writer-status').read_bytes(),b'23\n')
            self.assertFalse((p/'writer-bytes').exists());self.assertFalse((p/'done').exists())

if __name__=='__main__':unittest.main()
