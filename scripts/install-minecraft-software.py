#!/usr/bin/env python3
"""Import reviewed eggs into Panel v2 and write operator configuration locally.

Does not enable customer software or modify existing servers. Run control software
with the resulting JSON after validating installation on your nodes.
"""
import argparse
import json
import os
from pathlib import Path
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid


class Panel:
    def __init__(self, url, key):
        parsed = urllib.parse.urlsplit(url)
        if parsed.scheme not in ("https", "http") or not parsed.netloc or parsed.username:
            raise ValueError("invalid Panel URL")
        if parsed.scheme == "http" and parsed.hostname not in ("localhost", "127.0.0.1", "panel"):
            raise ValueError("remote Panel requires HTTPS")
        self.url = url.rstrip("/")
        self.key = key

    def request(self, path, body=None, content_type=None):
        headers = {"Accept": "application/json", "Authorization": "Bearer " + self.key}
        if content_type:
            headers["Content-Type"] = content_type
        request = urllib.request.Request(self.url + path, data=body, headers=headers)
        # A token must never follow a redirect to another origin.
        class NoRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, req, fp, code, msg, headers, newurl):
                return None
        with urllib.request.build_opener(NoRedirect()).open(request, timeout=60) as response:
            return json.load(response)

    def eggs(self):
        page = 1
        result = []
        while True:
            data = self.request(f"/api/admin/eggs?page={page}&per_page=100")
            result.extend(item["attributes"] for item in data["data"])
            if page >= data.get("meta", {}).get("pagination", {}).get("total_pages", 1):
                return result
            page += 1

    def import_egg(self, source, egg_id=None):
        boundary = "blockyard-" + uuid.uuid4().hex
        body = (f'--{boundary}\r\nContent-Disposition: form-data; name="import_file"; filename="egg.json"\r\nContent-Type: application/json\r\n\r\n'.encode()
                + source.read_bytes() + f"\r\n--{boundary}--\r\n".encode())
        return self.request(f"/api/admin/eggs/{egg_id}/import" if egg_id else "/api/admin/eggs/import", body, "multipart/form-data; boundary=" + boundary)["attributes"]["id"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--panel-url", required=True)
    parser.add_argument("--output", type=Path, default=Path(".local/software-config"))
    parser.add_argument("--update-reviewed", action="store_true", help="replace an existing project egg with the reviewed checked-in configuration")
    args = parser.parse_args()
    key = os.environ.get("PTERODACTYL_ADMIN_API_KEY")
    if not key:
        parser.error("set PTERODACTYL_ADMIN_API_KEY in the operator environment")
    panel = Panel(args.panel_url, key)
    existing = panel.eggs()
    source_root = Path(__file__).resolve().parent.parent / "deploy/pterodactyl"
    args.output.mkdir(parents=True, exist_ok=True)
    for source in sorted((source_root / "eggs").glob("*.json")):
        egg = json.loads(source.read_text())
        matches = [item for item in existing if item["name"] == egg["name"]]
        if len(matches) > 1:
            raise ValueError("duplicate egg name; review Panel before continuing")
        if matches:
            egg_id = matches[0]["id"]
            exported = panel.request(f"/api/admin/eggs/{egg_id}/export")
            # Ignore export timestamps and metadata; compare executable configuration.
            for field in ("startup", "docker_images", "config", "scripts", "variables"):
                if exported.get(field) != egg.get(field):
                    if not args.update_reviewed:
                        raise ValueError(f"existing {egg['name']} differs; review it before updating")
                    panel.import_egg(source, egg_id)
                    break
        else:
            egg_id = panel.import_egg(source)
        template = json.loads((source_root / "software" / (source.stem + ".example.json")).read_text())
        template["egg_id"] = egg_id
        destination = args.output / (source.stem + ".json")
        destination.write_text(json.dumps(template, indent=2) + "\n")
        print(f"{template['server_type']}: egg {egg_id}, configuration {destination}")


if __name__ == "__main__":
    try:
        main()
    except (urllib.error.HTTPError, urllib.error.URLError, ValueError, KeyError) as error:
        # Omit remote response bodies, URLs and request headers containing secrets.
        print(f"Software import failed ({type(error).__name__}). Check permissions and Panel configuration.", file=sys.stderr)
        sys.exit(1)
