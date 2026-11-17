# A fresh Debian 13 server, as a VPS provider gives it: systemd and SSH,
# for root with a key. The platform's end-to-end test prepares it.
FROM debian:13
ENV container=docker
RUN apt-get update \
 && apt-get install -y --no-install-recommends systemd systemd-sysv openssh-server python3 ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && install -d -m 700 /root/.ssh \
 && systemctl enable ssh
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
