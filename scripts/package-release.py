#!/usr/bin/env python3
"""Package a signed macOS .app for the in-app updater (does not publish)."""
import argparse,hashlib,subprocess,zipfile
import xml.etree.ElementTree as ET
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
parser=argparse.ArgumentParser()
parser.add_argument('--arch',choices=['arm64','amd64'],required=True)
args=parser.parse_args()
subprocess.run(['python3',str(ROOT/'scripts/version.py'),'--check'],check=True)
bundle=ROOT/'bin/ai-atlas.app'
if not bundle.is_dir():raise SystemExit('Run wails3 package first')
version=(ROOT/'internal/buildinfo/VERSION').read_text().strip()
items=list(ET.parse(bundle/'Contents/Info.plist').getroot().find('dict'))
info={items[i].text:items[i+1].text for i in range(0,len(items),2)}
if any(info.get(key)!=version for key in ['CFBundleVersion','CFBundleShortVersionString']):raise SystemExit('Bundle version is stale; rebuild first')
actual=subprocess.check_output(['lipo','-archs',str(bundle/'Contents/MacOS/ai-atlas')],text=True).strip().split()
expected='x86_64' if args.arch=='amd64' else 'arm64'
if actual != [expected]:raise SystemExit(f'Expected {expected}, found {actual}; rebuild with ARCH={args.arch}')
subprocess.run(['codesign','--verify','--deep','--strict',str(bundle)],check=True)
dist=ROOT/'release-dist';dist.mkdir(exist_ok=True)
target=dist/f'ai-atlas-darwin-{args.arch}.zip'
with zipfile.ZipFile(target,'w',zipfile.ZIP_DEFLATED,compresslevel=9) as z:
 for path in sorted(bundle.rglob('*')):
  if path.is_symlink():raise SystemExit('Unexpected symlink in release bundle')
  z.write(path,path.relative_to(bundle.parent))
lines=[]
for path in sorted(dist.glob('*.zip')):
 h=hashlib.sha256()
 with path.open('rb') as f:
  for chunk in iter(lambda:f.read(1024*1024),b''):h.update(chunk)
 digest=h.hexdigest()
 lines.append(f'{digest}  {path.name}\n')
(dist/'SHA256SUMS').write_text(''.join(lines))
print(target)
