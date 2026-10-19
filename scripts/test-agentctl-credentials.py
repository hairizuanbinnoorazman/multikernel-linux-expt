#!/usr/bin/env python3
"""Bounded Linux process/Unix-wire proof using generated synthetic credentials."""
import argparse
import hashlib
import hmac
import json
import os
from pathlib import Path
import secrets
import socket
import struct
import subprocess
import tempfile


def exact(peer, size):
    result = b''
    while len(result) < size:
        chunk = peer.recv(size-len(result))
        assert chunk, 'incomplete client request'
        result += chunk
    return result


def observe(binary, legacy=False):
    with tempfile.TemporaryDirectory(prefix='g7-control-credential-') as temporary:
        directory = Path(temporary)
        directory.chmod(0o700)
        token = secrets.token_hex(32)
        credential = directory/'token'
        credential.write_text(token+'\n')
        credential.chmod(0o600)
        address = str(directory/'relay.sock')
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as listener:
            listener.bind(address)
            listener.listen(1)
            listener.settimeout(10)
            argv = [str(binary), '--sandbox-id=synthetic-box', '--generation=synthetic-generation',
                    '--port=7717', '--unix-socket='+address,
                    '--token-hex='+token if legacy else '--token-file='+str(credential)]
            process = subprocess.Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            try:
                with listener.accept()[0] as peer:
                    peer.settimeout(10)
                    length = struct.unpack('!I', exact(peer, 4))[0]
                    assert 0 < length <= 1 << 20
                    request = json.loads(exact(peer, length))
                    mac = request['mac']
                    request['mac'] = ''
                    canonical = json.dumps(request, separators=(',', ':')).encode()
                    assert hmac.compare_digest(mac, hmac.new(bytes.fromhex(token), canonical, hashlib.sha256).hexdigest())
                    assert request['method'] == 'Capabilities'
                    command_line = Path(f'/proc/{process.pid}/cmdline').read_bytes()
                    visible = token.encode() in command_line
                    assert visible == legacy
                    memory_refused = False
                    try:
                        descriptor = os.open(f'/proc/{process.pid}/mem', os.O_RDONLY)
                    except PermissionError:
                        memory_refused = True
                    else:
                        os.close(descriptor)
                    if not legacy:
                        assert memory_refused, 'credential client memory readable by same-UID observer'
                    reply = {'version': 1, 'sequence': request['sequence'], 'error': {
                        'code': 'FAILED_PRECONDITION', 'message': 'synthetic-fixture-stop', 'retryable': False}}
                    payload = json.dumps(reply, separators=(',', ':')).encode()
                    peer.sendall(struct.pack('!I', len(payload))+payload)
                stdout, stderr = process.communicate(timeout=10)
                assert process.returncode == 1
                assert b'Capabilities: FAILED_PRECONDITION: synthetic-fixture-stop' in stderr
                assert token.encode() not in stdout+stderr
                assert credential.read_text() == token+'\n'
                return {'authenticated_request_mac_valid': True, 'token_visible_in_proc_cmdline': visible,
                        'same_uid_memory_open_refused': memory_refused, 'token_absent_from_output': True,
                        'primary_credential_retained': True, 'controlled_stop_exit': process.returncode}
            finally:
                if process.poll() is None:
                    process.kill()
                process.communicate(timeout=10)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--old-binary', type=Path)
    args = parser.parse_args()
    assert os.geteuid() != 0, 'run as ordinary Linux UID to verify same-UID memory refusal'
    binary = args.binary.resolve()
    result = {'actor_uid': os.geteuid(), 'candidate_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'candidate': observe(binary), 'scope': 'Local Linux processes and real authenticated Unix request; no child workload or full SEC-08 audit qualification.'}
    if args.old_binary:
        old = args.old_binary.resolve()
        result['old_binary_sha256'] = hashlib.sha256(old.read_bytes()).hexdigest()
        result['old_control'] = observe(old, legacy=True)
    secret = secrets.token_hex(32)
    refused = subprocess.run([str(binary), '--token-hex='+secret], capture_output=True, timeout=10)
    assert refused.returncode == 2 and secret.encode() not in refused.stdout+refused.stderr
    result['legacy_plaintext_flag_refused'] = True
    print(json.dumps(result, indent=2))
    print('G7_PRIMARY_CREDENTIAL_FILE_PROCESS_PROOF_PASS')


if __name__ == '__main__':
    main()
