#!/usr/bin/env python3
import importlib.util,json,struct,subprocess,sys,unittest,uuid
from pathlib import Path

path=Path(__file__).with_name('observe-runtime-storage-geometry.py')
spec=importlib.util.spec_from_file_location('geometry',path);subject=importlib.util.module_from_spec(spec);spec.loader.exec_module(subject)
OLD=50<<30;NEW=58<<30;UUID='11111111-2222-4333-8444-555555555555';LABEL='fixture-storage'


def superblock(size=OLD,reserved=None):
    data=bytearray(1024);blocks=size//4096;reserved=blocks//20 if reserved is None else reserved
    for offset,value in [(4,blocks&0xffffffff),(8,reserved&0xffffffff),(0x18,2),(0x60,0x80),(0x150,blocks>>32),(0x154,reserved>>32)]:struct.pack_into('<I',data,offset,value)
    struct.pack_into('<H',data,0x38,0xef53);data[0x68:0x78]=uuid.UUID(UUID).bytes
    data[0x78:0x78+len(LABEL)]=LABEL.encode();return bytes(data)


def qualify(data,phase='before',device=OLD,old=OLD,new=NEW):
    return subject.qualify(subject.parse_superblock(data),phase,device,old,new,UUID,LABEL)


class GeometryTests(unittest.TestCase):
    def test_before_disk_only_and_complete_growth_are_distinct_readonly_phases(self):
        for phase,device,fs in [('before',OLD,OLD),('disk-grown',NEW,OLD),('filesystem-grown',NEW,NEW)]:
            with self.subTest(phase=phase):
                result=qualify(superblock(fs),phase,device)
                self.assertEqual(result['device_bytes'],device);self.assertEqual(result['filesystem_bytes'],fs)
                self.assertFalse(result['growth_performed']);self.assertTrue(result['metadata_only']);self.assertTrue(result['five_percent_reserve_verified'])

    def test_larger_raw_device_never_proves_filesystem_growth(self):
        for phase,device,fs in [('filesystem-grown',NEW,OLD),('filesystem-grown',OLD,NEW),('before',NEW,OLD),('disk-grown',NEW,NEW),('disk-grown',OLD,OLD)]:
            with self.subTest(phase=phase,device=device,fs=fs),self.assertRaisesRegex(ValueError,'^filesystem and device growth phase differ$'):qualify(superblock(fs),phase,device)

    def test_high_block_and_reserve_words_are_used_only_with_64bit_feature(self):
        old=320<<40;new=384<<40;data=superblock(old)
        result=qualify(data,'before',old,old,new);self.assertEqual(result['filesystem_bytes'],old);self.assertGreater(result['reserved_blocks'],0xffffffff)
        broken=bytearray(data);struct.pack_into('<I',broken,0x60,0)
        with self.assertRaisesRegex(ValueError,'^unqualified ext4 high block counts$'):subject.parse_superblock(bytes(broken))

    def test_five_percent_rounding_is_preserved_without_lowering_reserve(self):
        blocks=NEW//4096
        for reserved in [blocks//20,(blocks+19)//20]:self.assertTrue(qualify(superblock(NEW,reserved),'filesystem-grown',NEW)['five_percent_reserve_verified'])
        for reserved in [0,blocks//25,blocks//20-2,blocks//20+2]:
            with self.subTest(reserved=reserved),self.assertRaisesRegex(ValueError,'^filesystem five-percent reserve differs$'):qualify(superblock(NEW,reserved),'filesystem-grown',NEW)

    def test_malformed_metadata_identity_and_noncanonical_authority_refuse(self):
        for change in ['short','long','magic','shift','empty','reserved','uuid','label']:
            data=bytearray(superblock())
            if change=='short':data=data[:-1]
            elif change=='long':data+=b'x'
            elif change=='magic':struct.pack_into('<H',data,0x38,0)
            elif change=='shift':struct.pack_into('<I',data,0x18,0xffffffff)
            elif change=='empty':struct.pack_into('<I',data,4,0)
            elif change=='reserved':struct.pack_into('<I',data,8,0xffffffff)
            elif change=='uuid':data[0x68]^=1
            else:data[0x78]^=1
            with self.subTest(change=change),self.assertRaises(ValueError):qualify(bytes(data))
        for phase,device,old,new in [('unknown',OLD,OLD,NEW),('before',True,OLD,NEW),('before',float(OLD),OLD,NEW),('before',OLD,NEW,OLD),('before',OLD,OLD,NEW+1)]:
            with self.assertRaises(ValueError):qualify(superblock(),phase,device,old,new)

    def test_cli_refusal_keeps_private_device_and_identity_values_out_of_output(self):
        r=subprocess.run([sys.executable,str(path),'--mountpoint','/srv/multikernel-storage','--device','/nonexistent/PRIVATE_GEOMETRY_CAUSE','--serial','private-serial','--uuid',UUID,'--label',LABEL,'--phase','before','--old-bytes',str(OLD),'--new-bytes',str(NEW)],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=5)
        self.assertEqual(r.returncode,1);self.assertEqual(r.stdout,b'');self.assertEqual(r.stderr,b'runtime storage geometry refused\n')

if __name__=='__main__':unittest.main()
