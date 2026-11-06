"""Shared output formats for one original-client extraction."""
import argparse
import shutil
import struct
import subprocess

MEDIA_PROFILE = 'png-vorbis-v1'
MEDIA_FORMAT_SCHEMA_VERSION = 1
IMAGE_SUFFIXES = frozenset(('.bmp','.jpg','.jpeg','.png','.gif','.ico','.tif','.tiff','.webp','.tga','.pcx'))
AUDIO_SUFFIXES = frozenset(('.wav','.ogg','.oga','.mp3','.flac','.aif','.aiff','.m4a'))
AUDIO_RATE = 44100
AUDIO_CHANNELS = 2
VORBIS_QUALITY = 6


def vorbis_format(raw):
    """Read the Ogg Vorbis identification packet without decoding the samples."""
    if len(raw) < 27 or raw[:5] != b'OggS\0':
        return None
    start = 27+raw[26]
    packet = raw[start:start+30]
    if len(packet) != 30 or packet[:7] != b'\x01vorbis':
        return None
    channels,rate = packet[11],struct.unpack_from('<I',packet,12)[0]
    if channels == 0 or rate == 0:
        return None
    return dict(format='ogg-vorbis',codec='vorbis',channels=channels,sample_rate=rate)


def export_audio_file(source, output):
    """Copy existing Vorbis exactly; convert other audio once at extraction."""
    if output.exists():
        raise ValueError(f'colliding audio output: {output}')
    with source.open('rb') as f:
        original = vorbis_format(f.read(4096))
    if original:
        # Exclusive creation also protects against colliding source basenames
        # being exported concurrently by the media worker pool.
        with source.open('rb') as incoming, output.open('xb') as outgoing:
            shutil.copyfileobj(incoming,outgoing)
    else:
        command=['ffmpeg','-nostdin','-n','-v','error','-i',str(source),'-map','0:a:0','-vn',
                 '-c:a','libvorbis','-q:a',str(VORBIS_QUALITY),'-ar',str(AUDIO_RATE),
                 '-ac',str(AUDIO_CHANNELS),'-threads','1',str(output)]
        result=subprocess.run(command,stdout=subprocess.DEVNULL,stderr=subprocess.PIPE)
        if result.returncode:
            raise ValueError(f'ffmpeg failed: {result.stderr.decode(errors="replace")}')
    with output.open('rb') as f:
        metadata=vorbis_format(f.read(4096))
    if metadata is None:
        raise ValueError(f'not an Ogg Vorbis output: {output}')
    return dict(metadata,conversion='copied' if original else 'transcoded',
                **({} if original else dict(vorbis_quality=VORBIS_QUALITY)))


def validate_output_formats(root):
    counts=dict(images=0,audio=0,video=0)
    for path in sorted(root.rglob('*')):
        if not path.is_file():
            continue
        suffix=path.suffix.lower()
        if suffix in IMAGE_SUFFIXES:
            if suffix != '.png':
                raise ValueError(f'non-PNG image output: {path}')
            with path.open('rb') as f:
                if f.read(8) != b'\x89PNG\r\n\x1a\n':
                    raise ValueError(f'invalid PNG output: {path}')
            counts['images']+=1
        elif suffix in AUDIO_SUFFIXES:
            with path.open('rb') as f:
                valid=suffix == '.ogg' and vorbis_format(f.read(4096)) is not None
            if not valid:
                raise ValueError(f'non-Vorbis audio output: {path}')
            counts['audio']+=1
        elif suffix == '.avi':
            counts['video']+=1
    return dict(schema_version=MEDIA_FORMAT_SCHEMA_VERSION,profile=MEDIA_PROFILE,images='png',audio='ogg-vorbis',video='avi',counts=counts)


if __name__ == '__main__':
    from pathlib import Path
    import json
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--data',type=Path,required=True)
    print(json.dumps(validate_output_formats(parser.parse_args().data),indent=2))
