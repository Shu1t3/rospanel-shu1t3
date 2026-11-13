import runpy
route=runpy.run_path('/lab/route-lab.py') if False else runpy.run_path(__file__.replace('mixed-lab.py','route-lab.py'))
api=route['api']; ids=route['ids']; prefix=route['prefix']; docker=route['docker']
for n in 'rc':
    api('PATCH',f'nodes/{ids[n]}',{'name':n.upper(),'host':f'rp6-{n}.test','vless_enabled':True})
profile=route['create']('Mixed families','arc',ip_families=['ipv6','ipv4'])
assert profile.get('ip_families')==['ipv6','ipv4']
route['check'](profile,'14')
capture=docker('exec','rp6-c','cat','/tmp/rp6-mixed-capture.log',capture_output=True).stdout
assert '172.29.206.12.' in capture and '> 172.29.206.14.443' in capture, 'no IPv4 on R to C'
from pathlib import Path
Path(__file__).with_name('mixed-capture.log').write_text(capture)
print('Mixed route captured IPv4 on R → C and exited over IPv6',flush=True)
