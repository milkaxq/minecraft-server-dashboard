#!/bin/bash
# Runs only inside the isolated Wings installation container, in /mnt/server.
set -euo pipefail
assets=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
version=${1:?Minecraft version required}
manifest="$assets/manifest.json"
jq -e --arg version "$version" '.versions[$version] | length == 3' "$manifest" >/dev/null

for file in plugins plugins/AuthMe server.properties .blockyard-default-plugins.json; do
  if [ -L "$file" ]; then echo "Refusing symlink in starter plugin setup" >&2; exit 1; fi
done
mkdir -p plugins/AuthMe
if [ -L plugins/AuthMe/config.yml ]; then echo "Refusing symlinked AuthMe configuration" >&2; exit 1; fi

stage=$(mktemp -d .blockyard-plugins.XXXXXXXX)
trap 'rm -rf -- "$stage"' EXIT
jq -r --arg version "$version" '.versions[$version][] | [.name,.filename,.url,(.size|tostring),.sha512] | @tsv' "$manifest" > "$stage/packages.tsv"

# Preflight the complete bundle before changing live files. Reinstallation keeps
# customer settings and never replaces a different or modified plugin silently.
while IFS=$'\t' read -r name filename url size hash; do
  [[ "$filename" =~ ^[A-Za-z0-9_.+-]+\.jar$ ]]
  [[ "$url" == "https://cdn.modrinth.com/data/"* ]]
  [[ "$hash" =~ ^[a-f0-9]{128}$ ]]
  [[ "$size" =~ ^[0-9]+$ ]] && [ "$size" -gt 0 ] && [ "$size" -le 67108864 ]
  for existing in plugins/*.jar; do
    [ -e "$existing" ] || [ -L "$existing" ] || continue
    base=$(basename -- "$existing")
    case "$(printf '%s' "$base" | tr '[:upper:]' '[:lower:]')" in
      authme*.jar) family=AuthMe ;;
      viaversion*.jar) family=ViaVersion ;;
      packetevents*.jar) family=PacketEvents ;;
      *) continue ;;
    esac
    if [ "$family" = "$name" ] && [ "$base" != "$filename" ]; then
      echo "A different $name jar is already installed; review it before reinstalling" >&2; exit 1
    fi
  done
  destination="plugins/$filename"
  if [ -L "$destination" ]; then echo "Refusing symlinked plugin" >&2; exit 1; fi
  if [ -e "$destination" ]; then
    [ -f "$destination" ]
    [ "$(sha512sum "$destination" | cut -d ' ' -f 1)" = "$hash" ]
    continue
  fi
  curl --fail --proto '=https' --connect-timeout 30 --max-time 180 --retry 2 \
    --max-filesize "$size" --user-agent 'Blockyard/1.0 (https://playblockyard.com)' \
    --url "$url" --output "$stage/$filename"
  [ "$(wc -c < "$stage/$filename" | tr -d '[:space:]')" = "$size" ]
  [ "$(sha512sum "$stage/$filename" | cut -d ' ' -f 1)" = "$hash" ]
  jar tf "$stage/$filename" >/dev/null
done < "$stage/packages.tsv"

# Seed authentication before installing jars or allowing offline account mode.
# AuthMe merges missing values from its packaged defaults on first start.
if [ ! -e plugins/AuthMe/config.yml ]; then
  (set -o noclobber; cat "$assets/authme.yml" > plugins/AuthMe/config.yml)
fi
for file in "$stage"/*.jar; do
  [ -f "$file" ] || continue
  mv -- "$file" "plugins/$(basename -- "$file")"
done
if [ ! -e server.properties ]; then
  (set -o noclobber; printf 'server-port=25565\nserver-ip=0.0.0.0\nonline-mode=false\nenforce-secure-profile=false\nwhite-list=true\nmotd=PlayBlockyard Minecraft Server\n' > server.properties)
fi
revision=$(sha512sum "$manifest" | cut -d ' ' -f 1)
jq --arg version "$version" --arg revision "$revision" \
  '{bundle_revision:$revision,minecraft_version:$version,plugins:.versions[$version]}' \
  "$manifest" > "$stage/receipt.json"
mv -- "$stage/receipt.json" .blockyard-default-plugins.json
echo 'Starter plugins installed. The owner must accept the EULA before starting.'
