#!/usr/bin/env bash
set -euo pipefail
lab="$(cd "$(dirname "$0")" && pwd)"
mode="$1"
printf "Starting the %s Linux lab\n" "$mode"
case "$mode" in
 old) binary=previous;;
 fresh) binary=rospanel-linux;;
esac
for pair in master:10 a:11 r:12 b:13 c:14 origin:30; do
 name="${pair%:*}"; suffix="${pair#*:}"
 if [ "$mode" = old ] && [[ $name = r || $name = b ]]; then continue; fi
 docker run -d --name "rp6-$name" --hostname "rp6-$name" --privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw --network rospanel-ipv6-release-lab --ip6 "fd42:629d:2026:11::$suffix" --tmpfs /run --tmpfs /run/lock -e container=docker -v "$lab:/lab:ro" rp6-systemd /lib/systemd/systemd >/dev/null
 docker exec "rp6-$name" sh -ec 'install -m 0755 /lab/xray-linux /usr/local/bin/xray; /usr/local/bin/xray version >/dev/null; mkdir -p /var/lib/rospanel/geo /var/lib/rospanel-node/geo; cp /lab/geoip.dat /lab/geosite.dat /var/lib/rospanel/geo/; cp /lab/geoip.dat /lab/geosite.dat /var/lib/rospanel-node/geo/'
done
echo "Containers and Xray assets ready"
docker exec rp6-origin ip -6 addr add 2600:1000::50/128 dev lo
docker exec -d rp6-origin python3 /lab/origin.py
for name in master a r b c; do
 if [ "$mode" = old ] && [[ $name = r || $name = b ]]; then continue; fi
 docker exec "rp6-$name" ip -6 route replace 2600:1000::50/128 via fd42:629d:2026:11::30
done
for attempt in {1..24}; do
 if docker exec rp6-master systemctl show-environment >/dev/null 2>&1; then break; fi
 sleep 1
done
docker exec -e XRAY_BIN=/usr/local/bin/xray -e ROSPANEL_HOST=fd42:629d:2026:11::10 rp6-master "/lab/$binary" install
if [ "$mode" = fresh ]; then
 docker exec rp6-r sh -ec 'install -d -m 700 /root/.ssh; install -m 600 /lab/lab-ssh-key.pub /root/.ssh/authorized_keys; install -d /run/sshd; ssh-keygen -A; /usr/sbin/sshd -p 2222 -o PermitRootLogin=prohibit-password -o ListenAddress=::'
fi
