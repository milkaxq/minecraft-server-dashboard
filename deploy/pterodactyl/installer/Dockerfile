# Wings installation scripts require root inside the isolated install container.
# This image is never the customer game runtime; Wings runs games as UID 988.
FROM ghcr.io/pterodactyl/yolks:java_21
USER root
RUN apt-get update && apt-get install -y --no-install-recommends jq && rm -rf /var/lib/apt/lists/*
COPY internal/provisioning/defaultplugins/manifest.json internal/provisioning/defaultplugins/authme.yml internal/provisioning/defaultplugins/install.sh /opt/blockyard/default-plugins/
# Wings supplies the command ["bash", "/mnt/install/install.sh"].
ENTRYPOINT []
