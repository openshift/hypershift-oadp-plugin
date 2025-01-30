#!/bin/bash

mkdir -p /opt/minio/data

podman run -d --net host --name minio-ipv6 -v /opt/minio/data:/data -e "MINIO_ROOT_USER=ROOTNAME" -e "MINIO_ROOT_PASSWORD=CHANGEME123" quay.io/minio/minio server /data --address :9002 --console-address :9001

cat <<EOF > /etc/systemd/system/minio.service
[Unit]
Description=Minio For Disconnected OCP Deployments
After=network.target

[Service]
User=root
Group=root
Restart=always
ExecStart=/usr/bin/podman start -a minio
ExecStop=/usr/bin/podman stop minio
ExecRestart=/usr/bin/podman stop minio;sleep 5;/usr/bin/podman start -a minio


[Install]
WantedBy=multi-user.target
EOF

systemctl enable minio
systemctl restart minio