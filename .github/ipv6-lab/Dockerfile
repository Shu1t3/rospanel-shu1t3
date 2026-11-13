FROM debian:stable-slim
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends systemd systemd-sysv dbus openssh-server iproute2 nftables iptables tcpdump curl python3 ca-certificates sudo && rm -rf /var/lib/apt/lists/*
