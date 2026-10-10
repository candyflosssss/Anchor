#!/usr/bin/env python3
"""Run the exact release executables on Windows and emit same-run evidence."""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tempfile
import zipfile

from ci_release import NATIVE_CHECKS, verify
from release import digest, standalone_name


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--directory', type=Path, required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--revision', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if sys.platform != 'win32' or platform.machine().lower() not in {'amd64', 'x86_64'}:
        raise RuntimeError('native Windows AMD64 is required')
    verify(args.directory, args.version, args.revision, allow_pending=True)
    binary = args.directory / standalone_name(args.version, 'windows-amd64')
    archive = args.directory / f'anchor_{args.version}_windows_amd64.zip'
    evidence = {'schema_version': 1, 'version': args.version, 'revision': args.revision,
                'platform': sys.platform, 'machine': platform.machine(),
                'os_version': platform.version(), 'run_id': os.environ['GITHUB_RUN_ID'],
                'run_attempt': os.environ['GITHUB_RUN_ATTEMPT'],
                'input_manifest_sha256': digest(args.directory / 'release.json')}
    with tempfile.TemporaryDirectory(prefix='anchor-windows-acceptance-') as temp:
        extracted = Path(temp) / 'anchor.exe'
        member = f'anchor_{args.version}_windows_amd64/anchor.exe'
        with zipfile.ZipFile(archive) as z:
            if z.namelist().count(member) != 1:
                raise ValueError('missing or duplicate archive executable')
            extracted.write_bytes(z.read(member))  # No archive paths are extracted.
        if digest(extracted) != digest(binary):
            raise ValueError('standalone and ZIP executables differ')
        for kind, executable, artifact in [('standalone', binary, binary), ('archive', extracted, archive)]:
            result = subprocess.run([sys.executable, str(Path(__file__).with_name('smoke_test.py')),
                                     '--binary', str(executable.resolve()), '--expected-version', args.version],
                                    text=True, encoding='utf-8', capture_output=True, timeout=180)
            if result.returncode:
                # Smoke diagnostics contain no real secrets and use only isolated state.
                raise RuntimeError(f'{kind} native smoke failed: {result.stderr}')
            checks = json.loads(result.stdout)
            if checks != {key: 'passed' for key in NATIVE_CHECKS}:
                raise ValueError('incomplete native smoke result')
            evidence[kind] = {'sha256': digest(artifact), 'checks': checks}
    args.output.write_text(json.dumps(evidence, indent=2) + '\n', encoding='utf-8')
    print('Windows standalone and ZIP native acceptance passed; evidence written.')


if __name__ == '__main__':
    main()
