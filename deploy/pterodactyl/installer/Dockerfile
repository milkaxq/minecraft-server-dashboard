# Wings installation scripts require root inside the isolated install container.
# This image is never the customer game runtime; Wings runs games as UID 988.
FROM ghcr.io/pterodactyl/yolks:java_21
USER root
# Wings supplies the command ["bash", "/mnt/install/install.sh"].
ENTRYPOINT []
