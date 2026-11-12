#!/usr/bin/env python3
import copy,tempfile,unittest
from pathlib import Path
import runtime_docker_log_rotation as subject
from runtime_trial_output import logging_config


class RotationTests(unittest.TestCase):
    def fixture(self, root):
        identifier='a'*64;directory=root/'containers'/identifier/'local-logs';directory.mkdir(parents=True)
        for name,size in [('container.log',4096),('container.log.1',1_000_020)]:
            with (directory/name).open('wb') as f:f.truncate(size)
            (directory/name).chmod(0o600)
        return directory,dict(Id=identifier,State=dict(Status='exited',ExitCode=0),HostConfig=dict(LogConfig=dict(Type='local',Config=logging_config()['log-opts'])),LogPath=str(directory/'container.log'))

    def test_rotated_owned_sample_with_record_overhead_accepts_without_reading(self):
        with tempfile.TemporaryDirectory() as raw:
            root=Path(raw);directory,item=self.fixture(root)
            result=subject.qualify_rotation(item,root)
            self.assertEqual(result['rotated_file_count'],2);self.assertEqual(result['observed_bytes'],1004116)
            self.assertFalse(result['log_contents_read'])

    def test_missing_extra_unrotated_and_overlarge_samples_refuse(self):
        for mode in ['missing','extra','short','oversized','empty']:
            with self.subTest(mode=mode),tempfile.TemporaryDirectory() as raw:
                root=Path(raw);directory,item=self.fixture(root)
                if mode=='missing':(directory/'container.log.1').unlink()
                if mode=='extra':(directory/'container.log.2').touch()
                if mode in ['short','oversized','empty']:
                    size={'short':999999,'oversized':subject.MAX_FILE+subject.RECORD_SLACK+1,'empty':0}[mode]
                    with (directory/'container.log.1').open('wb') as f:f.truncate(size)
                with self.assertRaises(ValueError):subject.qualify_rotation(item,root)

    def test_policy_exit_and_foreign_path_refuse(self):
        with tempfile.TemporaryDirectory() as raw:
            root=Path(raw);directory,item=self.fixture(root)
            for field,value in [('policy',{}),('exit',1),('boolean-exit',False),('path','/var/lib/docker/foreign.log'),('id','../foreign')]:
                bad=copy.deepcopy(item)
                if field=='policy':bad['HostConfig']['LogConfig']=value
                elif field in ['exit','boolean-exit']:bad['State']['ExitCode']=value
                elif field=='path':bad['LogPath']=value
                else:bad['Id']=value
                with self.subTest(field=field),self.assertRaises(ValueError):subject.qualify_rotation(bad,root)

    def test_file_directory_aliases_and_shared_files_refuse_without_reading(self):
        import os
        for mode in ['file-alias','directory-alias','shared','fifo']:
            with self.subTest(mode=mode),tempfile.TemporaryDirectory() as raw:
                root=Path(raw);directory,item=self.fixture(root);file=directory/'container.log'
                if mode=='file-alias':file.unlink();file.symlink_to('container.log.1')
                elif mode=='shared':os.link(file,directory.parent/'shared')
                elif mode=='fifo':file.unlink();os.mkfifo(file)
                else:
                    directory.rename(directory.with_name('actual'));directory.symlink_to('actual',target_is_directory=True)
                with self.assertRaises((ValueError,OSError)):subject.qualify_rotation(item,root)

if __name__=='__main__':unittest.main()
