import json, os, subprocess, sys, time
from pathlib import Path
password = 'test-' + os.environ['GITHUB_RUN_ID'] + '-ipv6-only'
root = Path(__file__).parent
prefix = 'fd42:629d:2026:11::'
panel = f'https://[{prefix}10]/rospanel/api/'
def docker(*args, **kwargs):
    return subprocess.run(['docker', *args], check=True, text=True, **kwargs)
def api(method, path, data=None, ok=(200,201), stream=False):
    cmd = ['exec','rp6-master','curl','-gksS','--max-time','55','--retry','3','--retry-connrefused','-b','/tmp/lab-session','-c','/tmp/lab-session','-X',method,'-H','Content-Type: application/json','-H','X-RosPanel-CSRF: 1','-w','\n%{http_code}',panel+path]
    if data is not None:
        cmd += ['--data',json.dumps(data)]
    for attempt in range(5):
        result = subprocess.run(['docker', *cmd], text=True, capture_output=True)
        if result.returncode == 0:
            break
        if attempt == 4:
            raise RuntimeError(f'{method} {path}: transport failure ({result.returncode})')
        time.sleep(1)
    raw = result.stdout
    body, status = raw.rsplit('\n',1)
    if int(status) not in ok:
        raise RuntimeError(f'{method} {path}: {status}')
    return body if stream else json.loads(body)
def until(message, get, predicate, seconds=75):
    start = time.monotonic()
    while time.monotonic() - start < seconds:
        value = get()
        if predicate(value):
            print(message, flush=True)
            return value
        time.sleep(0.6)
    raise RuntimeError(f'timed out: {message}')
def read_nodes():
    return api('GET','nodes')['nodes']
def enroll():
    api('POST','login',{'username':'admin','password':password})
    ids={}
    for letter,suffix in zip('arbc',('11','12','13','14')):
        host=prefix+suffix
        existing=next((n for n in read_nodes() if n['name']==letter.upper()),None)
        if existing and existing['joined']:
            ids[letter]=existing['id']
            continue
        node=existing or api('POST','nodes',{'name':letter.upper(),'host':f'[{host}]'})
        ids[letter]=node['id']
        api('PATCH',f'nodes/{ids[letter]}',{'name':letter.upper(),'host':f'[{host}]','vless_enabled':True})
        if letter == 'r':
            events=api('POST',f'nodes/{ids[letter]}/provision',{'ssh_host':f'[{host}]','ssh_port':2222,'ssh_user':'root','ssh_key':(root/'lab-ssh-key').read_text()},stream=True)
            assert 'event:done' in events, 'SSH provisioning failed'
            print('SSH provisioned R over IPv6', flush=True)
        else:
            command=node['offline_install_command']
            assert f'https://[{prefix}10]/' in command, 'bad master URL'
            docker('exec',f'rp6-{letter}','sh','-ec','cp /lab/rospanel-linux /tmp/rospanel; cd /tmp; '+command,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            print(f'Install command joined {letter.upper()} over IPv6',flush=True)
    (root/'node-ids.json').write_text(json.dumps(ids))
    until('All IPv6 nodes online with Xray',read_nodes,lambda items: all(any(n['id']==i and n['joined'] and n['xray_running'] for n in items) for i in ids.values()))
    assert all(n['host'] == prefix+dict(zip('arbc',('11','12','13','14')))[letter] for letter,i in ids.items() for n in read_nodes() if n['id']==i)
    docker('exec','rp6-a','systemctl','restart','rospanel-node')
    until('IPv6 command-installed agent synchronized after restart', lambda: docker('exec','rp6-a','journalctl','-u','rospanel-node','-n','25','--no-pager',capture_output=True).stdout, lambda s:'config=' in s or 'agent: starting' in s,seconds=70)
if __name__=='__main__':
    enroll()
