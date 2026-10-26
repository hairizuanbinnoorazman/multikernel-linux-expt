#!/usr/bin/env python3
import json,os,pathlib,socket,tempfile,threading,time,unittest
from runtime_network_checkpoint import inspect

class Daemon:
    pid=os.getpid()
    def poll(self): return None

class NetworkCheckpointTests(unittest.TestCase):
    def exercise(self, change=lambda r:r, daemon=None, endpoint=None):
        e=endpoint or dict(state='READY',owner='runtime',managed_namespace=True,primary_boot_id='test-boot',sandbox_id='box',sandbox_generation='a'*32,generation='b'*32,dns={'domain':'private-synthetic-secret'})
        with tempfile.TemporaryDirectory() as directory:
            path=pathlib.Path(directory)/'network.sock'
            with socket.socket(socket.AF_UNIX) as server:
                server.bind(str(path));server.listen(1);server.settimeout(.3)
                def serve():
                    try:
                        c,_=server.accept()
                        with c:
                            data=bytearray()
                            while True:
                                part=c.recv(4096)
                                if not part:break
                                data.extend(part)
                            if not data:return
                            request=json.loads(data)
                            reply=change(dict(version=1,request_id=request['request_id'],endpoint=e))
                            c.sendall(reply if isinstance(reply,bytes) else json.dumps(reply).encode())
                    except (TimeoutError,BrokenPipeError):pass
                t=threading.Thread(target=serve);t.start()
                try:return inspect(path,daemon or Daemon(),e,'test-boot',dict(sandbox_id='box',generation='a'*32),time.monotonic()+.2)
                finally:t.join(timeout=1);self.assertFalse(t.is_alive())
    def test_exact_authenticated_response_is_projected(self):
        r=self.exercise();self.assertTrue(r['authenticated_check_pass']);self.assertNotIn('private-synthetic-secret',json.dumps(r));self.assertNotIn('dns',r)
    def test_refusal_and_changed_identity_are_not_success(self):
        for change in [lambda r:dict(r,error={'code':'FAILED_PRECONDITION','message':'private'}),lambda r:dict(r,request_id='other'),lambda r:dict(r,version=True),lambda r:dict(r,endpoint=dict(r['endpoint'],generation='c'*32)),lambda r:b'{"version":1,"version":1}',lambda r:b'x'*65537]:
            with self.subTest(change=change),self.assertRaises(ValueError):self.exercise(change)
    def test_wrong_process_is_refused(self):
        d=Daemon();d.pid+=100000
        with self.assertRaises(ValueError):self.exercise(daemon=d)
    def test_wrong_owner_refused_before_connect(self):
        with self.assertRaises(ValueError):inspect('/not-a-socket',Daemon(),dict(state='DEGRADED'),'boot',dict(sandbox_id='box',generation='a'*32),time.monotonic()+1)

if __name__=='__main__':unittest.main()
