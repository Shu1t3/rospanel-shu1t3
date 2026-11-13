#!/usr/bin/env bash
set -euo pipefail
cd "$RP6_LAB"
mode="$1"
cleanup() {
 for node in master a r b c origin; do
  if docker inspect "rp6-$node" >/dev/null 2>&1; then
   docker rm -f "rp6-$node" >/dev/null
  fi
 done
 docker network rm rospanel-ipv6-release-lab >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker network create --ipv6 --ipv4=false --subnet fd42:629d:2026:11::/64 rospanel-ipv6-release-lab >/dev/null
bash start-arm.sh "$mode"
wait_panel() {
 for attempt in {1..50}; do
  if docker exec rp6-master curl -gksSf --max-time 2 'https://[fd42:629d:2026:11::10]/rospanel/' >/dev/null; then return; fi
  sleep 1
 done
 return 1
}
wait_panel
if [[ "$mode" = fresh ]]; then
 python3 - <<'PY'
from panel_lab import api, password
api('POST','login',{'username':'admin','password':'admin'})
api('POST','account/credentials',{'username':'admin','password':password,'current_password':'admin'})
api('POST','login',{'username':'admin','password':password})
api('POST','setup/finish')
PY
 python3 panel_lab.py
 python3 route-lab.py ordinary
 python3 route-lab.py reverse
 docker network create --subnet 172.29.206.0/24 rp6-dual >/dev/null
 docker network connect --ip 172.29.206.12 rp6-dual rp6-r
 docker network connect --ip 172.29.206.14 rp6-dual rp6-c
 for node in master a r b c; do
  docker exec -u root "rp6-$node" sh -c 'cat >> /etc/hosts <<HOSTS
fd42:629d:2026:11::12 rp6-r.test
172.29.206.12 rp6-r.test
fd42:629d:2026:11::14 rp6-c.test
172.29.206.14 rp6-c.test
HOSTS'
 done
 docker exec -d rp6-c sh -c 'tcpdump -nn -l -i any ip and tcp port 443 >/tmp/rp6-mixed-capture.log 2>&1'
 python3 mixed-lab.py
else
 python3 upgrade_lab.py baseline
 mkdir -p snapshot
 for node in master a c; do
  unit=rospanel-node
  data=/var/lib/rospanel-node
  if [[ "$node" = master ]]; then unit=rospanel; data=/var/lib/rospanel; fi
  docker exec "rp6-$node" systemctl stop "$unit"
  docker exec "rp6-$node" tar -C / -cf - "${data#/}" >"snapshot/$node.tar"
  docker exec "rp6-$node" systemctl start "$unit"
 done
 cp client-link.txt snapshot/
 swap_binary() {
  local node="$1" unit="$2"
  docker exec "rp6-$node" systemctl stop "$unit"
  docker exec "rp6-$node" install -m 0755 /lab/rospanel-linux /usr/local/bin/rospanel
  docker exec "rp6-$node" systemctl start "$unit"
 }
 swap_binary master rospanel
 wait_panel
 python3 upgrade_lab.py verify
 swap_binary a rospanel-node
 swap_binary c rospanel-node
 python3 upgrade_lab.py verify
 docker exec rp6-master systemctl restart rospanel
 wait_panel
 python3 upgrade_lab.py verify
fi
