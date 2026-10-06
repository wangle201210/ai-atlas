#!/usr/bin/env python3
"""Exercise the real Wails helper swap using disposable .app bundles and a fixture DB."""
import hashlib,json,os,plistlib,shutil,sqlite3,subprocess,sys,tempfile,time,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
if sys.platform!='darwin':raise SystemExit('This fixture currently targets macOS')
base=ROOT/'.test-data';base.mkdir(exist_ok=True)
work=Path(tempfile.mkdtemp(prefix='ai-atlas-update-smoke-',dir=base))
try:
 db=work/'ledger.sqlite'
 with sqlite3.connect(db) as c:c.execute('create table preserved(value text)');c.execute("insert into preserved values('keep this history')")
 before=hashlib.sha256(db.read_bytes()).hexdigest()
 for name,version in [('Old','1.0.0'),('Next','2.0.0')]:
  bundle=work/f'{name}.app';(bundle/'Contents/MacOS').mkdir(parents=True)
  info={'CFBundleExecutable':'probe','CFBundleIdentifier':'local.aiatlas.test.'+work.name,'CFBundleName':'AI Atlas Update Test','CFBundleVersion':version,'CFBundleShortVersionString':version,'CFBundlePackageType':'APPL','LSBackgroundOnly':True}
  (bundle/'Contents/Info.plist').write_bytes(plistlib.dumps(info))
  subprocess.run(['go','build','-ldflags',f'-X main.version={version}','-o',str(bundle/'Contents/MacOS/probe'),'./internal/updatetestprobe'],cwd=ROOT,check=True)
  subprocess.run(['codesign','--force','--sign','-',str(bundle)],check=True,capture_output=True)
 with zipfile.ZipFile(work/'update.zip','w',zipfile.ZIP_DEFLATED) as z:
  for p in (work/'Next.app').rglob('*'):z.write(p,p.relative_to(work))
 run=subprocess.run([str(work/'Old.app/Contents/MacOS/probe')],timeout=60,capture_output=True,text=True)
 if run.returncode:raise RuntimeError(run.stderr)
 deadline=time.monotonic()+45
 while not (work/'result.json').exists() and time.monotonic()<deadline:time.sleep(.25)
 result=json.loads((work/'result.json').read_text())
 assert result['version']=='2.0.0',result
 assert result['ledgerSHA256']==before,'Ledger changed during update'
 assert plistlib.loads((work/'Old.app/Contents/Info.plist').read_bytes())['CFBundleVersion']=='2.0.0'
 print('PASS: real helper replaced the isolated v1 .app with v2, relaunched it, and preserved the fixture database.')
finally:
 # Only this script's random fixture directory is removed; never an installed app.
 shutil.rmtree(work,ignore_errors=True)
