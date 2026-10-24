#!/usr/bin/env python3
"""Inject real-proxy ring-full writes without consuming bytes; exercise retry scope."""
import os,pathlib,runpy,socket,subprocess,tempfile,time
root=pathlib.Path(__file__).resolve().parent.parent
exercise=runpy.run_path(str(root/'scripts/test-native-stream-proxy.py'))['exercise']
with tempfile.TemporaryDirectory(prefix='mk-proxy-backpressure-') as directory:
    binary=pathlib.Path(directory)/'proxy'
    subprocess.run(['cc','-O2','-Wall','-Wextra','-Werror',str(root/'scripts/test-native-stream-backpressure.c'),'-o',str(binary)],check=True)
    os.environ['MK_PROXY_TEST_MODE']='transient'
    exercise(binary);exercise(binary,reply_after_eof=True)
    print('NATIVE_PROXY_MK_QUEUE_FULL_DUPLEX_BYTES_AND_HALF_CLOSE_PASS')
    for mode in ['permanent','left']:
        os.environ['MK_PROXY_TEST_MODE']=mode
        client,left=socket.socketpair();server,right=socket.socketpair()
        process=subprocess.Popen([str(binary),'0',str(left.fileno()),str(right.fileno())],pass_fds=[left.fileno(),right.fileno()],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        left.close();right.close()
        try:
            (server if mode=='left' else client).sendall(b'queued bytes must not be discarded')
            if mode=='permanent':server.close()
            started=time.monotonic();_,error=process.communicate(timeout=2)
            assert process.returncode==1,error
            assert b'errno=110' in error if mode=='permanent' else b'errno=28' in error,error
            assert time.monotonic()-started<1,error
            print('NATIVE_PROXY_MK_QUEUE_FULL_BOUNDED_TIMEOUT_PASS' if mode=='permanent' else 'NATIVE_PROXY_UNMARKED_ENOSPC_STILL_FATAL_PASS')
        finally:
            if process.poll() is None:process.kill();process.wait()
            client.close();server.close()
    del os.environ['MK_PROXY_TEST_MODE']
