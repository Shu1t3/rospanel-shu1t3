import runpy, json
from panel_lab import *
def get_route(name):
    return next((p for p in api('GET','multi-hop') if p['name']==name),None)
def baseline():
    api('POST','login',{'username':'admin','password':'admin'})
    api('POST','account/credentials',{'username':'admin','password':password,'current_password':'admin'})
    api('POST','login',{'username':'admin','password':password})
    api('POST','setup/finish')
    ids={}
    for letter,suffix in [('a','11'),('c','14')]:
        host=prefix+suffix
        n=api('POST','nodes',{'name':letter.upper(),'host':host})
        ids[letter]=n['id']
        api('PATCH',f"nodes/{n['id']}",{'name':letter.upper(),'host':host,'vless_enabled':True})
        join=n['install_command'].split(' --join ',1)[1]
        docker('exec',f'rp6-{letter}','sh','-ec','/lab/previous node install --join '+join,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    until('v5.10.0 agents serving',read_nodes,lambda items: all(any(n['id']==i and n['joined'] and n['xray_running'] for n in items) for i in ids.values()))
    (root/'node-ids.json').write_text(json.dumps(ids))
    route=runpy.run_path(str(root/'route-lab.py'))
    p=route['create']('Upgrade baseline','ac')
    finish_baseline(route,p)

def finish_baseline(route,p):
    user=next((u for u in api('GET','users/brief') if u['name']=='Upgrade client'),None) or api('POST','users',{'name':'Upgrade client'})
    group=next((g for g in api('GET','groups') if g['name']=='Upgrade group'),None) or api('POST','groups',{'name':'Upgrade group','grants':[f"inbound:{p['id']}"]})
    api('POST',f"users/{user['id']}/groups",{'group_ids':[group['id']]})
    def read_link():
        old=api('GET',f"users/{user['id']}")['sub_url']
        url=old.replace('https://'+prefix+'10/',f'https://[{prefix}10]/')
        payload=docker('exec','rp6-master','curl','-gkfsS','--retry','4','--retry-connrefused','-A','v2rayNG/1.9.0',url,capture_output=True).stdout
        from urllib.parse import urlsplit
        import base64
        if 'vless://' not in payload: payload=base64.b64decode(payload).decode()
        for line in payload.splitlines():
            if line.startswith('vless://'):
                normalized = urlsplit(line.replace('@'+prefix+'11:',f'@[{prefix}11]:'))
                if normalized.port==p['port']:
                    return normalized
    link=until('Pre-upgrade profile imported',read_link,bool)
    route['start_client'](link,p)
    until('v5.10.0 TCP via existing route',route['tcp'],lambda got:got==prefix+'14')
    until('v5.10.0 UDP via existing route',route['udp'],lambda got:got=='probe@'+prefix+'14')
    (root/'client-link.txt').write_text(link.geturl())
    (root/'baseline.json').write_text(json.dumps({'id':p['id'],'port':p['port'],'user':user['id'],'credential':link.username}))
def verify():
    route=runpy.run_path(str(root/'route-lab.py'))
    record=json.loads((root/'baseline.json').read_text())
    p=get_route('Upgrade baseline')
    assert p['id']==record['id'] and p['port']==record['port'] and not p.get('ip_families')
    user={'id':record['user']}
    link=until('Upgraded subscription yields the existing credential',lambda:route['subscription'](user,p),lambda l:l and l.username==record['credential'])
    from urllib.parse import urlsplit
    route['start_client'](urlsplit((root/'client-link.txt').read_text()),p)
    until('Upgraded master and node carry TCP',route['tcp'],lambda got:got==prefix+'14')
    until('Upgraded master and node carry UDP',route['udp'],lambda got:got=='probe@'+prefix+'14')
if __name__=='__main__':
    (baseline if sys.argv[1]=='baseline' else verify)()
