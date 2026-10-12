#!/usr/bin/env python3
"""Check the rendered deployment boundary without starting containers."""

import json
import os
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parent.parent


class ComposeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        environment = dict(os.environ)
        environment.update({
            "APP_ENV": "development",
            "RECOVERY_TOKEN_KEY": "compose-recovery-test-only",
            "POSTGRES_PASSWORD": "compose-test-only",
            "DATABASE_URL": "postgres://hosting:compose-test-only@postgres:5432/hosting?sslmode=disable",
            "PANEL_DATABASE_PASSWORD": "compose-panel-test-only",
            "PANEL_DATABASE_ROOT_PASSWORD": "compose-root-test-only",
            "PTERODACTYL_URL": "http://panel",
            "PTERODACTYL_CLIENT_API_KEY": "ptlc_compose-test-only",
            "PTERODACTYL_INSTALLATION_API_KEY": "ptlc_installation-test-only",
            "PTERODACTYL_APPLICATION_API_KEY": "ptla_compose-test-only",
            "PTERODACTYL_WEBSOCKET_ORIGINS": "ws://wings.internal:8180",
            "PTERODACTYL_TRANSFER_ORIGINS": "http://wings.internal:8180",
        })
        command = [
            "docker", "compose", "--env-file", ".env.example",
            "-f", "docker-compose.yml", "-f", "docker-compose.pterodactyl.yml",
            "-f", "docker-compose.wings.yml", "--profile", "frontend",
            "--profile", "pterodactyl", "--profile", "wings",
            "config", "--format", "json",
        ]
        result = subprocess.run(command, cwd=ROOT, env=environment, capture_output=True, check=True)
        cls.services = json.loads(result.stdout)["services"]
        vps_command = command.copy()
        vps_command[8:8] = ["-f", "docker-compose.vps.yml"]
        result = subprocess.run(vps_command, cwd=ROOT, env=environment, capture_output=True, check=True)
        cls.vps_services = json.loads(result.stdout)["services"]

    def test_vps_database_tls_and_private_ports(self):
        for name in ["postgres", "redis", "panel-database", "panel-cache"]:
            self.assertFalse(self.vps_services[name].get("ports"), name)
        postgres = self.vps_services["postgres"]
        self.assertIn("ssl=on", postgres["command"])
        self.assertIn("hba_file=/etc/postgresql/pg_hba.conf", postgres["command"])
        for name in ["backend", "worker", "migrate"]:
            mounts = self.vps_services[name]["volumes"]
            ca = next(m for m in mounts if m["target"] == "/etc/hosting/ca.crt")
            self.assertTrue(ca["read_only"])
        for name in ["backend", "frontend", "panel"]:
            for port in self.vps_services[name].get("ports", []):
                self.assertEqual(port["host_ip"], "127.0.0.1", name)

    def test_only_wings_has_host_privileges(self):
        for name, service in self.services.items():
            targets = {mount["target"] for mount in service.get("volumes", [])}
            if name != "wings":
                self.assertNotIn("/var/run/docker.sock", targets, name)
                self.assertNotIn("/var/lib/pterodactyl", targets, name)
                self.assertNotIn("/run/wings", targets, name)
                self.assertNotEqual(service.get("network_mode"), "host", name)
            self.assertFalse(service.get("privileged", False), name)
        self.assertIn("wings", self.services["wings"]["profiles"])

    def test_host_container_paths_match_for_docker_mounts(self):
        mounts = {mount["target"]: mount for mount in self.services["wings"]["volumes"]}
        for path in ["/var/lib/pterodactyl", "/tmp/pterodactyl", "/run/wings", "/var/run/docker.sock"]:
            self.assertEqual(mounts[path]["source"], path)
            # Compose omits false-valued bind options in its normalized output.
            self.assertFalse(mounts[path]["bind"].get("create_host_path", False))

    def test_api_credentials_stay_separate(self):
        backend = self.services["backend"]["environment"]
        worker = self.services["worker"]["environment"]
        self.assertNotIn("PTERODACTYL_APPLICATION_API_KEY", backend)
        self.assertNotIn("PTERODACTYL_CLIENT_API_KEY", worker)
        self.assertNotIn("PTERODACTYL_INSTALLATION_API_KEY", backend)
        self.assertEqual(worker["PTERODACTYL_INSTALLATION_API_KEY"], "ptlc_installation-test-only")
        frontend = self.services["frontend"].get("environment", {})
        self.assertFalse(any("KEY" in key or "SECRET" in key for key in frontend))
        build_args = self.services["frontend"]["build"].get("args", {})
        self.assertEqual(set(build_args), {"NEXT_PUBLIC_API_URL"})

    def test_recovery_key_is_backend_only(self):
        self.assertEqual(self.services["backend"]["environment"]["RECOVERY_TOKEN_KEY"], "compose-recovery-test-only")
        for name, service in self.services.items():
            if name != "backend":
                self.assertNotIn("RECOVERY_TOKEN_KEY", service.get("environment", {}), name)

    def test_internal_wings_origin_avoids_localhost_resolution(self):
        # cURL treats .localhost names as loopback even when /etc/hosts maps them.
        for name in ["panel", "backend"]:
            hosts = json.dumps(self.services[name]["extra_hosts"])
            self.assertIn("wings.internal", hosts)
            self.assertIn("host-gateway", hosts)
            self.assertNotIn("wings.localhost", hosts)
        backend = self.services["backend"]["environment"]
        self.assertEqual(backend["PTERODACTYL_WEBSOCKET_ORIGINS"], "ws://wings.internal:8180")
        self.assertEqual(backend["PTERODACTYL_TRANSFER_ORIGINS"], "http://wings.internal:8180")

    def test_panel_revision_and_persistent_keys(self):
        panel = self.services["panel"]
        self.assertRegex(panel["build"]["context"], r"panel\.git#[0-9a-f]{40}$")
        self.assertNotIn("latest", panel["image"])
        targets = {mount["target"] for mount in panel["volumes"]}
        self.assertIn("/app/var", targets)
        self.assertEqual(panel["environment"]["DB_CONNECTION"], "mysql")
        self.assertEqual(self.services["backend"]["environment"]["DATABASE_URL"].split(":")[0], "postgres")
        self.assertFalse(self.services["panel-database"].get("ports"))
        self.assertFalse(self.services["panel-cache"].get("ports"))


if __name__ == "__main__":
    unittest.main()
