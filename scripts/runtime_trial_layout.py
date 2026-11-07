"""Fixed disposable lab stage layouts; no arbitrary root override or mounts."""
from pathlib import Path
import os
import re

ROOTS = {'root':Path('/opt/mkruntime'), 'storage':Path('/srv/multikernel-storage/stages')}


def stage_path(revision, location='storage'):
    if type(revision) is not str or not re.fullmatch('[a-f0-9]{7,40}',revision) or type(location) is not str or location not in ROOTS:
        raise ValueError('invalid private trial layout')
    return ROOTS[location]/('g8-host-'+revision)


def validate_stage(value, revision=None):
    if not isinstance(value,(str,Path)):
        raise ValueError('invalid private trial layout')
    text=str(value);path=Path(text)
    if not path.is_absolute() or os.path.normpath(text)!=text or path.parent not in ROOTS.values() or not path.name.startswith('g8-host-'):
        raise ValueError('invalid private trial layout')
    suffix=path.name.removeprefix('g8-host-')
    if not re.fullmatch('[a-f0-9]{7,40}',suffix) or revision is not None and suffix!=revision:
        raise ValueError('invalid private trial layout')
    return path
