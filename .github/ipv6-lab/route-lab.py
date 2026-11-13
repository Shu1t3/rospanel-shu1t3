import base64, re
from urllib.parse import parse_qs, unquote, urlsplit
from panel_lab import *
ids=json.loads((root/'node-ids.json').read_text())
def ready(profile):
    return until(f"Route {profile['name']} applied",lambda: next(p for p in api('GET','multi-hop') if p['id']==profile['id']),lambda p:not p.get('applying'))
def subscription(user, profile):
    view=api('GET',f"users/{user['id']}")
    suburl=view["sub_url"]
    assert suburl.startswith(f"https://[{prefix}10]/"), "unbracketed master subscription URL"
    payload=docker('exec','rp6-master','curl','-gkfsS','-A','v2rayNG/1.9.0',suburl,capture_output=True).stdout
    if 'vless://' not in payload:
        payload=base64.b64decode(payload).decode()
    links=[urlsplit(line) for line in payload.splitlines() if line.startswith('vless://')]
    return next((link for link in links if link.port==profile['port']),None)
def start_client(link, profile):
    assert link.hostname == prefix+'11' and link.netloc.endswith(f'@[{prefix}11]:{profile["port"]}'), 'wrong subscription link'
    query=parse_qs(link.query)
    assert 'pcs' in query, 'no pin for IPv6-only test CA'
    cfg={
        'log':{'loglevel':'warning'},
        'inbounds':[
            {'listen':'127.0.0.1','port':18090,'protocol':'http'},
            {'listen':'127.0.0.1','port':18091,'protocol':'dokodemo-door','settings':{'address':'2600:1000::50','port':18081,'network':'udp'}},
        ],
        'outbounds':[{'protocol':'vless','settings':{'address':link.hostname,'port':link.port,'id':link.username,'encryption':'none','flow':query.get('flow',[''])[0]},
                       'streamSettings':{'network':'tcp','security':'tls','tlsSettings':{'serverName':query.get('sni',[link.hostname])[0],'pinnedPeerCertSha256':query['pcs'][0]}}}],
    }
    docker('exec','-i','rp6-master','sh','-c','cat >/tmp/lab-client.json',input=json.dumps(cfg))
    docker('exec','rp6-master','sh','-c','if [ -f /tmp/lab-client.pid ]; then kill "$(cat /tmp/lab-client.pid)" || true; fi')
    docker('exec','-d','rp6-master','sh','-c','echo $$ >/tmp/lab-client.pid; exec /lab/xray-linux run -c /tmp/lab-client.json >/tmp/lab-client.log 2>&1')
def tcp():
    proc=subprocess.run(['docker','exec','rp6-master','curl','-gfsS','--max-time','2','-x','http://127.0.0.1:18090','http://[2600:1000::50]:18080/'],text=True,capture_output=True)
    return proc.stdout.strip() if proc.returncode==0 else ''
def udp():
    proc=subprocess.run(['docker','exec','rp6-master','python3','-c','import socket; s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.settimeout(2); s.sendto(b"probe",("127.0.0.1",18091)); print(s.recv(100).decode())'],text=True,capture_output=True)
    return proc.stdout.strip() if proc.returncode==0 else ''
def check(profile, exit):
    user=api('POST','users',{'name':'Client '+profile['name']+' '+str(time.time_ns())[-6:]})
    group=api('POST','groups',{'name':'Group '+profile['name']+' '+str(time.time_ns())[-6:],'grants':[f"inbound:{profile['id']}"]})
    api('POST',f"users/{user['id']}/groups",{'group_ids':[group['id']]})
    link=until('Bracketed IPv6 subscription profile published',lambda:subscription(user,profile),bool)
    start_client(link,profile)
    until('TCP egress verified',tcp,lambda got:got==prefix+exit,seconds=25)
    until('UDP egress verified',udp,lambda got:got=='probe@'+prefix+exit,seconds=25)
    return user, link
def create(label, roles, **extra):
    existing=next((p for p in api('GET','multi-hop') if p['name']==label),None)
    return ready(existing or api('POST','multi-hop',{'name':label,'nodes':[ids[r] for r in roles],**extra}))
def run(mode):
    if mode=='ordinary':
        p=create('IPv6 ordinary','arc',ip_families=['ipv6','ipv6'])
        check(p,'14')
        docker('exec','rp6-r','nft','add','table','inet','lab_fault')
        docker('exec','rp6-r','nft','add','chain','inet','lab_fault','input','{ type filter hook input priority -5; }')
        docker('exec','rp6-r','nft','add','rule','inet','lab_fault','input','ip6','saddr',prefix+'11','tcp','dport','443','drop')
        assert not tcp() and not udp(), 'traffic leaked after IPv6 link break'
        docker('exec','rp6-r','nft','delete','table','inet','lab_fault')
        until('Ordinary route recovered after IPv6 break',tcp,lambda got:got==prefix+'14',seconds=20)
        p=create('IPv6 return','ara',ip_families=['ipv6','ipv6'])
        check(p,'11')
    elif mode=='reverse':
        reverse={'relay_port':29443,'transit_port':29444,'transit_ip':prefix+'13','relay_sources':[prefix+'12']}
        p=create('IPv6 protected','arbc',ip_families=['ipv6','ipv6'],reverse=reverse)
        check(p,'14')
        docker('exec','rp6-b','nft','list','table','inet','rospanel_reverse_chain',stdout=subprocess.DEVNULL)
        direct=subprocess.run(['docker','exec','rp6-a','curl','-gksS','--max-time','2',f'https://[{prefix}13]:29444/'],capture_output=True)
        assert direct.returncode, 'direct A to B must fail'
        stranger=subprocess.run(['docker','exec','rp6-origin','curl','-gksS','--max-time','2',f'https://[{prefix}13]:29444/'],capture_output=True)
        assert stranger.returncode, 'B must reject a source outside its allowlist'
        docker('exec','rp6-c','systemctl','stop','rospanel-node')
        until('Protected route closed with C offline',tcp,lambda got:not got,seconds=12)
        docker('exec','rp6-c','systemctl','start','rospanel-node')
        until('Protected route recovered when C returned',tcp,lambda got:got==prefix+'14',seconds=25)
if __name__=='__main__':run(sys.argv[1])
