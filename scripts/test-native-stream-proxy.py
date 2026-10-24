#!/usr/bin/env python3
"""Exercise the real proxy with small buffers, asynchronous NBD-like traffic."""
from pathlib import Path
import socket
import subprocess
import tempfile
import threading
import time

root = Path(__file__).resolve().parent.parent
request = bytes(range(256)) * 8192
reply = bytes(reversed(range(256))) * 8192


def exercise(binary, control=False, reply_after_eof=False):
    client, proxy_left = socket.socketpair()
    server, proxy_right = socket.socketpair()
    sockets = [client, proxy_left, server, proxy_right]
    for sock in sockets:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 4096)
    client.settimeout(8); server.settimeout(8)
    # Queue a request prefix so both directions are pending before forwarding.
    client.sendall(request[:1024])
    process = subprocess.Popen([str(binary), str(int(control)), str(proxy_left.fileno()), str(proxy_right.fileno())],
                               pass_fds=[proxy_left.fileno(), proxy_right.fileno()],
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    proxy_left.close(); proxy_right.close()
    results = {}; failures = []

    def receive(sock):
        data = bytearray()
        while True:
            part = sock.recv(65536)
            if not part:
                return bytes(data)
            data.extend(part)

    def writer():
        client.sendall(request[1024:])
        client.shutdown(socket.SHUT_WR)

    def reader():
        results['reply'] = receive(client)

    def responder():
        if reply_after_eof:
            results['request'] = receive(server)
            server.sendall(reply)
        else:
            server.sendall(reply)
            results['request'] = receive(server)
        server.shutdown(socket.SHUT_WR)

    def protected(function):
        try:
            function()
        except Exception as error:
            failures.append(type(error).__name__)

    threads = [threading.Thread(target=protected, args=(fn,), daemon=True) for fn in [writer, reader, responder]]
    try:
        for thread in threads: thread.start()
        deadline = time.monotonic() + (2 if control else 10)
        for thread in threads: thread.join(max(0, deadline-time.monotonic()))
        if control:
            assert any(thread.is_alive() for thread in threads) and process.poll() is None, 'blocking control did not reproduce backpressure deadlock'
            print('BLOCKING_PROXY_DUPLEX_DEADLOCK_CONTROL_CONFIRMED')
        else:
            assert not any(thread.is_alive() for thread in threads), 'proxy did not drain both directions'
            assert not failures, failures
            assert results == {'request': request, 'reply': reply}, 'lost, reordered or cross-direction bytes'
            assert process.wait(timeout=2) == 0
            print('NATIVE_PROXY_HALF_CLOSE_DRAIN_PASS' if reply_after_eof else 'NATIVE_PROXY_BIDIRECTIONAL_BACKPRESSURE_PASS')
    finally:
        if process.poll() is None:
            process.kill()
        process.communicate(timeout=2)
        client.close(); server.close()
        for thread in threads: thread.join(1)



def broken_peer(binary):
    client, left = socket.socketpair(); server, right = socket.socketpair()
    client.sendall(b'pending payload');client.shutdown(socket.SHUT_WR);server.close()
    process = subprocess.Popen([str(binary), '0', str(left.fileno()), str(right.fileno())], pass_fds=[left.fileno(),right.fileno()], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    left.close();right.close()
    try:
        _, error = process.communicate(timeout=2)
        assert process.returncode==1 and b'stream proxy' in error, 'broken stream was hidden or killed by SIGPIPE'
        print('NATIVE_PROXY_BROKEN_PEER_REFUSAL_PASS')
    finally:
        if process.poll() is None:process.kill();process.wait()
        client.close()

if __name__ == '__main__':
    with tempfile.TemporaryDirectory() as directory:
        binary = Path(directory) / 'proxy'
        subprocess.run(['cc', '-O2', '-Wall', '-Wextra', '-Werror', str(root/'scripts/test-native-stream-proxy.c'), '-o', str(binary)], check=True)
        exercise(binary, control=True)
        exercise(binary)
        exercise(binary, reply_after_eof=True)
        broken_peer(binary)
