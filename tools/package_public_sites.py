"""Package only tracked public portal files. Produces reproducible CI artifact and OCI context."""
import hashlib,json,pathlib,subprocess,sys,tarfile,io
root=pathlib.Path(__file__).resolve().parents[1]
out=pathlib.Path(sys.argv[1] if len(sys.argv)>1 else root/'dist/public-sites')
if not out.is_absolute():out=(root/out).resolve()
if out.exists() and any(out.iterdir()):raise SystemExit('Output must be empty; refusing to mix releases')
out.mkdir(parents=True,exist_ok=True)
sha=subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()
version=(root/'backend/cmd/server/VERSION').read_text().strip()
files=subprocess.check_output(['git','ls-files','-z','gateway/home-portal','gateway/docs-portal'],cwd=root).decode().split('\0')
manifest={}
for name in sorted(filter(None,files)):
 p=root/name
 if p.suffix not in {'.html','.css','.js','.png','.webp','.svg','.xml','.txt'}:continue
 if p.name in {'template.html','main-zh.png','add-zh.png','pipixia-provider.png','pipixia-provider-original.webp'}:continue
 rel=pathlib.PurePosixPath(name).relative_to('gateway').as_posix()
 data=p.read_bytes();target=out/'payload'/rel;target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(data);manifest[rel]=hashlib.sha256(data).hexdigest()
if len(manifest)<40:raise SystemExit('Missing tracked generated files')
release={'revision':sha,'application_version':version,'release':'public-sites-'+sha[:12],'files':manifest}
(out/'payload/release.json').write_text(json.dumps(release,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
with tarfile.open(out/'public-sites.tar','w') as tar:
 for p in sorted((out/'payload').rglob('*')):
  if not p.is_file():continue
  data=p.read_bytes();info=tarfile.TarInfo(p.relative_to(out/'payload').as_posix());info.size=len(data);info.mode=0o644;info.mtime=0;tar.addfile(info,io.BytesIO(data))
(out/'public-sites.tar.sha256').write_text(hashlib.sha256((out/'public-sites.tar').read_bytes()).hexdigest()+'  public-sites.tar\n')
(out/'Dockerfile').write_text('FROM scratch\nCOPY payload/ /public-sites/\n')
print(json.dumps({'revision':sha,'application_version':version,'files':len(manifest),'output':str(out)}))
