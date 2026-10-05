#!/usr/bin/env python3
"""Set/check the shared stable release version without relying on git history."""
import argparse,json,re
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
parser=argparse.ArgumentParser()
parser.add_argument('version',nargs='?')
parser.add_argument('--check',action='store_true')
args=parser.parse_args()
version=(args.version or (ROOT/'internal/buildinfo/VERSION').read_text().strip()).removeprefix('v')
if not re.fullmatch(r'(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)',version):
 raise SystemExit('Expected a stable semantic version, e.g. 0.1.2')
if args.check:
 assert (ROOT/'internal/buildinfo/VERSION').read_text().strip()==version,'Embedded version differs from release tag'
 for name in ['frontend/package.json','frontend/package-lock.json']:
  assert json.loads((ROOT/name).read_text())['version']==version,f'{name} version mismatch'
 for name in ['build/darwin/Info.plist','build/darwin/Info.dev.plist']:
  s=(ROOT/name).read_text()
  for key in ['CFBundleVersion','CFBundleShortVersionString']:
   assert re.search(r'<key>'+key+r'</key>\s*<string>([^<]+)</string>',s)[1]==version,f'{name} version mismatch'
 assert re.search(r'^  version: "([^"]+)"',(ROOT/'build/config.yml').read_text(),re.M)[1]==version,'Build config version mismatch'
 print('Version verified:',version)
else:
 (ROOT/'internal/buildinfo/VERSION').write_text(version+'\n')
 for name in ['frontend/package.json','frontend/package-lock.json']:
  p=ROOT/name;data=json.loads(p.read_text());data['version']=version
  if 'packages' in data:data['packages']['']['version']=version
  p.write_text(json.dumps(data,indent=2)+'\n')
 for name in ['build/darwin/Info.plist','build/darwin/Info.dev.plist']:
  p=ROOT/name;s=p.read_text()
  for key in ['CFBundleVersion','CFBundleShortVersionString']:
   s=re.sub(r'(<key>'+key+r'</key>\s*<string>)[^<]+',lambda m:m[1]+version,s)
  p.write_text(s)
 p=ROOT/'build/config.yml';p.write_text(re.sub(r'^  version: "[^"]+"','  version: "'+version+'"',p.read_text(),flags=re.M))
 print('Version set:',version)
